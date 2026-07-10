package api

import (
	"fmt"
	"net/http"
	"time"

	"craftdeck/internal/mcping"
)

// handleOnlinePlayers reports who's currently online via Minecraft's own
// Server List Ping protocol instead of RCON's "list" command -- plugins
// (EssentialsX confirmed on real hardware) can freely reformat "list"'s text
// output, silently breaking any parser built around vanilla's exact
// wording, whereas Status Ping is a fixed protocol no plugin can touch.
func (s *Server) handleOnlinePlayers(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	status, err := mcping.Ping(r.Context(), fmt.Sprintf("127.0.0.1:%d", inst.GamePort), 3*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	names := make([]string, 0, len(status.Players.Sample))
	for _, p := range status.Players.Sample {
		names = append(names, p.Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"online": status.Players.Online,
		"max":    status.Players.Max,
		"sample": names,
	})
}
