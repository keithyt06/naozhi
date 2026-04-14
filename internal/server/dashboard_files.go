package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileHandlers serves the file browsing and transfer API.
type FileHandlers struct{}

type fileEntry struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"` // "file" or "dir"
	Size      int64     `json:"size"`
	ItemCount int       `json:"item_count,omitempty"` // dir only
	ModTime   time.Time `json:"mod_time"`
}

type fileListResponse struct {
	Path    string      `json:"path"`
	Entries []fileEntry `json:"entries"`
}

type fileStatResponse struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Type        string    `json:"type"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	Permissions string    `json:"permissions"`
}

type uploadedFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type fileUploadResponse struct {
	Uploaded []uploadedFile `json:"uploaded"`
}

const maxUploadSize = 100 << 20 // 100 MB

// handleList returns the contents of a directory.
// GET /api/files/list?path=/abs/path&hidden=true
func (h *FileHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}
	dirPath = filepath.Clean(dirPath)

	info, err := os.Stat(dirPath)
	if err != nil {
		http.Error(w, "path not found", http.StatusNotFound)
		return
	}
	if !info.IsDir() {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	}

	showHidden := r.URL.Query().Get("hidden") == "true"

	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, "cannot read directory", http.StatusInternalServerError)
		return
	}

	var dirs, files []fileEntry
	for _, de := range dirEntries {
		name := de.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		fi, err := de.Info()
		if err != nil {
			continue
		}
		entry := fileEntry{
			Name:    name,
			ModTime: fi.ModTime(),
		}
		if de.IsDir() {
			entry.Type = "dir"
			// Count direct children
			if sub, err := os.ReadDir(filepath.Join(dirPath, name)); err == nil {
				entry.ItemCount = len(sub)
			}
			dirs = append(dirs, entry)
		} else {
			entry.Type = "file"
			entry.Size = fi.Size()
			files = append(files, entry)
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	resp := fileListResponse{
		Path:    dirPath,
		Entries: append(dirs, files...),
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, resp)
}

// handleStat returns metadata for a single file or directory.
// GET /api/files/stat?path=/abs/path
func (h *FileHandlers) handleStat(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}
	filePath = filepath.Clean(filePath)

	fi, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	resp := fileStatResponse{
		Name:        fi.Name(),
		Path:        filePath,
		ModTime:     fi.ModTime(),
		Permissions: fi.Mode().Perm().String(),
	}
	if fi.IsDir() {
		resp.Type = "dir"
	} else {
		resp.Type = "file"
		resp.Size = fi.Size()
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, resp)
}

// handleUpload accepts multipart file uploads to a destination directory.
// POST /api/files/upload  (multipart/form-data: "dest" field + "file" fields)
func (h *FileHandlers) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+4096)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "request too large or invalid multipart", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	dest := r.FormValue("dest")
	if dest == "" {
		http.Error(w, "missing dest field", http.StatusBadRequest)
		return
	}
	dest = filepath.Clean(dest)

	di, err := os.Stat(dest)
	if err != nil || !di.IsDir() {
		http.Error(w, "destination directory not found", http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		http.Error(w, "no files provided", http.StatusBadRequest)
		return
	}

	var uploaded []uploadedFile
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			slog.Warn("file-hub: open upload", "name", fh.Filename, "err", err)
			continue
		}

		targetPath := filepath.Join(dest, filepath.Base(fh.Filename))

		// Write to temp file then atomic rename
		tmpFile, err := os.CreateTemp(dest, ".upload-*")
		if err != nil {
			f.Close()
			slog.Warn("file-hub: create temp", "err", err)
			continue
		}

		n, copyErr := io.Copy(tmpFile, f)
		f.Close()
		tmpFile.Close()

		if copyErr != nil {
			os.Remove(tmpFile.Name())
			slog.Warn("file-hub: write upload", "name", fh.Filename, "err", copyErr)
			continue
		}

		if err := os.Rename(tmpFile.Name(), targetPath); err != nil {
			os.Remove(tmpFile.Name())
			slog.Warn("file-hub: rename upload", "name", fh.Filename, "err", err)
			continue
		}

		uploaded = append(uploaded, uploadedFile{
			Name: filepath.Base(fh.Filename),
			Path: targetPath,
			Size: n,
		})
		slog.Info("file-hub: uploaded", "path", targetPath, "size", n)
	}

	if len(uploaded) == 0 {
		http.Error(w, "all uploads failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, fileUploadResponse{Uploaded: uploaded})
}

// handleDownload streams a file to the client.
// GET /api/files/download?path=/abs/path
func (h *FileHandlers) handleDownload(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}
	filePath = filepath.Clean(filePath)

	fi, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if fi.IsDir() {
		http.Error(w, "cannot download directory", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(filePath)))
	http.ServeFile(w, r, filePath)
}

// handleMkdir creates a directory (with parents).
// POST /api/files/mkdir  {"path": "/abs/path"}
func (h *FileHandlers) handleMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	req.Path = filepath.Clean(req.Path)

	if err := os.MkdirAll(req.Path, 0o755); err != nil {
		http.Error(w, "failed to create directory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "ok", "path": req.Path})
}

// handleDelete removes a file or empty directory.
// DELETE /api/files/delete?path=/abs/path
func (h *FileHandlers) handleDelete(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}
	filePath = filepath.Clean(filePath)

	fi, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// For directories, only allow removing empty ones (os.Remove fails on non-empty)
	if err := os.Remove(filePath); err != nil {
		if fi.IsDir() {
			http.Error(w, "directory not empty, cannot delete", http.StatusBadRequest)
		} else {
			http.Error(w, "failed to delete", http.StatusInternalServerError)
		}
		return
	}

	slog.Info("file-hub: deleted", "path", filePath)
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "ok"})
}

// formatSize returns a human-readable file size string.
func formatSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
