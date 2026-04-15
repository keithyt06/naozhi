package patrol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// WebhookPayload is the expected format for external event webhooks.
type WebhookPayload struct {
	Event   string          `json:"event"`
	Source  string          `json:"source"`
	Payload json.RawMessage `json:"payload"`
}

// RegisterWebhookRoutes registers the webhook endpoint on the given mux.
func (h *APIHandler) RegisterWebhookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/webhooks/{name}", h.handleWebhook)
}

// POST /api/webhooks/{patrol-name} -- receive external event and trigger patrol.
func (h *APIHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"patrol name is required"}`, http.StatusBadRequest)
		return
	}

	p, ok := h.manager.GetPatrol(name)
	if !ok {
		http.Error(w, `{"error":"patrol not found"}`, http.StatusNotFound)
		return
	}
	if !p.CanRun() {
		http.Error(w, `{"error":"patrol is not active"}`, http.StatusConflict)
		return
	}

	// Read request body
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20)) // 1 MB
	if err != nil {
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}

	// Webhook authentication: verify secret if configured on the patrol.
	if p.WebhookSecret != "" {
		// GitHub webhooks: verify X-Hub-Signature-256 HMAC
		if ghSig := r.Header.Get("X-Hub-Signature-256"); ghSig != "" {
			if !verifyGitHubSignature(p.WebhookSecret, body, ghSig) {
				http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
				return
			}
		} else {
			// Generic webhooks: check X-Webhook-Secret header
			if r.Header.Get("X-Webhook-Secret") != p.WebhookSecret {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
	} else {
		slog.Warn("webhook secret not configured, accepting unauthenticated request", "patrol", name)
	}

	// Detect GitHub webhook via X-GitHub-Event header
	var wp WebhookPayload
	if ghEvent := r.Header.Get("X-GitHub-Event"); ghEvent != "" {
		wp = WebhookPayload{
			Source:  "github",
			Event:   mapGitHubEvent(ghEvent, body),
			Payload: body,
		}
	} else {
		// Parse generic webhook payload
		if err := json.Unmarshal(body, &wp); err != nil {
			// Treat entire body as opaque payload
			wp = WebhookPayload{
				Source:  "custom",
				Event:   "custom",
				Payload: body,
			}
		}
	}

	// Match trigger if configured
	if p.Trigger != "" {
		triggerKey := wp.Source + ":" + wp.Event
		if !matchTrigger(p.Trigger, triggerKey) {
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, map[string]string{
				"status": "skipped",
				"reason": "trigger mismatch",
			})
			return
		}
	}

	runID := generateID()

	// Async execution
	go func() {
		if err := h.manager.Execute(h.manager.stopCtx, name, wp.Payload); err != nil {
			slog.Warn("webhook patrol trigger failed", "name", name, "err", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{
		"status": "accepted",
		"patrol": name,
		"run_id": runID,
	})
}

// matchTrigger checks if triggerKey matches the patrol's trigger pattern.
// Supports wildcard: "custom:*" matches any event from "custom" source.
func matchTrigger(pattern, triggerKey string) bool {
	if pattern == triggerKey {
		return true
	}
	// Wildcard match: "github:*" matches "github:pull_request"
	if idx := strings.IndexByte(pattern, '*'); idx >= 0 {
		prefix := pattern[:idx]
		return strings.HasPrefix(triggerKey, prefix)
	}
	return false
}

// verifyGitHubSignature validates the X-Hub-Signature-256 header using HMAC-SHA256.
// The header value is expected in the format "sha256=<hex-digest>".
func verifyGitHubSignature(secret string, body []byte, signature string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return false
	}
	sigBytes, err := hex.DecodeString(signature[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sigBytes, mac.Sum(nil))
}

// mapGitHubEvent maps X-GitHub-Event header + payload action to a trigger event string.
func mapGitHubEvent(header string, body []byte) string {
	// Try to extract action from payload
	var payload struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(body, &payload)

	switch header {
	case "pull_request":
		if payload.Action != "" {
			return "pr_" + payload.Action // e.g., pr_opened, pr_closed
		}
		return "pull_request"
	case "issues":
		if payload.Action != "" {
			return "issue_" + payload.Action
		}
		return "issues"
	case "push":
		return "push"
	default:
		if payload.Action != "" {
			return header + "_" + payload.Action
		}
		return header
	}
}
