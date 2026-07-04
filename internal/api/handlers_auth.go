package api

import "net/http"

// TODO(auth): wire these to internal/auth + a users/sessions repository.
// handleLogin must require a valid TOTP code whenever the admin UI port is
// currently exposed externally (requirements.md FR-37); that check belongs
// here once the network/port-mapping state is queryable from this package.

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
