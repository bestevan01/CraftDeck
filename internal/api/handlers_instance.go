package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"craftdeck/internal/instance"
	"craftdeck/internal/javaruntime"
	"craftdeck/internal/loader"
	"craftdeck/internal/modrinth"
	"craftdeck/internal/process"
	"craftdeck/internal/swap"

	"github.com/google/uuid"
)

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	list, err := s.instances.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type createInstanceRequest struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Loader          string `json:"loader"`
	LoaderVersion   string `json:"loader_version"`
	MCVersion       string `json:"mc_version"`
	CPUQuotaPercent int    `json:"cpu_quota_percent"`
	MemoryMaxMB     int    `json:"memory_max_mb"`
	// AcceptEula must be true: Mojang's EULA requires explicit operator
	// consent before a server.jar may run (https://www.minecraft.net/eula).
	AcceptEula bool `json:"accept_eula"`
	// ExposeIndependently opts a Paper server out of sitting behind
	// CraftDeck's singleton Velocity proxy (the default for Paper -- see
	// addServerToProxy). Ignored for other loaders, which can't sit behind
	// the proxy at all (see resolveProxyBackendEntries) and are always
	// independently exposed.
	ExposeIndependently bool `json:"expose_independently"`
}

// handleCreateInstance only ever creates server instances -- the Velocity
// proxy is a singleton CraftDeck manages on the operator's behalf (see
// ensureProxyInstance/EnsureProxyRunning), not something created through
// this endpoint.
func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var req createInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if instance.Kind(req.Kind) != instance.KindServer {
		http.Error(w, "only server instances can be created directly; the Velocity proxy is managed automatically", http.StatusBadRequest)
		return
	}
	if !req.AcceptEula {
		http.Error(w, "accept_eula must be true to create a Minecraft server instance (see https://www.minecraft.net/eula)", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Loader) == "" {
		http.Error(w, "loader is required", http.StatusBadRequest)
		return
	}
	if req.LoaderVersion != "" {
		adapter, ok := loader.Get(req.Loader)
		if !ok {
			http.Error(w, "unknown loader", http.StatusBadRequest)
			return
		}
		if _, ok := adapter.(loader.BuildLister); !ok {
			http.Error(w, fmt.Sprintf("%s doesn't support pinning a specific build", req.Loader), http.StatusBadRequest)
			return
		}
	}

	// The operator never needs to know or choose a game_port: every server
	// either sits behind the proxy (reached by subdomain, see
	// handleSetServerSubdomain) or, if independently exposed, is something
	// an operator familiar enough to need the raw port can find via the API
	// directly. Auto-assigning also sidesteps the two-instances-sharing-a-
	// port bug a manual port entry used to allow (their rcon_port would
	// collide too, since it's derived from game_port, causing an endless
	// connect/auth-fail/reconnect loop -- confirmed on real hardware).
	gamePort, err := s.nextFreeGamePort(r.Context(), 25566)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	javaMajor, err := javaruntime.MajorForMCVersion(req.MCVersion)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid mc_version: %v", err), http.StatusBadRequest)
		return
	}

	id := uuid.NewString()
	workDir := filepath.Join(s.dataDir, "instances", id)

	// TODO: encrypt RCONPassword at rest (requirements.md FR-31 covers the
	// analogous DDNS token case; RCON passwords need the same treatment
	// before this is production-ready). Plaintext for now.
	rconPort := gamePort + 10000
	rconPassword, err := generateRCONPassword()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	inst := &instance.Instance{
		ID:              id,
		Name:            req.Name,
		Kind:            instance.KindServer,
		Loader:          req.Loader,
		LoaderVersion:   req.LoaderVersion,
		MCVersion:       req.MCVersion,
		JavaMajor:       javaMajor,
		GamePort:        gamePort,
		RCONPort:        rconPort,
		RCONPassword:    rconPassword,
		CPUQuotaPercent: req.CPUQuotaPercent,
		MemoryMaxMB:     req.MemoryMaxMB,
		WorkDir:         workDir,
		Status:          instance.StatusStopped,
	}

	// Vanilla has no plugin system at all, so it structurally can't trust
	// Velocity's modern-forwarding secret (see resolveProxyBackendEntries) --
	// it's always independently exposed regardless of what the operator
	// asked for. Paper/Purpur/Folia all carry the same proxies.velocity
	// config forward, so any of them defaults to sitting behind the proxy
	// unless the operator explicitly opts out. NeoForge additionally needs
	// its forwarding mod to actually have a build for this exact Minecraft
	// version (supportsVelocityForwardingForVersion) -- on an unsupported
	// version it falls back to independent exposure just like Vanilla,
	// rather than failing creation over something the operator can't fix.
	//
	// FR-1f: none of that matters without a real owned domain registered
	// (internal/ddns) -- forced-host subdomain routing needs actual DNS to
	// be reachable, and a free-subdomain provider can only ever point at
	// one server anyway (FR-27), so Velocity is disabled entirely in that
	// case (see ReconcileProxyMode) and every server defaults to
	// independent exposure regardless of loader/version.
	hasMainDomain, err := s.domains.HasMainDomain(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	behindProxy := hasMainDomain && supportsVelocityForwardingForVersion(r.Context(), req.Loader, req.MCVersion) && !req.ExposeIndependently

	// Fetch (creating if this is the very first server) the proxy's
	// forwarding secret *before* provisioning so paper-global.yml can be
	// pre-seeded with proxies.velocity already trusting it -- otherwise the
	// server boots once with forwarding disabled and every connection
	// through the proxy fails until an operator notices and hand-edits the
	// config (see handleGetForwardingSecret's doc comment for why CraftDeck
	// doesn't patch an *existing*, potentially operator-customized file, but
	// there's nothing to corrupt in a file that doesn't exist yet).
	var forwardingSecret string
	if behindProxy {
		forwardingSecret, err = s.forwardingSecret(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to prepare proxy for new server: %v", err), http.StatusInternalServerError)
			return
		}
	}

	if err := provisionServerFiles(r.Context(), inst, behindProxy, forwardingSecret); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.instances.Create(r.Context(), inst); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if behindProxy {
		if err := s.addServerToProxy(r.Context(), inst); err != nil {
			http.Error(w, fmt.Sprintf("instance created, but failed to register it behind the proxy: %v", err), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusCreated, inst)
}

// provisionServerFiles creates the instance's work directory, accepts the
// EULA on the operator's behalf (already confirmed via AcceptEula above),
// writes a minimal server.properties, downloads the loader jar if an
// adapter for it exists yet (FR-1, FR-2), and hands the whole directory
// over to a dedicated per-instance system user (see process.EnsureInstanceUser)
// so the eventual systemd-run process (running as that user, not root) can
// actually read/write it. Loaders without an adapter so far (everything
// except Vanilla/Paper) are left without a jar -- the operator can upload
// one manually per FR-3 once that's wired up.
//
// bindLoopback restricts the Minecraft process to 127.0.0.1 (server-ip) so
// only the proxy running on the same host can reach it -- the game_port
// never needs to be exposed to the LAN/WAN at all for a server sitting
// behind CraftDeck's Velocity proxy.
//
// forwardingSecret, when non-empty, is pre-seeded into config/paper-global.yml
// so the server trusts the proxy's modern player-info forwarding from its
// very first boot (see the call site's comment). Empty for servers not
// sitting behind the proxy.
func provisionServerFiles(ctx context.Context, inst *instance.Instance, bindLoopback bool, forwardingSecret string) error {
	// The parent ("<dataDir>/instances") must stay traversable (mode 0711:
	// enter a known subpath, but can't list siblings) by every per-instance
	// user, not just root -- otherwise CHDIR into the leaf directory fails
	// at the parent regardless of the leaf's own permissions. VERIFIED on
	// real hardware: chowning only the leaf directory still left the
	// systemd unit failing with "Changing to the requested working
	// directory failed: Permission denied" because MkdirAll had created the
	// parent as root-owned 0750.
	if err := os.MkdirAll(filepath.Dir(inst.WorkDir), 0o711); err != nil {
		return fmt.Errorf("create instances dir: %w", err)
	}
	if err := os.MkdirAll(inst.WorkDir, 0o750); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(inst.WorkDir, "eula.txt"), []byte("eula=true\n"), 0o640); err != nil {
		return fmt.Errorf("write eula.txt: %w", err)
	}
	if inst.GamePort > 0 {
		serverIP := ""
		// Modern forwarding has the proxy do the real Mojang session
		// verification and hand the backend an already-authenticated
		// player; a backend that also tries to online-mode-auth that
		// connection itself makes Velocity immediately drop it with
		// "Backend server is online-mode!" (confirmed against real client
		// connections -- every join reached the backend and was
		// disconnected right there). So online-mode must be off on any
		// server sitting behind the proxy; the proxy's own online-mode
		// (velocity.toml) is what actually matters for security.
		onlineMode := "true"
		if bindLoopback {
			serverIP = "127.0.0.1"
			onlineMode = "false"
		}
		props := fmt.Sprintf(
			"server-port=%d\nserver-ip=%s\nonline-mode=%s\nenable-rcon=true\nrcon.port=%d\nrcon.password=%s\n",
			inst.GamePort, serverIP, onlineMode, inst.RCONPort, inst.RCONPassword,
		)
		if err := os.WriteFile(filepath.Join(inst.WorkDir, "server.properties"), []byte(props), 0o640); err != nil {
			return fmt.Errorf("write server.properties: %w", err)
		}
	}

	if forwardingSecret != "" {
		switch {
		case strings.EqualFold(inst.Loader, "fabric"):
			// Fabric has no built-in equivalent of Paper's proxies.velocity
			// config -- FabricProxy-Lite (+ its Fabric API dependency) is
			// what makes a Fabric server trust the proxy's secret at all.
			if err := installFabricProxyMods(ctx, inst, forwardingSecret); err != nil {
				return fmt.Errorf("install fabric proxy mods: %w", err)
			}
		case strings.EqualFold(inst.Loader, "neoforge"):
			// Same idea as Fabric above, but NeoForge's equivalent mod is
			// NeoVelocity.
			if err := installNeoForgeProxyMod(ctx, inst, forwardingSecret); err != nil {
				return fmt.Errorf("install neoforge proxy mod: %w", err)
			}
		default:
			configDir := filepath.Join(inst.WorkDir, "config")
			if err := os.MkdirAll(configDir, 0o750); err != nil {
				return fmt.Errorf("create config dir: %w", err)
			}
			// Paper's config loader fills in every key this file doesn't
			// specify with its own defaults on first boot, so seeding just the
			// proxies.velocity block is enough -- no need to reproduce the rest
			// of paper-global.yml here.
			globalYML := fmt.Sprintf(
				"proxies:\n  velocity:\n    enabled: true\n    online-mode: true\n    secret: '%s'\n",
				forwardingSecret,
			)
			if err := os.WriteFile(filepath.Join(configDir, "paper-global.yml"), []byte(globalYML), 0o640); err != nil {
				return fmt.Errorf("write paper-global.yml: %w", err)
			}
		}
	}

	if adapter, ok := loader.Get(inst.Loader); ok {
		if err := downloadLoaderJar(ctx, adapter, inst.MCVersion, inst.LoaderVersion, inst.WorkDir); err != nil {
			return fmt.Errorf("download %s server jar: %w", inst.Loader, err)
		}
	} // else: no adapter yet for this loader; upload jar manually (FR-3)

	username, err := process.EnsureInstanceUser(ctx, inst.ID)
	if err != nil {
		return fmt.Errorf("create instance user: %w", err)
	}
	if err := process.ChownRecursive(ctx, inst.WorkDir, username); err != nil {
		return fmt.Errorf("chown work dir: %w", err)
	}
	return nil
}

// downloadLoaderJar resolves to adapter.DownloadBuild when loaderVersion is
// pinned (validated against BuildLister support by the caller -- see
// handleCreateInstance and handleReinstallLoader), otherwise adapter.Download
// for the usual "whatever's newest" behavior.
func downloadLoaderJar(ctx context.Context, adapter loader.Adapter, mcVersion, loaderVersion, destDir string) error {
	if loaderVersion == "" {
		_, err := adapter.Download(ctx, mcVersion, destDir)
		return err
	}
	lister, ok := adapter.(loader.BuildLister)
	if !ok {
		return fmt.Errorf("this loader doesn't support pinning a specific build")
	}
	_, err := lister.DownloadBuild(ctx, mcVersion, loaderVersion, destDir)
	return err
}

// installFabricProxyMods downloads FabricProxy-Lite and its Fabric API
// dependency from Modrinth into mods/, and pre-seeds FabricProxy-Lite's
// config with the proxy's forwarding secret. Fabric has no built-in
// equivalent of Paper's proxies.velocity config (see
// supportsVelocityForwarding), so without this a Fabric server behind the
// proxy would just reject every connection as untrusted.
func installFabricProxyMods(ctx context.Context, inst *instance.Instance, forwardingSecret string) error {
	modsDir := filepath.Join(inst.WorkDir, "mods")
	if err := os.MkdirAll(modsDir, 0o750); err != nil {
		return fmt.Errorf("create mods dir: %w", err)
	}

	for _, projectID := range []string{"fabric-api", "fabricproxy-lite"} {
		version, err := modrinth.BestVersion(ctx, projectID, "fabric", inst.MCVersion)
		if err != nil {
			return fmt.Errorf("find %s for fabric %s: %w", projectID, inst.MCVersion, err)
		}
		file, err := version.PrimaryFile()
		if err != nil {
			return fmt.Errorf("%s has no download: %w", projectID, err)
		}
		destPath := filepath.Join(modsDir, file.Filename)
		if err := downloadAndVerifySHA512(ctx, file.URL, file.Hashes["sha512"], destPath); err != nil {
			return fmt.Errorf("download %s: %w", projectID, err)
		}
	}

	configDir := filepath.Join(inst.WorkDir, "config")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	// FabricProxy-Lite fills in every other key with its own default the
	// first time it runs, so seeding just the secret is enough.
	toml := fmt.Sprintf("secret = \"%s\"\n", forwardingSecret)
	if err := os.WriteFile(filepath.Join(configDir, "FabricProxy-Lite.toml"), []byte(toml), 0o640); err != nil {
		return fmt.Errorf("write FabricProxy-Lite.toml: %w", err)
	}
	return nil
}

// installNeoForgeProxyMod downloads "NeoForged Velocity Support" (Modrinth
// slug "neoforged-velocity-support") into mods/ and writes the proxy's
// forwarding secret to a plain forwarding.secret file in the instance's
// work dir root -- NeoForge's equivalent of installFabricProxyMods above.
//
// This isn't NeoVelocity, despite that being the more well-known option:
// NeoVelocity 1.2.6 only implements Velocity's original ("v1") modern
// forwarding and throws a bare UnsupportedOperationException on login for
// anything using the newer forwarding versions modern Minecraft actually
// negotiates (signed/secure chat requires it) -- reproduced on real
// hardware against MC 1.21.11 and confirmed as a known, unfixed upstream
// bug the maintainer has no timeline for (github.com/Gabwasnt/NeoVelocity
// issue #31). NeoForged Velocity Support is a from-scratch reimplementation
// modeled directly on Paper's own VelocityProxy class and supports up
// through forwarding version 4 (MODERN_LAZY_SESSION), and reads the secret
// as a plain file rather than a mod-config-library-parsed TOML, sidestepping
// the NeoForge config system silently reformatting/rejecting a hand-written
// file the way it did for NeoVelocity's config (see git history).
func installNeoForgeProxyMod(ctx context.Context, inst *instance.Instance, forwardingSecret string) error {
	modsDir := filepath.Join(inst.WorkDir, "mods")
	if err := os.MkdirAll(modsDir, 0o750); err != nil {
		return fmt.Errorf("create mods dir: %w", err)
	}

	version, err := modrinth.BestVersion(ctx, "neoforged-velocity-support", "neoforge", inst.MCVersion)
	if err != nil {
		return fmt.Errorf("find neoforged-velocity-support for neoforge %s: %w", inst.MCVersion, err)
	}
	file, err := version.PrimaryFile()
	if err != nil {
		return fmt.Errorf("neoforged-velocity-support has no download: %w", err)
	}
	destPath := filepath.Join(modsDir, file.Filename)
	if err := downloadAndVerifySHA512(ctx, file.URL, file.Hashes["sha512"], destPath); err != nil {
		return fmt.Errorf("download neoforged-velocity-support: %w", err)
	}

	if err := os.WriteFile(filepath.Join(inst.WorkDir, "forwarding.secret"), []byte(forwardingSecret), 0o640); err != nil {
		return fmt.Errorf("write forwarding.secret: %w", err)
	}
	return nil
}

func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	inst, err := s.instances.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, inst)
}

// maxServerJarBytes caps a direct server.jar upload (FR-3) -- generous
// enough for a modpack-bundled server jar (much bigger than a plain
// loader's), well under what the Pi's disk realistically needs to guard
// against.
const maxServerJarBytes = 500 << 20 // 500MiB

// handleUploadServerJar accepts a direct server.jar upload (FR-3),
// replacing whatever's currently at inst.WorkDir/server.jar. This is the
// only way to give an instance created with a custom/unlisted loader (no
// adapter in internal/loader's registry, so provisionServerFiles silently
// skips the automatic download -- see its doc comment) an actual jar to
// run, but works for any instance -- e.g. manually swapping in a
// different build of a loader CraftDeck does support.
func (s *Server) handleUploadServerJar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inst, err := s.instances.Get(ctx, r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Status != instance.StatusStopped && inst.Status != instance.StatusCrashed {
		http.Error(w, "instance must be stopped before replacing its server jar", http.StatusConflict)
		return
	}

	// http.MaxBytesReader (not just ParseMultipartForm's maxMemory arg,
	// which only bounds in-memory buffering and happily spills the rest to
	// disk unbounded) actually rejects a request whose body exceeds the
	// limit, confirmed necessary for FR-40's size validation.
	r.Body = http.MaxBytesReader(w, r.Body, maxServerJarBytes)
	if err := r.ParseMultipartForm(maxServerJarBytes); err != nil {
		http.Error(w, "invalid multipart form or file too large (max 500MB): "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("jar")
	if err != nil {
		http.Error(w, "missing 'jar' file field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".jar") {
		http.Error(w, "only .jar files are accepted", http.StatusBadRequest)
		return
	}
	validated, err := requireJarMagicBytes(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	destPath := filepath.Join(inst.WorkDir, "server.jar")
	out, err := os.Create(destPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, validated); err != nil {
		out.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := out.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chownInstanceFile(ctx, inst.ID, destPath)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// reinstallLoaderRequest's LoaderVersion is optional -- empty means "whatever
// build the loader's distribution point currently considers newest" (the
// original FR-4 behavior); set it (to a loader.BuildInfo.ID from
// GET /api/loaders/{loader}/builds) to pin a specific build instead.
type reinstallLoaderRequest struct {
	LoaderVersion string `json:"loader_version"`
}

// handleReinstallLoader re-downloads inst's loader for its already-set
// mc_version, overwriting server.jar -- FR-4's "구동기 버전 교체" scoped down
// to the only variant that's actually safe to automate: staying on the exact
// same loader and Minecraft version, either picking up whatever build the
// loader's distribution point currently considers newest (e.g. a bugfix
// release) or, for loaders with a genuine per-version build concept
// (BuildLister), pinning one specific build. There's no endpoint to switch
// to a different loader or Minecraft version at all -- same as there's never
// been one to edit inst.Loader/inst.MCVersion after creation -- since that
// risks breaking world/plugin compatibility in ways CraftDeck can't safely
// automate.
//
// Only works for a loader CraftDeck has an adapter for; a custom/manually-
// uploaded loader (FR-3) has no adapter to re-download from at all -- the
// operator uploads a new jar directly via handleUploadServerJar instead,
// which already can't touch inst.Loader/inst.MCVersion either.
func (s *Server) handleReinstallLoader(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inst, err := s.instances.Get(ctx, r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Kind != instance.KindServer {
		http.Error(w, "only server instances can be reinstalled this way", http.StatusBadRequest)
		return
	}
	if inst.Status != instance.StatusStopped && inst.Status != instance.StatusCrashed {
		http.Error(w, "instance must be stopped before reinstalling its loader", http.StatusConflict)
		return
	}
	adapter, ok := loader.Get(inst.Loader)
	if !ok {
		http.Error(w, "this instance's loader has no CraftDeck adapter to reinstall from -- upload a jar directly instead (see the 파일 tab)", http.StatusBadRequest)
		return
	}

	var req reinstallLoaderRequest
	// Body is optional (a bare POST with no body means "reinstall latest",
	// the original behavior) -- only reject a body that's present but broken.
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	if req.LoaderVersion != "" {
		if _, ok := adapter.(loader.BuildLister); !ok {
			http.Error(w, fmt.Sprintf("%s doesn't support pinning a specific build", inst.Loader), http.StatusBadRequest)
			return
		}
	}

	if err := downloadLoaderJar(ctx, adapter, inst.MCVersion, req.LoaderVersion, inst.WorkDir); err != nil {
		http.Error(w, fmt.Sprintf("download %s server jar: %v", inst.Loader, err), http.StatusInternalServerError)
		return
	}
	if err := s.instances.UpdateLoaderVersion(ctx, inst.ID, req.LoaderVersion); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chownInstanceFile(ctx, inst.ID, filepath.Join(inst.WorkDir, "server.jar"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type updateInstanceRequest struct {
	CPUQuotaPercent int `json:"cpu_quota_percent"`
	MemoryMaxMB     int `json:"memory_max_mb"`
}

// handleUpdateInstance edits the resource-allocation fields (FR-12). The
// game_port is intentionally not editable here -- it's auto-assigned once at
// creation and never surfaced to the operator (see nextFreeGamePort), so
// there's nothing meaningful for this endpoint to change about it.
//
// Allowed even while the instance is running: CPU/memory limits are only
// ever applied to a fresh process, so writing the new values now has no
// effect on the currently-running unit -- they simply take effect the next
// time the operator restarts it (see handleRestartInstance).
func (s *Server) handleUpdateInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	var req updateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.MemoryMaxMB < 0 || req.CPUQuotaPercent < 0 {
		http.Error(w, "memory_max_mb and cpu_quota_percent must not be negative", http.StatusBadRequest)
		return
	}
	// The proxy's memory is fixed (see proxyMemoryMaxMB in handlers_proxy.go)
	// so the per-server slider can reliably reserve it off the top of total
	// system memory -- an operator-tunable proxy allocation would make that
	// reservation stale.
	if inst.Kind == instance.KindProxy {
		req.MemoryMaxMB = proxyMemoryMaxMB
	}

	if err := s.instances.UpdateSettings(ctx, id, inst.GamePort, inst.RCONPort, req.CPUQuotaPercent, req.MemoryMaxMB); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	updated, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	// Stop the unit *before* touching the user/files -- deleting a system
	// user or its files while a process still runs as that user leaves an
	// orphaned systemd unit (still holding the game port) and a userdel
	// that silently fails, exactly like the zombie instances found on the
	// Pi that never actually got cleaned up despite being "deleted" here.
	_ = s.supervisor.Stop(ctx, id) // best-effort: fine if it wasn't running
	s.rconMgr.StopMaintaining(id)
	_ = process.RemoveInstanceUser(ctx, id)

	if inst.Kind == instance.KindServer {
		if err := s.removeServerFromProxy(ctx, id); err != nil {
			log.Printf("remove %s from proxy backends: %v (continuing with delete)", id, err)
		}
	}

	if inst.WorkDir != "" {
		if err := os.RemoveAll(inst.WorkDir); err != nil {
			http.Error(w, "instance stopped, but failed to remove its files: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// plugins/backups both have a foreign key on instances(id) with no
	// ON DELETE CASCADE, so any instance that ever had a plugin/mod
	// installed or a backup taken would otherwise fail to delete with a
	// foreign key constraint error -- confirmed on real hardware.
	if err := s.plugins.DeleteByInstance(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.backups.DeleteByInstance(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Same FK issue for port_mappings.instance_id -- an independently-
	// exposed server that was running (and so had a game-port mapping via
	// ReconcileGamePorts) would otherwise fail to delete the same way.
	if err := s.removeGamePortMapping(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.instances.Delete(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStartInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	if err := s.startInstanceCore(ctx, inst); err != nil {
		if errors.Is(err, errNoServerJar) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

var errNoServerJar = errors.New("no server.jar for this instance: no loader adapter downloaded one and none was uploaded (see FR-3)")

// startInstanceCore does the actual work of launching an instance's systemd
// unit and RCON connection; shared by handleStartInstance and
// handleRestartInstance so a restart doesn't have to duplicate this logic.
func (s *Server) startInstanceCore(ctx context.Context, inst *instance.Instance) error {
	jarPath := filepath.Join(inst.WorkDir, "server.jar")
	if _, err := os.Stat(jarPath); errors.Is(err, os.ErrNotExist) {
		return errNoServerJar
	}

	// Idempotent: re-ensures the per-instance user exists in case it was
	// somehow removed since provisioning (e.g. manual cleanup).
	username, err := process.EnsureInstanceUser(ctx, inst.ID)
	if err != nil {
		return err
	}

	javaArgs := []string{}
	if inst.MemoryMaxMB > 0 {
		javaArgs = append(javaArgs, fmt.Sprintf("-Xmx%dM", inst.MemoryMaxMB))
	}
	javaArgs = append(javaArgs, "-jar", "server.jar")
	if inst.Kind == instance.KindServer {
		javaArgs = append(javaArgs, "nogui") // Velocity's own main() doesn't expect/want this arg
	}

	// Only let this instance page out to swap if CraftDeck's own swap file
	// is actually turned on -- otherwise a memory_max_mb above physical RAM
	// (see handleUpdateInstance's swap-aware ceiling) has nowhere real to
	// go and should OOM-kill rather than silently do nothing.
	swapStatus, err := swap.Status(ctx, s.dataDir)
	if err != nil {
		log.Printf("start %s: check swap status (continuing without swap): %v", inst.ID, err)
	}

	spec := process.StartSpec{
		InstanceID:      inst.ID,
		WorkDir:         inst.WorkDir,
		Username:        username,
		JavaBinary:      javaruntime.BinaryPath(inst.JavaMajor),
		JavaArgs:        javaArgs,
		CPUQuotaPercent: inst.CPUQuotaPercent,
		MemoryMaxMB:     inst.MemoryMaxMB,
		AllowSwap:       swapStatus != nil && swapStatus.Enabled,
	}

	if err := s.supervisor.Start(ctx, spec); err != nil {
		return err
	}
	if err := s.instances.UpdateStatus(ctx, inst.ID, instance.StatusStarting); err != nil {
		return err
	}

	// Kick off a persistent, auto-reconnecting RCON connection for this
	// instance (ARCHITECTURE.md 5.4). It'll keep retrying in the background
	// until the server's RCON listener comes up after boot -- that first
	// successful connection is also the only signal we have that the
	// server actually finished booting, so it's what flips the status from
	// "starting" to "running" (nothing else ever did, which is why
	// instances used to get stuck showing "starting" forever once they'd
	// actually finished coming up).
	if inst.RCONPort > 0 {
		instanceID := inst.ID
		s.rconMgr.StartMaintaining(inst.ID, fmt.Sprintf("127.0.0.1:%d", inst.RCONPort), inst.RCONPassword, func() {
			_ = s.instances.UpdateStatus(context.Background(), instanceID, instance.StatusRunning)
		})
	} else if inst.Kind == instance.KindProxy {
		// Proxies have no RCON in this MVP to key a "finished booting"
		// signal off of. Velocity boots in well under a second, so marking
		// it running right after systemd accepts the unit is a fine
		// approximation rather than leaving it stuck on "starting" forever.
		if err := s.instances.UpdateStatus(ctx, inst.ID, instance.StatusRunning); err != nil {
			return err
		}
	}
	// FR-21/22/25: bring this instance's game-port forwarding rule in sync
	// with the fact that it's now (about to be) running -- a soft failure
	// here (e.g. the router's briefly unreachable) shouldn't block the
	// instance from actually starting.
	if err := s.ReconcileGamePorts(ctx); err != nil {
		log.Printf("reconcile game ports after starting %s: %v", inst.ID, err)
	}
	return nil
}

func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	if err := s.stopInstanceCore(ctx, inst); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// stopInstanceCore does the actual work of stopping an instance's systemd
// unit and RCON connection; shared by handleStopInstance and
// handleRestartInstance.
func (s *Server) stopInstanceCore(ctx context.Context, inst *instance.Instance) error {
	// StatusStopping exists precisely for this multi-second window (a
	// graceful RCON stop can take up to ~30s for a big world to save) but
	// was never actually being set anywhere -- the operator's "종료"
	// button click just sat there showing "실행 중" the whole time and
	// then jumped straight to "중지됨", with nothing in between to show
	// the stop was actually in progress (confirmed: exactly what was
	// happening). Soft-fails like every other status write here: worst
	// case the UI just shows stale status for this one poll, not worth
	// aborting the actual stop over.
	if err := s.instances.UpdateStatus(ctx, inst.ID, instance.StatusStopping); err != nil {
		log.Printf("mark %s stopping (continuing anyway): %v", inst.ID, err)
	}

	// Prefer a graceful RCON "stop" (saves the world) over a hard
	// systemd-run kill. Give the server a window to actually exit before
	// falling back, since "stop" can take a few seconds to flush chunks.
	if graceful := s.tryGracefulStop(ctx, inst); !graceful {
		// A non-nil error here does NOT mean the unit is still running --
		// systemctl reports an error for "systemctl stop" on a unit whose
		// last run exited via a signal (Result=exit-code, e.g. our own
		// preceding RCON "stop" finishing the shutdown right as this fires),
		// which is an expected outcome, not a failure to stop. Log it and
		// fall through to check systemd's actual state below instead of
		// aborting the handler, which previously left the DB/RCON manager
		// stuck believing the instance was still running.
		if err := s.supervisor.Stop(ctx, inst.ID); err != nil {
			log.Printf("supervisor.Stop(%s): %v (continuing to verify actual state)", inst.ID, err)
		}
	}
	s.rconMgr.StopMaintaining(inst.ID)

	// Trust systemd's own view of reality over either code path above.
	status := instance.StatusStopped
	if active, _ := s.supervisor.IsActive(ctx, inst.ID); active {
		status = instance.StatusRunning
	}
	if err := s.instances.UpdateStatus(ctx, inst.ID, status); err != nil {
		return err
	}
	// FR-21/22/25: close this instance's game-port forwarding rule now that
	// it's (about to be) stopped -- soft failure only, same reasoning as
	// startInstanceCore's call.
	if err := s.ReconcileGamePorts(ctx); err != nil {
		log.Printf("reconcile game ports after stopping %s: %v", inst.ID, err)
	}
	return nil
}

// tryGracefulStop sends "stop" over the managed RCON connection and waits
// briefly for the unit to exit on its own. Returns false if RCON wasn't
// reachable or the connection was still up after the wait, signaling the
// caller to fall back to supervisor.Stop.
func (s *Server) tryGracefulStop(ctx context.Context, inst *instance.Instance) bool {
	if inst.RCONPort == 0 {
		return false
	}
	if _, err := s.rconMgr.Execute(inst.ID, "stop"); err != nil {
		return false
	}

	// Polls the managed RCON connection's own state rather than shelling
	// out to `systemctl is-active` every second -- that used to race
	// against systemd tearing down the just-exited transient unit file,
	// logging a harmless but noisy "Failed to open .../transient/....
	// service: No such file or directory" into the instance's own journal
	// (confirmed: an operator watching the live console saw exactly this,
	// repeated, right after a clean shutdown). The RCON connection drops
	// the moment the process exits, so this is both quieter and faster.
	for i := 0; i < 30; i++ { // up to ~30s for world save + shutdown
		time.Sleep(1 * time.Second)
		if !s.rconMgr.Connected(inst.ID) {
			// One systemd query to confirm, now that the noisy window
			// (unit exiting *right this instant*) has already passed.
			active, _ := s.supervisor.IsActive(ctx, inst.ID)
			return !active
		}
	}
	return false
}

// handleRestartInstance stops then starts an instance in one call, so an
// operator can apply a port/CPU/memory change made while the server was
// still running (see handleUpdateInstance) without having to hit stop and
// start separately. It's a no-op-safe stop (fine if already stopped) always
// followed by a start attempt.
func (s *Server) handleRestartInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	if err := s.stopInstanceCore(ctx, inst); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.startInstanceCore(ctx, inst); err != nil {
		if errors.Is(err, errNoServerJar) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type sendCommandRequest struct {
	Command string `json:"command"`
}

// handleSendCommand is the single execution path for both free-text
// console input (FR-15) and GUI command buttons (FR-17) -- the frontend
// calls this same endpoint either way (FR-18).
func (s *Server) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req sendCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if _, err := s.instances.Get(r.Context(), id); err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	result, err := s.rconMgr.Execute(id, req.Command)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": result})
}

// gamePortInUse reports whether some other instance already has gamePort as
// its game_port. excludeID lets a settings-update check exclude the
// instance's own current value.
func (s *Server) gamePortInUse(ctx context.Context, gamePort int, excludeID string) (bool, error) {
	list, err := s.instances.List(ctx)
	if err != nil {
		return false, err
	}
	for _, other := range list {
		if other.ID == excludeID {
			continue
		}
		if other.GamePort == gamePort {
			return true, nil
		}
	}
	return false, nil
}

// nextFreeGamePort finds the lowest free game_port at or after start --
// used to auto-assign new instances a port without the operator ever having
// to see or choose one (see handleCreateInstance).
func (s *Server) nextFreeGamePort(ctx context.Context, start int) (int, error) {
	port := start
	for {
		inUse, err := s.gamePortInUse(ctx, port, "")
		if err != nil {
			return 0, err
		}
		if !inUse {
			return port, nil
		}
		port++
	}
}

func generateRCONPassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate rcon password: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// jarMagicBytes is the ZIP local-file-header signature every real .jar (a
// ZIP archive under the hood) starts with. FR-40 requires validating an
// uploaded plugin/mod/loader jar "before running it" -- an extension check
// alone (see handleUploadPlugin/handleUploadServerJar) only looks at the
// filename an attacker fully controls, so this actually looks at the
// content instead.
var jarMagicBytes = [4]byte{0x50, 0x4B, 0x03, 0x04}

// requireJarMagicBytes peeks the first 4 bytes of an uploaded file and
// rejects it if they don't match jarMagicBytes, without consuming the
// stream for whatever copies it next -- the peeked bytes are replayed via
// the returned io.Reader.
func requireJarMagicBytes(file io.Reader) (io.Reader, error) {
	var peek [4]byte
	n, err := io.ReadFull(file, peek[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("read file header: %w", err)
	}
	if n < len(peek) || peek != jarMagicBytes {
		return nil, fmt.Errorf("file content doesn't look like a valid .jar (missing ZIP signature)")
	}
	return io.MultiReader(bytes.NewReader(peek[:]), file), nil
}
