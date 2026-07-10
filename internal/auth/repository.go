package auth

import (
	"context"
	"database/sql"
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
	ID           int64
	Username     string
	PasswordHash string
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
	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
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
	err := r.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.password_hash, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ?`, sessionID,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

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
