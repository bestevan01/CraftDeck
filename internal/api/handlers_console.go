package api

import (
	"bufio"
	"context"
	"net/http"
	"os/exec"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// consoleFrame matches the WebSocket protocol in ARCHITECTURE.md section 4.2.
type consoleFrame struct {
	Type    string `json:"type"`
	// Line intentionally has no omitempty: some RCON commands (e.g. "say")
	// return an empty string on success, and dropping the key entirely on
	// an empty result made the frontend receive an undefined "line" for
	// cmd_result frames, crashing its log-line parser.
	Line string `json:"line"`
	Status  string `json:"status,omitempty"`
	Command string `json:"command,omitempty"`
	Text    string `json:"text,omitempty"`
	OK      bool   `json:"ok,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handleConsoleWebSocket streams an instance's systemd journal output live
// and accepts free-text commands (FR-14, FR-15), executing them over the
// same RCON path GUI buttons use (FR-18).
func (s *Server) handleConsoleWebSocket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.instances.Get(r.Context(), id); err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	unit := "craftdeck-instance-" + id
	cmd := exec.CommandContext(ctx, "journalctl", "-u", unit, "-f", "-n", "50", "-o", "cat")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	defer cmd.Wait() //nolint:errcheck // killed via ctx cancellation on return

	// Nothing here ever wrote to the socket while the operator just sat
	// watching an idle console (no new log lines, no commands), so a
	// connection sitting behind any idle-timing-out intermediary -- a
	// reverse proxy, a home router's NAT table, a mobile carrier -- got
	// silently dropped with nothing on either side noticing (confirmed:
	// exactly this, consistently around 90s of inactivity, with the
	// frontend never reconnecting on its own afterward). A WS ping frame
	// every 20s keeps real traffic flowing on the socket so those
	// intermediaries see it as active; nhooyr's Ping already handles the
	// pong reply/wait internally, and a failed ping (the connection is
	// actually gone) cancels ctx so both goroutines below exit cleanly.
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := conn.Ping(ctx); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Stream journal lines to the client.
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			frame := consoleFrame{Type: "log", Line: scanner.Text()}
			if err := wsjson.Write(ctx, conn, frame); err != nil {
				return
			}
		}
	}()

	// Read commands from the client until it disconnects.
	for {
		var in consoleFrame
		if err := wsjson.Read(ctx, conn, &in); err != nil {
			return
		}
		if in.Type != "command" {
			continue
		}
		// Same execution path as the REST command endpoint (FR-18): both
		// go through the manager's persistent per-instance connection.
		result, execErr := s.rconMgr.Execute(id, in.Text)
		out := consoleFrame{Type: "cmd_result", Command: in.Text, OK: execErr == nil}
		if execErr != nil {
			out.Error = execErr.Error()
		} else {
			out.Line = result
		}
		if err := wsjson.Write(ctx, conn, out); err != nil {
			return
		}
	}
}
