package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"craftdeck/internal/instance"
)

// settingType is the kind of control the GUI settings form (FR-12) renders
// for one server.properties key -- distinct from the general file manager's
// raw text editing (FR-12a), which stays available for anything not on
// this curated list (custom loaders, advanced/rare keys, etc.).
type settingType string

const (
	settingBool   settingType = "bool"
	settingInt    settingType = "int"
	settingString settingType = "string"
	settingEnum   settingType = "enum"
)

// serverSettingDef describes one server.properties key the GUI form can
// show/edit. Options is only meaningful for settingEnum.
type serverSettingDef struct {
	Key         string
	Label       string
	Description string
	Type        settingType
	Options     []string
	// Default is shown when the key isn't present in server.properties yet
	// -- true for any instance CraftDeck has provisioned but never actually
	// booted, since provisionServerFiles only writes the handful of keys it
	// manages itself (see managedPropertyKeys); every other key, including
	// everything on this curated list, is normally filled in by Minecraft's
	// own server.properties defaults on first boot. Mirrors vanilla/Paper's
	// well-known default values so the form shows something meaningful
	// immediately instead of blank fields, without writing anything to disk
	// until the operator actually saves.
	Default string
}

// serverPropertyDefs is the curated list of common, safe-to-expose
// server.properties keys -- deliberately excludes anything CraftDeck itself
// writes and relies on (server-port, server-ip, online-mode, enable-rcon,
// rcon.port, rcon.password -- see provisionServerFiles/managedPropertyKeys),
// since editing those here without updating the instance DB/proxy
// registration would desync CraftDeck's own bookkeeping. The full raw file
// is still reachable (and editable) via the general file manager for
// anything not on this list.
var serverPropertyDefs = []serverSettingDef{
	{Key: "motd", Label: "서버 소개 문구 (MOTD)", Type: settingString, Default: "A Minecraft Server"},
	{Key: "difficulty", Label: "난이도", Type: settingEnum, Options: []string{"peaceful", "easy", "normal", "hard"}, Default: "easy"},
	{Key: "gamemode", Label: "게임 모드", Type: settingEnum, Options: []string{"survival", "creative", "adventure", "spectator"}, Default: "survival"},
	{Key: "hardcore", Label: "하드코어 모드", Type: settingBool, Default: "false"},
	{Key: "force-gamemode", Label: "접속 시 기본 게임 모드 강제", Type: settingBool, Default: "false"},
	{Key: "pvp", Label: "플레이어 간 전투(PVP) 허용", Type: settingBool, Default: "true"},
	{Key: "max-players", Label: "최대 접속 인원", Type: settingInt, Default: "20"},
	{Key: "view-distance", Label: "시야 거리 (청크)", Type: settingInt, Default: "10"},
	{Key: "simulation-distance", Label: "시뮬레이션 거리 (청크)", Type: settingInt, Default: "10"},
	{Key: "spawn-protection", Label: "스폰 보호 반경", Description: "op가 아니면 이 반경 안의 블록을 부수거나 설치할 수 없습니다.", Type: settingInt, Default: "16"},
	{Key: "allow-nether", Label: "네더 허용", Type: settingBool, Default: "true"},
	{Key: "allow-flight", Label: "비행 허용", Description: "모드 없이 비행하는 플레이어를 서버가 강제로 튕겨내지 않습니다 (서바이벌에서 부정행위 방지 목적으로 악용될 수 있음).", Type: settingBool, Default: "false"},
	{Key: "white-list", Label: "화이트리스트 사용", Type: settingBool, Default: "false"},
	{Key: "enforce-whitelist", Label: "화이트리스트 강제 (op도 예외 없음)", Type: settingBool, Default: "false"},
	{Key: "enable-command-block", Label: "커맨드 블록 허용", Type: settingBool, Default: "false"},
	{Key: "spawn-monsters", Label: "몬스터 스폰", Type: settingBool, Default: "true"},
	{Key: "spawn-animals", Label: "동물 스폰", Type: settingBool, Default: "true"},
	{Key: "spawn-npcs", Label: "마을 주민(NPC) 스폰", Type: settingBool, Default: "true"},
	{Key: "generate-structures", Label: "구조물 생성", Description: "마을, 요새 등 자연 생성 구조물 포함 여부입니다.", Type: settingBool, Default: "true"},
	{Key: "level-seed", Label: "월드 시드", Description: "새 월드를 처음 생성할 때만 적용됩니다.", Type: settingString, Default: ""},
	{Key: "max-world-size", Label: "월드 최대 반경 (블록)", Type: settingInt, Default: "29999984"},
	{Key: "player-idle-timeout", Label: "자동 추방까지의 유휴 시간 (분, 0=사용 안 함)", Type: settingInt, Default: "0"},
	{Key: "resource-pack", Label: "리소스 팩 URL", Type: settingString, Default: ""},
	{Key: "require-resource-pack", Label: "리소스 팩 강제 적용", Type: settingBool, Default: "false"},
}

// managedPropertyKeys are written and relied upon by CraftDeck itself (see
// provisionServerFiles) -- never exposed through the GUI settings form even
// if someone adds them to serverPropertyDefs by mistake.
var managedPropertyKeys = map[string]bool{
	"server-port":   true,
	"server-ip":     true,
	"online-mode":   true,
	"enable-rcon":   true,
	"rcon.port":     true,
	"rcon.password": true,
}

// parseProperty extracts one key's raw value from a server.properties-style
// file, ignoring blank lines and "#" comments. Returns ok=false if the key
// isn't present at all (as opposed to present with an empty value).
func parseProperty(content, key string) (value string, ok bool) {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq == -1 {
			continue
		}
		if strings.TrimSpace(line[:eq]) == key {
			return strings.TrimSpace(line[eq+1:]), true
		}
	}
	return "", false
}

// applyProperties rewrites content with updates applied -- existing
// key=value lines are updated in place (preserving every other line,
// comment, and ordering untouched); keys in updates that aren't already
// present are appended at the end, sorted for determinism.
func applyProperties(content string, updates map[string]string) string {
	lines := strings.Split(content, "\n")
	applied := map[string]bool{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq == -1 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if newVal, ok := updates[key]; ok {
			lines[i] = key + "=" + newVal
			applied[key] = true
		}
	}
	var toAppend []string
	for key := range updates {
		if !applied[key] {
			toAppend = append(toAppend, key)
		}
	}
	sort.Strings(toAppend)

	result := strings.Join(lines, "\n")
	if len(toAppend) == 0 {
		return result
	}
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	for _, key := range toAppend {
		result += key + "=" + updates[key] + "\n"
	}
	return result
}

type serverSettingValue struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	Description string      `json:"description,omitempty"`
	Type        settingType `json:"type"`
	Options     []string    `json:"options,omitempty"`
	Value       string      `json:"value"`
}

// handleGetServerSettings backs the instance detail page's GUI "설정" form
// (FR-12) -- a curated, labeled subset of server.properties, distinct from
// the general file manager's raw editing (FR-12a) which stays the tool for
// anything not on this list (custom loaders, rare/advanced keys).
func (s *Server) handleGetServerSettings(w http.ResponseWriter, r *http.Request) {
	inst, err := s.instances.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Kind != instance.KindServer {
		http.Error(w, "only server instances have server.properties", http.StatusBadRequest)
		return
	}
	content, err := os.ReadFile(filepath.Join(inst.WorkDir, "server.properties"))
	if err != nil {
		http.Error(w, "server.properties not found for this instance", http.StatusNotFound)
		return
	}

	out := make([]serverSettingValue, len(serverPropertyDefs))
	for i, def := range serverPropertyDefs {
		value, ok := parseProperty(string(content), def.Key)
		if !ok {
			value = def.Default
		}
		out[i] = serverSettingValue{
			Key:         def.Key,
			Label:       def.Label,
			Description: def.Description,
			Type:        def.Type,
			Options:     def.Options,
			Value:       value,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetServerSettings applies a batch of GUI-form edits (FR-12) to
// server.properties, rejecting any key not on the curated allowlist (either
// unrecognized entirely, or one of CraftDeck's own managed keys) so the
// form can never be used to desync the instance's actual game_port/RCON
// bookkeeping. Takes effect on the server's next boot, same as every other
// server.properties edit.
func (s *Server) handleSetServerSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inst, err := s.instances.Get(ctx, r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Kind != instance.KindServer {
		http.Error(w, "only server instances have server.properties", http.StatusBadRequest)
		return
	}

	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	allowed := make(map[string]serverSettingDef, len(serverPropertyDefs))
	for _, def := range serverPropertyDefs {
		allowed[def.Key] = def
	}
	updates := make(map[string]string, len(req))
	for key, value := range req {
		if managedPropertyKeys[key] {
			http.Error(w, fmt.Sprintf("%q is managed by CraftDeck itself and can't be edited here", key), http.StatusBadRequest)
			return
		}
		def, ok := allowed[key]
		if !ok {
			http.Error(w, fmt.Sprintf("unknown setting %q", key), http.StatusBadRequest)
			return
		}
		if def.Type == settingEnum {
			valid := false
			for _, opt := range def.Options {
				if opt == value {
					valid = true
					break
				}
			}
			if !valid {
				http.Error(w, fmt.Sprintf("invalid value %q for %q", value, key), http.StatusBadRequest)
				return
			}
		}
		updates[key] = value
	}

	propsPath := filepath.Join(inst.WorkDir, "server.properties")
	content, err := os.ReadFile(propsPath)
	if err != nil {
		http.Error(w, "server.properties not found for this instance", http.StatusNotFound)
		return
	}
	newContent := applyProperties(string(content), updates)
	if err := os.WriteFile(propsPath, []byte(newContent), 0o640); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chownInstanceFile(ctx, inst.ID, propsPath)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
