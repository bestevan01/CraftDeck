package api

import (
	"encoding/json"
	"net/http"
	"time"

	"craftdeck/internal/auth"
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
		"lan_bypass": isLANRequest(r),
		// username lets the frontend's change-password form identify the
		// account without asking the operator to retype who they are.
		"username": username,
	})
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
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

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := s.users.GetUserByUsername(r.Context(), req.Username)
	if err != nil || !auth.VerifyPassword(user.PasswordHash, req.Password) {
		// Same message either way -- don't reveal whether the username
		// exists.
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
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

// TODO(auth): wire these to internal/auth's TOTP helpers (already
// implemented) once FR-37's WAN-exposure-triggered 2FA enforcement is built.
func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
