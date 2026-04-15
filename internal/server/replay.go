package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/naozhi/naozhi/internal/cli"
	"github.com/naozhi/naozhi/internal/session"
)

// ReplayEvent is a single event in a replay timeline.
type ReplayEvent struct {
	Type      string `json:"type"`                 // message, tool_use, tool_result, thinking, cost, system
	Content   string `json:"content"`              // text content or summary
	ToolName  string `json:"tool_name,omitempty"`   // tool name for tool_use events
	Timestamp int64  `json:"timestamp"`            // unix ms
	DeltaMs   int64  `json:"delta_ms"`             // ms from session start
}

// ReplayTimeline is the full timeline of a session for replay.
type ReplayTimeline struct {
	SessionKey    string        `json:"session_key"`
	Events        []ReplayEvent `json:"events"`
	TotalDuration int64         `json:"total_duration_ms"` // ms
	CreatedAt     string        `json:"created_at"`
	Stats         ReplayStats   `json:"stats"`
}

// ReplayStats holds summary statistics for a replay timeline.
type ReplayStats struct {
	EventCount    int     `json:"event_count"`
	ToolCallCount int     `json:"tool_call_count"`
	TotalCost     float64 `json:"total_cost"`
}

// BuildTimeline reconstructs a replay timeline from event log entries.
func BuildTimeline(sessionKey string, events []cli.EventEntry) *ReplayTimeline {
	if len(events) == 0 {
		return &ReplayTimeline{
			SessionKey: sessionKey,
			Events:     []ReplayEvent{},
			CreatedAt:  time.Now().Format(time.RFC3339),
		}
	}

	// Sort events by time to ensure chronological order.
	sorted := make([]cli.EventEntry, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Time < sorted[j].Time
	})

	startTime := sorted[0].Time
	var replayEvents []ReplayEvent
	var stats ReplayStats
	var lastCost float64

	for _, e := range sorted {
		re := ReplayEvent{
			Type:      e.Type,
			Timestamp: e.Time,
			DeltaMs:   e.Time - startTime,
		}

		switch e.Type {
		case "tool_use":
			re.ToolName = e.Tool
			re.Content = e.Summary
			stats.ToolCallCount++
		case "text", "thinking":
			re.Content = e.Detail
			if re.Content == "" {
				re.Content = e.Summary
			}
		case "result":
			re.Content = e.Summary
			if e.Cost > 0 {
				lastCost = e.Cost
			}
		default:
			re.Content = e.Summary
		}

		replayEvents = append(replayEvents, re)
	}

	stats.EventCount = len(replayEvents)
	stats.TotalCost = lastCost

	lastTime := sorted[len(sorted)-1].Time
	totalDuration := lastTime - startTime

	return &ReplayTimeline{
		SessionKey:    sessionKey,
		Events:        replayEvents,
		TotalDuration: totalDuration,
		CreatedAt:     time.UnixMilli(startTime).Format(time.RFC3339),
		Stats:         stats,
	}
}

// ShareEntry stores a share link record.
type ShareEntry struct {
	Token      string `json:"token"`
	SessionKey string `json:"session_key"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// ShareStore manages share tokens with in-memory storage and expiry.
type ShareStore struct {
	mu     sync.RWMutex
	shares map[string]*ShareEntry // token -> entry
}

// NewShareStore creates a new ShareStore.
func NewShareStore() *ShareStore {
	return &ShareStore{
		shares: make(map[string]*ShareEntry),
	}
}

// GenerateShareToken creates a share token for a session key (32-char hex = 16 random bytes).
func (ss *ShareStore) GenerateShareToken(sessionKey string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate share token: %w", err)
	}
	token := hex.EncodeToString(b)

	entry := &ShareEntry{
		Token:      token,
		SessionKey: sessionKey,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		CreatedAt:  time.Now(),
	}

	ss.mu.Lock()
	ss.shares[token] = entry
	ss.mu.Unlock()

	return token, nil
}

// Lookup finds a share entry by token, returning nil if expired or not found.
func (ss *ShareStore) Lookup(token string) *ShareEntry {
	ss.mu.RLock()
	entry, ok := ss.shares[token]
	ss.mu.RUnlock()

	if !ok {
		return nil
	}
	if time.Now().After(entry.ExpiresAt) {
		ss.mu.Lock()
		delete(ss.shares, token)
		ss.mu.Unlock()
		return nil
	}
	return entry
}

// ListForSession returns all active share entries for a session key.
func (ss *ShareStore) ListForSession(sessionKey string) []ShareEntry {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	var result []ShareEntry
	now := time.Now()
	for _, entry := range ss.shares {
		if entry.SessionKey == sessionKey && now.Before(entry.ExpiresAt) {
			result = append(result, *entry)
		}
	}
	return result
}

// Revoke removes a share token.
func (ss *ShareStore) Revoke(token string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if _, ok := ss.shares[token]; ok {
		delete(ss.shares, token)
		return true
	}
	return false
}

// Cleanup removes expired entries.
func (ss *ShareStore) Cleanup() int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	now := time.Now()
	removed := 0
	for token, entry := range ss.shares {
		if now.After(entry.ExpiresAt) {
			delete(ss.shares, token)
			removed++
		}
	}
	return removed
}

// ReplayHandlers holds the replay API handler state.
type ReplayHandlers struct {
	router     *session.Router
	shareStore *ShareStore
}

// NewReplayHandlers creates ReplayHandlers.
func NewReplayHandlers(router *session.Router) *ReplayHandlers {
	return &ReplayHandlers{
		router:     router,
		shareStore: NewShareStore(),
	}
}

// handleReplay returns the replay timeline for a session.
// GET /api/sessions/replay?key=...
func (rh *ReplayHandlers) handleReplay(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
		return
	}

	sess := rh.router.GetSession(key)
	if sess == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	entries := sess.EventEntries()
	timeline := BuildTimeline(key, entries)

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, timeline)
}

// handleShare generates a share token for a session.
// POST /api/sessions/share  body: {"key": "..."}
func (rh *ReplayHandlers) handleShare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
		return
	}

	sess := rh.router.GetSession(req.Key)
	if sess == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	token, err := rh.shareStore.GenerateShareToken(req.Key)
	if err != nil {
		slog.Error("generate share token", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Build share URL from request host.
	scheme := "https"
	if strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.") {
		scheme = "http"
	}
	shareURL := fmt.Sprintf("%s://%s/api/shared/%s", scheme, r.Host, token)

	entry := rh.shareStore.Lookup(token)
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		"share_url":  shareURL,
		"token":      token,
		"expires_at": entry.ExpiresAt.Format(time.RFC3339),
	})
}

// handleSharedReplay returns a shared session replay (no auth required).
// GET /api/shared/{token}
func (rh *ReplayHandlers) handleSharedReplay(w http.ResponseWriter, r *http.Request) {
	// Extract token from path: /api/shared/{token}
	token := strings.TrimPrefix(r.URL.Path, "/api/shared/")
	if token == "" {
		http.Error(w, `{"error":"token is required"}`, http.StatusBadRequest)
		return
	}

	entry := rh.shareStore.Lookup(token)
	if entry == nil {
		http.Error(w, `{"error":"shared session expired or not found"}`, http.StatusGone)
		return
	}

	sess := rh.router.GetSession(entry.SessionKey)
	if sess == nil {
		http.Error(w, `{"error":"session no longer exists"}`, http.StatusNotFound)
		return
	}

	entries := sess.EventEntries()
	timeline := BuildTimeline(entry.SessionKey, entries)

	// Redact thinking events for shared replays.
	for i := range timeline.Events {
		if timeline.Events[i].Type == "thinking" {
			timeline.Events[i].Content = "[Thinking hidden]"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		"timeline":  timeline,
		"shared":    true,
		"read_only": true,
	})
}
