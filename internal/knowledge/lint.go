package knowledge

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LintIssue represents a single problem detected by the lint engine.
type LintIssue struct {
	Type        string `json:"type"`        // "orphan", "stale", "empty", "missing_frontmatter"
	Page        string `json:"page"`        // wiki page name
	Description string `json:"description"` // human-readable description
	Severity    string `json:"severity"`    // "info", "warning", "error"
}

// LintResult holds the complete output of a lint run.
type LintResult struct {
	RunAt    time.Time   `json:"run_at"`
	Duration string      `json:"duration"`
	Issues   []LintIssue `json:"issues"`
	Stats    LintStats   `json:"stats"`
}

// LintStats provides aggregate counts by issue type.
type LintStats struct {
	TotalPages        int `json:"total_pages"`
	OrphanPages       int `json:"orphan_pages"`
	StalePages        int `json:"stale_pages"`
	EmptyPages        int `json:"empty_pages"`
	MissingFrontmatter int `json:"missing_frontmatter"`
}

// LintEngine performs health checks on wiki compiled pages.
type LintEngine struct {
	mu         sync.Mutex
	wiki       *WikiManager
	lastResult *LintResult
	staleDays  int // pages not updated within this many days are flagged stale
}

// NewLintEngine creates a LintEngine. staleDays defaults to 30 if <= 0.
func NewLintEngine(wiki *WikiManager, staleDays int) *LintEngine {
	if staleDays <= 0 {
		staleDays = 30
	}
	return &LintEngine{
		wiki:      wiki,
		staleDays: staleDays,
	}
}

// RunLint scans all wiki pages and returns issues found.
// Issues checked:
//   - orphan: no incoming wikilinks from other pages
//   - stale: compiled_at older than staleDays
//   - empty: page has no meaningful content
//   - missing_frontmatter: page has no YAML frontmatter or missing title
func (le *LintEngine) RunLint() LintResult {
	le.mu.Lock()
	defer le.mu.Unlock()

	start := time.Now()
	result := LintResult{
		RunAt:  start,
		Issues: make([]LintIssue, 0),
	}

	if le.wiki == nil {
		result.Duration = time.Since(start).String()
		le.lastResult = &result
		return result
	}

	pages, err := le.wiki.ListPages()
	if err != nil {
		slog.Warn("lint: list pages failed", "err", err)
		result.Duration = time.Since(start).String()
		le.lastResult = &result
		return result
	}

	result.Stats.TotalPages = len(pages)

	if len(pages) == 0 {
		result.Duration = time.Since(start).String()
		le.lastResult = &result
		return result
	}

	// Load all page contents for cross-referencing wikilinks.
	pageNames := make(map[string]bool, len(pages))
	pageContents := make(map[string][]byte, len(pages))
	for _, p := range pages {
		pageNames[p.Name] = true
		data, readErr := os.ReadFile(filepath.Join(le.wiki.Dir(), p.Name+".md"))
		if readErr == nil {
			pageContents[p.Name] = data
		}
	}

	// Build incoming link map: target -> set of pages that link to it.
	incomingLinks := make(map[string]map[string]bool)
	for name, content := range pageContents {
		links := extractWikilinksFromContent(content)
		for _, link := range links {
			// Normalize link target: lowercase, strip .md extension.
			target := normalizeWikiTarget(link)
			if _, ok := incomingLinks[target]; !ok {
				incomingLinks[target] = make(map[string]bool)
			}
			incomingLinks[target][name] = true
		}
	}

	// Check each page for issues.
	for _, page := range pages {
		// Check orphan: no incoming wikilinks.
		result.Issues = append(result.Issues, le.checkOrphan(page, incomingLinks)...)

		// Check stale: compiled_at too old.
		result.Issues = append(result.Issues, le.checkStale(page)...)

		// Check empty: no meaningful content.
		result.Issues = append(result.Issues, le.checkEmpty(page, pageContents[page.Name])...)

		// Check missing frontmatter.
		result.Issues = append(result.Issues, le.checkFrontmatter(page)...)
	}

	// Compute stats from issues.
	for _, issue := range result.Issues {
		switch issue.Type {
		case "orphan":
			result.Stats.OrphanPages++
		case "stale":
			result.Stats.StalePages++
		case "empty":
			result.Stats.EmptyPages++
		case "missing_frontmatter":
			result.Stats.MissingFrontmatter++
		}
	}

	result.Duration = time.Since(start).String()
	le.lastResult = &result

	slog.Info("lint: completed",
		"pages", result.Stats.TotalPages,
		"issues", len(result.Issues),
		"duration", result.Duration,
	)
	return result
}

// LastResult returns the most recent lint result, or nil if lint has not run.
func (le *LintEngine) LastResult() *LintResult {
	le.mu.Lock()
	defer le.mu.Unlock()
	return le.lastResult
}

// checkOrphan returns an issue if the page has no incoming wikilinks from other pages.
// Pages named "index" or "CLAUDE" are exempt since they serve special roles.
func (le *LintEngine) checkOrphan(page WikiPage, incomingLinks map[string]map[string]bool) []LintIssue {
	nameLower := strings.ToLower(page.Name)
	if nameLower == "index" || nameLower == "claude" {
		return nil
	}

	target := normalizeWikiTarget(page.Name)
	linkers := incomingLinks[target]
	if len(linkers) == 0 {
		return []LintIssue{{
			Type:        "orphan",
			Page:        page.Name,
			Description: fmt.Sprintf("Page %q has no incoming wikilinks from other pages", page.Name),
			Severity:    "warning",
		}}
	}
	return nil
}

// checkStale returns an issue if compiled_at is older than staleDays.
func (le *LintEngine) checkStale(page WikiPage) []LintIssue {
	if page.CompiledAt == "" {
		return nil // missing compiled_at is caught by checkFrontmatter
	}

	compiled, err := time.Parse("2006-01-02T15:04:05Z", page.CompiledAt)
	if err != nil {
		// Try alternate format.
		compiled, err = time.Parse(time.RFC3339, page.CompiledAt)
		if err != nil {
			return nil
		}
	}

	age := time.Since(compiled)
	threshold := time.Duration(le.staleDays) * 24 * time.Hour
	if age > threshold {
		days := int(age.Hours() / 24)
		return []LintIssue{{
			Type:        "stale",
			Page:        page.Name,
			Description: fmt.Sprintf("Page %q was compiled %d days ago (threshold: %d days)", page.Name, days, le.staleDays),
			Severity:    "warning",
		}}
	}
	return nil
}

// checkEmpty returns an issue if the page content is trivially small.
func (le *LintEngine) checkEmpty(page WikiPage, content []byte) []LintIssue {
	if content == nil {
		return nil
	}

	// Strip frontmatter before checking content length.
	body := stripFrontmatter(content)
	trimmed := strings.TrimSpace(string(body))

	if len(trimmed) < 10 {
		return []LintIssue{{
			Type:        "empty",
			Page:        page.Name,
			Description: fmt.Sprintf("Page %q has no meaningful content (%d chars)", page.Name, len(trimmed)),
			Severity:    "error",
		}}
	}
	return nil
}

// checkFrontmatter returns an issue if the page has no YAML frontmatter or is
// missing required fields (title, compiled_at).
func (le *LintEngine) checkFrontmatter(page WikiPage) []LintIssue {
	if page.Title == "" && page.CompiledAt == "" {
		return []LintIssue{{
			Type:        "missing_frontmatter",
			Page:        page.Name,
			Description: fmt.Sprintf("Page %q has no frontmatter (missing title and compiled_at)", page.Name),
			Severity:    "error",
		}}
	}
	if page.CompiledAt == "" {
		return []LintIssue{{
			Type:        "missing_frontmatter",
			Page:        page.Name,
			Description: fmt.Sprintf("Page %q is missing compiled_at in frontmatter", page.Name),
			Severity:    "warning",
		}}
	}
	if page.Title == "" {
		return []LintIssue{{
			Type:        "missing_frontmatter",
			Page:        page.Name,
			Description: fmt.Sprintf("Page %q is missing a title in frontmatter", page.Name),
			Severity:    "info",
		}}
	}
	return nil
}

// wikilinkPattern matches [[target]] and [[target|display]] wikilinks.
var wikilinkPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

// extractWikilinksFromContent finds all [[wikilink]] references in markdown content.
func extractWikilinksFromContent(content []byte) []string {
	matches := wikilinkPattern.FindAllSubmatch(content, -1)
	seen := make(map[string]bool)
	var links []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		target := strings.TrimSpace(string(m[1]))
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		links = append(links, target)
	}
	return links
}

// normalizeWikiTarget normalizes a wiki page name or link target for comparison.
// Lowercases the name and strips .md extension.
func normalizeWikiTarget(name string) string {
	name = strings.TrimSuffix(name, ".md")
	return strings.ToLower(strings.TrimSpace(name))
}

// stripFrontmatter removes YAML frontmatter delimited by --- from content.
func stripFrontmatter(data []byte) []byte {
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		return data
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		return data
	}
	return []byte(strings.TrimSpace(s[4+end+4:]))
}
