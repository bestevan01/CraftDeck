// Package api wires the REST endpoints and WebSocket console handler
// described in ARCHITECTURE.md section 4. It uses the standard library's
// http.ServeMux (Go 1.22+ method+path patterns) rather than an external
// router, matching the project's "minimize apt/runtime dependencies"
// philosophy (NFR-9) at the code level too.
package api

import (
	"net/http"

	"craftdeck/internal/instance"
)

type Server struct {
	instances *instance.Repository
}

func NewServer(instances *instance.Repository) *Server {
	return &Server{instances: instances}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/system/health", s.handleHealth)

	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("POST /api/auth/2fa/setup", s.handleTOTPSetup)
	mux.HandleFunc("POST /api/auth/2fa/verify", s.handleTOTPVerify)

	mux.HandleFunc("GET /api/instances", s.handleListInstances)
	mux.HandleFunc("POST /api/instances", s.handleCreateInstance)
	mux.HandleFunc("GET /api/instances/{id}", s.handleGetInstance)
	mux.HandleFunc("PATCH /api/instances/{id}", s.handleUpdateInstance)
	mux.HandleFunc("DELETE /api/instances/{id}", s.handleDeleteInstance)
	mux.HandleFunc("POST /api/instances/{id}/start", s.handleStartInstance)
	mux.HandleFunc("POST /api/instances/{id}/stop", s.handleStopInstance)
	mux.HandleFunc("POST /api/instances/{id}/restart", s.handleRestartInstance)
	mux.HandleFunc("POST /api/instances/{id}/command", s.handleSendCommand)

	mux.HandleFunc("GET /api/instances/{id}/console", s.handleConsoleWebSocket)

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
