# File Hub Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add server file browsing, upload/download, and path-to-chat injection to Naozhi, accessible from Dashboard UI and IM `/ls` command.

**Architecture:** Naozhi-native REST API layer (`/api/files/*`) handles all file operations directly via Go's `os` package — no Claude CLI dependency. Dashboard gets a reusable modal component. `/ls` command in dispatch layer reuses the same list logic, formatting as text for IM platforms.

**Tech Stack:** Go stdlib (`os`, `filepath`, `io`, `net/http`), vanilla JavaScript (matching existing dashboard.html patterns).

**Spec:** `docs/superpowers/specs/2026-04-14-file-hub-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/server/dashboard_files.go` | FileHandlers struct + 6 HTTP handlers (list/stat/upload/download/mkdir/delete) |
| `internal/server/dashboard_files_test.go` | Unit tests for all file API endpoints |

### Modified Files

| File | Changes |
|------|---------|
| `internal/server/server.go` | Add `filesH *FileHandlers` field, register `/api/files/*` routes in `registerDashboard()` |
| `internal/dispatch/commands.go` | Add `/ls` command handler in `dispatchCommand()` switch |
| `internal/server/static/dashboard.html` | Add Files nav tab, File Hub modal, `/ls` enhanced rendering |

---

## Task 1: FileHandlers struct + list endpoint

**Files:**
- Create: `internal/server/dashboard_files.go`
- Test: `internal/server/dashboard_files_test.go`

- [ ] **Step 1: Write the failing test for list endpoint**

```go
// internal/server/dashboard_files_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesList(t *testing.T) {
	// Setup temp directory with known structure
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "subdir"), 0o755)
	os.WriteFile(filepath.Join(tmp, "hello.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".hidden"), []byte("secret"), 0o644)

	h := &FileHandlers{}

	t.Run("list directory", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/list?path="+tmp, nil)
		w := httptest.NewRecorder()
		h.handleList(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp fileListResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Path != tmp {
			t.Errorf("expected path %s, got %s", tmp, resp.Path)
		}
		// Should have 1 dir + 1 file (hidden excluded by default)
		if len(resp.Entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(resp.Entries))
		}
		// Dirs first
		if resp.Entries[0].Name != "subdir" || resp.Entries[0].Type != "dir" {
			t.Errorf("first entry should be dir 'subdir', got %+v", resp.Entries[0])
		}
		if resp.Entries[1].Name != "hello.txt" || resp.Entries[1].Type != "file" {
			t.Errorf("second entry should be file 'hello.txt', got %+v", resp.Entries[1])
		}
	})

	t.Run("list with hidden", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/list?path="+tmp+"&hidden=true", nil)
		w := httptest.NewRecorder()
		h.handleList(w, req)

		var resp fileListResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp.Entries) != 3 {
			t.Errorf("expected 3 entries with hidden, got %d", len(resp.Entries))
		}
	})

	t.Run("list nonexistent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/list?path=/nonexistent/path", nil)
		w := httptest.NewRecorder()
		h.handleList(w, req)

		if w.Code != 404 {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("missing path param", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/list", nil)
		w := httptest.NewRecorder()
		h.handleList(w, req)

		if w.Code != 400 {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestFilesList -v`
Expected: FAIL — `FileHandlers` and `fileListResponse` not defined.

- [ ] **Step 3: Implement FileHandlers struct and list handler**

```go
// internal/server/dashboard_files.go
package server

import (
	"encoding/json"
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

// handleList returns the contents of a directory.
// GET /api/files/list?path=/abs/path&hidden=true
func (h *FileHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		http.Error(w, `{"error":"missing path parameter"}`, http.StatusBadRequest)
		return
	}
	dirPath = filepath.Clean(dirPath)

	info, err := os.Stat(dirPath)
	if err != nil {
		http.Error(w, `{"error":"path not found","path":"`+dirPath+`"}`, http.StatusNotFound)
		return
	}
	if !info.IsDir() {
		http.Error(w, `{"error":"not a directory","path":"`+dirPath+`"}`, http.StatusBadRequest)
		return
	}

	showHidden := r.URL.Query().Get("hidden") == "true"

	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, `{"error":"cannot read directory"}`, http.StatusInternalServerError)
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
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestFilesList -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/dashboard_files.go internal/server/dashboard_files_test.go
git commit -m "feat(file-hub): add FileHandlers struct and list endpoint"
```

---

## Task 2: stat endpoint

**Files:**
- Modify: `internal/server/dashboard_files.go`
- Modify: `internal/server/dashboard_files_test.go`

- [ ] **Step 1: Write the failing test for stat**

Append to `dashboard_files_test.go`:

```go
func TestFilesStat(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "test.txt"), []byte("hello world"), 0o644)
	os.MkdirAll(filepath.Join(tmp, "mydir"), 0o755)

	h := &FileHandlers{}

	t.Run("stat file", func(t *testing.T) {
		p := filepath.Join(tmp, "test.txt")
		req := httptest.NewRequest("GET", "/api/files/stat?path="+p, nil)
		w := httptest.NewRecorder()
		h.handleStat(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp fileStatResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "test.txt" || resp.Type != "file" || resp.Size != 11 {
			t.Errorf("unexpected stat: %+v", resp)
		}
	})

	t.Run("stat dir", func(t *testing.T) {
		p := filepath.Join(tmp, "mydir")
		req := httptest.NewRequest("GET", "/api/files/stat?path="+p, nil)
		w := httptest.NewRecorder()
		h.handleStat(w, req)

		var resp fileStatResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Type != "dir" {
			t.Errorf("expected dir, got %s", resp.Type)
		}
	})

	t.Run("stat nonexistent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/stat?path=/no/such/file", nil)
		w := httptest.NewRecorder()
		h.handleStat(w, req)

		if w.Code != 404 {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestFilesStat -v`
Expected: FAIL — `handleStat` and `fileStatResponse` not defined.

- [ ] **Step 3: Implement stat handler**

Add to `dashboard_files.go`:

```go
type fileStatResponse struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Type        string    `json:"type"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	Permissions string    `json:"permissions"`
}

// handleStat returns metadata for a single file or directory.
// GET /api/files/stat?path=/abs/path
func (h *FileHandlers) handleStat(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, `{"error":"missing path parameter"}`, http.StatusBadRequest)
		return
	}
	filePath = filepath.Clean(filePath)

	fi, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, `{"error":"not found","path":"`+filePath+`"}`, http.StatusNotFound)
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
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestFilesStat -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/dashboard_files.go internal/server/dashboard_files_test.go
git commit -m "feat(file-hub): add stat endpoint"
```

---

## Task 3: upload endpoint

**Files:**
- Modify: `internal/server/dashboard_files.go`
- Modify: `internal/server/dashboard_files_test.go`

- [ ] **Step 1: Write the failing test for upload**

Append to `dashboard_files_test.go`:

```go
import (
	"bytes"
	"mime/multipart"
)

func TestFilesUpload(t *testing.T) {
	tmp := t.TempDir()
	h := &FileHandlers{}

	t.Run("upload single file", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.WriteField("dest", tmp)
		fw, _ := mw.CreateFormFile("file", "uploaded.txt")
		fw.Write([]byte("file content here"))
		mw.Close()

		req := httptest.NewRequest("POST", "/api/files/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		h.handleUpload(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp fileUploadResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp.Uploaded) != 1 {
			t.Fatalf("expected 1 uploaded, got %d", len(resp.Uploaded))
		}
		if resp.Uploaded[0].Name != "uploaded.txt" {
			t.Errorf("expected name 'uploaded.txt', got %s", resp.Uploaded[0].Name)
		}

		// Verify file on disk
		data, err := os.ReadFile(filepath.Join(tmp, "uploaded.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "file content here" {
			t.Errorf("file content mismatch: %s", data)
		}
	})

	t.Run("upload missing dest", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "test.txt")
		fw.Write([]byte("data"))
		mw.Close()

		req := httptest.NewRequest("POST", "/api/files/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		h.handleUpload(w, req)

		if w.Code != 400 {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("upload to nonexistent dir", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.WriteField("dest", "/no/such/dir")
		fw, _ := mw.CreateFormFile("file", "test.txt")
		fw.Write([]byte("data"))
		mw.Close()

		req := httptest.NewRequest("POST", "/api/files/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		h.handleUpload(w, req)

		if w.Code != 400 {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestFilesUpload -v`
Expected: FAIL — `handleUpload` and `fileUploadResponse` not defined.

- [ ] **Step 3: Implement upload handler**

Add to `dashboard_files.go`:

```go
import (
	"fmt"
	"io"
	"log/slog"
)

const maxUploadSize = 100 << 20 // 100 MB

type uploadedFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type fileUploadResponse struct {
	Uploaded []uploadedFile `json:"uploaded"`
}

// handleUpload accepts multipart file uploads to a destination directory.
// POST /api/files/upload  (multipart/form-data: "dest" field + "file" fields)
func (h *FileHandlers) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+4096)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, `{"error":"request too large or invalid multipart"}`, http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	dest := r.FormValue("dest")
	if dest == "" {
		http.Error(w, `{"error":"missing dest field"}`, http.StatusBadRequest)
		return
	}
	dest = filepath.Clean(dest)

	di, err := os.Stat(dest)
	if err != nil || !di.IsDir() {
		http.Error(w, `{"error":"destination directory not found","path":"`+dest+`"}`, http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		http.Error(w, `{"error":"no files provided"}`, http.StatusBadRequest)
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
		http.Error(w, `{"error":"all uploads failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fileUploadResponse{Uploaded: uploaded})
}
```

Note: add the new imports (`"fmt"`, `"io"`, `"log/slog"`) to the existing import block. Remove `"fmt"` if not used elsewhere — the `slog` package handles structured logging.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestFilesUpload -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/dashboard_files.go internal/server/dashboard_files_test.go
git commit -m "feat(file-hub): add upload endpoint with atomic write"
```

---

## Task 4: download endpoint

**Files:**
- Modify: `internal/server/dashboard_files.go`
- Modify: `internal/server/dashboard_files_test.go`

- [ ] **Step 1: Write the failing test for download**

Append to `dashboard_files_test.go`:

```go
func TestFilesDownload(t *testing.T) {
	tmp := t.TempDir()
	content := []byte("download me")
	os.WriteFile(filepath.Join(tmp, "dl.txt"), content, 0o644)
	os.MkdirAll(filepath.Join(tmp, "adir"), 0o755)

	h := &FileHandlers{}

	t.Run("download file", func(t *testing.T) {
		p := filepath.Join(tmp, "dl.txt")
		req := httptest.NewRequest("GET", "/api/files/download?path="+p, nil)
		w := httptest.NewRecorder()
		h.handleDownload(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if w.Body.String() != "download me" {
			t.Errorf("unexpected body: %s", w.Body.String())
		}
		cd := w.Header().Get("Content-Disposition")
		if !strings.Contains(cd, "dl.txt") {
			t.Errorf("expected Content-Disposition with filename, got: %s", cd)
		}
	})

	t.Run("download directory", func(t *testing.T) {
		p := filepath.Join(tmp, "adir")
		req := httptest.NewRequest("GET", "/api/files/download?path="+p, nil)
		w := httptest.NewRecorder()
		h.handleDownload(w, req)

		if w.Code != 400 {
			t.Errorf("expected 400 for directory, got %d", w.Code)
		}
	})

	t.Run("download nonexistent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/download?path=/no/file", nil)
		w := httptest.NewRecorder()
		h.handleDownload(w, req)

		if w.Code != 404 {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}
```

Add `"strings"` to the test file import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestFilesDownload -v`
Expected: FAIL — `handleDownload` not defined.

- [ ] **Step 3: Implement download handler**

Add to `dashboard_files.go`:

```go
// handleDownload streams a file to the client.
// GET /api/files/download?path=/abs/path
func (h *FileHandlers) handleDownload(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, `{"error":"missing path parameter"}`, http.StatusBadRequest)
		return
	}
	filePath = filepath.Clean(filePath)

	fi, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, `{"error":"not found","path":"`+filePath+`"}`, http.StatusNotFound)
		return
	}
	if fi.IsDir() {
		http.Error(w, `{"error":"cannot download directory"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(filePath)))
	http.ServeFile(w, r, filePath)
}
```

Add `"fmt"` to the import block if not already present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestFilesDownload -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/dashboard_files.go internal/server/dashboard_files_test.go
git commit -m "feat(file-hub): add download endpoint with streaming"
```

---

## Task 5: mkdir + delete endpoints

**Files:**
- Modify: `internal/server/dashboard_files.go`
- Modify: `internal/server/dashboard_files_test.go`

- [ ] **Step 1: Write the failing tests for mkdir and delete**

Append to `dashboard_files_test.go`:

```go
func TestFilesMkdir(t *testing.T) {
	tmp := t.TempDir()
	h := &FileHandlers{}

	t.Run("create directory", func(t *testing.T) {
		body := `{"path":"` + filepath.Join(tmp, "newdir") + `"}`
		req := httptest.NewRequest("POST", "/api/files/mkdir", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.handleMkdir(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if fi, err := os.Stat(filepath.Join(tmp, "newdir")); err != nil || !fi.IsDir() {
			t.Error("directory was not created")
		}
	})

	t.Run("idempotent mkdir", func(t *testing.T) {
		body := `{"path":"` + filepath.Join(tmp, "newdir") + `"}`
		req := httptest.NewRequest("POST", "/api/files/mkdir", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.handleMkdir(w, req)

		if w.Code != 200 {
			t.Errorf("expected 200 for idempotent mkdir, got %d", w.Code)
		}
	})
}

func TestFilesDelete(t *testing.T) {
	tmp := t.TempDir()
	h := &FileHandlers{}

	t.Run("delete file", func(t *testing.T) {
		f := filepath.Join(tmp, "todelete.txt")
		os.WriteFile(f, []byte("bye"), 0o644)

		req := httptest.NewRequest("DELETE", "/api/files/delete?path="+f, nil)
		w := httptest.NewRecorder()
		h.handleDelete(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Error("file was not deleted")
		}
	})

	t.Run("delete empty dir", func(t *testing.T) {
		d := filepath.Join(tmp, "emptydir")
		os.MkdirAll(d, 0o755)

		req := httptest.NewRequest("DELETE", "/api/files/delete?path="+d, nil)
		w := httptest.NewRecorder()
		h.handleDelete(w, req)

		if w.Code != 200 {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("delete non-empty dir", func(t *testing.T) {
		d := filepath.Join(tmp, "fulldir")
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "child.txt"), []byte("x"), 0o644)

		req := httptest.NewRequest("DELETE", "/api/files/delete?path="+d, nil)
		w := httptest.NewRecorder()
		h.handleDelete(w, req)

		if w.Code != 400 {
			t.Errorf("expected 400 for non-empty dir, got %d", w.Code)
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/files/delete?path=/no/such/file", nil)
		w := httptest.NewRecorder()
		h.handleDelete(w, req)

		if w.Code != 404 {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run "TestFilesMkdir|TestFilesDelete" -v`
Expected: FAIL — `handleMkdir` and `handleDelete` not defined.

- [ ] **Step 3: Implement mkdir and delete handlers**

Add to `dashboard_files.go`:

```go
// handleMkdir creates a directory (with parents).
// POST /api/files/mkdir  {"path": "/abs/path"}
func (h *FileHandlers) handleMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		http.Error(w, `{"error":"path is required"}`, http.StatusBadRequest)
		return
	}
	req.Path = filepath.Clean(req.Path)

	if err := os.MkdirAll(req.Path, 0o755); err != nil {
		http.Error(w, `{"error":"failed to create directory"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"missing path parameter"}`, http.StatusBadRequest)
		return
	}
	filePath = filepath.Clean(filePath)

	fi, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, `{"error":"not found","path":"`+filePath+`"}`, http.StatusNotFound)
		return
	}

	// For directories, only allow removing empty ones (os.Remove fails on non-empty)
	if err := os.Remove(filePath); err != nil {
		if fi.IsDir() {
			http.Error(w, `{"error":"directory not empty, cannot delete"}`, http.StatusBadRequest)
		} else {
			http.Error(w, `{"error":"failed to delete"}`, http.StatusInternalServerError)
		}
		return
	}

	slog.Info("file-hub: deleted", "path", filePath)
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -run "TestFilesMkdir|TestFilesDelete" -v`
Expected: PASS

- [ ] **Step 5: Run all file handler tests together**

Run: `go test ./internal/server/ -run "TestFiles" -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/dashboard_files.go internal/server/dashboard_files_test.go
git commit -m "feat(file-hub): add mkdir and delete endpoints"
```

---

## Task 6: Register file routes in server.go

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Add FileHandlers field to Server struct**

In `server.go`, find the handler fields block (around line 52-61) and add `filesH`:

```go
// Find this block:
healthH     *HealthHandler
sendH       *SendHandler

// Add after sendH:
filesH      *FileHandlers
```

- [ ] **Step 2: Initialize filesH in the Server constructor**

Find where other handler fields are initialized (look for `sessionH:` or `cronH:` initialization). Add:

```go
filesH: &FileHandlers{},
```

- [ ] **Step 3: Register routes in registerDashboard()**

In the `registerDashboard()` method, find the authenticated API routes block (after the existing `s.mux.HandleFunc` calls, before unauthenticated routes). Add:

```go
	// File Hub API
	s.mux.HandleFunc("GET /api/files/list", auth(s.filesH.handleList))
	s.mux.HandleFunc("GET /api/files/stat", auth(s.filesH.handleStat))
	s.mux.HandleFunc("POST /api/files/upload", auth(s.filesH.handleUpload))
	s.mux.HandleFunc("GET /api/files/download", auth(s.filesH.handleDownload))
	s.mux.HandleFunc("POST /api/files/mkdir", auth(s.filesH.handleMkdir))
	s.mux.HandleFunc("DELETE /api/files/delete", auth(s.filesH.handleDelete))
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(file-hub): register file API routes in dashboard"
```

---

## Task 7: /ls command in dispatch

**Files:**
- Modify: `internal/dispatch/commands.go`

- [ ] **Step 1: Add /ls case to dispatchCommand switch**

In `commands.go`, find the `dispatchCommand()` switch block. Add a new case before `default:`:

```go
	case trimmed == "/ls" || strings.HasPrefix(trimmed, "/ls "):
		d.handleLsCommand(ctx, msg, trimmed, log)
		return true
```

- [ ] **Step 2: Implement handleLsCommand**

Add the following function to `commands.go`:

```go
// handleLsCommand lists directory contents directly (no Claude CLI, no token cost).
func (d *Dispatcher) handleLsCommand(ctx context.Context, msg platform.IncomingMessage, trimmed string, log *slog.Logger) {
	p := d.Platforms[msg.Platform]
	if p == nil {
		return
	}

	arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/ls"))

	// Resolve path
	var absPath string
	chatKey := session.ChatKey(msg.Platform, msg.ChatType, msg.ChatID)
	cwd := d.Router.GetWorkspace(chatKey)

	switch {
	case arg == "":
		absPath = cwd
	case filepath.IsAbs(arg):
		absPath = filepath.Clean(arg)
	default:
		absPath = filepath.Join(cwd, arg)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		p.Reply(ctx, platform.OutgoingMessage{ChatID: msg.ChatID, Text: "❌ 路径不存在: " + absPath})
		return
	}
	if !info.IsDir() {
		p.Reply(ctx, platform.OutgoingMessage{ChatID: msg.ChatID, Text: "❌ 不是目录: " + absPath})
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		p.Reply(ctx, platform.OutgoingMessage{ChatID: msg.ChatID, Text: "❌ 权限不足: " + absPath})
		return
	}

	var dirs, files []os.DirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	const maxItems = 50
	var sb strings.Builder
	sb.WriteString("📂 " + absPath + "\n\n")

	count := 0
	for _, de := range dirs {
		if count >= maxItems {
			break
		}
		sub, _ := os.ReadDir(filepath.Join(absPath, de.Name()))
		sb.WriteString(fmt.Sprintf("  📁 %-28s %d items\n", de.Name()+"/", len(sub)))
		count++
	}
	for _, de := range files {
		if count >= maxItems {
			break
		}
		fi, _ := de.Info()
		size := ""
		if fi != nil {
			size = formatSize(fi.Size())
		}
		sb.WriteString(fmt.Sprintf("  📄 %-24s %7s\n", de.Name(), size))
		count++
	}

	total := len(dirs) + len(files)
	if total > maxItems {
		sb.WriteString(fmt.Sprintf("\n... and %d more items\n", total-maxItems))
	}
	sb.WriteString(fmt.Sprintf("\n%d items (%d dirs, %d files)", total, len(dirs), len(files)))

	p.Reply(ctx, platform.OutgoingMessage{ChatID: msg.ChatID, Text: sb.String()})
	log.Info("ls command", "path", absPath, "items", total)
}

// formatSize returns a human-readable file size string.
func formatSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
```

Add required imports to the file's import block: `"os"`, `"path/filepath"` (check if already imported).

- [ ] **Step 3: Update /help text**

In `handleHelpCommand()`, add `/ls` to the help string:

```go
// Find this line:
"  /pwd — 显示当前工作目录\n" +

// Add after it:
"  /ls [路径] — 列出目录内容\n" +
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add internal/dispatch/commands.go
git commit -m "feat(file-hub): add /ls command for directory listing"
```

---

## Task 8: Dashboard — File Hub modal structure + Files nav tab

**Files:**
- Modify: `internal/server/static/dashboard.html`

This task adds the HTML/CSS skeleton and navigation entry point. The interactive JavaScript logic comes in Task 9.

- [ ] **Step 1: Add Files button to sidebar header**

Find the `hdr-btns` div in the sidebar header (contains btn-history, new session, btn-cron buttons). Add a File Hub button:

```html
<!-- Add before the btn-cron button -->
<button class="hdr-btn" id="btn-files" onclick="openFileHub()" title="Files">
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
  </svg>
</button>
```

- [ ] **Step 2: Add File Hub modal CSS**

Find the `</style>` closing tag for the main CSS block. Add before it:

```css
/* File Hub Modal */
.fh-overlay{position:fixed;inset:0;background:rgba(0,0,0,.6);z-index:1000;display:flex;align-items:center;justify-content:center}
.fh-modal{background:#161b22;border:1px solid #30363d;border-radius:12px;width:70vw;height:80vh;display:flex;flex-direction:column;overflow:hidden}
.fh-header{display:flex;align-items:center;gap:8px;padding:10px 14px;border-bottom:1px solid #30363d;background:#0d1117;flex-wrap:wrap}
.fh-breadcrumb{display:flex;align-items:center;gap:4px;flex:1;overflow-x:auto;white-space:nowrap;font-size:13px}
.fh-breadcrumb span.seg{color:#58a6ff;cursor:pointer}
.fh-breadcrumb span.seg:hover{text-decoration:underline}
.fh-breadcrumb span.cur{color:#e6edf3;font-weight:600}
.fh-breadcrumb span.sep{color:#484f58}
.fh-path-input{background:#0d1117;border:1px solid #30363d;color:#c9d1d9;padding:4px 8px;border-radius:6px;font-size:12px;width:200px;font-family:monospace}
.fh-close{color:#484f58;cursor:pointer;font-size:20px;padding:4px 8px;border:none;background:none}
.fh-close:hover{color:#c9d1d9}
.fh-context{padding:6px 14px;background:#161b22;border-bottom:1px solid #21262d;font-size:11px;color:#484f58}
.fh-context .badge{background:#1f6feb;color:#fff;padding:2px 8px;border-radius:10px;font-size:10px;margin-right:6px}
.fh-list{flex:1;overflow-y:auto;padding:4px 0}
.fh-row{display:flex;align-items:center;padding:7px 14px;gap:10px;cursor:pointer;font-size:13px;border-bottom:1px solid #161b22}
.fh-row:hover{background:#1c2333}
.fh-row.selected{background:#1c2333}
.fh-row input[type=checkbox]{accent-color:#58a6ff}
.fh-row .icon{width:20px;text-align:center;flex-shrink:0}
.fh-row .name{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.fh-row .name.dir{color:#58a6ff}
.fh-row .name.file{color:#c9d1d9}
.fh-row .meta{color:#484f58;font-size:11px;flex-shrink:0}
.fh-toolbar{display:flex;align-items:center;gap:8px;padding:10px 14px;border-top:1px solid #30363d;background:#0d1117;flex-wrap:wrap}
.fh-toolbar button{padding:6px 12px;border-radius:6px;font-size:12px;cursor:pointer;border:1px solid #30363d;background:#21262d;color:#c9d1d9}
.fh-toolbar button:hover{background:#30363d}
.fh-toolbar button.primary{background:#238636;color:#fff;border-color:#238636}
.fh-toolbar button.primary:hover{background:#2ea043}
.fh-toolbar .info{flex:1;text-align:right;color:#484f58;font-size:11px}
.fh-upload-zone{border:2px dashed #30363d;border-radius:8px;padding:24px;text-align:center;margin:12px 14px;cursor:pointer}
.fh-upload-zone:hover{border-color:#58a6ff}
.fh-upload-zone.dragging{border-color:#58a6ff;background:#0d1824}
.fh-progress{padding:4px 14px}
.fh-progress-item{background:#161b22;border-radius:6px;padding:8px 12px;margin-bottom:6px}
.fh-progress-bar{background:#21262d;border-radius:3px;height:4px;margin-top:4px;overflow:hidden}
.fh-progress-bar div{background:#238636;height:100%;transition:width .3s}

/* Mobile File Hub */
@media(max-width:700px){
  .fh-overlay{align-items:flex-end}
  .fh-modal{width:100vw;height:95vh;border-radius:12px 12px 0 0;border-bottom:none}
  .fh-modal::before{content:'';display:block;width:36px;height:4px;background:#30363d;border-radius:2px;margin:6px auto 0}
  .fh-path-input{display:none}
  .fh-toolbar{flex-wrap:wrap}
  .fh-toolbar button.primary{width:100%;order:-1}
  .fh-toolbar .info{width:100%;text-align:center;order:99}
}
```

- [ ] **Step 3: Add File Hub modal HTML template**

Add the modal template as a hidden element before the closing `</body>` tag (or as a template that JS clones). Since the dashboard uses dynamic creation, we'll add it in JS (Task 9). For now, add an empty container:

```html
<div id="file-hub-root"></div>
```

- [ ] **Step 4: Verify the dashboard loads without errors**

Run: `go build ./... && go vet ./...`
Expected: No errors. The CSS and HTML are embedded in the static file, so compilation verifies the Go embed works.

- [ ] **Step 5: Commit**

```bash
git add internal/server/static/dashboard.html
git commit -m "feat(file-hub): add Files nav button, modal CSS, and mobile styles"
```

---

## Task 9: Dashboard — File Hub JavaScript logic

**Files:**
- Modify: `internal/server/static/dashboard.html`

This is the main interactive logic. Add the following JavaScript before the closing `</script>` tag.

- [ ] **Step 1: Add File Hub state variables and API helper**

```javascript
// ─── File Hub State ───
let fhOpen = false;
let fhPath = '';        // current browsing path
let fhEntries = [];     // current directory entries
let fhSelected = new Set();
let fhSessionContext = null; // {key, node} if opened from a session

async function fhFetch(endpoint) {
  const headers = {};
  const t = getToken();
  if (t) headers['Authorization'] = 'Bearer ' + t;
  const r = await fetch(endpoint, { headers });
  if (!r.ok) {
    const text = await r.text();
    throw new Error(text);
  }
  return r.json();
}
```

- [ ] **Step 2: Implement openFileHub and closeFileHub**

```javascript
function openFileHub(initialPath, sessionCtx) {
  if (fhOpen) return;
  fhOpen = true;
  fhSelected.clear();
  fhSessionContext = sessionCtx || null;
  fhPath = initialPath || defaultWorkspace || '/';
  renderFileHub();
  fhNavigate(fhPath);
}

function closeFileHub() {
  fhOpen = false;
  const el = document.getElementById('file-hub-root');
  el.innerHTML = '';
}

function renderFileHub() {
  const root = document.getElementById('file-hub-root');
  const ctxHtml = fhSessionContext
    ? `<div class="fh-context"><span class="badge">来自: ${esc(fhSessionContext.key)}</span>选中路径将插入该会话聊天框</div>`
    : '';
  root.innerHTML =
    '<div class="fh-overlay" onclick="if(event.target===this)closeFileHub()">' +
      '<div class="fh-modal">' +
        '<div class="fh-header">' +
          '<span style="font-size:16px">📁</span>' +
          '<div class="fh-breadcrumb" id="fh-breadcrumb"></div>' +
          '<input class="fh-path-input" id="fh-path-input" placeholder="输入路径..." onkeydown="if(event.key===\'Enter\')fhNavigate(this.value)">' +
          '<button class="fh-close" onclick="closeFileHub()">✕</button>' +
        '</div>' +
        ctxHtml +
        '<div class="fh-list" id="fh-list"><div style="padding:20px;text-align:center;color:#484f58">loading...</div></div>' +
        '<div class="fh-toolbar" id="fh-toolbar"></div>' +
      '</div>' +
    '</div>';
}
```

- [ ] **Step 3: Implement directory navigation and rendering**

```javascript
async function fhNavigate(path) {
  try {
    const data = await fhFetch('/api/files/list?path=' + encodeURIComponent(path));
    fhPath = data.path;
    fhEntries = data.entries || [];
    fhSelected.clear();
    fhRenderBreadcrumb();
    fhRenderList();
    fhRenderToolbar();
    const inp = document.getElementById('fh-path-input');
    if (inp) inp.value = fhPath;
  } catch (e) {
    const list = document.getElementById('fh-list');
    if (list) list.innerHTML = '<div style="padding:20px;text-align:center;color:#f85149">❌ ' + esc(e.message) + '</div>';
  }
}

function fhRenderBreadcrumb() {
  const bc = document.getElementById('fh-breadcrumb');
  if (!bc) return;
  const parts = fhPath.split('/').filter(Boolean);
  let html = '<span class="seg" onclick="fhNavigate(\'/\')">/</span>';
  let cumulative = '';
  parts.forEach((p, i) => {
    cumulative += '/' + p;
    const path = cumulative;
    if (i === parts.length - 1) {
      html += '<span class="sep">/</span><span class="cur">' + esc(p) + '</span>';
    } else {
      html += '<span class="sep">/</span><span class="seg" onclick="fhNavigate(\'' + escAttr(path) + '\')">' + esc(p) + '</span>';
    }
  });
  bc.innerHTML = html;
}

function fhRenderList() {
  const list = document.getElementById('fh-list');
  if (!list) return;
  if (fhEntries.length === 0) {
    list.innerHTML = '<div style="padding:20px;text-align:center;color:#484f58">空目录</div>';
    return;
  }
  list.innerHTML = fhEntries.map((e, i) => {
    const isDir = e.type === 'dir';
    const icon = isDir ? '📁' : '📄';
    const cls = isDir ? 'dir' : 'file';
    const size = isDir ? (e.item_count + ' items') : fhFormatSize(e.size);
    const checked = fhSelected.has(i) ? 'checked' : '';
    return '<div class="fh-row' + (fhSelected.has(i) ? ' selected' : '') + '" data-idx="' + i + '">' +
      '<input type="checkbox" ' + checked + ' onclick="fhToggle(' + i + ',event)">' +
      '<span class="icon">' + icon + '</span>' +
      '<span class="name ' + cls + '" onclick="' + (isDir ? "fhNavigate('" + escAttr(fhPath + '/' + e.name) + "')" : 'fhToggle(' + i + ',event)') + '">' + esc(e.name) + (isDir ? '/' : '') + '</span>' +
      '<span class="meta">' + size + '</span>' +
      '<span class="meta">' + fhFormatDate(e.mod_time) + '</span>' +
    '</div>';
  }).join('');
}

function fhToggle(idx, ev) {
  if (ev) ev.stopPropagation();
  if (fhSelected.has(idx)) fhSelected.delete(idx); else fhSelected.add(idx);
  fhRenderList();
  fhRenderToolbar();
}

function fhFormatSize(b) {
  if (b >= 1073741824) return (b / 1073741824).toFixed(1) + 'G';
  if (b >= 1048576) return (b / 1048576).toFixed(1) + 'M';
  if (b >= 1024) return (b / 1024).toFixed(1) + 'K';
  return b + 'B';
}

function fhFormatDate(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
```

- [ ] **Step 4: Implement toolbar rendering (insert path, upload, download, mkdir, delete)**

```javascript
function fhRenderToolbar() {
  const tb = document.getElementById('fh-toolbar');
  if (!tb) return;
  const n = fhSelected.size;
  const insertLabel = fhSessionContext ? '📋 插入路径到聊天' : '📋 复制路径';
  const insertAction = fhSessionContext ? 'fhInsertPath()' : 'fhCopyPath()';
  tb.innerHTML =
    '<button class="primary" onclick="' + insertAction + '" ' + (n === 0 ? 'disabled style="opacity:.5"' : '') + '>' + insertLabel + '</button>' +
    '<button onclick="fhShowUpload()">⬆️ 上传</button>' +
    '<button onclick="fhDownloadSelected()" ' + (n === 0 ? 'disabled style="opacity:.5"' : '') + '>⬇️ 下载</button>' +
    '<button onclick="fhPromptMkdir()">📁 新建</button>' +
    '<button onclick="fhDeleteSelected()" ' + (n === 0 ? 'disabled style="opacity:.5"' : '') + '>🗑️ 删除</button>' +
    '<span class="info">' + (n > 0 ? '已选 ' + n + ' 项 · ' : '') + fhEntries.length + ' items</span>';
}
```

- [ ] **Step 5: Implement path insertion / copy**

```javascript
function fhGetSelectedPaths() {
  return [...fhSelected].map(i => fhPath + '/' + fhEntries[i].name);
}

function fhInsertPath() {
  const paths = fhGetSelectedPaths().join(' ');
  closeFileHub();
  // Insert into the active session's chat input
  const input = document.querySelector('.chat-input');
  if (input) {
    const start = input.selectionStart || input.value.length;
    input.value = input.value.slice(0, start) + paths + input.value.slice(start);
    input.focus();
    input.selectionStart = input.selectionEnd = start + paths.length;
  }
}

function fhCopyPath() {
  const paths = fhGetSelectedPaths().join('\n');
  navigator.clipboard.writeText(paths).catch(() => {});
  closeFileHub();
}
```

- [ ] **Step 6: Implement upload, download, mkdir, delete actions**

```javascript
function fhShowUpload() {
  const list = document.getElementById('fh-list');
  list.innerHTML =
    '<div class="fh-upload-zone" id="fh-dropzone" onclick="document.getElementById(\'fh-file-input\').click()" ondrop="fhDrop(event)" ondragover="event.preventDefault();this.classList.add(\'dragging\')" ondragleave="this.classList.remove(\'dragging\')">' +
      '<div style="font-size:24px;margin-bottom:8px">⬆️</div>' +
      '<div>拖拽文件到此处，或点击选择文件</div>' +
      '<div style="color:#484f58;font-size:11px;margin-top:4px">最大 100MB · 支持多文件</div>' +
    '</div>' +
    '<div class="fh-progress" id="fh-progress"></div>' +
    '<input type="file" id="fh-file-input" multiple style="display:none" onchange="fhUploadFiles(this.files)">';
}

function fhDrop(ev) {
  ev.preventDefault();
  ev.currentTarget.classList.remove('dragging');
  if (ev.dataTransfer.files.length) fhUploadFiles(ev.dataTransfer.files);
}

async function fhUploadFiles(fileList) {
  const prog = document.getElementById('fh-progress');
  for (const file of fileList) {
    const id = 'up-' + Math.random().toString(36).slice(2);
    prog.insertAdjacentHTML('beforeend',
      '<div class="fh-progress-item" id="' + id + '">' +
        '<div style="display:flex;justify-content:space-between"><span>📄 ' + esc(file.name) + '</span><span id="' + id + '-pct">0%</span></div>' +
        '<div class="fh-progress-bar"><div id="' + id + '-bar" style="width:0%"></div></div>' +
      '</div>');

    const fd = new FormData();
    fd.append('dest', fhPath);
    fd.append('file', file);

    try {
      const headers = {};
      const t = getToken();
      if (t) headers['Authorization'] = 'Bearer ' + t;
      const r = await fetch('/api/files/upload', { method: 'POST', headers, body: fd });
      if (!r.ok) throw new Error(await r.text());
      document.getElementById(id + '-pct').textContent = '✓';
      document.getElementById(id + '-pct').style.color = '#3fb950';
      document.getElementById(id + '-bar').style.width = '100%';
    } catch (e) {
      document.getElementById(id + '-pct').textContent = '✕';
      document.getElementById(id + '-pct').style.color = '#f85149';
    }
  }
  // Refresh file list after uploads
  setTimeout(() => fhNavigate(fhPath), 500);
}

function fhDownloadSelected() {
  const paths = fhGetSelectedPaths();
  const t = getToken();
  paths.forEach(p => {
    const url = '/api/files/download?path=' + encodeURIComponent(p) + (t ? '&token=' + encodeURIComponent(t) : '');
    const a = document.createElement('a');
    a.href = url;
    a.download = '';
    document.body.appendChild(a);
    a.click();
    a.remove();
  });
}

function fhPromptMkdir() {
  const name = prompt('新文件夹名称:');
  if (!name) return;
  const headers = { 'Content-Type': 'application/json' };
  const t = getToken();
  if (t) headers['Authorization'] = 'Bearer ' + t;
  fetch('/api/files/mkdir', {
    method: 'POST', headers,
    body: JSON.stringify({ path: fhPath + '/' + name })
  }).then(() => fhNavigate(fhPath));
}

async function fhDeleteSelected() {
  const paths = fhGetSelectedPaths();
  if (!confirm('确认删除 ' + paths.length + ' 项？')) return;
  const headers = {};
  const t = getToken();
  if (t) headers['Authorization'] = 'Bearer ' + t;
  for (const p of paths) {
    await fetch('/api/files/delete?path=' + encodeURIComponent(p), { method: 'DELETE', headers });
  }
  fhNavigate(fhPath);
}
```

- [ ] **Step 7: Add session-context entry points**

Add a 📁 button to the session detail header. Find the session detail rendering function (likely `showSession()` or `renderSessionDetail()`). Add a file hub button near the session title:

```javascript
// In the session header rendering, add:
// <button class="hdr-btn" onclick="openFileHub(sess.workspace, {key: sess.key, node: sess.node})" title="Browse files">📁</button>
```

The exact integration point depends on the session header template — look for where `sess.workspace` or `sess.key` is rendered and add the button there.

- [ ] **Step 8: Verify compilation and basic functionality**

Run: `go build ./... && go vet ./...`
Expected: No errors.

- [ ] **Step 9: Commit**

```bash
git add internal/server/static/dashboard.html
git commit -m "feat(file-hub): add File Hub modal with browse, upload, download, mkdir, delete"
```

---

## Task 10: Dashboard — /ls enhanced rendering

**Files:**
- Modify: `internal/server/static/dashboard.html`

When a `/ls` response appears in the Dashboard event stream, render it with clickable directories and action buttons instead of plain text.

- [ ] **Step 1: Add /ls result detection and enhanced rendering**

Find the function that renders event text in the chat area (likely `renderEventText()` or similar markdown rendering). Add a detection for `/ls` output format:

```javascript
function fhEnhanceLsOutput(text) {
  // Detect /ls output by the 📂 header pattern
  if (!text.startsWith('📂 ')) return null;
  const lines = text.split('\n');
  const pathLine = lines[0].replace('📂 ', '').trim();

  let html = '<div style="font-family:monospace;font-size:12px">';
  html += '<div style="color:#58a6ff;font-weight:600;margin-bottom:6px">📂 ' + esc(pathLine) + '</div>';

  for (let i = 1; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line) continue;
    if (line.startsWith('📁 ')) {
      const name = line.replace('📁 ', '').split(/\s+/)[0];
      const rest = line.slice(line.indexOf(name) + name.length);
      html += '<div><span style="cursor:pointer;color:#58a6ff;text-decoration:underline" onclick="fhLsClick(\'' + escAttr(pathLine + '/' + name.replace(/\/$/, '')) + '\')">' + esc('📁 ' + name) + '</span><span style="color:#484f58">' + esc(rest) + '</span></div>';
    } else if (line.startsWith('📄 ')) {
      html += '<div style="color:#c9d1d9">' + esc(line) + '</div>';
    } else if (line.match(/^\d+ items/)) {
      html += '<div style="color:#484f58;margin-top:6px;padding-top:6px;border-top:1px solid #21262d">' + esc(line) + '</div>';
    }
  }

  html += '<div style="margin-top:6px;display:flex;gap:6px">' +
    '<span style="background:#1f6feb;color:#fff;padding:3px 8px;border-radius:4px;font-size:10px;cursor:pointer" onclick="openFileHub(\'' + escAttr(pathLine) + '\')">📁 在 File Hub 打开</span>' +
    '</div></div>';

  return html;
}

function fhLsClick(path) {
  // Send /ls for the clicked directory
  const input = document.querySelector('.chat-input');
  if (input) {
    input.value = '/ls ' + path;
    // Trigger send
    const sendBtn = document.querySelector('.btn-send');
    if (sendBtn) sendBtn.click();
  }
}
```

- [ ] **Step 2: Wire the enhanced rendering into the event display**

Find where system/command responses are rendered in the event stream. Add a check:

```javascript
// In the text rendering path, before default text output:
const enhanced = fhEnhanceLsOutput(text);
if (enhanced) {
  // Render as HTML instead of plain text
  el.innerHTML = enhanced;
} else {
  // Existing plain text / markdown rendering
}
```

The exact integration point depends on how the dashboard renders different event types — look for where `type === 'result'` or text output events are handled.

- [ ] **Step 3: Verify compilation**

Run: `go build ./... && go vet ./...`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/server/static/dashboard.html
git commit -m "feat(file-hub): enhance /ls output with clickable dirs and File Hub link"
```

---

## Task 11: Download endpoint auth for direct links

**Files:**
- Modify: `internal/server/dashboard_files.go`

The download endpoint needs to support `?token=` query parameter for direct `<a href>` downloads from the browser, since `fetch()` downloads can't trigger the browser's save dialog with auth headers.

- [ ] **Step 1: Add token query parameter support to download**

In `handleDownload()`, add token extraction before the existing logic:

```go
// At the top of handleDownload, after parsing filePath:
// Support ?token= for direct browser downloads (no Authorization header)
// The auth middleware on the route handles normal Bearer auth.
// This is a fallback for <a href="...?token=..."> links.
```

Actually, since all `/api/files/*` routes go through `auth()` middleware, the token-in-query approach needs to be handled at the auth layer. For V1, the browser-side `fhDownloadSelected()` creates a temporary `<a>` link. The auth middleware already checks cookies, which the browser sends automatically on same-origin requests. No changes needed — cookie-based auth covers this case.

- [ ] **Step 2: Verify download works with cookie auth**

The existing `auth.requireAuth` middleware checks for Bearer token OR valid cookie. Since the dashboard sets a cookie on login, same-origin `<a href="/api/files/download?path=...">` requests automatically include the cookie. No code change needed.

- [ ] **Step 3: Commit (skip if no changes)**

No changes needed — cookie auth already covers this case.

---

## Task 12: Integration test — full flow verification

**Files:**
- All files from previous tasks

- [ ] **Step 1: Run all unit tests**

Run: `go test ./internal/server/ -run "TestFiles" -v`
Expected: All PASS (list, stat, upload, download, mkdir, delete).

- [ ] **Step 2: Run full project tests**

Run: `go test ./... -v`
Expected: All PASS, no regressions.

- [ ] **Step 3: Run linter**

Run: `go vet ./...`
Expected: No issues.

- [ ] **Step 4: Build binary**

Run: `CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/`
Expected: Clean build.

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat(file-hub): File Hub V1 complete — REST API, /ls command, Dashboard UI"
```
