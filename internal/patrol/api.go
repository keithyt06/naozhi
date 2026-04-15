package patrol

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIHandler exposes patrol REST API endpoints.
type APIHandler struct {
	manager *Manager
}

// NewAPIHandler creates an API handler backed by the given manager.
func NewAPIHandler(manager *Manager) *APIHandler {
	return &APIHandler{manager: manager}
}

// RegisterRoutes registers all patrol API routes on the given mux.
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux, auth func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/patrols", auth(h.handleList))
	mux.HandleFunc("GET /api/patrols/{name}", auth(h.handleGet))
	mux.HandleFunc("PUT /api/patrols/{name}/state", auth(h.handleSetState))
	mux.HandleFunc("POST /api/patrols/{name}/trigger", auth(h.handleTrigger))
	mux.HandleFunc("GET /api/patrols/{name}/logs", auth(h.handleLogs))
	mux.HandleFunc("GET /api/patrols/{name}/logs/{id}", auth(h.handleLogDetail))
}

// GET /api/patrols -- list all patrols with stats.
func (h *APIHandler) handleList(w http.ResponseWriter, r *http.Request) {
	patrols := h.manager.ListPatrols()

	var active, paused, disabled, running int
	type patrolView struct {
		Name             string   `json:"name"`
		Agent            string   `json:"agent"`
		Model            string   `json:"model,omitempty"`
		Schedule         string   `json:"schedule,omitempty"`
		Trigger          string   `json:"trigger,omitempty"`
		Prompt           string   `json:"prompt"`
		State            State    `json:"state"`
		NotifyTargets    []string `json:"notify,omitempty"`
		ApprovalRequired bool     `json:"approval_required,omitempty"`
		TotalRuns        int64    `json:"total_runs"`
		TotalErrors      int64    `json:"total_errors"`
		TotalCost        float64  `json:"total_cost"`
		LastRun          *RunLog  `json:"last_run,omitempty"`
		CreatedAt        int64    `json:"created_at"`
	}

	views := make([]patrolView, 0, len(patrols))
	for _, p := range patrols {
		switch p.State {
		case StateActive:
			active++
		case StatePaused:
			paused++
		case StateDisabled:
			disabled++
		case StateRunning:
			running++
		}
		views = append(views, patrolView{
			Name:             p.Name,
			Agent:            p.Agent,
			Model:            p.Model,
			Schedule:         p.Schedule,
			Trigger:          p.Trigger,
			Prompt:           p.Prompt,
			State:            p.State,
			NotifyTargets:    p.NotifyTargets,
			ApprovalRequired: p.ApprovalRequired,
			TotalRuns:        p.TotalRuns,
			TotalErrors:      p.TotalErrors,
			TotalCost:        p.TotalCost,
			LastRun:          p.LastRun,
			CreatedAt:        p.CreatedAt.UnixMilli(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		"patrols": views,
		"stats": map[string]int{
			"active":   active,
			"paused":   paused,
			"disabled": disabled,
			"running":  running,
		},
	})
}

// GET /api/patrols/{name} -- single patrol detail.
func (h *APIHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	p, ok := h.manager.GetPatrol(name)
	if !ok {
		http.Error(w, `{"error":"patrol not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, p)
}

// PUT /api/patrols/{name}/state -- change patrol state.
func (h *APIHandler) handleSetState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		State string `json:"state"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	newState := State(req.State)
	switch newState {
	case StateActive, StatePaused, StateDisabled:
		// valid
	default:
		http.Error(w, `{"error":"invalid state, must be active/paused/disabled"}`, http.StatusBadRequest)
		return
	}

	if err := h.manager.SetState(name, newState); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		}
		return
	}

	slog.Info("patrol state changed via API", "name", name, "state", newState)
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "ok"})
}

// POST /api/patrols/{name}/trigger -- manual trigger.
func (h *APIHandler) handleTrigger(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	p, ok := h.manager.GetPatrol(name)
	if !ok {
		http.Error(w, `{"error":"patrol not found"}`, http.StatusNotFound)
		return
	}
	if !p.CanRun() {
		http.Error(w, `{"error":"patrol is not in active state"}`, http.StatusConflict)
		return
	}

	// Parse optional context
	var req struct {
		Context string `json:"context,omitempty"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	_ = json.NewDecoder(r.Body).Decode(&req) // optional body

	runID := generateID()

	// Async execution
	go func() {
		var payload json.RawMessage
		if req.Context != "" {
			payload, _ = json.Marshal(map[string]string{"context": req.Context})
		}
		if err := h.manager.Execute(h.manager.stopCtx, name, payload); err != nil {
			slog.Warn("manual patrol trigger failed", "name", name, "err", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"status": "accepted", "run_id": runID})
}

// GET /api/patrols/{name}/logs -- execution log list.
func (h *APIHandler) handleLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	logs, err := h.manager.ReadLogs(name, limit)
	if err != nil {
		http.Error(w, `{"error":"read logs: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = make([]*RunLog, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		"logs":  logs,
		"total": len(logs),
	})
}

// GET /api/patrols/{name}/logs/{id} -- single log entry detail.
func (h *APIHandler) handleLogDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	runID := r.PathValue("id")
	if name == "" || runID == "" {
		http.Error(w, `{"error":"name and id are required"}`, http.StatusBadRequest)
		return
	}

	rl, err := h.manager.ReadLogByID(name, runID)
	if err != nil {
		http.Error(w, `{"error":"read log: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if rl == nil {
		http.Error(w, `{"error":"log entry not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, rl)
}

// writeJSON is a local helper matching the server package pattern.
func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("patrol api write json", "err", err)
	}
}

// writeJSONError writes a JSON error response with proper encoding to prevent
// JSON injection (I6). The error message is safely escaped via json.Marshal.
func writeJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{"error": msg}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Debug("patrol api write json error", "err", err)
	}
}

// nextRunTime returns the next scheduled run time for display.
func nextRunTime(schedule string) *time.Time {
	if schedule == "" {
		return nil
	}
	sched, err := cronParser.Parse(schedule)
	if err != nil {
		return nil
	}
	t := sched.Next(time.Now())
	return &t
}
