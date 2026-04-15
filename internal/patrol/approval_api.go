package patrol

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ApprovalHandlers groups the approval management API endpoints.
type ApprovalHandlers struct {
	Store *ApprovalStore
	// Hub is called after approve/reject to broadcast updates via WebSocket.
	// Nil-safe: callers may leave this unset during testing.
	Hub interface {
		BroadcastApprovalUpdate(a Approval)
	}
}

// RegisterApprovalRoutes registers all approval REST API routes on the given mux.
// The auth wrapper is applied by the caller (server.go).
func (h *ApprovalHandlers) RegisterApprovalRoutes(mux *http.ServeMux, auth func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/approvals", auth(h.handleList))
	mux.HandleFunc("GET /api/approvals/{id}", auth(h.handleGet))
	mux.HandleFunc("POST /api/approvals/{id}/approve", auth(h.handleApprove))
	mux.HandleFunc("POST /api/approvals/{id}/reject", auth(h.handleReject))
}

// GET /api/approvals — list approvals with optional ?status=pending filter.
func (h *ApprovalHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	var approvals []Approval
	if strings.EqualFold(statusFilter, "pending") {
		approvals = h.Store.ListPending()
	} else {
		approvals = h.Store.ListAll()
	}
	if approvals == nil {
		approvals = []Approval{}
	}

	// Compute stats
	var pending, approved, rejected int
	for i := range approvals {
		switch approvals[i].Status {
		case ApprovalPending:
			pending++
		case ApprovalApproved:
			approved++
		case ApprovalRejected:
			rejected++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	writeApprovalJSON(w, map[string]any{
		"approvals": approvals,
		"stats": map[string]int{
			"pending":  pending,
			"approved": approved,
			"rejected": rejected,
		},
	})
}

// GET /api/approvals/{id} — single approval detail.
func (h *ApprovalHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	a, err := h.Store.Get(id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeApprovalJSON(w, a)
}

// POST /api/approvals/{id}/approve — approve a pending request.
func (h *ApprovalHandlers) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		ApprovedBy string `json:"approved_by"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.ApprovedBy == "" {
		req.ApprovedBy = "dashboard"
	}

	if err := h.Store.Approve(id, req.ApprovedBy); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		}
		return
	}

	// Broadcast update via WebSocket
	if h.Hub != nil {
		if a, err := h.Store.Get(id); err == nil {
			h.Hub.BroadcastApprovalUpdate(a)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	writeApprovalJSON(w, map[string]string{"status": "approved"})
}

// POST /api/approvals/{id}/reject — reject a pending request.
func (h *ApprovalHandlers) handleReject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		RejectedBy string `json:"rejected_by"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.RejectedBy == "" {
		req.RejectedBy = "dashboard"
	}

	if err := h.Store.Reject(id, req.RejectedBy); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		}
		return
	}

	// Broadcast update via WebSocket
	if h.Hub != nil {
		if a, err := h.Store.Get(id); err == nil {
			h.Hub.BroadcastApprovalUpdate(a)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	writeApprovalJSON(w, map[string]string{"status": "rejected"})
}

// writeApprovalJSON is a local helper to encode JSON responses.
func writeApprovalJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}
