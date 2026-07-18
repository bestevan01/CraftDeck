package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"craftdeck/internal/auth"

	"github.com/skip2/go-qrcode"
)

const sessionCookieName = "craftdeck_session"

// currentUser resolves the request's session cookie to a logged-in user.
// Used both by requireAuth (router.go) and any handler that needs to know
// who's calling.
func (s *Server) currentUser(r *http.Request) (*auth.User, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, false
	}
	user, err := s.users.UserForSession(r.Context(), cookie.Value)
	if err != nil {
		return nil, false
	}
	return user, true
}

// setSessionCookie issues the session cookie for a freshly created session.
// Secure is only set when the request itself arrived over TLS -- forcing it
// unconditionally would break cookies on a plain-HTTP LAN-only setup, which
// is this app's default (FR-33 turns TLS on automatically once WAN exposure
// is enabled; until then there's no certificate to require Secure against).
func setSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

// handleAuthStatus tells the frontend which of three screens to show: the
// first-run setup form (no admin account exists yet), the login form (an
// account exists but this browser has no valid session), or the app itself.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	count, err := s.users.CountUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	user, authenticated := s.currentUser(r)
	username := ""
	if authenticated {
		username = user.Username
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setup_required": count == 0,
		"authenticated":  authenticated,
		// lan_bypass tells the frontend requireAuth won't actually demand a
		// session for this client right now (see router.go) -- so it should
		// skip the login redirect even if authenticated is false.
		"lan_bypass": s.authBypassed(r),
		// username lets the frontend's change-password form identify the
		// account without asking the operator to retype who they are.
		"username": username,
		// totp_enabled lets the network-settings page decide whether to send
		// the operator through 2FA setup before letting them turn WAN
		// exposure on (FR-38) or just show "already enabled".
		"totp_enabled": authenticated && user.TOTPEnabled,
	})
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// TOTPCode is required on a second submission once handleLogin has
	// already told the frontend totp_required (FR-37) -- omitted on the
	// first attempt, when the frontend doesn't yet know whether this
	// account has 2FA enabled.
	TOTPCode string `json:"totp_code,omitempty"`
}

// handleSetup creates the single admin account this tool ever has. It only
// works once -- if an account already exists, callers must use
// handleLogin instead.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	count, err := s.users.CountUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "setup already completed", http.StatusConflict)
		return
	}

	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Username == "" || len(req.Password) < 8 {
		http.Error(w, "username is required and password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	userID, err := s.users.CreateUser(r.Context(), req.Username, hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionID, expiresAt, err := s.users.CreateSession(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, r, sessionID, expiresAt)
	w.WriteHeader(http.StatusCreated)
}

// loginLockoutPolicy implements FR-33(b): once the web UI port is actually
// exposed to the WAN (FR-21/25's toggle), brute-forcing the login becomes a
// real threat from anywhere on the internet, not just whoever's already on
// the home network -- so the threshold/lockout duration auto-switches to a
// stricter default the moment that's true, with no separate setting for the
// operator to remember to configure.
func (s *Server) loginLockoutPolicy(ctx context.Context) (maxAttempts int, lockoutDuration time.Duration) {
	if settings, err := s.networkSettings.Get(ctx); err == nil && settings.WANEnabled {
		return 5, 15 * time.Minute
	}
	return 10, 5 * time.Minute
}

// handleLogin implements FR-35 on top of the basic username/password check:
// an account already locked out is rejected before its password is even
// checked (locked-out attempts don't get to keep guessing), a wrong
// password counts against the lockout threshold (locking the account
// outright once it's reached), and a correct one clears the counter.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := s.users.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		// Same message as a wrong password -- don't reveal whether the
		// username exists.
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	if user.Locked() {
		http.Error(w, fmt.Sprintf("too many failed attempts -- account locked until %s", user.LockedUntil.Format(time.RFC3339)), http.StatusTooManyRequests)
		return
	}

	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		maxAttempts, lockoutDuration := s.loginLockoutPolicy(r.Context())
		locked, until, lockErr := s.users.RecordFailedLogin(r.Context(), user.ID, maxAttempts, lockoutDuration)
		if lockErr != nil {
			log.Printf("record failed login for %s: %v", user.Username, lockErr)
		}
		if locked {
			http.Error(w, fmt.Sprintf("too many failed attempts -- account locked until %s", until.Format(time.RFC3339)), http.StatusTooManyRequests)
			return
		}
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}

	// FR-37: password alone isn't enough once 2FA is enabled -- the
	// frontend doesn't know that in advance, so a first submission with no
	// code gets a distinct "totp_required" response (not counted as a
	// failed attempt -- the password was correct) telling it to ask for one
	// and resubmit the same request with totp_code filled in.
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]bool{"totp_required": true})
			return
		}
		ok, backupIndex := verifyTOTPOrBackupCode(user, req.TOTPCode)
		if !ok {
			maxAttempts, lockoutDuration := s.loginLockoutPolicy(r.Context())
			locked, until, lockErr := s.users.RecordFailedLogin(r.Context(), user.ID, maxAttempts, lockoutDuration)
			if lockErr != nil {
				log.Printf("record failed login for %s: %v", user.Username, lockErr)
			}
			if locked {
				http.Error(w, fmt.Sprintf("too many failed attempts -- account locked until %s", until.Format(time.RFC3339)), http.StatusTooManyRequests)
				return
			}
			http.Error(w, "invalid two-factor code", http.StatusUnauthorized)
			return
		}
		if backupIndex >= 0 {
			if err := s.users.ConsumeBackupCode(r.Context(), user.ID, user.BackupCodeHashes, backupIndex); err != nil {
				log.Printf("consume backup code for %s: %v", user.Username, err)
			}
		}
	}

	if err := s.users.ResetFailedLogins(r.Context(), user.ID); err != nil {
		log.Printf("reset failed logins for %s: %v", user.Username, err)
	}

	sessionID, expiresAt, err := s.users.CreateSession(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, r, sessionID, expiresAt)
	w.WriteHeader(http.StatusOK)
}

type changePasswordRequest struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword re-verifies the current password before allowing a
// change -- that check is what actually authorizes this action (not just
// the requireAuth session/LAN gate it also sits behind), so it works the
// same way whether or not the caller currently has a session cookie.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.NewPassword) < 8 {
		http.Error(w, "new password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	user, err := s.users.GetUserByUsername(r.Context(), req.Username)
	if err != nil || !auth.VerifyPassword(user.PasswordHash, req.CurrentPassword) {
		http.Error(w, "current username or password is incorrect", http.StatusUnauthorized)
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.users.UpdatePasswordHash(r.Context(), user.ID, hash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.users.DeleteSession(r.Context(), cookie.Value)
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// verifyTOTPOrBackupCode checks code against the account's real TOTP
// secret first, then each unused backup code (FR-39's recovery path) if
// that fails. Returns the matched backup code's index (for
// ConsumeBackupCode) or -1 when it was the TOTP code (or nothing) that
// matched.
func verifyTOTPOrBackupCode(user *auth.User, code string) (ok bool, backupIndex int) {
	if auth.ValidateTOTPCode(user.TOTPSecret, code) {
		return true, -1
	}
	for i, hash := range user.BackupCodeHashes {
		if auth.VerifyPassword(hash, code) {
			return true, i
		}
	}
	return false, -1
}

// handleTOTPSetup starts FR-39's enrollment flow: generates a fresh secret
// (stored but not yet trusted -- see SetPendingTOTPSecret) and returns a
// scannable QR code plus the raw secret for manual entry. Only usable while
// 2FA isn't already enabled -- swapping an active account's secret without
// re-proving control of the old one is a bigger security hole than this
// tool needs to solve right now (FR-39's backup codes are the intended
// recovery path for a lost authenticator).
func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	if user.TOTPEnabled {
		http.Error(w, "two-factor authentication is already enabled", http.StatusConflict)
		return
	}

	secret, otpauthURL, err := auth.GenerateTOTPSecret(user.Username, "CraftDeck")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.users.SetPendingTOTPSecret(r.Context(), user.ID, secret); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	png, err := qrcode.Encode(otpauthURL, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret":      secret,
		"otpauth_url": otpauthURL,
		"qr_code_png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	})
}

type totpVerifyRequest struct {
	Code string `json:"code"`
}

// handleTOTPVerify completes enrollment: the operator proves they actually
// captured the secret (scanned the QR, or copied it in manually) by
// submitting one valid code back. Only then does 2FA actually turn on --
// FR-38's gate on enabling WAN exposure checks totp_enabled, not just
// whether a secret happens to exist, precisely so an abandoned/never-
// confirmed setup attempt can't be mistaken for real coverage. Backup
// codes (FR-39) are generated here and returned exactly once -- the
// operator has to save them now, since only their bcrypt hashes are kept
// from this point on.
func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	if user.TOTPEnabled {
		http.Error(w, "two-factor authentication is already enabled", http.StatusConflict)
		return
	}
	if user.TOTPSecret == "" {
		http.Error(w, "call /api/auth/2fa/setup first", http.StatusBadRequest)
		return
	}

	var req totpVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !auth.ValidateTOTPCode(user.TOTPSecret, req.Code) {
		http.Error(w, "invalid code", http.StatusUnauthorized)
		return
	}

	backupCodes, err := auth.GenerateBackupCodes(8)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hashes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash, err := auth.HashPassword(code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hashes[i] = hash
	}
	if err := s.users.EnableTOTP(r.Context(), user.ID, hashes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      true,
		"backup_codes": backupCodes,
	})
}

type totpDisableRequest struct {
	Password string `json:"password"`
}

// handleTOTPDisable turns 2FA off entirely. Requires the current password
// as re-confirmation, same as handleChangePassword, since this is a
// security downgrade rather than a routine settings change -- an active
// session alone isn't proof the person at the keyboard is still the
// account owner. Also refuses outright while WAN exposure is on: FR-38
// exists specifically so an operator can't lock themselves out after
// exposing the panel to the internet, and letting 2FA be turned off while
// still exposed would defeat that entirely.
func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	if !user.TOTPEnabled {
		http.Error(w, "two-factor authentication is not enabled", http.StatusConflict)
		return
	}

	var req totpDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		http.Error(w, "password is incorrect", http.StatusUnauthorized)
		return
	}

	settings, err := s.networkSettings.Get(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if settings.WANEnabled {
		http.Error(w, "외부 접속이 켜져 있는 동안은 2단계 인증을 끌 수 없습니다 -- 먼저 외부 접속을 꺼주세요", http.StatusPreconditionFailed)
		return
	}

	if err := s.users.DisableTOTP(r.Context(), user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTOTPRegenerateBackupCodes issues a fresh set of backup codes,
// invalidating all previous ones -- for an operator who's used most of
// theirs up, without having to fully turn 2FA off and re-enroll from
// scratch.
func (s *Server) handleTOTPRegenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	if !user.TOTPEnabled {
		http.Error(w, "two-factor authentication is not enabled", http.StatusConflict)
		return
	}

	backupCodes, err := auth.GenerateBackupCodes(8)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hashes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash, err := auth.HashPassword(code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hashes[i] = hash
	}
	if err := s.users.SetBackupCodeHashes(r.Context(), user.ID, hashes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": backupCodes})
}
