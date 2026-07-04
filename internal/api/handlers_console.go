package api

import "net/http"

// TODO(console): implement the WebSocket protocol described in
// ARCHITECTURE.md section 4.2 (log/state/cmd_result frames from the server,
// command frames from the client). Needs a WebSocket library decision
// (e.g. nhooyr.io/websocket) before this can be implemented; not yet added
// to go.mod pending that choice.
func (s *Server) handleConsoleWebSocket(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
