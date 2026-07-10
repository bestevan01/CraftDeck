// Command craftdeckd is the CraftDeck management daemon: a single static
// binary that serves the web UI, the REST/WebSocket API, and supervises
// Minecraft server/proxy instances via systemd-run (see ARCHITECTURE.md).
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"craftdeck/internal/api"
	"craftdeck/internal/auth"
	"craftdeck/internal/backup"
	"craftdeck/internal/config"
	"craftdeck/internal/db"
	"craftdeck/internal/instance"
	"craftdeck/internal/plugin"
	"craftdeck/internal/process"
	"craftdeck/internal/rcon"
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
	apiServer := api.NewServer(instances, supervisor, rconMgr, users, backups, plugins, cfg.DataDir)

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

	// The Velocity proxy is a singleton CraftDeck manages on the operator's
	// behalf (see internal/api/handlers_proxy.go) -- it should always exist
	// and always be running, not something the operator has to remember to
	// start themselves after every deploy/reboot.
	if err := apiServer.EnsureProxyRunning(context.Background()); err != nil {
		log.Printf("ensure velocity proxy running failed (continuing anyway): %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Routes())
	mux.Handle("/", staticHandler())

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("craftdeckd listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
