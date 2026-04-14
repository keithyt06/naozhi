package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─── handleRename ──────────────────────────────────────────────────────────

func TestHandleRename_Success(t *testing.T) {
	srv := newTestServer(&mockPlatform{})
	key := "dashboard:direct:test:general"
	srv.router.RegisterForResume(key, "sess-001", "/tmp", "")

	body := `{"key":"` + key + `","name":"my session"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/sessions/rename", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.hub.handleRename(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok=true")
	}

	// Verify the name was set
	s := srv.router.GetSession(key)
	if s == nil {
		t.Fatal("session not found")
	}
	if got := s.GetName(); got != "my session" {
		t.Errorf("name = %q, want %q", got, "my session")
	}
}

func TestHandleRename_ClearName(t *testing.T) {
	srv := newTestServer(&mockPlatform{})
	key := "dashboard:direct:test:general"
	srv.router.RegisterForResume(key, "sess-002", "/tmp", "")

	// Set a name first
	srv.router.RenameSession(key, "old name")

	// Clear it
	body := `{"key":"` + key + `","name":""}`
	req := httptest.NewRequest(http.MethodPatch, "/api/sessions/rename", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.hub.handleRename(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	s := srv.router.GetSession(key)
	if got := s.GetName(); got != "" {
		t.Errorf("name = %q, want empty", got)
	}
}

func TestHandleRename_NotFound(t *testing.T) {
	srv := newTestServer(&mockPlatform{})

	body := `{"key":"no:such:session:general","name":"test"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/sessions/rename", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.hub.handleRename(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleRename_MissingKey(t *testing.T) {
	srv := newTestServer(&mockPlatform{})

	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/sessions/rename", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.hub.handleRename(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// ─── handlePin ──────────────────────────────────────────────────────────────

func TestHandlePin_Success(t *testing.T) {
	srv := newTestServer(&mockPlatform{})
	key := "dashboard:direct:test:general"
	srv.router.RegisterForResume(key, "sess-003", "/tmp", "")

	body := `{"key":"` + key + `","pinned":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/sessions/pin", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.hub.handlePin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	s := srv.router.GetSession(key)
	if s == nil {
		t.Fatal("session not found")
	}
	if !s.IsPinned() {
		t.Error("expected session to be pinned")
	}
}

func TestHandlePin_NotFound(t *testing.T) {
	srv := newTestServer(&mockPlatform{})

	body := `{"key":"no:such:session:general","pinned":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/sessions/pin", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.hub.handlePin(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
