package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// handleListBans reports currently banned players. Vanilla has no
// machine-friendly "list bans" query, so this runs the "banlist" RCON
// command (the same one an operator would type) and parses its human-
// readable text response.
func (s *Server) handleListBans(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.instances.Get(r.Context(), id); err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	result, err := s.rconMgr.Execute(id, "banlist")
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"players": parseBanlist(result)})
}

// banlistLineRE matches vanilla's per-line banlist format:
// "<name> was banned by <banner>[: <reason>]". Matched on the Minecraft
// username character set ([A-Za-z0-9_]{1,16}) rather than \S+, since the
// real server output glues the summary prefix directly onto the first name
// with no space (e.g. "There are 1 ban(s):Steve was banned by ...") --
// \S+ would have swallowed "ban(s):" into the captured name.
var banlistLineRE = regexp.MustCompile(`([A-Za-z0-9_]{1,16}) was banned by`)

func parseBanlist(result string) []string {
	names := []string{}
	for _, line := range strings.Split(result, "\n") {
		if m := banlistLineRE.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
		}
	}
	return names
}

// handleListWhitelist reports whether whitelist enforcement is on (read
// from server.properties -- there's no RCON query for this, only the
// "whitelist on"/"whitelist off" commands to set it) and, only when it is,
// the whitelisted players via the "whitelist list" RCON command.
func (s *Server) handleListWhitelist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	enabled, err := readServerPropertyBool(inst.WorkDir, "white-list")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !enabled {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "players": []string{}})
		return
	}

	result, err := s.rconMgr.Execute(id, "whitelist list")
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "players": parseColonSeparatedNames(result)})
}

// readServerProperty reads a single "key=value" line from server.properties.
// found is false (not an error) if the file or key doesn't exist yet -- a
// server that's never been started has no server.properties at all.
func readServerProperty(workDir, key string) (value string, found bool, err error) {
	data, err := os.ReadFile(filepath.Join(workDir, "server.properties"))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			return strings.TrimSpace(v), true, nil
		}
	}
	return "", false, nil
}

// readServerPropertyBool is readServerProperty for boolean-valued keys
// (e.g. white-list), defaulting to false when the key/file is missing --
// vanilla's own default for white-list.
func readServerPropertyBool(workDir, key string) (bool, error) {
	value, found, err := readServerProperty(workDir, key)
	if err != nil || !found {
		return false, err
	}
	return value == "true", nil
}

// parseColonSeparatedNames extracts a comma-separated name list following
// the first colon in a vanilla status response (used by both "list" and
// "whitelist list"). Splitting on the colon rather than matching a fixed
// amount of whitespace after it is deliberate: the banlist parsing bug
// (FR ref: real output glues "ban(s):" directly onto the first name with no
// space) showed this server's text formatting isn't reliably spaced.
func parseColonSeparatedNames(result string) []string {
	idx := strings.Index(result, ":")
	if idx == -1 {
		return []string{}
	}
	namesPart := strings.TrimSpace(result[idx+1:])
	if namesPart == "" {
		return []string{}
	}
	parts := strings.Split(namesPart, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	return names
}

// opEntry mirrors one record in a Minecraft server's ops.json.
type opEntry struct {
	UUID                string `json:"uuid"`
	Name                string `json:"name"`
	Level               int    `json:"level"`
	BypassesPlayerLimit bool   `json:"bypassesPlayerLimit"`
}

// handleListOps reports server operators. Unlike bans, vanilla has no RCON
// command for this at all (op status is only ever written to disk), so we
// read ops.json directly from the instance's work directory instead of
// going through RCON.
func (s *Server) handleListOps(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(filepath.Join(inst.WorkDir, "ops.json"))
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, []opEntry{}) // no one has ever been opped
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var ops []opEntry
	if err := json.Unmarshal(data, &ops); err != nil {
		http.Error(w, "malformed ops.json: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ops)
}
