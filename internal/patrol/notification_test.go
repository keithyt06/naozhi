package patrol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNotificationCRUD(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "notifications.json")
	store := NewNotificationStore(storePath)

	// Add a notification
	n := &Notification{
		Title:       "Cost Alert",
		Description: "Daily cost exceeded $50",
		Urgency:     "urgent",
		Source:      "cost-alert",
	}
	if err := store.Add(n); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if n.ID == "" {
		t.Fatal("expected non-empty ID after Add")
	}
	if n.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt after Add")
	}

	// Verify unread count
	if count := store.UnreadCount(); count != 1 {
		t.Fatalf("expected 1 unread, got %d", count)
	}

	// Add another
	n2 := &Notification{
		Title:       "Patrol Complete",
		Description: "infra-health completed OK",
		Source:      "infra-health",
	}
	if err := store.Add(n2); err != nil {
		t.Fatalf("Add second: %v", err)
	}

	// List returns most recent first
	list := store.List(10)
	if len(list) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(list))
	}
	if list[0].Title != "Patrol Complete" {
		t.Fatalf("expected most recent first, got %s", list[0].Title)
	}
	if list[1].Title != "Cost Alert" {
		t.Fatalf("expected second to be Cost Alert, got %s", list[1].Title)
	}

	// List with limit
	limited := store.List(1)
	if len(limited) != 1 {
		t.Fatalf("expected 1 with limit=1, got %d", len(limited))
	}

	// Unread count should be 2
	if count := store.UnreadCount(); count != 2 {
		t.Fatalf("expected 2 unread, got %d", count)
	}

	// Mark one as read
	if err := store.MarkRead(n.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if count := store.UnreadCount(); count != 1 {
		t.Fatalf("expected 1 unread after marking one read, got %d", count)
	}

	// Mark all read
	if err := store.MarkAllRead(); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if count := store.UnreadCount(); count != 0 {
		t.Fatalf("expected 0 unread after mark all, got %d", count)
	}

	// Verify persistence
	store2 := NewNotificationStore(storePath)
	list2 := store2.List(10)
	if len(list2) != 2 {
		t.Fatalf("reloaded store: expected 2, got %d", len(list2))
	}
}

func TestNotificationCap(t *testing.T) {
	dir := t.TempDir()
	store := NewNotificationStore(filepath.Join(dir, "notifications.json"))

	// Fill beyond the cap
	for i := 0; i < maxNotifications+50; i++ {
		n := &Notification{
			Title:  "test",
			Source: "test",
		}
		if err := store.Add(n); err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
	}

	store.mu.RLock()
	count := len(store.notifications)
	store.mu.RUnlock()

	if count > maxNotifications {
		t.Fatalf("expected at most %d notifications, got %d", maxNotifications, count)
	}
}

func TestNotificationCapEvictsReadFirst(t *testing.T) {
	dir := t.TempDir()
	store := NewNotificationStore(filepath.Join(dir, "notifications.json"))

	// Add maxNotifications notifications, mark first half as read
	for i := 0; i < maxNotifications; i++ {
		n := &Notification{
			Title:  "test",
			Source: "test",
		}
		if err := store.Add(n); err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
	}

	// Mark the first 100 as read
	list := store.List(maxNotifications)
	// list is in reverse order (most recent first), so the "oldest" are at the end
	for i := len(list) - 1; i >= len(list)-100; i-- {
		_ = store.MarkRead(list[i].ID)
	}

	readBefore := 0
	store.mu.RLock()
	for _, n := range store.notifications {
		if n.ReadAt != nil {
			readBefore++
		}
	}
	store.mu.RUnlock()

	// Now add one more to trigger eviction
	n := &Notification{
		Title:  "new",
		Source: "test",
	}
	if err := store.Add(n); err != nil {
		t.Fatalf("Add trigger eviction: %v", err)
	}

	store.mu.RLock()
	count := len(store.notifications)
	readAfter := 0
	for _, notif := range store.notifications {
		if notif.ReadAt != nil {
			readAfter++
		}
	}
	store.mu.RUnlock()

	if count > maxNotifications {
		t.Fatalf("expected at most %d, got %d", maxNotifications, count)
	}

	// A read notification should have been evicted
	if readAfter >= readBefore {
		t.Fatalf("expected fewer read notifications after eviction: before=%d after=%d", readBefore, readAfter)
	}
}

func TestNotificationMarkReadNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewNotificationStore(filepath.Join(dir, "notifications.json"))

	if err := store.MarkRead("nonexistent"); err == nil {
		t.Fatal("expected error marking nonexistent notification as read")
	}
}

func TestNotificationEmptyPath(t *testing.T) {
	store := NewNotificationStore("")
	n := &Notification{
		Title:  "test",
		Source: "test",
	}
	if err := store.Add(n); err != nil {
		t.Fatalf("Add with empty path: %v", err)
	}
	if len(store.List(10)) != 1 {
		t.Fatal("expected 1 notification in memory")
	}
}

func TestNotificationPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")

	store := NewNotificationStore(path)
	n := &Notification{
		Title:       "Persist Test",
		Description: "Should survive reload",
		Source:      "test",
	}
	if err := store.Add(n); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected notifications.json to exist on disk")
	}

	// Load from fresh store
	store2 := NewNotificationStore(path)
	list := store2.List(10)
	if len(list) != 1 {
		t.Fatalf("expected 1 after reload, got %d", len(list))
	}
	if list[0].Title != "Persist Test" {
		t.Fatalf("expected title 'Persist Test', got %s", list[0].Title)
	}
}

func TestNotificationListEmpty(t *testing.T) {
	store := NewNotificationStore("")
	list := store.List(10)
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
	if count := store.UnreadCount(); count != 0 {
		t.Fatalf("expected 0 unread, got %d", count)
	}
}
