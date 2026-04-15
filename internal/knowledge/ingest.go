package knowledge

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// IngestEngine populates the SearchEngine with documents from all knowledge sources:
// vault .md files, wiki compiled pages, CLI history prompts, and bookmarks.
// This is a simple indexing pass — LLM-based compilation is a future enhancement.
type IngestEngine struct {
	mu     sync.Mutex
	wiki   *WikiManager
	vault  *Vault
	search *SearchEngine
}

// NewIngestEngine creates an IngestEngine that can populate the search index
// from wiki, vault, and bookmark sources.
func NewIngestEngine(wiki *WikiManager, vault *Vault, search *SearchEngine) *IngestEngine {
	return &IngestEngine{
		wiki:   wiki,
		vault:  vault,
		search: search,
	}
}

// IngestFromVault scans all .md files in the Obsidian vault and indexes them
// into the SearchEngine with source "vault".
func (ie *IngestEngine) IngestFromVault() error {
	ie.mu.Lock()
	defer ie.mu.Unlock()

	if ie.vault == nil || !ie.vault.Configured() {
		return fmt.Errorf("vault not configured")
	}
	if ie.search == nil {
		return fmt.Errorf("search engine not initialized")
	}

	count := 0
	err := ie.vault.WalkMdFiles(func(relPath string, info fs.FileInfo) error {
		data, readErr := ie.vault.ReadFile(relPath)
		if readErr != nil {
			slog.Debug("ingest: skip vault file", "path", relPath, "err", readErr)
			return nil
		}

		title := extractTitleFromContent(string(data))
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(relPath), ".md")
		}

		doc := SearchDocument{
			ID:        "vault:" + relPath,
			Source:    "vault",
			Title:     title,
			Text:      string(data),
			Timestamp: info.ModTime(),
			Meta:      relPath,
		}
		ie.search.IndexDocument(doc)
		count++
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk vault files: %w", err)
	}

	slog.Info("ingest: indexed vault files", "count", count)
	return nil
}

// IngestFromHistory reads a CLI history JSONL file and indexes user prompts
// into the SearchEngine with source "cli".
// historyPath should point to a directory containing .jsonl files
// (e.g. ~/.claude/projects/*/).
func (ie *IngestEngine) IngestFromHistory(historyPath string) error {
	ie.mu.Lock()
	defer ie.mu.Unlock()

	if ie.search == nil {
		return fmt.Errorf("search engine not initialized")
	}

	entries, err := os.ReadDir(historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read history dir: %w", err)
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}

		filePath := filepath.Join(historyPath, e.Name())
		sessionID := strings.TrimSuffix(e.Name(), ".jsonl")

		prompts, parseErr := parseHistoryPrompts(filePath)
		if parseErr != nil {
			slog.Debug("ingest: skip history file", "path", filePath, "err", parseErr)
			continue
		}

		for i, prompt := range prompts {
			doc := SearchDocument{
				ID:        fmt.Sprintf("cli:%s:%d", sessionID, i),
				Source:    "cli",
				Title:     truncate(prompt, 80),
				Text:      prompt,
				Timestamp: info.ModTime(),
				Meta:      sessionID,
			}
			ie.search.IndexDocument(doc)
			count++
		}
	}

	slog.Info("ingest: indexed cli history", "path", historyPath, "prompts", count)
	return nil
}

// IngestWikiPages indexes all wiki compiled pages into the SearchEngine
// with source "wiki".
func (ie *IngestEngine) IngestWikiPages() error {
	ie.mu.Lock()
	defer ie.mu.Unlock()

	if ie.wiki == nil {
		return fmt.Errorf("wiki manager not initialized")
	}
	if ie.search == nil {
		return fmt.Errorf("search engine not initialized")
	}

	pages, err := ie.wiki.ListPages()
	if err != nil {
		return fmt.Errorf("list wiki pages: %w", err)
	}

	count := 0
	for _, page := range pages {
		full, readErr := ie.wiki.ReadPage(page.Name)
		if readErr != nil {
			slog.Debug("ingest: skip wiki page", "name", page.Name, "err", readErr)
			continue
		}

		compiledAt, _ := time.Parse("2006-01-02T15:04:05Z", page.CompiledAt)
		if compiledAt.IsZero() {
			compiledAt = time.Now()
		}

		doc := SearchDocument{
			ID:        "wiki:" + page.Name,
			Source:    "wiki",
			Title:     page.Title,
			Text:      full.Content,
			Tags:      page.Entities,
			Timestamp: compiledAt,
			Meta:      page.Name,
		}
		ie.search.IndexDocument(doc)
		count++
	}

	slog.Info("ingest: indexed wiki pages", "count", count)
	return nil
}

// IndexBookmarks indexes all provided bookmarks into the SearchEngine
// with source "bookmarks".
func (ie *IngestEngine) IndexBookmarks(bookmarks []Bookmark) {
	if ie.search == nil {
		return
	}

	for _, bm := range bookmarks {
		doc := SearchDocument{
			ID:        "bookmarks:" + bm.ID,
			Source:    "bookmarks",
			Title:     truncate(bm.Text, 80),
			Text:      bm.Text,
			Tags:      bm.Tags,
			Timestamp: bm.CreatedAt,
			Meta:      bm.SessionKey,
		}
		ie.search.IndexDocument(doc)
	}
	slog.Debug("ingest: indexed bookmarks", "count", len(bookmarks))
}

// parseHistoryPrompts reads a JSONL file and extracts all user prompt texts.
func parseHistoryPrompts(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var prompts []string
	scanner := newLineScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		entry, parseErr := parseHistoryLineForPrompt(line)
		if parseErr != nil || entry == "" {
			continue
		}
		prompts = append(prompts, entry)
	}
	return prompts, scanner.Err()
}

// extractTitleFromContent extracts the first H1 heading from markdown content.
func extractTitleFromContent(content string) string {
	for _, line := range strings.SplitN(content, "\n", 20) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

// truncate returns s truncated to maxLen runes with "..." appended if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
