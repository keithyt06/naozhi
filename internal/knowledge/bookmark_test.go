package knowledge

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBookmarkAddRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.json")
	bs := NewBookmarkStore(path)

	// Add two bookmarks.
	bm1 := Bookmark{
		Text:       "WAF rate limiting best practices",
		Source:     "dashboard",
		SessionKey: "sess-1",
		Tags:       []string{"security", "waf"},
	}
	bm2 := Bookmark{
		Text:       "CloudFront cache invalidation notes",
		Source:     "cli",
		SessionKey: "sess-2",
		Tags:       []string{"cdn"},
	}
	if err := bs.Add(bm1); err != nil {
		t.Fatalf("Add bm1: %v", err)
	}
	if err := bs.Add(bm2); err != nil {
		t.Fatalf("Add bm2: %v", err)
	}

	list := bs.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(list))
	}

	// IDs should be auto-generated (16 hex chars).
	for _, bm := range list {
		if len(bm.ID) != 16 {
			t.Errorf("expected 16-char hex ID, got %q (len %d)", bm.ID, len(bm.ID))
		}
	}

	// Remove the first bookmark by its ID.
	id := list[len(list)-1].ID // oldest is last (sorted newest first)
	if err := bs.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	list = bs.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 bookmark after remove, got %d", len(list))
	}

	// Remove non-existent ID returns error.
	if err := bs.Remove("nonexistent"); err == nil {
		t.Fatal("expected error removing non-existent bookmark")
	}
}

func TestBookmarkSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.json")
	bs := NewBookmarkStore(path)

	bookmarks := []Bookmark{
		{Text: "AWS WAF rate limiting configuration", Source: "dashboard", Tags: []string{"security", "waf"}},
		{Text: "CloudFront cache invalidation strategy", Source: "cli", Tags: []string{"cdn", "cache"}},
		{Text: "S3 bucket policy best practices", Source: "vault", Tags: []string{"security", "s3"}},
	}
	for _, bm := range bookmarks {
		if err := bs.Add(bm); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// Search by text content.
	results := bs.Search("WAF")
	if len(results) != 1 {
		t.Errorf("search 'WAF': expected 1 result, got %d", len(results))
	}

	// Search by tag.
	results = bs.Search("security")
	if len(results) != 2 {
		t.Errorf("search 'security': expected 2 results, got %d", len(results))
	}

	// Case-insensitive search.
	results = bs.Search("cloudfront")
	if len(results) != 1 {
		t.Errorf("search 'cloudfront': expected 1 result, got %d", len(results))
	}

	// No match.
	results = bs.Search("nonexistent-term")
	if len(results) != 0 {
		t.Errorf("search 'nonexistent-term': expected 0 results, got %d", len(results))
	}

	// Empty query returns all.
	results = bs.Search("")
	if len(results) != 3 {
		t.Errorf("search '': expected 3 results, got %d", len(results))
	}
}

func TestBookmarkBySession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.json")
	bs := NewBookmarkStore(path)

	bookmarks := []Bookmark{
		{Text: "note A", Source: "dashboard", SessionKey: "sess-alpha"},
		{Text: "note B", Source: "dashboard", SessionKey: "sess-beta"},
		{Text: "note C", Source: "cli", SessionKey: "sess-alpha"},
		{Text: "note D", Source: "vault", SessionKey: "sess-gamma"},
	}
	for _, bm := range bookmarks {
		if err := bs.Add(bm); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	results := bs.BySession("sess-alpha")
	if len(results) != 2 {
		t.Errorf("BySession 'sess-alpha': expected 2, got %d", len(results))
	}

	results = bs.BySession("sess-beta")
	if len(results) != 1 {
		t.Errorf("BySession 'sess-beta': expected 1, got %d", len(results))
	}

	results = bs.BySession("nonexistent")
	if len(results) != 0 {
		t.Errorf("BySession 'nonexistent': expected 0, got %d", len(results))
	}
}

func TestBookmarkPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.json")

	// Create store and add bookmarks.
	bs1 := NewBookmarkStore(path)
	bm := Bookmark{
		Text:       "persistent bookmark test",
		Source:     "dashboard",
		SessionKey: "sess-persist",
		Tags:       []string{"test", "persistence"},
		CreatedAt:  time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
	}
	if err := bs1.Add(bm); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Create a second store from the same file — should load persisted data.
	bs2 := NewBookmarkStore(path)
	list := bs2.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 bookmark after reload, got %d", len(list))
	}

	got := list[0]
	if got.Text != "persistent bookmark test" {
		t.Errorf("Text mismatch: %q", got.Text)
	}
	if got.Source != "dashboard" {
		t.Errorf("Source mismatch: %q", got.Source)
	}
	if got.SessionKey != "sess-persist" {
		t.Errorf("SessionKey mismatch: %q", got.SessionKey)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "test" || got.Tags[1] != "persistence" {
		t.Errorf("Tags mismatch: %v", got.Tags)
	}
	if !got.CreatedAt.Equal(bm.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", got.CreatedAt, bm.CreatedAt)
	}

	// Verify the ID was preserved across reload.
	origList := bs1.List()
	if got.ID != origList[0].ID {
		t.Errorf("ID not preserved: got %q, want %q", got.ID, origList[0].ID)
	}
}
