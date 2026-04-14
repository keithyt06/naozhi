package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── TestFilesList ──────────────────────────────────────────────────────────

func TestFilesList(t *testing.T) {
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

// ─── TestFilesStat ──────────────────────────────────────────────────────────

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

// ─── TestFilesUpload ────────────────────────────────────────────────────────

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

// ─── TestFilesDownload ──────────────────────────────────────────────────────

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

// ─── TestFilesMkdir ─────────────────────────────────────────────────────────

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

// ─── TestFilesDelete ────────────────────────────────────────────────────────

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

// ─── TestFormatSize ─────────────────────────────────────────────────────────

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0B"},
		{100, "100B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1048576, "1.0M"},
		{1073741824, "1.0G"},
		{1099511627776, "1024.0G"},
	}
	for _, tt := range tests {
		got := formatSize(tt.input)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
