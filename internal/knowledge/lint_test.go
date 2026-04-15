package knowledge

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLintOrphanDetection(t *testing.T) {
	wikiDir := t.TempDir()
	wiki := NewWikiManager(wikiDir)

	// Create two pages: page-a links to page-b, but page-b links to nothing.
	// page-a should be detected as orphan (no incoming links).
	pageA := WikiPage{Title: "Page A", CompiledAt: time.Now().Format("2006-01-02T15:04:05Z")}
	pageB := WikiPage{Title: "Page B", CompiledAt: time.Now().Format("2006-01-02T15:04:05Z")}

	wiki.WritePage("page-a", pageA, "This page links to [[page-b]] for reference.")
	wiki.WritePage("page-b", pageB, "Standalone content with no outgoing links.")

	le := NewLintEngine(wiki, 30)
	result := le.RunLint()

	if result.Stats.TotalPages != 2 {
		t.Errorf("expected 2 total pages, got %d", result.Stats.TotalPages)
	}

	// page-a has no incoming links, so it should be flagged as orphan.
	// page-b has an incoming link from page-a, so it should not be flagged.
	orphans := filterIssuesByType(result.Issues, "orphan")
	if len(orphans) != 1 {
		t.Errorf("expected 1 orphan issue, got %d: %+v", len(orphans), orphans)
	}
	if len(orphans) > 0 && orphans[0].Page != "page-a" {
		t.Errorf("expected orphan page 'page-a', got %q", orphans[0].Page)
	}
}

func TestLintStaleDetection(t *testing.T) {
	wikiDir := t.TempDir()
	wiki := NewWikiManager(wikiDir)

	// Create a page with an old compiled_at date.
	staleTime := time.Now().Add(-60 * 24 * time.Hour).Format("2006-01-02T15:04:05Z")
	stalePage := WikiPage{Title: "Old Page", CompiledAt: staleTime}
	wiki.WritePage("old-page", stalePage, "This is old content.")

	// Create a fresh page.
	freshPage := WikiPage{Title: "Fresh Page", CompiledAt: time.Now().Format("2006-01-02T15:04:05Z")}
	wiki.WritePage("fresh-page", freshPage, "This is fresh content.")

	le := NewLintEngine(wiki, 30)
	result := le.RunLint()

	staleIssues := filterIssuesByType(result.Issues, "stale")
	if len(staleIssues) != 1 {
		t.Errorf("expected 1 stale issue, got %d", len(staleIssues))
	}
	if len(staleIssues) > 0 && staleIssues[0].Page != "old-page" {
		t.Errorf("expected stale page 'old-page', got %q", staleIssues[0].Page)
	}
}

func TestLintEmptyDetection(t *testing.T) {
	wikiDir := t.TempDir()
	wiki := NewWikiManager(wikiDir)

	// Create an empty page (frontmatter only, no body).
	emptyPage := WikiPage{Title: "Empty Page", CompiledAt: time.Now().Format("2006-01-02T15:04:05Z")}
	wiki.WritePage("empty-page", emptyPage, "")

	// Create a page with content.
	fullPage := WikiPage{Title: "Full Page", CompiledAt: time.Now().Format("2006-01-02T15:04:05Z")}
	wiki.WritePage("full-page", fullPage, "This page has substantial content about AWS services.")

	le := NewLintEngine(wiki, 30)
	result := le.RunLint()

	emptyIssues := filterIssuesByType(result.Issues, "empty")
	if len(emptyIssues) != 1 {
		t.Errorf("expected 1 empty issue, got %d", len(emptyIssues))
	}
	if len(emptyIssues) > 0 && emptyIssues[0].Page != "empty-page" {
		t.Errorf("expected empty page 'empty-page', got %q", emptyIssues[0].Page)
	}
}

func TestLintMissingFrontmatter(t *testing.T) {
	wikiDir := t.TempDir()
	wiki := NewWikiManager(wikiDir)

	// Create a page with no frontmatter (just raw markdown).
	// Write directly to skip WikiManager's frontmatter insertion.
	rawPath := filepath.Join(wikiDir, "raw-page.md")
	content := "# Raw Page\n\nThis page has no YAML frontmatter."
	if err := os.WriteFile(rawPath, []byte(content), 0644); err != nil {
		t.Fatalf("write raw page: %v", err)
	}

	le := NewLintEngine(wiki, 30)
	result := le.RunLint()

	fmIssues := filterIssuesByType(result.Issues, "missing_frontmatter")
	if len(fmIssues) != 1 {
		t.Errorf("expected 1 missing_frontmatter issue, got %d: %+v", len(fmIssues), fmIssues)
	}
}

func TestLintEmptyWiki(t *testing.T) {
	wikiDir := t.TempDir()
	wiki := NewWikiManager(wikiDir)

	le := NewLintEngine(wiki, 30)
	result := le.RunLint()

	if result.Stats.TotalPages != 0 {
		t.Errorf("expected 0 total pages, got %d", result.Stats.TotalPages)
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues for empty wiki, got %d", len(result.Issues))
	}
	if result.Duration == "" {
		t.Error("expected non-empty duration")
	}
}

func TestLintLastResult(t *testing.T) {
	wiki := NewWikiManager(t.TempDir())
	le := NewLintEngine(wiki, 30)

	if le.LastResult() != nil {
		t.Error("expected nil LastResult before first run")
	}

	le.RunLint()

	if le.LastResult() == nil {
		t.Error("expected non-nil LastResult after run")
	}
}

func TestExtractWikilinksFromContent(t *testing.T) {
	tests := []struct {
		content  string
		expected []string
	}{
		{"No links here.", nil},
		{"See [[page-a]] for details.", []string{"page-a"}},
		{"Link to [[page-a|Page A]] and [[page-b]].", []string{"page-a", "page-b"}},
		{"Duplicate [[foo]] and [[foo]] again.", []string{"foo"}},
		{"Complex [[my-page|My Custom Display]].", []string{"my-page"}},
	}

	for _, tt := range tests {
		links := extractWikilinksFromContent([]byte(tt.content))
		if !stringSliceEqual(links, tt.expected) {
			t.Errorf("extractWikilinksFromContent(%q) = %v, want %v", tt.content, links, tt.expected)
		}
	}
}

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"No frontmatter here.", "No frontmatter here."},
		{"---\ntitle: Test\n---\n\nBody content.", "Body content."},
		{"---\ntitle: Test\ntags: [a, b]\n---\n\nMulti-line\nbody.", "Multi-line\nbody."},
	}

	for _, tt := range tests {
		got := string(stripFrontmatter([]byte(tt.input)))
		if got != tt.expected {
			t.Errorf("stripFrontmatter(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// helpers

func filterIssuesByType(issues []LintIssue, issueType string) []LintIssue {
	var filtered []LintIssue
	for _, issue := range issues {
		if issue.Type == issueType {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

