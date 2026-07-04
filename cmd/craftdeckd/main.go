// Command craftdeckd is the CraftDeck management daemon: a single static
// binary that serves the web UI, the REST/WebSocket API, and supervises
// Minecraft server/proxy instances via systemd-run (see ARCHITECTURE.md).
package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"craftdeck/internal/api"
	"craftdeck/internal/config"
	"craftdeck/internal/db"
	"craftdeck/internal/instance"
	"craftdeck/internal/process"
	"craftdeck/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}

	database, err := db.Open(filepath.Join(cfg.DataDir, "craftdeck.db"))
	if err != nil {
		return err
	}
	defer database.Close()

	instances := instance.NewRepository(database)
	supervisor := process.NewSupervisor()
	apiServer := api.NewServer(instances, supervisor, cfg.DataDir)

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

// staticHandler serves the embedded SvelteKit build (see web/embed.go).
func staticHandler() http.Handler {
	assets, err := fs.Sub(web.Assets, "build")
	if err != nil {
		log.Fatalf("embedded web assets missing (run `npm run build` in web/ first): %v", err)
	}
	return http.FileServer(http.FS(assets))
}
