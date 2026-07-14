package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// sessionTTL is how long a login session stays valid. There's no sliding
// renewal in this first pass -- once it expires, the operator just logs in
// again.
const sessionTTL = 7 * 24 * time.Hour

var ErrNotFound = errors.New("not found")

type User struct {
	ID             int64
	Username       string
	PasswordHash   string
	FailedAttempts int
	// LockedUntil is the zero time when the account isn't currently locked
	// out (FR-35).
	LockedUntil time.Time
	// TOTPSecret is set the moment 2FA setup starts (handleTOTPSetup), even
	// before TOTPEnabled turns true -- see handleTOTPVerify, which is what
	// actually flips TOTPEnabled once the operator proves they scanned the
	// QR code correctly by submitting one valid code back.
	TOTPSecret  string
	TOTPEnabled bool
	// BackupCodeHashes are FR-39's recovery codes, bcrypt-hashed (same as a
	// password) before storage -- never round-tripped back to the frontend
	// except once, immediately after handleTOTPVerify generates them.
	BackupCodeHashes []string
}

// Locked reports whether this account is currently locked out (FR-35),
// i.e. LockedUntil is set and still in the future.
func (u *User) Locked() bool {
	return !u.LockedUntil.IsZero() && time.Now().UTC().Before(u.LockedUntil)
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CountUsers is used to decide whether the first-run setup flow (create the
// one admin account) should be offered instead of a login form.
func (r *Repository) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser is only ever called by the first-run setup handler, which
// itself checks CountUsers == 0 first -- this is a single-admin tool, not a
// multi-user system, so there's no separate "invite"/"register" flow.
func (r *Repository) CreateUser(ctx context.Context, username, passwordHash string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("insert user: %w", err)
	}
	return res.LastInsertId()
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	var lockedUntil, totpSecret, backupCodesJSON sql.NullString
	var totpEnabled int
	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, failed_attempts, locked_until,
		       totp_secret, totp_enabled, backup_codes_json
		FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.FailedAttempts, &lockedUntil,
		&totpSecret, &totpEnabled, &backupCodesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lockedUntil.Valid {
		u.LockedUntil, err = time.Parse(time.RFC3339, lockedUntil.String)
		if err != nil {
			return nil, fmt.Errorf("parse locked_until: %w", err)
		}
	}
	u.TOTPSecret = totpSecret.String
	u.TOTPEnabled = totpEnabled != 0
	if backupCodesJSON.Valid {
		if err := json.Unmarshal([]byte(backupCodesJSON.String), &u.BackupCodeHashes); err != nil {
			return nil, fmt.Errorf("parse backup codes: %w", err)
		}
	}
	return &u, nil
}

// CreateSession issues a new session for userID, valid for sessionTTL.
func (r *Repository) CreateSession(ctx context.Context, userID int64) (id string, expiresAt time.Time, err error) {
	id, err = NewSessionID()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now().UTC()
	expiresAt = now.Add(sessionTTL)
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		id, userID, expiresAt.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("insert session: %w", err)
	}
	return id, expiresAt, nil
}

// UserForSession resolves a session cookie value to its user, rejecting
// expired sessions. Expired rows aren't deleted here (lazy cleanup is fine
// for a single-admin tool); DeleteSession/logout removes them explicitly.
func (r *Repository) UserForSession(ctx context.Context, sessionID string) (*User, error) {
	var u User
	var expiresAt string
	var totpSecret sql.NullString
	var totpEnabled int
	err := r.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.password_hash, u.totp_secret, u.totp_enabled, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ?`, sessionID,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &totpSecret, &totpEnabled, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.TOTPSecret = totpSecret.String
	u.TOTPEnabled = totpEnabled != 0

	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse session expiry: %w", err)
	}
	if time.Now().UTC().After(expiry) {
		return nil, ErrNotFound
	}
	return &u, nil
}

func (r *Repository) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID)
	return err
}

// UpdatePasswordHash changes a user's stored password hash. Existing
// sessions aren't invalidated -- the sessions table only references
// user_id, not the password, so a password change doesn't need to force a
// re-login on this or other already-logged-in browsers.
func (r *Repository) UpdatePasswordHash(ctx context.Context, userID int64, newHash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, newHash, userID)
	return err
}

// RecordFailedLogin implements FR-35: increments the account's consecutive
// failed-attempt counter and, once it reaches maxAttempts, locks the
// account until lockoutDuration from now (resetting the counter so the next
// lockout starts counting fresh once it expires). maxAttempts/
// lockoutDuration are supplied by the caller rather than fixed here because
// FR-33(b) requires stricter defaults the moment WAN exposure is on --
// internal/auth only implements the counting/locking mechanism, not that
// policy decision (see handleLogin).
func (r *Repository) RecordFailedLogin(ctx context.Context, userID int64, maxAttempts int, lockoutDuration time.Duration) (locked bool, until time.Time, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, time.Time{}, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	var attempts int
	if err := tx.QueryRowContext(ctx, `SELECT failed_attempts FROM users WHERE id = ?`, userID).Scan(&attempts); err != nil {
		return false, time.Time{}, err
	}
	attempts++

	if attempts >= maxAttempts {
		until = time.Now().UTC().Add(lockoutDuration)
		if _, err := tx.ExecContext(ctx, `UPDATE users SET failed_attempts = 0, locked_until = ? WHERE id = ?`,
			until.Format(time.RFC3339), userID); err != nil {
			return false, time.Time{}, err
		}
		locked = true
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET failed_attempts = ? WHERE id = ?`, attempts, userID); err != nil {
			return false, time.Time{}, err
		}
	}
	return locked, until, tx.Commit()
}

// ResetFailedLogins clears a successful-login account back to a clean
// slate (FR-35) -- called right after password+2FA verification succeeds.
func (r *Repository) ResetFailedLogins(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET failed_attempts = 0, locked_until = NULL WHERE id = ?`, userID)
	return err
}

// SetPendingTOTPSecret stores a freshly generated secret (handleTOTPSetup)
// without enabling 2FA yet -- handleTOTPVerify only flips totp_enabled once
// the operator proves they actually scanned it by submitting one valid
// code back. Calling this again before verifying (e.g. the operator
// re-scans a fresh QR code) simply replaces the still-unconfirmed secret;
// it never touches an already-enabled account's secret since callers only
// invoke this from the setup handler, not a "re-generate" one.
func (r *Repository) SetPendingTOTPSecret(ctx context.Context, userID int64, secret string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET totp_secret = ?, totp_enabled = 0 WHERE id = ?`, secret, userID)
	return err
}

// EnableTOTP flips 2FA on for good (FR-38's gate on WAN exposure only
// passes once this is true) and stores the backup codes' bcrypt hashes
// generated alongside it (FR-39).
func (r *Repository) EnableTOTP(ctx context.Context, userID int64, backupCodeHashes []string) error {
	encoded, err := json.Marshal(backupCodeHashes)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE users SET totp_enabled = 1, backup_codes_json = ? WHERE id = ?`, encoded, userID)
	return err
}

// ConsumeBackupCode implements FR-39's recovery path: checks code (already
// known to be a plausible backup code, not a 6-digit TOTP one -- see
// handleLogin) against every stored hash, and if one matches, removes it
// from the list (single-use) and persists the shorter list. Returns false,
// nil (not an error) if none matched.
func (r *Repository) ConsumeBackupCode(ctx context.Context, userID int64, hashes []string, matchIndex int) error {
	remaining := append(hashes[:matchIndex:matchIndex], hashes[matchIndex+1:]...)
	encoded, err := json.Marshal(remaining)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE users SET backup_codes_json = ? WHERE id = ?`, encoded, userID)
	return err
}
