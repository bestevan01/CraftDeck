// Command craftdeckd is the CraftDeck management daemon: a single static
// binary that serves the web UI, the REST/WebSocket API, and supervises
// Minecraft server/proxy instances via systemd-run (see ARCHITECTURE.md).
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"craftdeck/internal/api"
	"craftdeck/internal/auth"
	"craftdeck/internal/backup"
	"craftdeck/internal/config"
	"craftdeck/internal/db"
	"craftdeck/internal/ddns"
	"craftdeck/internal/instance"
	"craftdeck/internal/network"
	"craftdeck/internal/plugin"
	"craftdeck/internal/process"
	"craftdeck/internal/rcon"
	"craftdeck/internal/secrets"
	"craftdeck/internal/tlscert"
	"craftdeck/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.Load()

	// 0711: traversable by per-instance users (so they can CHDIR into their
	// own subdirectory) without letting them list its contents; see
	// handlers_instance.go's provisionServerFiles for the matching
	// "instances" subdirectory permission.
	if err := os.MkdirAll(cfg.DataDir, 0o711); err != nil {
		return err
	}

	database, err := db.Open(filepath.Join(cfg.DataDir, "craftdeck.db"))
	if err != nil {
		return err
	}
	defer database.Close()

	instances := instance.NewRepository(database)
	supervisor := process.NewSupervisor()
	rconMgr := rcon.NewManager()
	users := auth.NewRepository(database)
	backups := backup.NewRepository(database)
	plugins := plugin.NewRepository(database)
	networkSettings := network.NewSettingsRepository(database)
	portMappings := network.NewMappingRepository(database)
	netManager := network.NewManager(portMappings)
	domains := ddns.NewRepository(database)
	masterKey, err := secrets.LoadOrCreateMasterKey(cfg.MasterKeyPath)
	if err != nil {
		return fmt.Errorf("load/create master key: %w", err)
	}
	ddnsManager := ddns.NewManager(domains, masterKey)
	webUIPort, err := portFromAddr(cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("determine web UI port from %q: %w", cfg.ListenAddr, err)
	}
	apiServer := api.NewServer(instances, supervisor, rconMgr, users, backups, plugins, cfg.DataDir, networkSettings, portMappings, netManager, webUIPort, domains, ddnsManager, masterKey)
	// FR-28/30: wired up after apiServer exists (ddnsManager is built first
	// since api.NewServer takes it as a constructor argument) -- see
	// ddns.Manager.SetMainDomainSync's doc comment.
	ddnsManager.SetMainDomainSync(apiServer.SyncMainDomainDNS)

	// craftdeckd restarts (deploys, crashes, reboots) don't touch already-
	// running Minecraft instances -- their systemd units are independent.
	// But a fresh process starts with an empty rcon.Manager, so without
	// this, any instance that was running before the restart loses its
	// RCON connection forever (confirmed: this is exactly what happened
	// after a deploy mid-session -- the game server kept running but
	// commands stopped working until the user manually restarted it).
	if err := reconcileInstances(context.Background(), instances, supervisor, rconMgr); err != nil {
		log.Printf("instance reconciliation failed (continuing anyway): %v", err)
	}

	// FR-1f: Velocity only exists/runs when an owned domain is registered
	// (see ReconcileProxyMode) -- bring the proxy's existence and every
	// server's exposure mode back in sync with that in case they drifted
	// while craftdeckd wasn't running (e.g. the DB was edited directly, or
	// this is the first boot after upgrading to a build that added this
	// check). Only then does the always-on "make sure it's actually
	// started" step (EnsureProxyRunning) apply, and only if a domain is in
	// fact registered -- otherwise ReconcileProxyMode just tore the proxy
	// down and EnsureProxyRunning would immediately recreate it.
	if err := apiServer.ReconcileProxyMode(context.Background()); err != nil {
		log.Printf("reconcile proxy mode failed (continuing anyway): %v", err)
	}
	if hasMainDomain, err := domains.HasMainDomain(context.Background()); err != nil {
		log.Printf("check domain registration failed (continuing anyway): %v", err)
	} else if hasMainDomain {
		if err := apiServer.EnsureProxyRunning(context.Background()); err != nil {
			log.Printf("ensure velocity proxy running failed (continuing anyway): %v", err)
		}
	}

	// FR-21/22/25: bring every running instance's game-port forwarding rule
	// back in sync too, now that the proxy's own running state (just above)
	// is settled -- covers the case where craftdeckd restarted while
	// instances kept running (their systemd units are independent of this
	// process, same reasoning as reconcileInstances above).
	if err := apiServer.ReconcileGamePorts(context.Background()); err != nil {
		log.Printf("reconcile game ports failed (continuing anyway): %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Routes())
	mux.Handle("/", staticHandler())

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	// FR-33: plain HTTP while WAN exposure is off (LAN-only default, zero
	// friction), real TLS the instant it's turned on -- ConditionalListener
	// decides per-connection so this never needs a listener/server restart
	// when the toggle flips. certManager picks a real Let's Encrypt cert
	// (Cloudflare DNS-01, reusing the token FR-31 already verified) when a
	// main domain is registered, or a self-signed fallback otherwise.
	rawListener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	certManager := tlscert.NewManager(domains, masterKey, cfg.DataDir)
	listener := &tlscert.ConditionalListener{
		Listener:  rawListener,
		TLSConfig: &tls.Config{GetCertificate: certManager.GetCertificate},
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("craftdeckd listening on %s", cfg.ListenAddr)
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// FR-26/30: periodically push/monitor the registered free-subdomain
	// hostname against the router's current WAN IP. Tied to the same
	// shutdown-aware ctx as everything else so it stops cleanly alongside
	// the HTTP server.
	ddnsManager.Start(ctx)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

// reconcileInstances runs once at startup: for every known instance, it
// checks systemd's own view of whether the unit is actually active and
// brings both instances.status and the RCON manager back in sync with
// reality, regardless of what was last written to the DB before this
// process started.
func reconcileInstances(ctx context.Context, instances *instance.Repository, supervisor *process.Supervisor, rconMgr *rcon.Manager) error {
	list, err := instances.List(ctx)
	if err != nil {
		return err
	}
	for _, inst := range list {
		active, err := supervisor.IsActive(ctx, inst.ID)
		if err != nil {
			log.Printf("reconcile %s: checking systemd state: %v", inst.ID, err)
			continue
		}

		if active {
			if inst.RCONPort > 0 {
				// No onConnect callback here: unlike a fresh start, the unit
				// is already confirmed active via systemd, so status is set
				// to running immediately below rather than waiting on RCON.
				rconMgr.StartMaintaining(inst.ID, fmt.Sprintf("127.0.0.1:%d", inst.RCONPort), inst.RCONPassword, nil)
			}
			if inst.Status != instance.StatusRunning {
				_ = instances.UpdateStatus(ctx, inst.ID, instance.StatusRunning)
			}
		} else if inst.Status == instance.StatusRunning || inst.Status == instance.StatusStarting {
			_ = instances.UpdateStatus(ctx, inst.ID, instance.StatusStopped)
		}
	}
	return nil
}

// portFromAddr extracts the numeric port from a listen address like
// ":8080" or "0.0.0.0:8080" -- what FR-21/25's web-UI port-forwarding
// (internal/network) needs to know which port to map, since cfg.ListenAddr
// is normally host-less.
func portFromAddr(addr string) (int, error) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portStr)
}

// staticHandler serves the embedded SvelteKit build (see web/embed.go) with
// SPA fallback: client-side routes like /instances/<id> have no matching
// file in the static build (only index.html + the _app/ asset bundle do),
// so a plain http.FileServer 404s on hard reload / direct navigation to
// them. Falling back to index.html for any path that isn't a real static
// asset lets SvelteKit's client router take over instead.
func staticHandler() http.Handler {
	assets, err := fs.Sub(web.Assets, "build")
	if err != nil {
		log.Fatalf("embedded web assets missing (run `npm run build` in web/ first): %v", err)
	}
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}
		if f, err := assets.Open(cleanPath); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
