package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"craftdeck/internal/instance"
	"craftdeck/internal/loader"
	"craftdeck/internal/process"

	"github.com/google/uuid"
)

// proxyMemoryMaxMB is the fixed memory allocation for the singleton Velocity
// proxy: it only relays packets (no world data, no per-player game state), so
// 1GB is generous even under a full house. Fixed rather than operator-tunable
// so the per-server memory slider (see handleCreateInstance/handleUpdateInstance)
// can reliably reserve it off the top and offer the rest to servers.
const proxyMemoryMaxMB = 1024

// ensureProxyInstance returns CraftDeck's singleton Velocity proxy,
// creating and provisioning one (newest stable Velocity version, lowest
// free port from 25577) the first time it's needed. There is only ever one
// -- servers default to sitting behind it (see handleCreateInstance)
// instead of being independently exposed, so it has to always exist rather
// than being something the operator creates and configures by hand.
func (s *Server) ensureProxyInstance(ctx context.Context) (*instance.Instance, error) {
	list, err := s.instances.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, inst := range list {
		if inst.Kind == instance.KindProxy {
			// Repairs installs that created their proxy before its memory
			// allocation was fixed at proxyMemoryMaxMB (see that const).
			if inst.MemoryMaxMB != proxyMemoryMaxMB {
				if err := s.instances.UpdateSettings(ctx, inst.ID, inst.GamePort, inst.RCONPort, inst.CPUQuotaPercent, proxyMemoryMaxMB); err != nil {
					return nil, err
				}
				inst.MemoryMaxMB = proxyMemoryMaxMB
			}
			return inst, nil
		}
	}

	versions, err := loader.FetchVelocityVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch velocity versions: %w", err)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no velocity versions available")
	}

	port := 25577
	for {
		inUse, err := s.gamePortInUse(ctx, port, "")
		if err != nil {
			return nil, err
		}
		if !inUse {
			break
		}
		port++
	}

	id := uuid.NewString()
	inst := &instance.Instance{
		ID:        id,
		Name:      "Velocity 프록시",
		Kind:      instance.KindProxy,
		Loader:    "velocity",
		MCVersion: versions[0],
		// Velocity 3.x requires Java 17+; 21 is what CraftDeck already
		// installs and uses as its own modern default elsewhere.
		JavaMajor:   21,
		GamePort:    port,
		WorkDir:     filepath.Join(s.dataDir, "instances", id),
		Status:      instance.StatusStopped,
		MemoryMaxMB: proxyMemoryMaxMB,
	}
	if err := provisionProxyFiles(ctx, inst); err != nil {
		return nil, err
	}
	if err := s.instances.Create(ctx, inst); err != nil {
		return nil, err
	}
	return inst, nil
}

// EnsureProxyRunning creates the singleton proxy if it doesn't exist yet
// and starts it if it isn't already running. Called once at daemon
// startup (see cmd/craftdeckd/main.go) so it's always available without
// the operator having to remember to start it after every deploy/reboot.
func (s *Server) EnsureProxyRunning(ctx context.Context) error {
	proxy, err := s.ensureProxyInstance(ctx)
	if err != nil {
		return err
	}
	active, err := s.supervisor.IsActive(ctx, proxy.ID)
	if err != nil {
		return err
	}
	if active {
		return nil
	}
	return s.startInstanceCore(ctx, proxy)
}

// addServerToProxy registers server as a backend of the singleton proxy
// (creating the proxy first if it doesn't exist yet), appended after any
// existing backends. Called automatically when a Paper server is created
// without explicitly opting out (see handleCreateInstance).
func (s *Server) addServerToProxy(ctx context.Context, server *instance.Instance) error {
	proxy, err := s.ensureProxyInstance(ctx)
	if err != nil {
		return err
	}
	existing, err := s.instances.ListProxyBackends(ctx, proxy.ID)
	if err != nil {
		return err
	}
	backends := append(existing, &instance.ProxyBackend{
		ProxyID:           proxy.ID,
		BackendInstanceID: server.ID,
		Priority:          len(existing),
	})
	return s.applyProxyBackends(ctx, proxy, backends)
}

// findProxy returns CraftDeck's singleton Velocity proxy, or nil if one
// hasn't been created yet (no server has ever needed to sit behind it -- see
// ensureProxyInstance). Unlike ensureProxyInstance, this never creates one:
// callers that only want to inspect/clean up existing state (deleting a
// server, looking up its subdomain) have nothing to do if there's no proxy.
func (s *Server) findProxy(ctx context.Context) (*instance.Instance, error) {
	list, err := s.instances.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, inst := range list {
		if inst.Kind == instance.KindProxy {
			return inst, nil
		}
	}
	return nil, nil
}

// removeServerFromProxy drops serverID from the proxy's backend list, if
// it's registered at all. Called when a server instance is deleted (see
// handleDeleteInstance) so it doesn't linger as a dangling entry.
func (s *Server) removeServerFromProxy(ctx context.Context, serverID string) error {
	proxy, err := s.findProxy(ctx)
	if err != nil || proxy == nil {
		return err
	}

	existing, err := s.instances.ListProxyBackends(ctx, proxy.ID)
	if err != nil {
		return err
	}
	filtered := make([]*instance.ProxyBackend, 0, len(existing))
	changed := false
	for _, b := range existing {
		if b.BackendInstanceID == serverID {
			changed = true
			continue
		}
		filtered = append(filtered, b)
	}
	if !changed {
		return nil
	}
	return s.applyProxyBackends(ctx, proxy, filtered)
}

// serverSubdomain returns the forced-host subdomain a server is registered
// under, if it's currently one of the proxy's backends. This is the only
// per-server proxy setting an operator manages day-to-day -- the proxy
// itself is a fixed, always-on singleton with no operator-facing settings
// (see ensureProxyInstance/proxyMemoryMaxMB), so its UI moved onto each
// server's own console instead of a separate proxy instance page.
func (s *Server) serverSubdomain(ctx context.Context, serverID string) (forcedHost string, registered bool, err error) {
	proxy, err := s.findProxy(ctx)
	if err != nil || proxy == nil {
		return "", false, err
	}
	backends, err := s.instances.ListProxyBackends(ctx, proxy.ID)
	if err != nil {
		return "", false, err
	}
	for _, b := range backends {
		if b.BackendInstanceID == serverID {
			return b.ForcedHost, true, nil
		}
	}
	return "", false, nil
}

// setServerSubdomain updates the forced-host subdomain for a server already
// sitting behind the proxy (see addServerToProxy).
func (s *Server) setServerSubdomain(ctx context.Context, serverID, forcedHost string) error {
	proxy, err := s.findProxy(ctx)
	if err != nil {
		return err
	}
	if proxy == nil {
		return fmt.Errorf("proxy does not exist yet")
	}
	backends, err := s.instances.ListProxyBackends(ctx, proxy.ID)
	if err != nil {
		return err
	}
	found := false
	for _, b := range backends {
		if b.BackendInstanceID == serverID {
			b.ForcedHost = forcedHost
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("server is not registered behind the proxy")
	}
	return s.applyProxyBackends(ctx, proxy, backends)
}

// forwardingSecret ensures the singleton proxy exists (creating it if this
// is the very first server CraftDeck has ever put behind it) and returns its
// player-info-forwarding secret, so a new Paper server can have its
// paper-global.yml pre-seeded to trust the proxy before its first launch
// (see provisionServerFiles).
func (s *Server) forwardingSecret(ctx context.Context) (string, error) {
	proxy, err := s.ensureProxyInstance(ctx)
	if err != nil {
		return "", err
	}
	secret, err := os.ReadFile(filepath.Join(proxy.WorkDir, "forwarding.secret"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(secret)), nil
}

// provisionProxyFiles sets up a Velocity proxy instance's work directory:
// downloads velocity.jar for the requested version, generates a random
// player-info-forwarding secret (FR-1c's "modern forwarding", the only mode
// CraftDeck wires up -- see resolveProxyBackendEntries for why backends
// must be Paper), and writes an initial velocity.toml with no backend
// servers yet.
func provisionProxyFiles(ctx context.Context, inst *instance.Instance) error {
	if err := os.MkdirAll(filepath.Dir(inst.WorkDir), 0o711); err != nil {
		return fmt.Errorf("create instances dir: %w", err)
	}
	if err := os.MkdirAll(inst.WorkDir, 0o750); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}

	secret, err := generateForwardingSecret()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(inst.WorkDir, "forwarding.secret"), []byte(secret), 0o640); err != nil {
		return fmt.Errorf("write forwarding.secret: %w", err)
	}
	if err := writeVelocityConfig(inst.WorkDir, inst.GamePort, nil); err != nil {
		return fmt.Errorf("write velocity.toml: %w", err)
	}

	if adapter, ok := loader.Get(inst.Loader); ok {
		if _, err := adapter.Download(ctx, inst.MCVersion, inst.WorkDir); err != nil {
			return fmt.Errorf("download %s server jar: %w", inst.Loader, err)
		}
	}

	username, err := process.EnsureInstanceUser(ctx, inst.ID)
	if err != nil {
		return fmt.Errorf("create instance user: %w", err)
	}
	if err := process.ChownRecursive(ctx, inst.WorkDir, username); err != nil {
		return fmt.Errorf("chown work dir: %w", err)
	}
	return nil
}

func generateForwardingSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate forwarding secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// proxyBackendEntry is the resolved (instance-name + address) form of a
// instance.ProxyBackend, ready to render into velocity.toml.
type proxyBackendEntry struct {
	Name       string // Velocity's internal server name -- the backend instance's own Name
	Address    string // "127.0.0.1:<game_port>"
	ForcedHost string // subdomain to route to this server; empty = not forced-hosted
}

// velocityConfigTemplate is Velocity 3.x's default velocity.toml, with the
// dynamic parts ([servers]/try/[forced-hosts], bind port, query port)
// templated in. The [advanced]/[query] sections are left at Velocity's own
// stock defaults.
const velocityConfigTemplate = `config-version = "2.7"
bind = "0.0.0.0:%d"
motd = "<green>A Velocity Server, managed by CraftDeck"
show-max-players = 500
online-mode = true
player-info-forwarding-mode = "modern"
forwarding-secret-file = "forwarding.secret"
announce-forge = false
kick-existing-players = false
ping-passthrough = "disabled"
enable-player-address-logging = true

[servers]
%stry = [
%s]

[forced-hosts]
%s
[advanced]
compression-threshold = 256
compression-level = -1
login-ratelimit = 3000
connection-timeout = 5000
read-timeout = 30000
haproxy-protocol = false
tcp-fast-open = false
bungee-plugin-message-channel = true
show-ping-requests = false
failover-on-unexpected-server-disconnect = true
announce-proxy-commands = true
log-command-executions = false
log-player-connections = true
accepts-transfers = false

[query]
enabled = false
port = %d
map = "Velocity"
show-plugins = false
`

func writeVelocityConfig(workDir string, listenPort int, backends []proxyBackendEntry) error {
	var servers, try, forcedHosts strings.Builder
	for _, b := range backends {
		fmt.Fprintf(&servers, "%q = %q\n", b.Name, b.Address)
		fmt.Fprintf(&try, "    %q,\n", b.Name)
		if b.ForcedHost != "" {
			fmt.Fprintf(&forcedHosts, "%q = [%q]\n", b.ForcedHost, b.Name)
		}
	}

	content := fmt.Sprintf(velocityConfigTemplate,
		listenPort, servers.String(), try.String(), forcedHosts.String(), listenPort)
	return os.WriteFile(filepath.Join(workDir, "velocity.toml"), []byte(content), 0o640)
}

// resolveProxyBackendEntries turns DB-level backend assignments into the
// (name, address) pairs velocity.toml needs, rejecting any non-Paper
// backend: Velocity's "modern" player info forwarding (the only mode
// CraftDeck wires up, chosen over "legacy" because it's far harder to
// spoof) requires the backend server to trust a shared secret via Paper's
// own velocity-support config -- something only Paper (not vanilla)
// exposes. See requirements.md FR-1c.
func resolveProxyBackendEntries(ctx context.Context, s *Server, backends []*instance.ProxyBackend) ([]proxyBackendEntry, error) {
	entries := make([]proxyBackendEntry, 0, len(backends))
	for _, b := range backends {
		backend, err := s.instances.Get(ctx, b.BackendInstanceID)
		if err != nil {
			return nil, fmt.Errorf("backend instance %s not found", b.BackendInstanceID)
		}
		if !strings.EqualFold(backend.Loader, "paper") {
			return nil, fmt.Errorf(
				"'%s'는 %s 구동기라 Velocity 뒤에 놓을 수 없습니다 (모던 포워딩은 Paper만 지원합니다)",
				backend.Name, backend.Loader,
			)
		}
		entries = append(entries, proxyBackendEntry{
			Name:       backend.Name,
			Address:    fmt.Sprintf("127.0.0.1:%d", backend.GamePort),
			ForcedHost: b.ForcedHost,
		})
	}
	return entries, nil
}

// applyProxyBackends regenerates the proxy's velocity.toml to match
// backends, persists the assignment, and -- since the proxy is meant to
// always be available rather than something the operator manually restarts
// after every change -- restarts it immediately if it's currently running
// so the new backend list takes effect right away.
func (s *Server) applyProxyBackends(ctx context.Context, proxy *instance.Instance, backends []*instance.ProxyBackend) error {
	entries, err := resolveProxyBackendEntries(ctx, s, backends)
	if err != nil {
		return err
	}
	if err := writeVelocityConfig(proxy.WorkDir, proxy.GamePort, entries); err != nil {
		return err
	}
	if err := s.instances.SetProxyBackends(ctx, proxy.ID, backends); err != nil {
		return err
	}

	if proxy.Status == instance.StatusRunning || proxy.Status == instance.StatusStarting {
		if err := s.stopInstanceCore(ctx, proxy); err != nil {
			return fmt.Errorf("restart proxy to apply new backend list: %w", err)
		}
		if err := s.startInstanceCore(ctx, proxy); err != nil {
			return fmt.Errorf("restart proxy to apply new backend list: %w", err)
		}
	}
	return nil
}

// handleListProxyBackends returns the backend servers currently assigned to
// the singleton proxy, in priority order.
func (s *Server) handleListProxyBackends(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil || inst.Kind != instance.KindProxy {
		http.Error(w, "proxy instance not found", http.StatusNotFound)
		return
	}
	backends, err := s.instances.ListProxyBackends(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, backends)
}

type setProxyBackendsRequest struct {
	Backends []struct {
		BackendInstanceID string `json:"backend_instance_id"`
		Priority          int    `json:"priority"`
		ForcedHost        string `json:"forced_host"`
	} `json:"backends"`
}

// handleSetProxyBackends replaces the proxy's entire backend list -- used
// for manual fine-tuning (reordering priority, adding a forced-host
// subdomain, removing a server) on top of what handleCreateInstance/
// handleDeleteInstance already keep in sync automatically.
func (s *Server) handleSetProxyBackends(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	proxy, err := s.instances.Get(ctx, id)
	if err != nil || proxy.Kind != instance.KindProxy {
		http.Error(w, "proxy instance not found", http.StatusNotFound)
		return
	}

	var req setProxyBackendsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	backends := make([]*instance.ProxyBackend, 0, len(req.Backends))
	for _, b := range req.Backends {
		backends = append(backends, &instance.ProxyBackend{
			ProxyID:           id,
			BackendInstanceID: b.BackendInstanceID,
			Priority:          b.Priority,
			ForcedHost:        b.ForcedHost,
		})
	}

	if err := s.applyProxyBackends(ctx, proxy, backends); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updated, err := s.instances.ListProxyBackends(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleGetForwardingSecret returns the proxy's player-info-forwarding
// secret. New Paper servers get this pre-seeded into paper-global.yml
// automatically at creation (see provisionServerFiles/forwardingSecret), so
// this endpoint mainly exists for pre-existing servers created before that,
// or ones an operator wants to attach to the proxy by hand -- CraftDeck
// still won't rewrite an *existing* server's paper-global.yml itself, since
// hand-patching a config format we don't otherwise parse risks corrupting
// settings we don't know about, but pasting one string in is simple and
// safe. The instance list/get endpoints never include this (same treatment
// as RCONPassword), so it's only reachable via this dedicated endpoint.
func (s *Server) handleGetForwardingSecret(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil || inst.Kind != instance.KindProxy {
		http.Error(w, "proxy instance not found", http.StatusNotFound)
		return
	}
	secret, err := os.ReadFile(filepath.Join(inst.WorkDir, "forwarding.secret"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": strings.TrimSpace(string(secret))})
}

type serverSubdomainResponse struct {
	Registered bool   `json:"registered"`
	ForcedHost string `json:"forced_host"`
}

// handleGetServerSubdomain returns the subdomain a server is reachable
// under, from the server's own instance ID rather than the proxy's -- the
// proxy is hidden from the UI entirely (see requirements.md's proxy-only-by-
// default design), so subdomain management lives on each server's own
// console instead of a "backends" tab on a proxy instance page.
func (s *Server) handleGetServerSubdomain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	forcedHost, registered, err := s.serverSubdomain(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, serverSubdomainResponse{Registered: registered, ForcedHost: forcedHost})
}

type setServerSubdomainRequest struct {
	ForcedHost string `json:"forced_host"`
}

// handleSetServerSubdomain updates the subdomain for a server already
// sitting behind the proxy (see addServerToProxy -- every Paper server not
// explicitly opted out gets registered there at creation).
func (s *Server) handleSetServerSubdomain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req setServerSubdomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ForcedHost) == "" {
		http.Error(w, "forced_host must not be empty", http.StatusBadRequest)
		return
	}
	if err := s.setServerSubdomain(r.Context(), id, req.ForcedHost); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	forcedHost, registered, err := s.serverSubdomain(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, serverSubdomainResponse{Registered: registered, ForcedHost: forcedHost})
}
