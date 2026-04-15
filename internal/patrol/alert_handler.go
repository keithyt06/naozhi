package patrol

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// AlertConfig holds configuration for the alert webhook subsystem.
type AlertConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	DefaultPatrol string `yaml:"default_patrol" json:"default_patrol"` // patrol to trigger on alert
}

// RegisterAlertRoutes registers the POST /api/webhooks/alert endpoint.
// This endpoint is unauthenticated (external monitoring systems push here).
func (h *APIHandler) RegisterAlertRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/webhooks/alert", h.handleAlertWebhook)
}

// POST /api/webhooks/alert -- receive an external alert, normalise it,
// create a notification, and optionally trigger a patrol with alert context.
func (h *APIHandler) handleAlertWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20)) // 1 MB
	if err != nil {
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}

	// Detect source
	source := DetectAlertSource(body, r.Header)
	if source == "" {
		http.Error(w, `{"error":"cannot detect alert source, set X-Alert-Source header or include source field; supported: `+
			strings.Join(supportedSources, ", ")+`"}`, http.StatusBadRequest)
		return
	}

	// Get normalizer
	normalizer, err := GetNormalizer(source)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// Normalize
	alert, err := normalizer.Normalize(body, r.Header)
	if err != nil {
		slog.Warn("alert normalize failed", "source", source, "err", err)
		http.Error(w, `{"error":"normalize failed: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	slog.Info("alert received", "source", alert.Source, "title", alert.Title, "severity", alert.Severity)

	// Try to trigger a matching patrol with alert context injected into prompt.
	// Look for an "alert-investigator" patrol first, then fall back to
	// any patrol whose name contains "alert" or "infra".
	patrolName := h.findAlertPatrol(alert)
	triggered := false
	if patrolName != "" {
		alertJSON, _ := json.Marshal(alert)
		go func() {
			if err := h.manager.Execute(h.manager.stopCtx, patrolName, alertJSON); err != nil {
				slog.Warn("alert patrol trigger failed", "patrol", patrolName, "err", err)
			}
		}()
		triggered = true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{
		"status":           "accepted",
		"alert_id":         alert.ID,
		"source":           alert.Source,
		"severity":         alert.Severity,
		"patrol_triggered": triggered,
		"patrol_name":      patrolName,
	})
}

// findAlertPatrol looks for a patrol suitable for investigating this alert.
// Priority: "alert-investigator" > name contains alert source > name contains "alert" > name contains "infra".
func (h *APIHandler) findAlertPatrol(alert *Alert) string {
	patrols := h.manager.ListPatrols()

	// Priority 1: exact "alert-investigator"
	if p, ok := patrols["alert-investigator"]; ok && p.CanRun() {
		return "alert-investigator"
	}

	// Priority 2: name contains the alert source (e.g. "cloudwatch-patrol")
	for name, p := range patrols {
		if p.CanRun() && strings.Contains(name, alert.Source) {
			return name
		}
	}

	// Priority 3: name contains "alert"
	for name, p := range patrols {
		if p.CanRun() && strings.Contains(name, "alert") {
			return name
		}
	}

	// Priority 4: name contains "infra"
	for name, p := range patrols {
		if p.CanRun() && strings.Contains(name, "infra") {
			return name
		}
	}

	return ""
}
