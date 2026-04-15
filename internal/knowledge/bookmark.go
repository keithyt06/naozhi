package knowledge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Bookmark represents a saved message snippet that can be tagged and searched.
type Bookmark struct {
	ID         string    `json:"id"`
	Text       string    `json:"text"`                  // saved content snippet
	Source     string    `json:"source"`                // "dashboard", "cli", "feishu", "vault"
	SessionKey string   `json:"session_key,omitempty"`
	Tags       []string  `json:"tags"`
	CreatedAt  time.Time `json:"created_at"`
}

// BookmarkStore manages bookmark CRUD operations with JSON file persistence.
// It uses an in-memory slice protected by a read-write mutex and persists
// changes to disk via atomic write (write .tmp then rename).
type BookmarkStore struct {
	mu        sync.RWMutex
	bookmarks []Bookmark
	path      string // e.g. ~/.naozhi/bookmarks.json
	dirty     bool
}

// NewBookmarkStore creates a BookmarkStore and loads any existing data from path.
func NewBookmarkStore(path string) *BookmarkStore {
	bs := &BookmarkStore{
		path:      path,
		bookmarks: make([]Bookmark, 0),
	}
	if err := bs.Load(); err != nil {
		slog.Warn("load bookmark store failed", "err", err, "path", path)
	}
	return bs
}

// Load reads bookmarks from the JSON file on disk into memory.
// Returns nil if the file does not exist (empty store).
func (bs *BookmarkStore) Load() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.path == "" {
		return nil
	}

	data, err := os.ReadFile(bs.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read bookmark store: %w", err)
	}

	var entries []Bookmark
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse bookmark store: %w", err)
	}

	bs.bookmarks = entries
	bs.dirty = false
	slog.Info("loaded bookmark store", "count", len(entries), "path", bs.path)
	return nil
}

// Save persists bookmarks to disk using atomic write (write to .tmp, then rename).
func (bs *BookmarkStore) Save() error {
	bs.mu.RLock()
	entries := make([]Bookmark, len(bs.bookmarks))
	copy(entries, bs.bookmarks)
	bs.mu.RUnlock()

	if bs.path == "" {
		return nil
	}

	if dir := filepath.Dir(bs.path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create bookmark store directory: %w", err)
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bookmarks: %w", err)
	}

	// Atomic write: write to temp file, then rename.
	tmp := bs.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write bookmark tmp file: %w", err)
	}
	if err := os.Rename(tmp, bs.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename bookmark tmp file: %w", err)
	}

	bs.mu.Lock()
	bs.dirty = false
	bs.mu.Unlock()
	return nil
}

// Add appends a bookmark. If bm.ID is empty, a random 16-char hex ID is generated.
// The bookmark is saved to disk immediately.
func (bs *BookmarkStore) Add(bm Bookmark) error {
	if bm.ID == "" {
		id, err := randomHexID(8)
		if err != nil {
			return fmt.Errorf("generate bookmark id: %w", err)
		}
		bm.ID = id
	}
	if bm.CreatedAt.IsZero() {
		bm.CreatedAt = time.Now()
	}
	if bm.Tags == nil {
		bm.Tags = []string{}
	}

	bs.mu.Lock()
	bs.bookmarks = append(bs.bookmarks, bm)
	bs.dirty = true
	bs.mu.Unlock()

	return bs.Save()
}

// Remove deletes the bookmark with the given id and saves to disk.
// Returns an error if the id is not found.
func (bs *BookmarkStore) Remove(id string) error {
	bs.mu.Lock()
	found := false
	filtered := make([]Bookmark, 0, len(bs.bookmarks))
	for _, bm := range bs.bookmarks {
		if bm.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, bm)
	}
	if !found {
		bs.mu.Unlock()
		return fmt.Errorf("bookmark not found: %s", id)
	}
	bs.bookmarks = filtered
	bs.dirty = true
	bs.mu.Unlock()

	return bs.Save()
}

// List returns a copy of all bookmarks sorted by CreatedAt descending (newest first).
func (bs *BookmarkStore) List() []Bookmark {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	result := make([]Bookmark, len(bs.bookmarks))
	copy(result, bs.bookmarks)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// Search returns bookmarks whose Text or Tags contain the query string
// (case-insensitive substring match). Results are sorted newest first.
func (bs *BookmarkStore) Search(query string) []Bookmark {
	if query == "" {
		return bs.List()
	}
	q := strings.ToLower(query)

	bs.mu.RLock()
	defer bs.mu.RUnlock()

	var result []Bookmark
	for _, bm := range bs.bookmarks {
		if strings.Contains(strings.ToLower(bm.Text), q) {
			result = append(result, bm)
			continue
		}
		for _, tag := range bm.Tags {
			if strings.Contains(strings.ToLower(tag), q) {
				result = append(result, bm)
				break
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// BySession returns all bookmarks matching the given sessionKey,
// sorted by CreatedAt descending.
func (bs *BookmarkStore) BySession(sessionKey string) []Bookmark {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	var result []Bookmark
	for _, bm := range bs.bookmarks {
		if bm.SessionKey == sessionKey {
			result = append(result, bm)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// randomHexID generates a random hex string of 2*n characters.
func randomHexID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
