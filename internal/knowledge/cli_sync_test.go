package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLISyncScanHistory(t *testing.T) {
	// Set up a fake ~/.claude/projects/ structure.
	claudeDir := t.TempDir()
	projDir := filepath.Join(claudeDir, "projects", "-home-user-myproject")
	os.MkdirAll(projDir, 0755)

	jsonl := `{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":"How do I set up CloudFront?"}}
{"type":"assistant","timestamp":"2026-04-15T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Here is how..."}]}}
{"type":"user","timestamp":"2026-04-15T10:01:00Z","message":{"role":"user","content":"What about cache invalidation?"}}
`
	os.WriteFile(filepath.Join(projDir, "session-123.jsonl"), []byte(jsonl), 0644)

	search := newTestSearch(t)
	csm := NewCLISyncManager(search)

	n, err := csm.ScanHistory(claudeDir)
	if err != nil {
		t.Fatalf("ScanHistory: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 new entries, got %d", n)
	}

	// Verify indexed documents.
	count, cErr := search.DocumentCount()
	if cErr != nil {
		t.Fatalf("DocumentCount: %v", cErr)
	}
	if count != 2 {
		t.Errorf("expected 2 docs in search, got %d", count)
	}

	results, sErr := search.Search("CloudFront", "cli", 10)
	if sErr != nil {
		t.Fatalf("Search CloudFront: %v", sErr)
	}
	if len(results) != 1 {
		t.Errorf("search 'CloudFront': expected 1, got %d", len(results))
	}

	results, sErr = search.Search("cache invalidation", "cli", 10)
	if sErr != nil {
		t.Fatalf("Search cache invalidation: %v", sErr)
	}
	if len(results) != 1 {
		t.Errorf("search 'cache invalidation': expected 1, got %d", len(results))
	}
}

func TestCLISyncIncremental(t *testing.T) {
	claudeDir := t.TempDir()
	projDir := filepath.Join(claudeDir, "projects", "-home-user-project2")
	os.MkdirAll(projDir, 0755)

	jsonl1 := `{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":"First prompt"}}
`
	os.WriteFile(filepath.Join(projDir, "sess-a.jsonl"), []byte(jsonl1), 0644)

	search := newTestSearch(t)
	csm := NewCLISyncManager(search)

	// First scan.
	n, err := csm.ScanHistory(claudeDir)
	if err != nil {
		t.Fatalf("first ScanHistory: %v", err)
	}
	if n != 1 {
		t.Errorf("first scan: expected 1, got %d", n)
	}

	// Second scan without changes — should find 0 new (file mtime unchanged).
	n, err = csm.ScanHistory(claudeDir)
	if err != nil {
		t.Fatalf("second ScanHistory: %v", err)
	}
	if n != 0 {
		t.Errorf("second scan: expected 0 new, got %d", n)
	}
}

func TestCLISyncMissingDir(t *testing.T) {
	search := newTestSearch(t)
	csm := NewCLISyncManager(search)

	n, err := csm.ScanHistory("/tmp/nonexistent-claude-dir-99999")
	if err != nil {
		t.Fatalf("expected no error for missing dir, got: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 entries for missing dir, got %d", n)
	}
}

func TestParseHistoryLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		prompt  string
	}{
		{
			name:   "valid user message with string content",
			line:   `{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":"Hello world"}}`,
			prompt: "Hello world",
		},
		{
			name:   "valid user message with block content",
			line:   `{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Block content"}]}}`,
			prompt: "Block content",
		},
		{
			name:    "assistant message is skipped",
			line:    `{"type":"assistant","timestamp":"2026-04-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"response"}]}}`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			line:    `{broken json`,
			wantErr: true,
		},
		{
			name:    "user message with empty content",
			line:    `{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":""}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := ParseHistoryLine([]byte(tt.line))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.Prompt != tt.prompt {
				t.Errorf("prompt = %q, want %q", entry.Prompt, tt.prompt)
			}
		})
	}
}

func TestDocCountBySource(t *testing.T) {
	search := newTestSearch(t)

	for _, doc := range []SearchDocument{
		{ID: "a", Source: "vault", Title: "A", Text: "content"},
		{ID: "b", Source: "vault", Title: "B", Text: "content"},
		{ID: "c", Source: "cli", Title: "C", Text: "content"},
		{ID: "d", Source: "wiki", Title: "D", Text: "content"},
	} {
		if err := search.IndexDocument(doc); err != nil {
			t.Fatalf("IndexDocument(%s): %v", doc.ID, err)
		}
	}

	counts, err := search.DocCountBySource()
	if err != nil {
		t.Fatalf("DocCountBySource: %v", err)
	}
	if counts["vault"] != 2 {
		t.Errorf("vault count = %d, want 2", counts["vault"])
	}
	if counts["cli"] != 1 {
		t.Errorf("cli count = %d, want 1", counts["cli"])
	}
	if counts["wiki"] != 1 {
		t.Errorf("wiki count = %d, want 1", counts["wiki"])
	}
}
