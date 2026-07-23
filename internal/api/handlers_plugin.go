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

// searchSupported reports whether Modrinth-backed search/install (FR-5,
// FR-6) is available for this instance's loader -- requires a loader tag
// Modrinth actually recognizes (see modrinthProjectType). Paper, Purpur,
// Folia, Pufferfish, and Leaf all load plugins the same Bukkit-API way (all
// Paper forks); Fabric and NeoForge use mods instead, a different file
// format and Modrinth project_type, but the same underlying "drop a jar in
// a directory, restart to apply" idea (see pluginsDirName). A custom loader
// jar uploaded manually per FR-3 has no such tag for Modrinth to filter
// by, so it can't be searched even though it can still receive manually
// uploaded jars -- see uploadSupported, which is deliberately broader than
// this.
func searchSupported(loader string) bool {
	switch strings.ToLower(loader) {
	case "paper", "purpur", "folia", "pufferfish", "leaf", "fabric", "neoforge":
		return true
	default:
		return false
	}
}

// uploadSupported reports whether manually uploading/listing/enabling/
// deleting a plugin or mod jar applies at all. Broader than
// searchSupported: it doesn't need Modrinth to recognize the loader, just
// a directory (pluginsDirName) the server itself scans for extension jars
// -- so a server running a custom, manually-uploaded loader jar (FR-3)
// still gets this, just without search. Only Vanilla (no extension
// mechanism at all) and the Velocity proxy (a different, unmanaged plugin
// ecosystem) are excluded.
func uploadSupported(loader string) bool {
	switch strings.ToLower(loader) {
	case "vanilla", "velocity", "":
		return false
	default:
		return true
	}
}

// pluginsDirName is the directory a loader scans for extension jars --
// "plugins" for the Bukkit-API family, "mods" for Fabric/NeoForge. Falls
// back to "plugins" for a custom/unrecognized loader (FR-3) since there's
// no way to know which convention it follows; the operator can tell where
// the file actually landed from the upload response.
func pluginsDirName(loader string) string {
	switch strings.ToLower(loader) {
	case "fabric", "neoforge":
		return "mods"
	default:
		return "plugins"
	}
}

// modrinthProjectType is the Modrinth project_type to search/resolve
// against for a given loader -- "mod" for Fabric/NeoForge, "plugin"
// otherwise (see modrinth.Search/BestVersion's loader-compatibility
// filtering, which already takes the loader name itself as a separate
// parameter).
func modrinthProjectType(loader string) string {
	switch strings.ToLower(loader) {
	case "fabric", "neoforge":
		return "mod"
	default:
		return "plugin"
	}
}

func (s *Server) handleSearchPlugins(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if !searchSupported(inst.Loader) {
		http.Error(w, "Modrinth search isn't supported for this instance's loader (upload a .jar manually instead)", http.StatusBadRequest)
		return
	}

	hits, err := modrinth.Search(r.Context(), r.URL.Query().Get("query"), modrinthProjectType(inst.Loader), inst.Loader, inst.MCVersion)
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
	if !searchSupported(inst.Loader) {
		http.Error(w, "Modrinth install isn't supported for this instance's loader (upload a .jar manually instead)", http.StatusBadRequest)
		return
	}

	var req installPluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProjectID == "" {
		http.Error(w, "project_id is required", http.StatusBadRequest)
		return
	}

	installed, err := s.installModrinthPlugin(ctx, inst, req.ProjectID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, installed)
}

// installModrinthPlugin downloads projectID's newest version compatible
// with inst's loader/Minecraft version, verifies its SHA-512 against what
// Modrinth published (FR-6d), and recursively installs any required
// dependency that isn't already present (FR-6c). parentID is the ID of the
// plugin whose dependency resolution triggered this install, or "" for a
// top-level install the operator requested directly -- recorded so the UI
// can group auto-installed dependencies under the plugin that pulled them
// in instead of showing an undifferentiated flat list. When a dependency
// is shared by multiple plugins, it's grouped under whichever one
// triggered its install first (FindByModrinthProject below short-circuits
// any later ones).
func (s *Server) installModrinthPlugin(ctx context.Context, inst *instance.Instance, projectID string, parentID string) (*plugin.Plugin, error) {
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

	pluginsDir := filepath.Join(inst.WorkDir, pluginsDirName(inst.Loader))
	if err := os.MkdirAll(pluginsDir, 0o750); err != nil {
		return nil, err
	}
	// A directory created just now by this (root) daemon process is
	// root-owned -- the loader itself runs as the instance's own unprivileged
	// user and can't even list a directory it doesn't own, regardless of the
	// file's own ownership inside it (confirmed on real hardware: Fabric's
	// mod scan failed with AccessDeniedException walking mods/ for an
	// instance that never had mods/ created during provisioning, e.g. one
	// that was independently exposed and so skipped installFabricProxyMods).
	// A pre-existing directory (already owned by the instance user from
	// provisioning) is unaffected by this -- chown is idempotent.
	chownInstanceFile(ctx, inst.ID, pluginsDir)
	destPath := filepath.Join(pluginsDir, file.Filename)
	expectedSHA512 := file.Hashes["sha512"]
	if err := downloadAndVerifySHA512(ctx, file.URL, expectedSHA512, destPath); err != nil {
		return nil, fmt.Errorf("download %s: %w", file.Filename, err)
	}
	chownInstanceFile(ctx, inst.ID, destPath)

	p := &plugin.Plugin{
		ID:                    uuid.NewString(),
		InstanceID:            inst.ID,
		Source:                "modrinth",
		ModrinthProjectID:     projectID,
		ModrinthVersionID:     version.ID,
		Filename:              file.Filename,
		SHA512:                expectedSHA512,
		Enabled:               true,
		InstalledAsDependency: parentID != "",
		ParentPluginID:        parentID,
	}
	if err := s.plugins.Create(ctx, p); err != nil {
		os.Remove(destPath)
		return nil, err
	}

	for _, dep := range version.Dependencies {
		if dep.DependencyType != "required" || dep.ProjectID == "" {
			continue
		}
		if _, err := s.installModrinthPlugin(ctx, inst, dep.ProjectID, p.ID); err != nil {
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
	if !uploadSupported(inst.Loader) {
		http.Error(w, "plugin/mod upload is not supported for this instance's loader", http.StatusBadRequest)
		return
	}

	const maxUploadBytes = 100 << 20 // 100MiB (FR-8/FR-40 size limit)
	// http.MaxBytesReader (not just ParseMultipartForm's maxMemory arg,
	// which only bounds in-memory buffering and happily spills the rest to
	// disk unbounded) actually rejects a request whose body exceeds the
	// limit, confirmed necessary for FR-40's size validation.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
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
	validated, err := requireJarMagicBytes(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// filepath.Base, not the raw (fully attacker-controlled) header.Filename
	// -- confirmed this was a real path-traversal gap: a filename like
	// "../../etc/cron.d/x.jar" would otherwise make filepath.Join below
	// write outside pluginsDir entirely.
	safeFilename := filepath.Base(header.Filename)

	pluginsDir := filepath.Join(inst.WorkDir, pluginsDirName(inst.Loader))
	if err := os.MkdirAll(pluginsDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// See installModrinthPlugin's identical call for why this is needed --
	// a directory created just now by this root process isn't readable by
	// the instance's own unprivileged user otherwise.
	chownInstanceFile(ctx, inst.ID, pluginsDir)
	destPath := filepath.Join(pluginsDir, safeFilename)

	out, err := os.Create(destPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hasher := sha512.New()
	if _, err := io.Copy(io.MultiWriter(out, hasher), validated); err != nil {
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
	chownInstanceFile(ctx, inst.ID, destPath)

	p := &plugin.Plugin{
		ID:         uuid.NewString(),
		InstanceID: inst.ID,
		Source:     "upload",
		Filename:   safeFilename,
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

	pluginsDir := filepath.Join(inst.WorkDir, pluginsDirName(inst.Loader))
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

	path := filepath.Join(inst.WorkDir, pluginsDirName(inst.Loader), p.DiskFilename())
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

// chownInstanceFile hands ownership of a newly written or edited file
// (plugin/mod jar, config file, ...) to the instance's per-instance system
// user, the same as provisionServerFiles does at creation time -- otherwise
// a file written by this root-owned daemon wouldn't be readable by the
// process that actually runs the server. Best-effort: a failure here just
// means the operator has to fix ownership manually before it'll load.
func chownInstanceFile(ctx context.Context, instanceID, path string) {
	username, err := process.EnsureInstanceUser(ctx, instanceID)
	if err != nil {
		log.Printf("chown %s: ensure instance user: %v", path, err)
		return
	}
	if err := process.ChownRecursive(ctx, path, username); err != nil {
		log.Printf("chown %s: %v", path, err)
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
