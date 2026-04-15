package patrol

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// NotificationHandlers groups the notification center API endpoints.
type NotificationHandlers struct {
	Store *NotificationStore
	// Hub is called after adding notifications to broadcast via WebSocket.
	// Nil-safe: callers may leave this unset during testing.
	Hub interface {
		BroadcastNotification(n Notification)
	}
}

// RegisterNotificationRoutes registers all notification REST API routes on the given mux.
// The auth wrapper is applied by the caller (server.go).
func (h *NotificationHandlers) RegisterNotificationRoutes(mux *http.ServeMux, auth func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/notifications", auth(h.handleList))
	mux.HandleFunc("POST /api/notifications/read-all", auth(h.handleReadAll))
	mux.HandleFunc("GET /api/notifications/count", auth(h.handleCount))
}

// GET /api/notifications — list notifications (supports ?limit=N, default 50).
func (h *NotificationHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}

	notifications := h.Store.List(limit)
	if notifications == nil {
		notifications = []Notification{}
	}

	w.Header().Set("Content-Type", "application/json")
	writeNotifJSON(w, map[string]any{
		"notifications": notifications,
		"unread_count":  h.Store.UnreadCount(),
	})
}

// POST /api/notifications/read-all — mark all notifications as read.
func (h *NotificationHandlers) handleReadAll(w http.ResponseWriter, r *http.Request) {
	if err := h.Store.MarkAllRead(); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeNotifJSON(w, map[string]string{"status": "ok"})
}

// GET /api/notifications/count — return unread notification count.
func (h *NotificationHandlers) handleCount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	writeNotifJSON(w, map[string]int{"unread_count": h.Store.UnreadCount()})
}

// writeNotifJSON is a local helper to encode JSON responses.
func writeNotifJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}
