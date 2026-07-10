package api

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"craftdeck/internal/instance"
	"craftdeck/internal/modrinth"
	"craftdeck/internal/plugin"
	"craftdeck/internal/process"

	"github.com/google/uuid"
)

// pluginsSupported reports whether instance-level plugin management applies
// to this instance's loader (FR-5). Only Paper is implemented as a loader
// adapter so far -- Vanilla has no plugin support at all, and mod loaders
// (Forge/Fabric) will need their own project_type ("mod") handling later.
func pluginsSupported(loader string) bool {
	return strings.EqualFold(loader, "paper")
}

func (s *Server) handleSearchPlugins(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if !pluginsSupported(inst.Loader) {
		http.Error(w, "plugin management is only supported for Paper instances currently", http.StatusBadRequest)
		return
	}

	hits, err := modrinth.Search(r.Context(), r.URL.Query().Get("query"), "plugin", inst.Loader, inst.MCVersion)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, hits)
}

// handleListPlugins reads from our own DB records rather than re-scanning
// the plugins/ directory, so a Modrinth outage never affects viewing or
// managing already-installed plugins (FR-6b).
func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.instances.Get(r.Context(), id); err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	list, err := s.plugins.ListByInstance(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type installPluginRequest struct {
	ProjectID string `json:"project_id"`
}

func (s *Server) handleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if !pluginsSupported(inst.Loader) {
		http.Error(w, "plugin management is only supported for Paper instances currently", http.StatusBadRequest)
		return
	}

	var req installPluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProjectID == "" {
		http.Error(w, "project_id is required", http.StatusBadRequest)
		return
	}

	installed, err := s.installModrinthPlugin(ctx, inst, req.ProjectID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, installed)
}

// installModrinthPlugin downloads projectID's newest version compatible
// with inst's loader/Minecraft version, verifies its SHA-512 against what
// Modrinth published (FR-6d), and recursively installs any required
// dependency that isn't already present (FR-6c). asDependency marks
// whether this call is itself satisfying another plugin's dependency, so
// the UI can show it was pulled in automatically rather than by the
// operator directly.
func (s *Server) installModrinthPlugin(ctx context.Context, inst *instance.Instance, projectID string, asDependency bool) (*plugin.Plugin, error) {
	if existing, err := s.plugins.FindByModrinthProject(ctx, inst.ID, projectID); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil // already installed -- common for shared dependencies
	}

	version, err := modrinth.BestVersion(ctx, projectID, inst.Loader, inst.MCVersion)
	if err != nil {
		return nil, err
	}
	file, err := version.PrimaryFile()
	if err != nil {
		return nil, err
	}

	pluginsDir := filepath.Join(inst.WorkDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o750); err != nil {
		return nil, err
	}
	destPath := filepath.Join(pluginsDir, file.Filename)
	expectedSHA512 := file.Hashes["sha512"]
	if err := downloadAndVerifySHA512(ctx, file.URL, expectedSHA512, destPath); err != nil {
		return nil, fmt.Errorf("download %s: %w", file.Filename, err)
	}
	chownPluginFile(ctx, inst.ID, destPath)

	p := &plugin.Plugin{
		ID:                    uuid.NewString(),
		InstanceID:            inst.ID,
		Source:                "modrinth",
		ModrinthProjectID:     projectID,
		ModrinthVersionID:     version.ID,
		Filename:              file.Filename,
		SHA512:                expectedSHA512,
		Enabled:               true,
		InstalledAsDependency: asDependency,
	}
	if err := s.plugins.Create(ctx, p); err != nil {
		os.Remove(destPath)
		return nil, err
	}

	for _, dep := range version.Dependencies {
		if dep.DependencyType != "required" || dep.ProjectID == "" {
			continue
		}
		if _, err := s.installModrinthPlugin(ctx, inst, dep.ProjectID, true); err != nil {
			// The primary plugin is already installed successfully; a
			// dependency failure shouldn't undo that, just get logged so
			// the operator can install it manually if actually needed.
			log.Printf("install dependency %s for plugin %s: %v (continuing)", dep.ProjectID, p.Filename, err)
		}
	}
	return p, nil
}

// handleUploadPlugin accepts a direct .jar upload (FR-5, FR-8) instead of a
// Modrinth-listed plugin.
func (s *Server) handleUploadPlugin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if !pluginsSupported(inst.Loader) {
		http.Error(w, "plugin management is only supported for Paper instances currently", http.StatusBadRequest)
		return
	}

	const maxUploadBytes = 100 << 20 // 100MiB (FR-8 size limit)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "invalid multipart form or file too large (max 100MB): "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("plugin")
	if err != nil {
		http.Error(w, "missing 'plugin' file field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".jar") {
		http.Error(w, "only .jar files are accepted", http.StatusBadRequest)
		return
	}

	pluginsDir := filepath.Join(inst.WorkDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	destPath := filepath.Join(pluginsDir, header.Filename)

	out, err := os.Create(destPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hasher := sha512.New()
	if _, err := io.Copy(io.MultiWriter(out, hasher), file); err != nil {
		out.Close()
		os.Remove(destPath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := out.Close(); err != nil {
		os.Remove(destPath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chownPluginFile(ctx, inst.ID, destPath)

	p := &plugin.Plugin{
		ID:         uuid.NewString(),
		InstanceID: inst.ID,
		Source:     "upload",
		Filename:   header.Filename,
		SHA512:     hex.EncodeToString(hasher.Sum(nil)),
		Enabled:    true,
	}
	if err := s.plugins.Create(ctx, p); err != nil {
		os.Remove(destPath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

type setPluginEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// handleSetPluginEnabled toggles a plugin on/off by renaming its file to/from
// a ".disabled" suffix -- Paper (like Bukkit/Spigot before it) only loads
// files directly named "*.jar" in plugins/, so this is a real, effective
// disable rather than just a UI-only flag (still requires a restart per
// FR-9, same as install/delete).
func (s *Server) handleSetPluginEnabled(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	pluginID := r.PathValue("pluginId")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	p, err := s.plugins.Get(ctx, pluginID)
	if err != nil || p.InstanceID != id {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}
	var req setPluginEnabledRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Enabled == p.Enabled {
		writeJSON(w, http.StatusOK, p)
		return
	}

	pluginsDir := filepath.Join(inst.WorkDir, "plugins")
	oldPath := filepath.Join(pluginsDir, p.DiskFilename())
	p.Enabled = req.Enabled
	newPath := filepath.Join(pluginsDir, p.DiskFilename())
	if err := os.Rename(oldPath, newPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.plugins.SetEnabled(ctx, pluginID, req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	pluginID := r.PathValue("pluginId")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	p, err := s.plugins.Get(ctx, pluginID)
	if err != nil || p.InstanceID != id {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}

	path := filepath.Join(inst.WorkDir, "plugins", p.DiskFilename())
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.plugins.Delete(ctx, pluginID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// chownPluginFile hands ownership of a newly written plugin jar to the
// instance's per-instance system user, the same as provisionServerFiles
// does at creation time -- otherwise a plugin downloaded/uploaded via this
// root-owned daemon wouldn't be readable by the process that actually runs
// the server. Best-effort: a failure here just means the operator has to
// fix ownership manually before the plugin will load.
func chownPluginFile(ctx context.Context, instanceID, path string) {
	username, err := process.EnsureInstanceUser(ctx, instanceID)
	if err != nil {
		log.Printf("chown plugin %s: ensure instance user: %v", path, err)
		return
	}
	if err := process.ChownRecursive(ctx, path, username); err != nil {
		log.Printf("chown plugin %s: %v", path, err)
	}
}

// downloadAndVerifySHA512 streams url to destPath, verifying its SHA-512
// against expectedHex if provided (Modrinth always provides one).
func downloadAndVerifySHA512(ctx context.Context, url, expectedHex, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d downloading %s", resp.StatusCode, url)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	hasher := sha512.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if expectedHex != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(got, expectedHex) {
			os.Remove(tmpPath)
			return fmt.Errorf("sha512 mismatch for %s: got %s, want %s", destPath, got, expectedHex)
		}
	}
	return os.Rename(tmpPath, destPath)
}
