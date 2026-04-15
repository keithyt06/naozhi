package knowledge

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func newTestSearchEngine(t *testing.T) *SearchEngine {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.bleve")
	se, err := NewSearchEngine(path)
	if err != nil {
		t.Fatalf("NewSearchEngine(%q): %v", path, err)
	}
	t.Cleanup(func() { se.Close() })
	return se
}

func TestSearchIndexAndQuery(t *testing.T) {
	se := newTestSearchEngine(t)

	doc := SearchDocument{
		ID:        "test:1",
		Source:    "vault",
		Title:     "AWS WAF Best Practices",
		Text:      "Rate limiting rules help prevent DDoS attacks on CloudFront distributions.",
		Tags:      []string{"security", "waf"},
		Timestamp: time.Now(),
		Meta:      "notes/waf.md",
	}

	if err := se.IndexDocument(doc); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}

	// Verify document count.
	count, err := se.DocumentCount()
	if err != nil {
		t.Fatalf("DocumentCount: %v", err)
	}
	if count != 1 {
		t.Errorf("DocumentCount = %d, want 1", count)
	}

	// Search by title keyword.
	results, err := se.Search("WAF", "all", 10)
	if err != nil {
		t.Fatalf("Search 'WAF': %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search 'WAF': got %d results, want 1", len(results))
	}
	if results[0].DocID != "test:1" {
		t.Errorf("result DocID = %q, want %q", results[0].DocID, "test:1")
	}
	if results[0].Source != "vault" {
		t.Errorf("result Source = %q, want %q", results[0].Source, "vault")
	}

	// Search by text content keyword.
	results, err = se.Search("DDoS", "all", 10)
	if err != nil {
		t.Fatalf("Search 'DDoS': %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search 'DDoS': got %d results, want 1", len(results))
	}

	// Search by tag.
	results, err = se.Search("security", "all", 10)
	if err != nil {
		t.Fatalf("Search 'security': %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search 'security' (tag): got %d results, want 1", len(results))
	}

	// Empty query returns nil.
	results, err = se.Search("", "all", 10)
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if results != nil {
		t.Errorf("Search empty: got %d results, want nil", len(results))
	}

	// No match returns empty slice.
	results, err = se.Search("nonexistent", "all", 10)
	if err != nil {
		t.Fatalf("Search 'nonexistent': %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search 'nonexistent': got %d results, want 0", len(results))
	}
}

func TestSearchBySource(t *testing.T) {
	se := newTestSearchEngine(t)

	docs := []SearchDocument{
		{ID: "vault:note1", Source: "vault", Title: "WAF Note", Text: "WAF content from vault", Timestamp: time.Now()},
		{ID: "cli:prompt1", Source: "cli", Title: "CLI Prompt", Text: "WAF question from CLI", Timestamp: time.Now()},
		{ID: "wiki:page1", Source: "wiki", Title: "Wiki WAF", Text: "WAF wiki page", Timestamp: time.Now()},
		{ID: "bm:bm1", Source: "bookmarks", Title: "Bookmark WAF", Text: "WAF bookmark saved", Timestamp: time.Now()},
	}
	for _, doc := range docs {
		if err := se.IndexDocument(doc); err != nil {
			t.Fatalf("IndexDocument(%s): %v", doc.ID, err)
		}
	}

	// Search all sources.
	results, err := se.Search("WAF", "all", 20)
	if err != nil {
		t.Fatalf("Search all: %v", err)
	}
	if len(results) != 4 {
		t.Errorf("Search 'WAF' all: got %d, want 4", len(results))
	}

	// Filter by vault.
	results, err = se.Search("WAF", "vault", 20)
	if err != nil {
		t.Fatalf("Search vault: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search 'WAF' vault: got %d, want 1", len(results))
	}
	if len(results) > 0 && results[0].Source != "vault" {
		t.Errorf("result source = %q, want vault", results[0].Source)
	}

	// Filter by cli.
	results, err = se.Search("WAF", "cli", 20)
	if err != nil {
		t.Fatalf("Search cli: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search 'WAF' cli: got %d, want 1", len(results))
	}

	// Filter by bookmarks.
	results, err = se.Search("WAF", "bookmarks", 20)
	if err != nil {
		t.Fatalf("Search bookmarks: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search 'WAF' bookmarks: got %d, want 1", len(results))
	}

	// Filter by non-existent source returns empty.
	results, err = se.Search("WAF", "dashboard", 20)
	if err != nil {
		t.Fatalf("Search dashboard: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search 'WAF' dashboard: got %d, want 0", len(results))
	}
}

func TestSearchMultipleDocuments(t *testing.T) {
	se := newTestSearchEngine(t)

	// Index many documents and verify search, delete, and count operations.
	for i := 0; i < 25; i++ {
		doc := SearchDocument{
			ID:        fmt.Sprintf("doc:%d", i),
			Source:    "wiki",
			Title:     fmt.Sprintf("Page %d about AWS services", i),
			Text:      fmt.Sprintf("Content %d covering CloudFront and WAF configuration", i),
			Tags:      []string{"aws"},
			Timestamp: time.Now(),
		}
		if err := se.IndexDocument(doc); err != nil {
			t.Fatalf("IndexDocument %d: %v", i, err)
		}
	}

	// Count all documents.
	count, err := se.DocumentCount()
	if err != nil {
		t.Fatalf("DocumentCount: %v", err)
	}
	if count != 25 {
		t.Errorf("DocumentCount = %d, want 25", count)
	}

	// Search with limit.
	results, err := se.Search("AWS", "all", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Search with limit 5: got %d, want 5", len(results))
	}

	// Search with limit > 100 gets capped.
	results, err = se.Search("AWS", "all", 200)
	if err != nil {
		t.Fatalf("Search capped: %v", err)
	}
	if len(results) > 100 {
		t.Errorf("Search with limit 200: got %d, want <= 100", len(results))
	}

	// Delete one document.
	if err := se.DeleteDocument("doc:0"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	count, err = se.DocumentCount()
	if err != nil {
		t.Fatalf("DocumentCount after delete: %v", err)
	}
	if count != 24 {
		t.Errorf("DocumentCount after delete = %d, want 24", count)
	}

	// DocCountBySource should report all as wiki.
	bySource, err := se.DocCountBySource()
	if err != nil {
		t.Fatalf("DocCountBySource: %v", err)
	}
	if bySource["wiki"] != 24 {
		t.Errorf("DocCountBySource[wiki] = %d, want 24", bySource["wiki"])
	}
}

func TestSearchDocumentUpdate(t *testing.T) {
	se := newTestSearchEngine(t)

	// Index a document, then update it with the same ID.
	doc := SearchDocument{
		ID:     "update:1",
		Source: "vault",
		Title:  "Original Title",
		Text:   "Original content about Lambda functions",
		Timestamp: time.Now(),
	}
	if err := se.IndexDocument(doc); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}

	// Update same ID with different content.
	doc.Title = "Updated Title"
	doc.Text = "Updated content about S3 buckets"
	if err := se.IndexDocument(doc); err != nil {
		t.Fatalf("IndexDocument update: %v", err)
	}

	// Should still be one document.
	count, err := se.DocumentCount()
	if err != nil {
		t.Fatalf("DocumentCount: %v", err)
	}
	if count != 1 {
		t.Errorf("DocumentCount after update = %d, want 1", count)
	}

	// Old content should not be found.
	results, err := se.Search("Lambda", "all", 10)
	if err != nil {
		t.Fatalf("Search Lambda: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search old content 'Lambda': got %d, want 0", len(results))
	}

	// New content should be found.
	results, err = se.Search("S3 buckets", "all", 10)
	if err != nil {
		t.Fatalf("Search S3: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search new content 'S3 buckets': got %d, want 1", len(results))
	}
}

func TestSearchReopenIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reopen.bleve")

	// Create, index, close.
	se, err := NewSearchEngine(path)
	if err != nil {
		t.Fatalf("NewSearchEngine create: %v", err)
	}
	doc := SearchDocument{
		ID:     "persist:1",
		Source: "wiki",
		Title:  "Persistent Document",
		Text:   "This should survive index close and reopen",
		Timestamp: time.Now(),
	}
	if err := se.IndexDocument(doc); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}
	if err := se.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and verify data persisted.
	se2, err := NewSearchEngine(path)
	if err != nil {
		t.Fatalf("NewSearchEngine reopen: %v", err)
	}
	defer se2.Close()

	count, err := se2.DocumentCount()
	if err != nil {
		t.Fatalf("DocumentCount after reopen: %v", err)
	}
	if count != 1 {
		t.Errorf("DocumentCount after reopen = %d, want 1", count)
	}

	results, err := se2.Search("Persistent", "all", 10)
	if err != nil {
		t.Fatalf("Search after reopen: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search after reopen: got %d, want 1", len(results))
	}
}
