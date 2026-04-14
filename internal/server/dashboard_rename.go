package server

import (
	"encoding/json"
	"net/http"
)

// handleRename sets or clears the user-defined name for a session.
// PATCH /api/sessions/rename  {key, name}
func (h *Hub) handleRename(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	var req struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	if !h.router.RenameSession(req.Key, req.Name) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	h.BroadcastSessionsUpdate()
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]bool{"ok": true})
}

// handlePin sets or clears the pin-to-top state for a session.
// PATCH /api/sessions/pin  {key, pinned}
func (h *Hub) handlePin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	var req struct {
		Key    string `json:"key"`
		Pinned bool   `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	if !h.router.PinSession(req.Key, req.Pinned) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	h.BroadcastSessionsUpdate()
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]bool{"ok": true})
}
