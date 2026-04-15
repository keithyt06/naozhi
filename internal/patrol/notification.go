package patrol

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// maxNotifications is the capacity cap for the in-memory notification list.
const maxNotifications = 200

// Notification represents a single notification entry in the notification center.
type Notification struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Urgency     string    `json:"urgency"` // "urgent" or "normal" (priority only)
	Read        bool      `json:"read"`
	Source      string    `json:"source"`
	SessionKey  string    `json:"session_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
}

// generateNotifID returns a 16-char hex ID for notifications.
func generateNotifID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("%x", b)
}

// NotificationStore manages notifications with JSON file persistence.
// The list is capped at maxNotifications entries; oldest read notifications
// are evicted first when the cap is reached.
type NotificationStore struct {
	mu            sync.RWMutex
	notifications []*Notification
	filePath      string
}

// NewNotificationStore creates a new store, loading existing data from disk.
func NewNotificationStore(filePath string) *NotificationStore {
	s := &NotificationStore{filePath: filePath}
	s.notifications = loadNotifications(filePath)
	if s.notifications == nil {
		s.notifications = make([]*Notification, 0)
	}
	return s
}

// Add inserts a new notification. An ID and CreatedAt are assigned automatically.
// If the store exceeds maxNotifications, the oldest read notification is removed.
// If all are unread, the oldest notification is removed regardless.
func (s *NotificationStore) Add(n *Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n.ID = generateNotifID()
	n.CreatedAt = time.Now()
	if n.Urgency == "" {
		n.Urgency = "normal"
	}

	s.notifications = append(s.notifications, n)
	s.evictLocked()
	return s.saveLocked()
}

// MarkRead marks a single notification as read by ID.
func (s *NotificationStore) MarkRead(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, n := range s.notifications {
		if n.ID == id {
			now := time.Now()
			n.ReadAt = &now
			n.Read = true
			return s.saveLocked()
		}
	}
	return fmt.Errorf("notification %s not found", id)
}

// MarkAllRead marks all notifications as read.
func (s *NotificationStore) MarkAllRead() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, n := range s.notifications {
		if !n.Read {
			n.ReadAt = &now
			n.Read = true
		}
	}
	return s.saveLocked()
}

// List returns up to limit notifications, most recent first.
func (s *NotificationStore) List(limit int) []Notification {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.notifications)
	if limit <= 0 || limit > total {
		limit = total
	}
	// Return most recent first.
	result := make([]Notification, 0, limit)
	for i := total - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, *s.notifications[i])
	}
	return result
}

// UnreadCount returns the number of notifications that have not been read.
func (s *NotificationStore) UnreadCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, n := range s.notifications {
		if !n.Read {
			count++
		}
	}
	return count
}

// evictLocked removes excess notifications when over maxNotifications.
// Prioritizes removing oldest read notifications; falls back to oldest overall.
// Must be called with s.mu write-lock held.
func (s *NotificationStore) evictLocked() {
	for len(s.notifications) > maxNotifications {
		// Find the oldest read notification.
		oldestReadIdx := -1
		for i, n := range s.notifications {
			if n.Read {
				oldestReadIdx = i
				break // earliest in slice is oldest
			}
		}
		if oldestReadIdx >= 0 {
			s.notifications = append(s.notifications[:oldestReadIdx], s.notifications[oldestReadIdx+1:]...)
		} else {
			// All unread: remove the oldest (first element).
			s.notifications = s.notifications[1:]
		}
	}
}

// saveLocked persists notifications to disk atomically. Must be called with mu held.
func (s *NotificationStore) saveLocked() error {
	return saveNotifications(s.filePath, s.notifications)
}

// --- Atomic JSON persistence ---

func saveNotifications(path string, notifications []*Notification) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create notification store directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(notifications, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func loadNotifications(path string) []*Notification {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("load notification store failed", "err", err)
		}
		return nil
	}
	var notifications []*Notification
	if err := json.Unmarshal(data, &notifications); err != nil {
		slog.Warn("parse notification store failed", "err", err)
		return nil
	}
	slog.Info("loaded notification store", "count", len(notifications), "path", path)
	return notifications
}
