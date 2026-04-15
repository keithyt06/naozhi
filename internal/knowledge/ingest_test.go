package knowledge

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIngestFromVault(t *testing.T) {
	// Create a temporary vault with some .md files.
	vaultDir := t.TempDir()
	os.WriteFile(filepath.Join(vaultDir, "note1.md"), []byte("# Note One\n\nSome content about AWS WAF."), 0644)
	os.WriteFile(filepath.Join(vaultDir, "note2.md"), []byte("# Note Two\n\nSome content about CloudFront."), 0644)
	os.MkdirAll(filepath.Join(vaultDir, "sub"), 0755)
	os.WriteFile(filepath.Join(vaultDir, "sub", "note3.md"), []byte("# Sub Note\n\nNested content."), 0644)

	vault := NewVault(VaultConfig{VaultPath: vaultDir})
	search := NewSearchEngine()
	wiki := NewWikiManager(filepath.Join(t.TempDir(), "wiki"))
	engine := NewIngestEngine(wiki, vault, search)

	if err := engine.IngestFromVault(); err != nil {
		t.Fatalf("IngestFromVault: %v", err)
	}

	count := search.DocumentCount()
	if count != 3 {
		t.Errorf("expected 3 indexed docs, got %d", count)
	}

	// Verify vault docs are searchable.
	results := search.Search("WAF", "vault", 10)
	if len(results) != 1 {
		t.Errorf("search 'WAF' in vault: expected 1 result, got %d", len(results))
	}

	results = search.Search("CloudFront", "vault", 10)
	if len(results) != 1 {
		t.Errorf("search 'CloudFront' in vault: expected 1 result, got %d", len(results))
	}
}

func TestIngestFromVaultNotConfigured(t *testing.T) {
	vault := NewVault(VaultConfig{}) // no path
	search := NewSearchEngine()
	wiki := NewWikiManager(filepath.Join(t.TempDir(), "wiki"))
	engine := NewIngestEngine(wiki, vault, search)

	err := engine.IngestFromVault()
	if err == nil {
		t.Fatal("expected error for unconfigured vault")
	}
}

func TestIngestFromHistory(t *testing.T) {
	// Create a temporary history directory with a JSONL file.
	histDir := t.TempDir()
	jsonl := `{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":"How do I configure AWS WAF?"}}
{"type":"assistant","timestamp":"2026-04-15T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Here is how..."}]}}
{"type":"user","timestamp":"2026-04-15T10:01:00Z","message":{"role":"user","content":"What about rate limiting?"}}
`
	os.WriteFile(filepath.Join(histDir, "session-abc.jsonl"), []byte(jsonl), 0644)

	vault := NewVault(VaultConfig{})
	search := NewSearchEngine()
	wiki := NewWikiManager(filepath.Join(t.TempDir(), "wiki"))
	engine := NewIngestEngine(wiki, vault, search)

	if err := engine.IngestFromHistory(histDir); err != nil {
		t.Fatalf("IngestFromHistory: %v", err)
	}

	count := search.DocumentCount()
	if count != 2 {
		t.Errorf("expected 2 indexed docs (user prompts only), got %d", count)
	}

	results := search.Search("WAF", "cli", 10)
	if len(results) != 1 {
		t.Errorf("search 'WAF' in cli: expected 1 result, got %d", len(results))
	}
}

func TestIngestFromHistoryMissingDir(t *testing.T) {
	search := NewSearchEngine()
	wiki := NewWikiManager(filepath.Join(t.TempDir(), "wiki"))
	engine := NewIngestEngine(wiki, NewVault(VaultConfig{}), search)

	// Non-existent directory should not error.
	if err := engine.IngestFromHistory("/tmp/nonexistent-dir-12345"); err != nil {
		t.Fatalf("expected no error for missing dir, got: %v", err)
	}
}

func TestIngestWikiPages(t *testing.T) {
	wikiDir := t.TempDir()
	wiki := NewWikiManager(wikiDir)

	// Write a wiki page.
	page := WikiPage{
		Title:      "AWS WAF Best Practices",
		CompiledAt: time.Now().Format("2006-01-02T15:04:05Z"),
		Sources:    3,
		Entities:   []string{"WAF", "CloudFront"},
	}
	if err := wiki.WritePage("aws-waf", page, "Detailed content about AWS WAF rate limiting and rules."); err != nil {
		t.Fatalf("WritePage: %v", err)
	}

	search := NewSearchEngine()
	engine := NewIngestEngine(wiki, NewVault(VaultConfig{}), search)

	if err := engine.IngestWikiPages(); err != nil {
		t.Fatalf("IngestWikiPages: %v", err)
	}

	if search.DocumentCount() != 1 {
		t.Errorf("expected 1 indexed wiki doc, got %d", search.DocumentCount())
	}

	results := search.Search("rate limiting", "wiki", 10)
	if len(results) != 1 {
		t.Errorf("search 'rate limiting' in wiki: expected 1, got %d", len(results))
	}
}

func TestIndexBookmarks(t *testing.T) {
	search := NewSearchEngine()
	wiki := NewWikiManager(filepath.Join(t.TempDir(), "wiki"))
	engine := NewIngestEngine(wiki, NewVault(VaultConfig{}), search)

	bookmarks := []Bookmark{
		{ID: "bm1", Text: "WAF rule tuning notes", Tags: []string{"security"}, CreatedAt: time.Now()},
		{ID: "bm2", Text: "CloudFront cache strategy", Tags: []string{"cdn"}, CreatedAt: time.Now()},
	}

	engine.IndexBookmarks(bookmarks)

	if search.DocumentCount() != 2 {
		t.Errorf("expected 2 indexed bookmark docs, got %d", search.DocumentCount())
	}

	results := search.Search("WAF", "bookmarks", 10)
	if len(results) != 1 {
		t.Errorf("search 'WAF' in bookmarks: expected 1, got %d", len(results))
	}
}
