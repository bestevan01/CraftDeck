package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"craftdeck/internal/ddns"
	"craftdeck/internal/dns"
	"craftdeck/internal/instance"
	"craftdeck/internal/javaruntime"
	"craftdeck/internal/loader"
	"craftdeck/internal/modrinth"
	"craftdeck/internal/network"
	"craftdeck/internal/process"
	"craftdeck/internal/secrets"

	"github.com/google/uuid"
)

// proxyMemoryMaxMB is the fixed memory allocation for the singleton Velocity
// proxy: it only relays packets (no world data, no per-player game state), so
// 1GB is generous even under a full house. Fixed rather than operator-tunable
// so the per-server memory slider (see handleCreateInstance/handleUpdateInstance)
// can reliably reserve it off the top and offer the rest to servers.
const proxyMemoryMaxMB = 1024

// proxyDefaultJavaMajor is used when the fill API's per-version Java
// requirement can't be determined (network hiccup, field missing) -- this
// only needs to be "recent enough for most Velocity releases", since a real
// mismatch just means a slower failure (crash on launch) rather than a
// silent one, the same as before this lookup existed at all.
const proxyDefaultJavaMajor = 21

// proxyJavaMajor asks the fill API which Java major a given Velocity
// version actually requires and maps it to the nearest one CraftDeck has
// installed, falling back to proxyDefaultJavaMajor if that lookup fails --
// see loader.FetchVelocityJavaMinimum's doc comment for why this can't just
// be hardcoded once and forgotten (Velocity 4.0.0 bumped the requirement to
// Java 25, breaking the "21 is enough" assumption this used to make).
func proxyJavaMajor(ctx context.Context, velocityVersion string) int {
	minimum, ok := loader.FetchVelocityJavaMinimum(ctx, velocityVersion)
	if !ok {
		return proxyDefaultJavaMajor
	}
	return javaruntime.NearestInstalledMajor(minimum)
}

// ensureProxyInstance returns CraftDeck's singleton Velocity proxy,
// creating and provisioning one (newest stable Velocity version, lowest
// free port starting from 25565 -- Minecraft's own standard port, so a
// player who just types the bare IP/domain with no port suffix reaches the
// proxy the same way they'd reach any single vanilla server) the first
// time it's needed. There is only ever one -- servers default to sitting
// behind it (see handleCreateInstance) instead of being independently
// exposed, so it has to always exist rather than being something the
// operator creates and configures by hand. Backend servers themselves
// start their own port search one above this range (see
// nextFreeGamePort(ctx, 25566) in handlers_instance.go) so they never
// collide with the proxy's own port.
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

	latestVersion, err := loader.FetchLatestBuildableVelocityVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch velocity versions: %w", err)
	}

	port := 25565
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
		MCVersion: latestVersion,
		// A hardcoded Java major used to live here ("21 is enough") --
		// confirmed on real hardware that it isn't: Velocity 4.0.0 requires
		// Java 25 and crashes on launch with UnsupportedClassVersionError
		// under 21, so this now asks the fill API what the *actual* build
		// needs and falls back to CraftDeck's newest installed Temurin if
		// that lookup itself fails (better to try than refuse to start).
		JavaMajor:   proxyJavaMajor(ctx, latestVersion),
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
// without explicitly opting out (see handleCreateInstance), and manually
// via handleRegisterBehindProxy. A no-op (but not an error) if server is
// already registered, so the manual endpoint stays safe to call more than
// once (e.g. retried after an unrelated failure).
func (s *Server) addServerToProxy(ctx context.Context, server *instance.Instance) error {
	proxy, err := s.ensureProxyInstance(ctx)
	if err != nil {
		return err
	}
	existing, err := s.instances.ListProxyBackends(ctx, proxy.ID)
	if err != nil {
		return err
	}
	for _, b := range existing {
		if b.BackendInstanceID == server.ID {
			return nil
		}
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
	if err := s.applyProxyBackends(ctx, proxy, backends); err != nil {
		return err
	}
	// FR-28: the whole point of assigning a forced-host subdomain is to
	// actually be able to connect to it, so create/update its A record right
	// away instead of waiting for SyncMainDomainDNS's next periodic pass
	// (FR-30, see internal/ddns.Manager's reconcile loop). Best-effort: a
	// Cloudflare hiccup here shouldn't undo the subdomain assignment that
	// already succeeded above, so this only logs.
	if err := s.SyncMainDomainDNS(ctx); err != nil {
		log.Printf("set subdomain: sync main-domain DNS records: %v", err)
	}
	return nil
}

// SyncMainDomainDNS implements FR-28/30 for the owned-main-domain path:
// for every server currently assigned a forced-host subdomain (see
// setServerSubdomain), make sure Cloudflare's A record for that subdomain
// actually points at the router's current public IP. A no-op if no
// main_domain is registered (nothing to sync) or no proxy exists yet (no
// forced-host subdomain could have been assigned). Called synchronously
// right after an operator assigns/changes a subdomain (instant feedback,
// same reasoning as the free-subdomain path's immediate Reconcile call),
// and periodically by internal/ddns.Manager's reconcile loop so a WAN IP
// change eventually gets picked up too (FR-30) without the operator having
// to touch anything.
func (s *Server) SyncMainDomainDNS(ctx context.Context) error {
	config, err := s.domains.Get(ctx)
	if err != nil {
		return err
	}
	if config == nil || config.Kind != ddns.KindMainDomain || config.ZoneID == "" {
		return nil
	}

	proxy, err := s.findProxy(ctx)
	if err != nil || proxy == nil {
		return err
	}
	backends, err := s.instances.ListProxyBackends(ctx, proxy.ID)
	if err != nil {
		return err
	}
	hasForcedHost := false
	for _, b := range backends {
		if b.ForcedHost != "" {
			hasForcedHost = true
			break
		}
	}
	if !hasForcedHost {
		return nil
	}

	token, err := secrets.Decrypt(s.masterKey, config.TokenEncrypted)
	if err != nil {
		return fmt.Errorf("decrypt cloudflare token: %w", err)
	}
	publicIP, err := network.FetchPublicIP(ctx)
	if err != nil {
		return fmt.Errorf("fetch public ip: %w", err)
	}
	// Best-effort: most home connections have no public IPv6 at all, so a
	// failure here just means "skip AAAA", not "abort the whole sync" (see
	// FetchPublicIPv6's doc comment).
	publicIPv6, ipv6Available := "", false
	if ip, err := network.FetchPublicIPv6(ctx); err == nil {
		publicIPv6, ipv6Available = ip, true
	}

	seen := map[string]bool{} // several backends can share one forced host (see writeVelocityConfig)
	for _, b := range backends {
		if b.ForcedHost == "" || seen[b.ForcedHost] {
			continue
		}
		seen[b.ForcedHost] = true
		if err := dns.UpsertARecord(ctx, token, config.ZoneID, b.ForcedHost, publicIP); err != nil {
			log.Printf("sync main-domain DNS: upsert A record for %s: %v (continuing with the rest)", b.ForcedHost, err)
		}
		if ipv6Available {
			if err := dns.UpsertAAAARecord(ctx, token, config.ZoneID, b.ForcedHost, publicIPv6); err != nil {
				log.Printf("sync main-domain DNS: upsert AAAA record for %s: %v (continuing with the rest)", b.ForcedHost, err)
			}
		}
		// FR-29: every forced-host subdomain routes through this same
		// singleton proxy, so the SRV target port is always the proxy's own
		// game_port, not something per-server (see UpsertSRVRecord's doc
		// comment).
		if err := dns.UpsertSRVRecord(ctx, token, config.ZoneID, b.ForcedHost, proxy.GamePort); err != nil {
			log.Printf("sync main-domain DNS: upsert SRV record for %s: %v (continuing with the rest)", b.ForcedHost, err)
		}
	}
	return nil
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
	var servers, try strings.Builder
	// Group backends sharing the same ForcedHost into one ordered list
	// instead of one "host = [name]" line per backend -- a second backend
	// on the same subdomain would otherwise produce a duplicate TOML key
	// (broken config). Grouping them instead gives that subdomain its own
	// priority-ordered fallback list, using exactly the same "try the next
	// one down if this one's unreachable" mechanism Velocity already
	// applies to the top-level try list (requirements.md FR-1d) -- no
	// separate health-check poller needed: a new connection attempt (or a
	// reconnect after failover-on-unexpected-server-disconnect kicks in)
	// re-evaluates the list fresh every time, so it naturally prefers a
	// recovered higher-priority server again on its own. backends is
	// already priority-ordered (see ListProxyBackends), so the order
	// entries are appended in below is preserved within each group.
	forcedHostOrder := make([]string, 0)
	forcedHostServers := make(map[string][]string)
	for _, b := range backends {
		fmt.Fprintf(&servers, "%q = %q\n", b.Name, b.Address)
		fmt.Fprintf(&try, "    %q,\n", b.Name)
		if b.ForcedHost != "" {
			if _, seen := forcedHostServers[b.ForcedHost]; !seen {
				forcedHostOrder = append(forcedHostOrder, b.ForcedHost)
			}
			forcedHostServers[b.ForcedHost] = append(forcedHostServers[b.ForcedHost], b.Name)
		}
	}

	var forcedHosts strings.Builder
	for _, host := range forcedHostOrder {
		names := forcedHostServers[host]
		quoted := make([]string, len(names))
		for i, name := range names {
			quoted[i] = fmt.Sprintf("%q", name)
		}
		fmt.Fprintf(&forcedHosts, "%q = [%s]\n", host, strings.Join(quoted, ", "))
	}

	content := fmt.Sprintf(velocityConfigTemplate,
		listenPort, servers.String(), try.String(), forcedHosts.String(), listenPort)
	return os.WriteFile(filepath.Join(workDir, "velocity.toml"), []byte(content), 0o640)
}

// supportsVelocityForwarding reports whether loaderName's server software
// can be made to trust Velocity's "modern" player-info forwarding (the only
// mode CraftDeck wires up, chosen over "legacy" because it's far harder to
// spoof). Purpur, Folia, Pufferfish, and Leaf are all Paper forks that
// carry the same proxies.velocity config forward unchanged, so they
// qualify exactly like Paper itself. Fabric/NeoForge have no built-in
// equivalent, but installFabricProxyMods/installNeoForgeProxyMod (see
// handlers_instance.go) install a companion forwarding mod (FabricProxy-Lite
// +Fabric API, or NeoForged Velocity Support) and pre-seed its config/a
// forwarding.secret file with the same secret, so they qualify too --
// everything else (Vanilla, and Forge until an equivalent mod is wired up)
// doesn't. This governs the *automatic* default at creation time
// (handleCreateInstance) -- a manually-uploaded custom loader (FR-3) can
// still be added to the proxy afterward through an explicit operator
// action; see handleSetProxyBackends's relaxed check.
//
// This is necessary but not sufficient for NeoForge specifically -- see
// supportsVelocityForwardingForVersion, which callers that know the actual
// Minecraft version should use instead.
func supportsVelocityForwarding(loaderName string) bool {
	switch strings.ToLower(loaderName) {
	case "paper", "purpur", "folia", "pufferfish", "leaf", "fabric", "neoforge":
		return true
	default:
		return false
	}
}

// supportsVelocityForwardingForVersion refines supportsVelocityForwarding
// for loaders whose forwarding mod isn't published for every Minecraft
// version the loader itself supports -- currently just NeoForge, whose
// NeoForged Velocity Support mod (see installNeoForgeProxyMod) only has
// builds for a handful of versions on Modrinth so far. A NeoForge instance
// on any other version falls back to independent exposure, same as
// Vanilla, rather than failing creation outright over something the
// operator can't fix from CraftDeck's side.
func supportsVelocityForwardingForVersion(ctx context.Context, loaderName, mcVersion string) bool {
	if !supportsVelocityForwarding(loaderName) {
		return false
	}
	if strings.EqualFold(loaderName, "neoforge") {
		_, err := modrinth.BestVersion(ctx, "neoforged-velocity-support", "neoforge", mcVersion)
		return err == nil
	}
	return true
}

// resolveProxyBackendEntries turns DB-level backend assignments into the
// (name, address) pairs velocity.toml needs. It doesn't gatekeep by loader
// here -- supportsVelocityForwardingForVersion already decides *whether a
// backend gets added automatically* at creation time (handleCreateInstance),
// so by the time an entry reaches this function it's either already known
// good, or the operator added it deliberately through the manual
// register/unregister endpoints below (a custom/unlisted loader, FR-3,
// that the operator has confirmed themselves trusts the forwarding secret
// -- CraftDeck has no way to verify that itself for an arbitrary jar).
func resolveProxyBackendEntries(ctx context.Context, s *Server, backends []*instance.ProxyBackend) ([]proxyBackendEntry, error) {
	entries := make([]proxyBackendEntry, 0, len(backends))
	for _, b := range backends {
		backend, err := s.instances.Get(ctx, b.BackendInstanceID)
		if err != nil {
			return nil, fmt.Errorf("backend instance %s not found", b.BackendInstanceID)
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
	// ProxyPort is the singleton proxy's own game_port, included whenever
	// Registered is true -- a server sitting behind the proxy is bound to
	// 127.0.0.1 only, so *this* is the port a player actually connects to
	// reach it (directly, or via ForcedHost), not the server's own
	// game_port. Lets the instance detail page's "접속 주소" card show a
	// real, working address for a proxied server too (see
	// web/src/routes/instances/[id]/+page.svelte).
	ProxyPort int `json:"proxy_port,omitempty"`
}

// handleGetServerSubdomain returns the subdomain a server is reachable
// under, from the server's own instance ID rather than the proxy's -- the
// proxy is hidden from the UI entirely (see requirements.md's proxy-only-by-
// default design), so subdomain management lives on each server's own
// console instead of a "backends" tab on a proxy instance page.
func (s *Server) handleGetServerSubdomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	forcedHost, registered, err := s.serverSubdomain(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := serverSubdomainResponse{Registered: registered, ForcedHost: forcedHost}
	if registered {
		if proxy, err := s.findProxy(ctx); err == nil && proxy != nil {
			resp.ProxyPort = proxy.GamePort
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type setServerSubdomainRequest struct {
	ForcedHost string `json:"forced_host"`
}

// handleSetServerSubdomain updates the subdomain for a server already
// sitting behind the proxy (see addServerToProxy -- every Paper server not
// explicitly opted out gets registered there at creation).
func (s *Server) handleSetServerSubdomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	if err := s.setServerSubdomain(ctx, id, req.ForcedHost); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	forcedHost, registered, err := s.serverSubdomain(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := serverSubdomainResponse{Registered: registered, ForcedHost: forcedHost}
	if registered {
		if proxy, err := s.findProxy(ctx); err == nil && proxy != nil {
			resp.ProxyPort = proxy.GamePort
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type proxyStatusResponse struct {
	Exists bool `json:"exists"`
	// Running lets the frontend free up the 1GB reserved for the proxy
	// (proxyMemoryMaxMB) toward independently-exposed instances' memory
	// sliders whenever it isn't actually running -- not just whenever it
	// doesn't exist at all. In steady state the two track each other
	// closely (FR-1f's ReconcileProxyMode/EnsureProxyRunning keep a
	// registered main domain's proxy running, and tear the instance down
	// entirely otherwise), but this stays correct through the brief window
	// where it exists yet hasn't (re)started yet.
	Running         bool   `json:"running"`
	CurrentVersion  string `json:"current_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

// handleGetProxyStatus reports the singleton proxy's current Velocity
// version against the newest one available, so the UI can surface an
// "update available" affordance. Needed because ensureProxyInstance only
// ever picks a version once, at creation -- it never re-checks afterward,
// so the proxy can silently fall behind new Velocity releases (e.g. the
// build that first added support for Minecraft's 26.x protocol).
func (s *Server) handleGetProxyStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	proxy, err := s.findProxy(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if proxy == nil {
		writeJSON(w, http.StatusOK, proxyStatusResponse{Exists: false})
		return
	}

	latest, err := loader.FetchLatestBuildableVelocityVersion(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, proxyStatusResponse{
		Exists:          true,
		Running:         proxy.Status == instance.StatusRunning,
		CurrentVersion:  proxy.MCVersion,
		LatestVersion:   latest,
		UpdateAvailable: latest != "" && latest != proxy.MCVersion,
	})
}

type upgradeProxyResponse struct {
	Upgraded bool   `json:"upgraded"`
	Version  string `json:"version"`
}

// handleUpgradeProxy replaces the singleton proxy's jar with the newest
// available Velocity build and restarts it to pick up support for newer
// Minecraft protocol versions (see handleGetProxyStatus's doc comment).
// This is a brief connectivity outage for every backend server sitting
// behind the proxy while it restarts.
func (s *Server) handleUpgradeProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	proxy, err := s.findProxy(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if proxy == nil {
		http.Error(w, "proxy does not exist yet", http.StatusNotFound)
		return
	}

	latest, err := loader.FetchLatestBuildableVelocityVersion(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("fetch velocity versions: %v", err), http.StatusBadGateway)
		return
	}
	if latest == proxy.MCVersion {
		writeJSON(w, http.StatusOK, upgradeProxyResponse{Upgraded: false, Version: latest})
		return
	}

	if err := s.stopInstanceCore(ctx, proxy); err != nil {
		http.Error(w, fmt.Sprintf("stop proxy: %v", err), http.StatusInternalServerError)
		return
	}

	adapter, _ := loader.Get("velocity") // always registered -- see loader.go's registry
	if _, err := adapter.Download(ctx, latest, proxy.WorkDir); err != nil {
		http.Error(w, fmt.Sprintf("download velocity %s: %v", latest, err), http.StatusInternalServerError)
		return
	}
	username, err := process.EnsureInstanceUser(ctx, proxy.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := process.ChownRecursive(ctx, proxy.WorkDir, username); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	javaMajor := proxyJavaMajor(ctx, latest)
	if err := s.instances.UpdateVersion(ctx, proxy.ID, latest, javaMajor); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	proxy.MCVersion = latest
	proxy.JavaMajor = javaMajor

	if err := s.startInstanceCore(ctx, proxy); err != nil {
		http.Error(w, fmt.Sprintf("restart proxy after upgrade (jar was already replaced -- retry starting it manually): %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, upgradeProxyResponse{Upgraded: true, Version: latest})
}

// patchServerProperties reads workDir/server.properties (if it exists),
// overwrites/inserts the given key=value pairs, and writes it back --
// preserving every other line and its original order, unlike
// provisionServerFiles' server.properties which is only ever written fresh
// at creation. Used by handleRegisterBehindProxy/handleUnregisterFromProxy
// to flip server-ip/online-mode without touching anything else the
// operator (or the server itself) has configured since.
func patchServerProperties(workDir string, updates map[string]string) error {
	path := filepath.Join(workDir, "server.properties")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	remaining := make(map[string]string, len(updates))
	for k, v := range updates {
		remaining[k] = v
	}

	var lines []string
	if len(existing) > 0 {
		lines = strings.Split(strings.TrimRight(string(existing), "\n"), "\n")
	}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key := strings.SplitN(trimmed, "=", 2)[0]
		if newValue, ok := remaining[key]; ok {
			lines[i] = key + "=" + newValue
			delete(remaining, key)
		}
	}
	for k, v := range remaining { // wasn't already present -- append it
		lines = append(lines, k+"="+v)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o640)
}

type registerProxyResponse struct {
	ForwardingSecret string `json:"forwarding_secret"`
}

// handleRegisterBehindProxy manually adds an already-independently-exposed
// server instance to the singleton proxy's backend list -- the escape
// hatch for a custom/manually-uploaded loader (FR-3) that
// supportsVelocityForwardingForVersion doesn't recognize, so it never got
// added automatically at creation. Registering here doesn't verify the
// server software actually understands Velocity's forwarding secret --
// CraftDeck has no general way to check that for an arbitrary jar -- so
// the operator is responsible for having configured that trust themselves
// (e.g. via the file manager, editing whatever config their loader needs)
// before this is actually useful; the response includes the secret so they
// can go do exactly that.
func (s *Server) handleRegisterBehindProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Kind != instance.KindServer {
		http.Error(w, "only server instances can be registered behind the proxy", http.StatusBadRequest)
		return
	}
	if inst.Status != instance.StatusStopped && inst.Status != instance.StatusCrashed {
		http.Error(w, "instance must be stopped before changing its proxy registration", http.StatusConflict)
		return
	}

	secret, err := s.registerServerBehindProxyCore(ctx, inst)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, registerProxyResponse{ForwardingSecret: secret})
}

// handleUnregisterFromProxy reverses handleRegisterBehindProxy -- removes
// the instance from the proxy's backend list and reopens it to direct
// connections (server-ip cleared, online-mode back to Mojang
// authentication).
func (s *Server) handleUnregisterFromProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Status != instance.StatusStopped && inst.Status != instance.StatusCrashed {
		http.Error(w, "instance must be stopped before changing its proxy registration", http.StatusConflict)
		return
	}
	if err := s.unregisterServerFromProxyCore(ctx, inst); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// registerServerBehindProxyCore does the actual work of putting server
// behind the singleton proxy: adds it to the backend list, flips its
// server.properties to trust modern forwarding, and (re-)enables whatever
// loader-specific mechanism actually enforces that trust (see
// setForwardingTrust) -- without this last step a Paper-family server
// still has `proxies.velocity.enabled: true` sitting from before, or a
// Fabric/NeoForge one is still missing its forwarding mod, either of which
// break direct connections independently of the backend-list/
// server.properties state. Shared by handleRegisterBehindProxy (manual,
// requires the instance to already be stopped) and ReconcileProxyMode
// (automatic, triggered by domain registration changes -- FR-1f -- which
// doesn't require the instance to be stopped first since these file edits
// only take effect on the server's next boot anyway, same as every other
// property change).
func (s *Server) registerServerBehindProxyCore(ctx context.Context, inst *instance.Instance) (forwardingSecret string, err error) {
	secret, err := s.forwardingSecret(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to prepare proxy: %w", err)
	}
	if err := s.addServerToProxy(ctx, inst); err != nil {
		return "", err
	}
	if err := patchServerProperties(inst.WorkDir, map[string]string{
		"server-ip":   "127.0.0.1",
		"online-mode": "false",
	}); err != nil {
		return "", fmt.Errorf("registered behind proxy, but failed to update server.properties: %w", err)
	}
	if err := setForwardingTrust(ctx, inst, secret, true); err != nil {
		return "", fmt.Errorf("registered behind proxy, but failed to enable forwarding trust: %w", err)
	}
	// setForwardingTrust's Fabric/NeoForge branch writes config/*.toml and
	// forwarding.secret fresh (this instance may never have had them, e.g.
	// one created while independently exposed) -- unlike provisionServerFiles
	// at instance creation, nothing chowns them here otherwise, and a file
	// newly created by this root process is unreadable by the instance's own
	// unprivileged user. Confirmed on real hardware: FabricProxy-Lite failed
	// to boot with a FileNotFoundException/permission-denied reading its own
	// config/FabricProxy-Lite.toml after converting an instance from
	// independent exposure to behind-the-proxy. Recursive chown is a no-op
	// for files that already existed and were already owned correctly (e.g.
	// Paper's paper-global.yml edited in place).
	chownInstanceFile(ctx, inst.ID, inst.WorkDir)
	return secret, nil
}

// unregisterServerFromProxyCore is registerServerBehindProxyCore's inverse
// -- see its doc comment for why no "must be stopped" check happens here,
// and setForwardingTrust's doc comment for why this needs to do more than
// just patch server.properties (confirmed on real hardware: skipping this
// left the server refusing every direct connection with "This server
// requires you to connect with Velocity." even after conversion to
// independent exposure, since paper-global.yml's proxies.velocity.enabled
// was never flipped back off).
func (s *Server) unregisterServerFromProxyCore(ctx context.Context, inst *instance.Instance) error {
	if err := s.removeServerFromProxy(ctx, inst.ID); err != nil {
		return err
	}
	return s.revertServerProxySettings(ctx, inst)
}

// revertServerProxySettings undoes just the per-server side of proxy
// registration (server.properties, forwarding trust, ownership) without
// touching the proxy's own backend list/restart -- the part of
// unregisterServerFromProxyCore that's actually specific to this one
// server. ReconcileProxyMode's teardown path (no main domain registered)
// calls this directly instead of unregisterServerFromProxyCore for every
// registered server: going through removeServerFromProxy there would
// rewrite the proxy's backend list and restart it (stop+start, see
// applyProxyBackends) once per server, and since removeProxyInstance stops
// and deletes the whole proxy right after the loop anyway, those
// intermediate restarts were pure churn -- confirmed on real hardware as
// the proxy's UPnP port mapping visibly flickering off/on once per
// registered server before finally disappearing for good.
func (s *Server) revertServerProxySettings(ctx context.Context, inst *instance.Instance) error {
	if err := patchServerProperties(inst.WorkDir, map[string]string{
		"server-ip":   "",
		"online-mode": "true",
	}); err != nil {
		return fmt.Errorf("unregistered from proxy, but failed to update server.properties: %w", err)
	}
	if err := setForwardingTrust(ctx, inst, "", false); err != nil {
		return fmt.Errorf("unregistered from proxy, but failed to disable forwarding trust: %w", err)
	}
	// Defensive/symmetric with registerServerBehindProxyCore -- this branch
	// only edits existing files in place or deletes files, so nothing should
	// actually need re-chowning, but it's a cheap no-op if so.
	chownInstanceFile(ctx, inst.ID, inst.WorkDir)
	return nil
}

// setForwardingTrust (re-)enables or disables the loader-specific
// mechanism that makes a server actually enforce/trust Velocity's modern
// player-info forwarding -- the thing that produces "This server requires
// you to connect with Velocity." when it's on and the connection isn't
// actually coming through the proxy. This is separate from (and just as
// necessary as) the backend-list membership and server.properties changes
// register/unregister already made: those control whether the proxy
// *routes* to this server and whether the server *binds* to the LAN, but
// this is what makes the server itself *require* (or not) a forwarded
// connection at all.
//
//   - Paper-family (paper/purpur/folia/pufferfish/leaf): flips
//     config/paper-global.yml's proxies.velocity.enabled (see
//     setPaperVelocityEnabled) -- a no-op if that file/block doesn't exist,
//     which is fine for a custom/unrecognized loader an operator registered
//     manually (FR-3) and is expected to configure trust for themselves.
//   - Fabric: (re-)installs or removes the FabricProxy-Lite mod
//     (installFabricProxyMods/removeModByNameSubstring) -- its mere
//     presence is what enforces the trust requirement, unlike Paper's
//     config toggle.
//   - NeoForge: same idea via the neoforged-velocity-support mod.
//   - Anything else (custom/unrecognized loaders): left untouched --
//     CraftDeck has no general way to know what such a loader needs (same
//     reasoning as handleRegisterBehindProxy's doc comment).
func setForwardingTrust(ctx context.Context, inst *instance.Instance, forwardingSecret string, enable bool) error {
	switch {
	case strings.EqualFold(inst.Loader, "fabric"):
		if enable {
			return installFabricProxyMods(ctx, inst, forwardingSecret)
		}
		return removeModByNameSubstring(inst.WorkDir, "fabricproxy-lite")
	case strings.EqualFold(inst.Loader, "neoforge"):
		if enable {
			return installNeoForgeProxyMod(ctx, inst, forwardingSecret)
		}
		return removeModByNameSubstring(inst.WorkDir, "neoforged-velocity-support")
	case supportsVelocityForwarding(inst.Loader):
		return setPaperVelocityEnabled(inst.WorkDir, enable)
	default:
		return nil
	}
}

// setPaperVelocityEnabled flips config/paper-global.yml's
// proxies.velocity.enabled key between true/false without a full YAML
// parser (matching the project's minimal-dependency philosophy, NFR-9) --
// finds the "velocity:" line, then the "enabled:" line nested under it by
// indentation, and rewrites just that value in place. A server that's
// never had this file written (never registered behind the proxy, or not
// a loader CraftDeck configures this for) is left untouched.
func setPaperVelocityEnabled(workDir string, enabled bool) error {
	path := filepath.Join(workDir, "config", "paper-global.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")

	velocityIndent := -1
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if velocityIndent == -1 {
			if strings.HasPrefix(trimmed, "velocity:") {
				velocityIndent = indent
			}
			continue
		}
		if trimmed == "" {
			continue
		}
		if indent <= velocityIndent {
			break // left the velocity: block without finding "enabled:"
		}
		if strings.HasPrefix(trimmed, "enabled:") {
			lines[i] = fmt.Sprintf("%s%s: %v", line[:indent], "enabled", enabled)
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o640)
		}
	}
	return nil // no proxies.velocity block found -- nothing to flip
}

// removeModByNameSubstring deletes every file in workDir/mods whose
// filename contains needle (case-insensitive) -- undoes
// installFabricProxyMods/installNeoForgeProxyMod's install without needing
// to re-resolve the exact Modrinth file that was originally downloaded
// (its filename varies by version).
func removeModByNameSubstring(workDir, needle string) error {
	modsDir := filepath.Join(workDir, "mods")
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	needle = strings.ToLower(needle)
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(strings.ToLower(e.Name()), needle) {
			continue
		}
		if err := os.Remove(filepath.Join(modsDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// removeProxyInstance stops and permanently deletes the singleton proxy --
// used by ReconcileProxyMode when no owned domain is registered, since
// keeping Velocity running (its fixed 1GB allocation, proxyMemoryMaxMB)
// serves no purpose without real DNS to make forced-host subdomain routing
// reachable. ensureProxyInstance recreates a fresh one on demand if an
// owned domain is registered again later. Mirrors handleDeleteInstance's
// cleanup ordering (stop unit -> remove system user -> delete files ->
// clear DB rows that would otherwise foreign-key-block the instance
// delete), minus the steps that don't apply to a proxy (no plugins,
// backups, or proxy-backend membership of its own).
func (s *Server) removeProxyInstance(ctx context.Context) error {
	proxy, err := s.findProxy(ctx)
	if err != nil || proxy == nil {
		return err
	}
	_ = s.supervisor.Stop(ctx, proxy.ID) // best-effort: fine if it wasn't running
	_ = process.RemoveInstanceUser(ctx, proxy.ID)
	if proxy.WorkDir != "" {
		if err := os.RemoveAll(proxy.WorkDir); err != nil {
			return fmt.Errorf("remove proxy work dir: %w", err)
		}
	}
	// proxy_backends has no ON DELETE CASCADE on proxy_id (same issue
	// handleDeleteInstance works around for plugins/backups), so clear it
	// first via the existing "replace with an empty list" method.
	if err := s.instances.SetProxyBackends(ctx, proxy.ID, nil); err != nil {
		return err
	}
	// Same FK issue for port_mappings.instance_id -- the proxy's own
	// game-port mapping (see ReconcileGamePorts) must be torn down before
	// its instance row can be deleted.
	if err := s.removeGamePortMapping(ctx, proxy.ID); err != nil {
		return fmt.Errorf("remove proxy's game-port mapping: %w", err)
	}
	return s.instances.Delete(ctx, proxy.ID)
}

// ReconcileProxyMode implements FR-1f: Velocity only makes sense with a
// real owned domain registered (internal/ddns) -- without one, forced-host
// subdomain routing was never actually reachable (no DNS resolves to it),
// and a free-subdomain DDNS provider can only ever point at one server
// anyway (FR-27), so there's nothing for a multi-server proxy to do. Called
// whenever domain registration changes (handlers_domain.go) and once at
// daemon startup (see cmd/craftdeckd/main.go), so the proxy's existence
// and each server's exposure mode never drift from the registered domain
// state:
//   - owned domain registered: every proxy-capable server not already
//     behind the proxy gets registered (mirrors what new servers get by
//     default at creation -- see handleCreateInstance).
//   - no owned domain (none registered, or only a free-subdomain one):
//     every server currently behind the proxy is converted to independent
//     exposure, then the proxy itself is torn down (removeProxyInstance).
func (s *Server) ReconcileProxyMode(ctx context.Context) error {
	hasMainDomain, err := s.domains.HasMainDomain(ctx)
	if err != nil {
		return err
	}

	list, err := s.instances.List(ctx)
	if err != nil {
		return err
	}

	if hasMainDomain {
		for _, inst := range list {
			if inst.Kind != instance.KindServer || !supportsVelocityForwardingForVersion(ctx, inst.Loader, inst.MCVersion) {
				continue
			}
			_, registered, err := s.serverSubdomain(ctx, inst.ID)
			if err != nil {
				return err
			}
			if registered {
				continue
			}
			if _, err := s.registerServerBehindProxyCore(ctx, inst); err != nil {
				log.Printf("reconcile proxy mode: register %s behind proxy: %v (continuing with the rest)", inst.ID, err)
			}
		}
	} else {
		for _, inst := range list {
			if inst.Kind != instance.KindServer {
				continue
			}
			_, registered, err := s.serverSubdomain(ctx, inst.ID)
			if err != nil {
				return err
			}
			if !registered {
				continue
			}
			// revertServerProxySettings, not unregisterServerFromProxyCore --
			// the proxy itself is being torn down by removeProxyInstance right
			// below (which deletes its backend rows, work dir, and mapping in
			// one shot), so there's no point rewriting its backend list and
			// restarting it for every server in this loop first. See
			// revertServerProxySettings's doc comment.
			if err := s.revertServerProxySettings(ctx, inst); err != nil {
				log.Printf("reconcile proxy mode: unregister %s from proxy: %v (continuing with the rest)", inst.ID, err)
			}
		}
		if err := s.removeProxyInstance(ctx); err != nil {
			return err
		}
	}

	// FR-21/22/25: a server's proxy-registration state just changed above,
	// which flips whether it's directly reachable at all -- e.g. a server
	// that was behind the proxy and is still running becomes independently
	// exposed here, and (if WAN exposure is on) now needs its own game-port
	// mapping it didn't have a moment ago.
	return s.ReconcileGamePorts(ctx)
}
