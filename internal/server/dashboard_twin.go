package server

import (
	"encoding/json"
	"net/http"

	"github.com/naozhi/naozhi/internal/knowledge"
)

// TwinHandlers provides REST API endpoints for the CTO Digital Twin.
type TwinHandlers struct {
	twin     *knowledge.TwinManager
	delegate *knowledge.DelegateHandler
}

// NewTwinHandlers creates TwinHandlers.
func NewTwinHandlers(twin *knowledge.TwinManager) *TwinHandlers {
	return &TwinHandlers{
		twin:     twin,
		delegate: knowledge.NewDelegateHandler(twin),
	}
}

// handleTwinConfigGet returns the current Twin configuration.
// GET /api/twin/config
func (th *TwinHandlers) handleTwinConfigGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, th.twin.Config())
}

// handleTwinConfigPut updates the Twin configuration.
// PUT /api/twin/config
func (th *TwinHandlers) handleTwinConfigPut(w http.ResponseWriter, r *http.Request) {
	var cfg knowledge.TwinConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if err := th.twin.UpdateConfig(cfg); err != nil {
		http.Error(w, `{"error":"failed to save config"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, cfg)
}

// handleTwinTest tests the Twin with a sample query.
// POST /api/twin/test  body: {"query": "..."}
func (th *TwinHandlers) handleTwinTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		http.Error(w, `{"error":"query is required"}`, http.StatusBadRequest)
		return
	}

	// Build the twin prompt.
	prompt := th.twin.BuildTwinPrompt(req.Query, nil)

	// Confidence scoring is handled inside delegate.HandleQuestion.
	_ = th.twin.Config() // used indirectly via twin manager

	// Score confidence using a delegate request simulation.
	result, err := th.delegate.HandleQuestion(knowledge.DelegateRequest{
		ID:       "test",
		Question: req.Query,
		Source:   "dashboard-test",
		Platform: "dashboard",
		Asker:    "test-user",
	})

	resp := map[string]any{
		"prompt_length": len(prompt),
		"query":         req.Query,
	}

	if err != nil {
		resp["error"] = err.Error()
		resp["note"] = "Twin may be disabled. Enable it in Twin config."
	} else {
		resp["action"] = result.Action
		resp["draft"] = result.Draft
		resp["confidence"] = result.Confidence
		resp["tag"] = result.Tag
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, resp)
}

// handleTwinQueue returns the review queue.
// GET /api/twin/queue
func (th *TwinHandlers) handleTwinQueue(w http.ResponseWriter, r *http.Request) {
	queue := th.delegate.ReviewQueue()
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		"items": queue,
		"count": len(queue),
	})
}

// handleTwinDismiss removes an item from the review queue.
// POST /api/twin/queue/dismiss  body: {"id": "..."}
func (th *TwinHandlers) handleTwinDismiss(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}
	if th.delegate.DismissFromQueue(req.ID) {
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]string{"status": "dismissed"})
	} else {
		http.Error(w, `{"error":"item not found"}`, http.StatusNotFound)
	}
}
