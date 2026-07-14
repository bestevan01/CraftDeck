package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"craftdeck/internal/instance"
)

// maxEditableFileBytes caps what handleGetFileContent will read as text and
// what handleSetFileContent will accept -- large enough for any real
// config file, small enough to keep the daemon from loading a multi-
// gigabyte world/region file into memory just because someone double-
// clicked it.
const maxEditableFileBytes = 4 << 20 // 4MiB

// maxUploadFileBytes caps a single file-manager upload (drag-and-drop or
// the file picker). Generous enough for a plugin/mod jar or a small
// datapack zip; anything world-sized belongs in the dedicated world
// export/import flow (handlers_world.go), not here.
const maxUploadFileBytes = 200 << 20 // 200MiB

// resolveInstancePath validates that relPath (as given by the operator --
// a query param, form field, or JSON body field) stays within inst.WorkDir:
// no absolute path, no "../" escape. Unlike the old config-only editor,
// this is a general file-manager path resolver with no extension
// allowlist, since browsing/downloading/uploading/renaming/deleting is
// meant to work on any file in the instance's directory. The leading-
// separator-then-Clean trick is the standard way to sanitize a user-
// supplied relative path: Clean collapses any ".." segments against a
// synthetic root instead of letting them climb past inst.WorkDir.
func resolveInstancePath(inst *instance.Instance, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("invalid path")
	}
	cleaned := filepath.Clean(string(filepath.Separator) + relPath)
	full := filepath.Join(inst.WorkDir, cleaned)
	if full != inst.WorkDir && !strings.HasPrefix(full, inst.WorkDir+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path")
	}
	return full, nil
}

type fileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // relative to the instance's work dir
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

// handleListFiles lists the immediate contents of one directory inside the
// instance's work dir (path="" means the work dir root itself) -- a
// standard file-manager "open this folder" listing, not a recursive walk.
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	inst, err := s.instances.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	dirPath, err := resolveInstancePath(inst, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	relDir := strings.TrimPrefix(strings.TrimPrefix(dirPath, inst.WorkDir), string(filepath.Separator))

	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, "directory not found", http.StatusNotFound)
		return
	}

	entries := make([]fileEntry, 0, len(dirEntries))
	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			continue // best-effort: a broken entry (e.g. dangling symlink) shouldn't fail the whole listing
		}
		entries = append(entries, fileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(relDir, e.Name()),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // directories first
		}
		return entries[i].Name < entries[j].Name
	})
	writeJSON(w, http.StatusOK, entries)
}

// looksLikeText reports whether the first chunk of data contains no NUL
// bytes -- the same heuristic `file`/git use to guess binary vs. text.
// Used to steer the file manager's double-click behavior (open an editor
// vs. offer a download) and to refuse overwriting something that clearly
// isn't a text file through the text-content endpoint.
func looksLikeText(data []byte) bool {
	return !bytes.ContainsRune(data, 0)
}

// handleGetFileContent reads one file's content as text, for the file
// manager's double-click-to-edit action.
func (s *Server) handleGetFileContent(w http.ResponseWriter, r *http.Request) {
	inst, err := s.instances.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	fullPath, err := resolveInstancePath(inst, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if info.Size() > maxEditableFileBytes {
		http.Error(w, "file too large to edit here -- download it instead", http.StatusRequestEntityTooLarge)
		return
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !looksLikeText(content) {
		http.Error(w, "this doesn't look like a text file -- download it instead", http.StatusUnsupportedMediaType)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": string(content)})
}

type setFileContentRequest struct {
	Content string `json:"content"`
}

// handleSetFileContent overwrites an existing file's content -- the file
// manager's "save" action after editing. Deliberately can't create a
// brand-new file this way (use handleUploadFile for that); this is only
// for editing something already there.
func (s *Server) handleSetFileContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inst, err := s.instances.Get(ctx, r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	fullPath, err := resolveInstancePath(inst, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if info, err := os.Stat(fullPath); err != nil || info.IsDir() {
		http.Error(w, "file not found (this can only edit an existing file, not create a new one)", http.StatusNotFound)
		return
	}

	var req setFileContentRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxEditableFileBytes+1)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Content) > maxEditableFileBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := os.WriteFile(fullPath, []byte(req.Content), 0o640); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chownInstanceFile(ctx, inst.ID, fullPath)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleDownloadFile streams a file's raw bytes for the file manager's
// download action -- no size cap (unlike the text-edit endpoints), since
// downloading doesn't need to hold the whole thing in memory.
func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	inst, err := s.instances.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	fullPath, err := resolveInstancePath(inst, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		downloadDirAsZip(w, fullPath)
		return
	}

	f, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(fullPath)))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, filepath.Base(fullPath), info.ModTime(), f)
}

// downloadDirAsZip streams dirPath's contents as a zip archive -- zip
// rather than the tar.gz the world export/backup features use, since it's
// natively double-click-extractable on both Windows and macOS with no
// extra tooling, matching the file manager's Explorer/Finder-style intent.
// zip.Writer writes progressively as files are added rather than building
// the whole archive in memory first, so this stays cheap even for a large
// directory -- streamed straight to the HTTP response via chunked
// transfer encoding, since the final size isn't known upfront.
func downloadDirAsZip(w http.ResponseWriter, dirPath string) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(dirPath)+".zip"))
	w.Header().Set("Content-Type", "application/zip")

	zw := zip.NewWriter(w)
	defer zw.Close()

	_ = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // best-effort: skip unreadable entries rather than aborting the whole archive
		}
		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return nil
		}
		zf, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		_, _ = io.Copy(zf, f)
		return nil
	})
}

// handleUploadFile accepts a multipart file upload (drag-and-drop or the
// file picker) into the given directory, keeping the uploaded filename.
func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inst, err := s.instances.Get(ctx, r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	dirPath, err := resolveInstancePath(inst, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		http.Error(w, "target directory not found", http.StatusNotFound)
		return
	}

	// http.MaxBytesReader (not just ParseMultipartForm's maxMemory arg,
	// which only bounds in-memory buffering and happily spills the rest to
	// disk unbounded) actually rejects a request whose body exceeds the
	// limit -- same fix as FR-40's jar upload handlers.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadFileBytes)
	if err := r.ParseMultipartForm(maxUploadFileBytes); err != nil {
		http.Error(w, "invalid multipart form or file too large (max 200MB): "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing 'file' field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// filepath.Base strips any directory components a crafted filename
	// might carry, so the upload can never land outside dirPath.
	name := filepath.Base(header.Filename)
	if name == "." || name == string(filepath.Separator) {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	destPath := filepath.Join(dirPath, name)

	out, err := os.Create(destPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
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

	info, _ := os.Stat(destPath)
	relDir := strings.TrimPrefix(strings.TrimPrefix(dirPath, inst.WorkDir), string(filepath.Separator))
	writeJSON(w, http.StatusCreated, fileEntry{
		Name: name, Path: filepath.Join(relDir, name), IsDir: false,
		Size: info.Size(), ModTime: info.ModTime().UTC().Format(time.RFC3339),
	})
}

type renameFileRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// handleRenameFile renames or moves a file/directory within the instance's
// work dir. Both endpoints are validated the same way as everything else
// here, so a rename can't be used to escape the work dir either.
func (s *Server) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inst, err := s.instances.Get(ctx, r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	var req renameFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	fromPath, err := resolveInstancePath(inst, req.From)
	if err != nil {
		http.Error(w, "invalid 'from' path", http.StatusBadRequest)
		return
	}
	toPath, err := resolveInstancePath(inst, req.To)
	if err != nil {
		http.Error(w, "invalid 'to' path", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(fromPath); err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if _, err := os.Stat(toPath); err == nil {
		http.Error(w, "a file already exists at the destination name", http.StatusConflict)
		return
	}
	if err := os.Rename(fromPath, toPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chownInstanceFile(ctx, inst.ID, toPath)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleDeleteFile deletes a file, or a directory and everything in it --
// there's no trash/undo, so the frontend is expected to confirm with the
// operator before calling this (same expectation as backup/instance
// deletion elsewhere in the API).
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	inst, err := s.instances.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	fullPath, err := resolveInstancePath(inst, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if fullPath == inst.WorkDir {
		http.Error(w, "cannot delete the instance's root directory", http.StatusBadRequest)
		return
	}
	if err := os.RemoveAll(fullPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
