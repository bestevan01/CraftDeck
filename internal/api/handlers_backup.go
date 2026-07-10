package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"craftdeck/internal/backup"
	"craftdeck/internal/instance"
	"craftdeck/internal/process"

	"github.com/google/uuid"
)

// backupsDir is where every instance's backup archives live, namespaced by
// instance ID: dataDir/backups/<instance_id>/<backup_id>.tar.gz.
func (s *Server) backupsDir(instanceID string) string {
	return filepath.Join(s.dataDir, "backups", instanceID)
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.instances.Get(r.Context(), id); err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}

	list, err := s.backups.ListByInstance(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleCreateBackup archives the instance's entire work directory (world
// data, configs, plugins/mods, server jar) into a single gzip-compressed
// tar file. Requires the instance to be stopped first -- archiving region
// files while the server is actively writing to them risks capturing a
// torn/inconsistent world state.
func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Status != instance.StatusStopped && inst.Status != instance.StatusCrashed {
		http.Error(w, "instance must be stopped before creating a backup", http.StatusConflict)
		return
	}

	dir := s.backupsDir(id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	backupID := uuid.NewString()
	filename := time.Now().UTC().Format("2006-01-02T15-04-05Z") + ".tar.gz"
	destPath := filepath.Join(dir, filename)

	size, err := backup.Create(inst.WorkDir, destPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b := &backup.Backup{ID: backupID, InstanceID: id, Filename: filename, SizeBytes: size}
	if err := s.backups.Create(ctx, b); err != nil {
		os.Remove(destPath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// handleRestoreBackup replaces the instance's work directory with the
// contents of a previously created backup. Requires the instance to be
// stopped, and wipes the existing work directory first so leftover files
// that aren't part of the backup (e.g. from a differently-configured backup
// taken earlier) don't linger in an inconsistent mix with the restored ones.
func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	backupID := r.PathValue("backupId")

	inst, err := s.instances.Get(ctx, id)
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.Status != instance.StatusStopped && inst.Status != instance.StatusCrashed {
		http.Error(w, "instance must be stopped before restoring a backup", http.StatusConflict)
		return
	}

	b, err := s.backups.Get(ctx, backupID)
	if err != nil || b.InstanceID != id {
		http.Error(w, "backup not found", http.StatusNotFound)
		return
	}
	archivePath := filepath.Join(s.backupsDir(id), b.Filename)

	if err := os.RemoveAll(inst.WorkDir); err != nil {
		http.Error(w, fmt.Errorf("clear work dir before restore: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(inst.WorkDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := backup.Restore(archivePath, inst.WorkDir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The restored files are owned by whichever user ran this daemon
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

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	backupID := r.PathValue("backupId")

	b, err := s.backups.Get(ctx, backupID)
	if err != nil || b.InstanceID != id {
		http.Error(w, "backup not found", http.StatusNotFound)
		return
	}

	archivePath := filepath.Join(s.backupsDir(id), b.Filename)
	if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.backups.Delete(ctx, backupID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
