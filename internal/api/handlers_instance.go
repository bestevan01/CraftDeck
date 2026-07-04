package api

import (
	"encoding/json"
	"net/http"

	"craftdeck/internal/instance"

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
}

func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var req createInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	id := uuid.NewString()
	inst := &instance.Instance{
		ID:              id,
		Name:            req.Name,
		Kind:            instance.Kind(req.Kind),
		Loader:          req.Loader,
		LoaderVersion:   req.LoaderVersion,
		MCVersion:       req.MCVersion,
		GamePort:        req.GamePort,
		CPUQuotaPercent: req.CPUQuotaPercent,
		MemoryMaxMB:     req.MemoryMaxMB,
		WorkDir:         "/var/lib/craftdeck/instances/" + id,
		Status:          instance.StatusStopped,
		// TODO: generate RCONPort/RCONPassword, encrypt RCONPassword at rest
		// (see requirements.md FR-31, ARCHITECTURE.md section 3 design notes)
		// before this handler is wired up for real use.
	}

	if err := s.instances.Create(r.Context(), inst); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, inst)
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
	// TODO: resolve StartSpec (java binary per instance.JavaMajor, loader
	// launch args) and call process.Supervisor.Start; see
	// ARCHITECTURE.md section 5.1 and 5.2.
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	// TODO: prefer RCON "stop" for a graceful shutdown before falling back
	// to process.Supervisor.Stop.
	http.Error(w, "not implemented", http.StatusNotImplemented)
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
