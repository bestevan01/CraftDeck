package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"craftdeck/internal/backup"
	"craftdeck/internal/instance"
	"craftdeck/internal/process"
	"craftdeck/internal/worldinfo"
)

// levelName reads server.properties's level-name (the world folder
// prefix), defaulting to vanilla's own default ("world") if the key or the
// file itself is missing (e.g. an instance that's never been started yet).
func levelName(workDir string) string {
	name, found, err := readServerProperty(workDir, "level-name")
	if err != nil || !found || name == "" {
		return "world"
	}
	return name
}

// worldDirNames returns the (up to) three world folders a vanilla/paper
// instance uses: the overworld plus its nether/end companions, which are
// always suffixed onto the same level-name.
func worldDirNames(workDir string) []string {
	base := levelName(workDir)
	return []string{base, base + "_nether", base + "_the_end"}
}

// handleWorldInfo reports the world's on-disk folder name and, best-effort,
// the Minecraft version its level.dat says it was last saved with -- so an
// operator can sanity-check compatibility before exporting/importing.
func (s *Server) handleWorldInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	name := levelName(inst.WorkDir)
	levelDatPath := filepath.Join(inst.WorkDir, name, "level.dat")
	resp := map[string]any{
		"level_name":       name,
		"instance_version": inst.MCVersion,
		"detected_version": "",
		"detect_error":     "",
	}
	detected, verr := worldinfo.DetectVersionFromLevelDat(levelDatPath)
	if verr != nil {
		resp["detect_error"] = verr.Error()
	} else {
		resp["detected_version"] = detected
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleExportWorld streams just the instance's world folders (not the
// whole work directory -- no server jar, no plugin/mod jars) as a
// gzip-compressed tar download. Requires the instance to be stopped, same
// as backups: archiving region files mid-write risks a torn read.
func (s *Server) handleExportWorld(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Status != instance.StatusStopped && inst.Status != instance.StatusCrashed {
		http.Error(w, "instance must be stopped before exporting world data", http.StatusConflict)
		return
	}

	tmpFile, err := os.CreateTemp("", "craftdeck-world-export-*.tar.gz")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	names := worldDirNames(inst.WorkDir)
	include := func(topLevel string) bool {
		for _, n := range names {
			if topLevel == n {
				return true
			}
		}
		return false
	}
	if _, err := backup.CreateFiltered(inst.WorkDir, tmpPath, include); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-world.tar.gz"`, inst.Name))
	http.ServeFile(w, r, tmpPath)
}

// handleImportWorld replaces the instance's world folders with the contents
// of an uploaded gzip-compressed tar (as produced by handleExportWorld or
// internal/backup). Requires the instance to be stopped. Before applying,
// it best-effort-detects the uploaded world's Minecraft version and blocks
// the import if it looks like a downgrade (Minecraft doesn't support
// opening a world in an older version -- vanilla itself refuses this and it
// commonly corrupts chunks) -- unless the "force" form field overrides it.
func (s *Server) handleImportWorld(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Status != instance.StatusStopped && inst.Status != instance.StatusCrashed {
		http.Error(w, "instance must be stopped before importing world data", http.StatusConflict)
		return
	}

	if err := r.ParseMultipartForm(1 << 30); err != nil { // up to 1GiB
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("world")
	if err != nil {
		http.Error(w, "missing 'world' file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmpFile, err := os.CreateTemp("", "craftdeck-world-import-*.tar.gz")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	detected, detectErr := worldinfo.DetectVersionFromArchive(tmpPath)
	force := r.FormValue("force") == "true"
	if detectErr == nil && !force {
		if newer, comparable := worldinfo.CompareClassicVersions(detected, inst.MCVersion); comparable && newer {
			http.Error(w, fmt.Sprintf(
				"업로드한 월드는 %s 버전이고 이 인스턴스는 %s 버전입니다. 마인크래프트는 월드를 이전 버전으로 여는 것을 지원하지 않아 손상될 수 있습니다. 그래도 진행하려면 강제 적용을 선택하세요.",
				detected, inst.MCVersion,
			), http.StatusConflict)
			return
		}
	}

	for _, name := range worldDirNames(inst.WorkDir) {
		if err := os.RemoveAll(filepath.Join(inst.WorkDir, name)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := backup.Restore(tmpPath, inst.WorkDir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The extracted files are owned by whichever user ran this daemon
	// (root), not the instance's per-instance system user -- re-chown so
	// the next start (running as that user) can actually read/write them.
	username, err := process.EnsureInstanceUser(ctx, inst.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := process.ChownRecursive(ctx, inst.WorkDir, username); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]any{"detected_version": detected}
	if detectErr != nil {
		resp["detect_error"] = detectErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}
