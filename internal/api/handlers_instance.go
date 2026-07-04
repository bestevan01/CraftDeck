package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"craftdeck/internal/instance"
	"craftdeck/internal/javaruntime"
	"craftdeck/internal/loader"
	"craftdeck/internal/process"

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
	GamePort        int    `json:"game_port"`
	CPUQuotaPercent int    `json:"cpu_quota_percent"`
	MemoryMaxMB     int    `json:"memory_max_mb"`
	// AcceptEula must be true for kind=server: Mojang's EULA requires
	// explicit operator consent before a server.jar may run
	// (https://www.minecraft.net/eula). Proxy instances (Velocity/
	// BungeeCord) don't run a world and don't need this.
	AcceptEula bool `json:"accept_eula"`
}

func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var req createInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	kind := instance.Kind(req.Kind)

	if kind == instance.KindServer && !req.AcceptEula {
		http.Error(w, "accept_eula must be true to create a Minecraft server instance (see https://www.minecraft.net/eula)", http.StatusBadRequest)
		return
	}

	var javaMajor int
	if kind == instance.KindServer {
		major, err := javaruntime.MajorForMCVersion(req.MCVersion)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid mc_version: %v", err), http.StatusBadRequest)
			return
		}
		javaMajor = major
	}

	id := uuid.NewString()
	workDir := filepath.Join(s.dataDir, "instances", id)

	inst := &instance.Instance{
		ID:              id,
		Name:            req.Name,
		Kind:            kind,
		Loader:          req.Loader,
		LoaderVersion:   req.LoaderVersion,
		MCVersion:       req.MCVersion,
		JavaMajor:       javaMajor,
		GamePort:        req.GamePort,
		CPUQuotaPercent: req.CPUQuotaPercent,
		MemoryMaxMB:     req.MemoryMaxMB,
		WorkDir:         workDir,
		Status:          instance.StatusStopped,
		// TODO: generate RCONPort/RCONPassword, encrypt RCONPassword at rest
		// (see requirements.md FR-31, ARCHITECTURE.md section 3 design notes)
		// before RCON support lands.
	}

	if kind == instance.KindServer {
		if err := provisionServerFiles(r.Context(), inst); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := s.instances.Create(r.Context(), inst); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, inst)
}

// provisionServerFiles creates the instance's work directory, accepts the
// EULA on the operator's behalf (already confirmed via AcceptEula above),
// writes a minimal server.properties, and downloads the loader jar if an
// adapter for it exists yet (FR-1, FR-2). Loaders without an adapter so far
// (everything except Vanilla) are left without a jar -- the operator can
// upload one manually per FR-3 once that's wired up.
func provisionServerFiles(ctx context.Context, inst *instance.Instance) error {
	if err := os.MkdirAll(inst.WorkDir, 0o750); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(inst.WorkDir, "eula.txt"), []byte("eula=true\n"), 0o640); err != nil {
		return fmt.Errorf("write eula.txt: %w", err)
	}
	if inst.GamePort > 0 {
		props := fmt.Sprintf("server-port=%d\n", inst.GamePort)
		if err := os.WriteFile(filepath.Join(inst.WorkDir, "server.properties"), []byte(props), 0o640); err != nil {
			return fmt.Errorf("write server.properties: %w", err)
		}
	}

	adapter, ok := loader.Get(inst.Loader)
	if !ok {
		return nil // no adapter yet for this loader; upload jar manually (FR-3)
	}
	if _, err := adapter.Download(ctx, inst.MCVersion, inst.WorkDir); err != nil {
		return fmt.Errorf("download %s server jar: %w", inst.Loader, err)
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

func (s *Server) handleUpdateInstance(w http.ResponseWriter, r *http.Request) {
	// TODO: support editing server.properties and resource limits (FR-12).
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	if err := s.instances.Delete(r.Context(), r.PathValue("id")); err != nil {
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

	jarPath := filepath.Join(inst.WorkDir, "server.jar")
	if _, err := os.Stat(jarPath); errors.Is(err, os.ErrNotExist) {
		http.Error(w, fmt.Sprintf("no server.jar for instance %s: no loader adapter downloaded one and none was uploaded (see FR-3)", id), http.StatusConflict)
		return
	}

	javaArgs := []string{}
	if inst.MemoryMaxMB > 0 {
		javaArgs = append(javaArgs, fmt.Sprintf("-Xmx%dM", inst.MemoryMaxMB))
	}
	javaArgs = append(javaArgs, "-jar", "server.jar", "nogui")

	spec := process.StartSpec{
		InstanceID:      inst.ID,
		WorkDir:         inst.WorkDir,
		JavaBinary:      javaruntime.BinaryPath(inst.JavaMajor),
		JavaArgs:        javaArgs,
		CPUQuotaPercent: inst.CPUQuotaPercent,
		MemoryMaxMB:     inst.MemoryMaxMB,
	}

	if err := s.supervisor.Start(ctx, spec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.instances.UpdateStatus(ctx, id, instance.StatusStarting); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	// TODO: once the RCON client exists, send "stop" over RCON first for a
	// graceful shutdown (world save included) and only fall back to
	// supervisor.Stop if the instance doesn't exit in time.
	ctx := r.Context()
	id := r.PathValue("id")

	if err := s.supervisor.Stop(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.instances.UpdateStatus(ctx, id, instance.StatusStopped); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleRestartInstance(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	// TODO: wire to the RCON client. Both this REST endpoint and the GUI
	// buttons (FR-17) must call the same execution path (FR-18).
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
