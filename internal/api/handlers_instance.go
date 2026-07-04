package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

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

	var rconPort int
	var rconPassword string
	if kind == instance.KindServer {
		// TODO: encrypt RCONPassword at rest (requirements.md FR-31 covers
		// the analogous DDNS token case; RCON passwords need the same
		// treatment before this is production-ready). Plaintext for now.
		rconPort = req.GamePort + 10000
		var err error
		rconPassword, err = generateRCONPassword()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	inst := &instance.Instance{
		ID:              id,
		Name:            req.Name,
		Kind:            kind,
		Loader:          req.Loader,
		LoaderVersion:   req.LoaderVersion,
		MCVersion:       req.MCVersion,
		JavaMajor:       javaMajor,
		GamePort:        req.GamePort,
		RCONPort:        rconPort,
		RCONPassword:    rconPassword,
		CPUQuotaPercent: req.CPUQuotaPercent,
		MemoryMaxMB:     req.MemoryMaxMB,
		WorkDir:         workDir,
		Status:          instance.StatusStopped,
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
// writes a minimal server.properties, downloads the loader jar if an
// adapter for it exists yet (FR-1, FR-2), and hands the whole directory
// over to a dedicated per-instance system user (see process.EnsureInstanceUser)
// so the eventual systemd-run process (running as that user, not root) can
// actually read/write it. Loaders without an adapter so far (everything
// except Vanilla) are left without a jar -- the operator can upload one
// manually per FR-3 once that's wired up.
func provisionServerFiles(ctx context.Context, inst *instance.Instance) error {
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
		props := fmt.Sprintf(
			"server-port=%d\nenable-rcon=true\nrcon.port=%d\nrcon.password=%s\n",
			inst.GamePort, inst.RCONPort, inst.RCONPassword,
		)
		if err := os.WriteFile(filepath.Join(inst.WorkDir, "server.properties"), []byte(props), 0o640); err != nil {
			return fmt.Errorf("write server.properties: %w", err)
		}
	}

	if adapter, ok := loader.Get(inst.Loader); ok {
		if _, err := adapter.Download(ctx, inst.MCVersion, inst.WorkDir); err != nil {
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
	id := r.PathValue("id")
	s.rconMgr.StopMaintaining(id) // safety net in case it was still running
	// Best-effort: don't fail the delete if the user was already gone.
	_ = process.RemoveInstanceUser(r.Context(), id)
	if err := s.instances.Delete(r.Context(), id); err != nil {
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

	// Idempotent: re-ensures the per-instance user exists in case it was
	// somehow removed since provisioning (e.g. manual cleanup).
	username, err := process.EnsureInstanceUser(ctx, inst.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		Username:        username,
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

	// Kick off a persistent, auto-reconnecting RCON connection for this
	// instance (ARCHITECTURE.md 5.4). It'll keep retrying in the background
	// until the server's RCON listener comes up after boot.
	if inst.RCONPort > 0 {
		s.rconMgr.StartMaintaining(inst.ID, fmt.Sprintf("127.0.0.1:%d", inst.RCONPort), inst.RCONPassword)
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	// Prefer a graceful RCON "stop" (saves the world) over a hard
	// systemd-run kill. Give the server a window to actually exit before
	// falling back, since "stop" can take a few seconds to flush chunks.
	if graceful := s.tryGracefulStop(ctx, inst); !graceful {
		if err := s.supervisor.Stop(ctx, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.rconMgr.StopMaintaining(id)

	if err := s.instances.UpdateStatus(ctx, id, instance.StatusStopped); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// tryGracefulStop sends "stop" over the managed RCON connection and waits
// briefly for the unit to exit on its own. Returns false if RCON wasn't
// reachable or the unit was still active after the wait, signaling the
// caller to fall back to supervisor.Stop.
func (s *Server) tryGracefulStop(ctx context.Context, inst *instance.Instance) bool {
	if inst.RCONPort == 0 {
		return false
	}
	if _, err := s.rconMgr.Execute(inst.ID, "stop"); err != nil {
		return false
	}

	for i := 0; i < 20; i++ { // up to ~20s for world save + shutdown
		time.Sleep(1 * time.Second)
		active, _ := s.supervisor.IsActive(ctx, inst.ID)
		if !active {
			return true
		}
	}
	return false
}

func (s *Server) handleRestartInstance(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
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
