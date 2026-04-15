package knowledge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// HistoryEntry represents a single user prompt extracted from a CLI JSONL file.
type HistoryEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	SessionID   string    `json:"session_id"`
	ProjectPath string    `json:"project_path"`
	Prompt      string    `json:"prompt"`
}

// CLISyncManager periodically scans CLI history JSONL files and indexes
// new user prompts into the SearchEngine. It tracks the last scan time
// to perform incremental scans.
type CLISyncManager struct {
	mu           sync.Mutex
	search       *SearchEngine
	lastScanTime time.Time
}

// NewCLISyncManager creates a CLISyncManager that indexes CLI prompts into
// the given search engine.
func NewCLISyncManager(search *SearchEngine) *CLISyncManager {
	return &CLISyncManager{
		search: search,
	}
}

// ScanHistory reads JSONL files under claudeDir/projects/ that have been
// modified since the last scan. Each user prompt is indexed into the
// SearchEngine with source "cli". Returns the number of new entries indexed.
func (csm *CLISyncManager) ScanHistory(claudeDir string) (int, error) {
	csm.mu.Lock()
	defer csm.mu.Unlock()

	if csm.search == nil {
		return 0, fmt.Errorf("search engine not initialized")
	}

	projectsDir := claudeDir + "/projects"
	projEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read projects dir: %w", err)
	}

	sinceMs := csm.lastScanTime.UnixMilli()
	if csm.lastScanTime.IsZero() {
		sinceMs = 0
	}

	totalIndexed := 0

	for _, projEntry := range projEntries {
		if !projEntry.IsDir() {
			continue
		}
		projDir := projectsDir + "/" + projEntry.Name()
		files, readErr := os.ReadDir(projDir)
		if readErr != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}

			info, infoErr := f.Info()
			if infoErr != nil {
				continue
			}

			// Skip files not modified since last scan (incremental).
			if sinceMs > 0 && info.ModTime().UnixMilli() <= sinceMs {
				continue
			}

			sessionID := strings.TrimSuffix(f.Name(), ".jsonl")
			filePath := projDir + "/" + f.Name()

			entries, parseErr := csm.scanFile(filePath, sessionID, projEntry.Name())
			if parseErr != nil {
				slog.Debug("cli_sync: skip file", "path", filePath, "err", parseErr)
				continue
			}

			for _, entry := range entries {
				doc := SearchDocument{
					ID:        fmt.Sprintf("cli:%s:%d", entry.SessionID, entry.Timestamp.UnixMilli()),
					Source:    "cli",
					Title:     entry.ProjectPath,
					Text:      entry.Prompt,
					Timestamp: entry.Timestamp,
					Meta:      entry.SessionID,
				}
				csm.search.IndexDocument(doc)
				totalIndexed++
			}
		}
	}

	csm.lastScanTime = time.Now()

	if totalIndexed > 0 {
		slog.Debug("cli_sync: indexed new prompts", "count", totalIndexed)
	}
	return totalIndexed, nil
}

// scanFile reads a single JSONL file and extracts user prompts.
func (csm *CLISyncManager) scanFile(path, sessionID, projectName string) ([]HistoryEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 16*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		entry, parseErr := ParseHistoryLine(line)
		if parseErr != nil {
			continue
		}
		entry.SessionID = sessionID
		entry.ProjectPath = projectName
		entries = append(entries, entry)
	}

	return entries, scanner.Err()
}

// ParseHistoryLine parses a single JSONL line and extracts user prompt info.
// Returns an error if the line is not a user message or is malformed.
func ParseHistoryLine(line []byte) (HistoryEntry, error) {
	// Quick pre-check: only parse lines with "type":"user".
	if !jsonContains(line, `"type":"user"`) && !jsonContains(line, `"type": "user"`) {
		return HistoryEntry{}, fmt.Errorf("not a user message")
	}

	var raw struct {
		Type      string          `json:"type"`
		Timestamp string          `json:"timestamp"`
		Message   json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return HistoryEntry{}, fmt.Errorf("unmarshal line: %w", err)
	}
	if raw.Type != "user" {
		return HistoryEntry{}, fmt.Errorf("not a user message")
	}

	prompt := extractPromptText(raw.Message)
	if prompt == "" {
		return HistoryEntry{}, fmt.Errorf("empty prompt")
	}

	ts := parseTimestampStr(raw.Timestamp)

	return HistoryEntry{
		Timestamp: ts,
		Prompt:    prompt,
	}, nil
}

// extractPromptText extracts the user's text from a message JSON blob.
func extractPromptText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil || len(msg.Content) == 0 {
		return ""
	}

	// Try string content first.
	var s string
	if json.Unmarshal(msg.Content, &s) == nil {
		return strings.TrimSpace(s)
	}

	// Try array of blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(msg.Content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				return strings.TrimSpace(b.Text)
			}
		}
	}
	return ""
}

// parseTimestampStr parses an RFC3339 timestamp string.
func parseTimestampStr(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}

// jsonContains is a fast substring check on raw JSON bytes.
func jsonContains(data []byte, substr string) bool {
	return strings.Contains(string(data), substr)
}

// newLineScanner creates a bufio.Scanner suitable for reading JSONL files.
func newLineScanner(f *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 16*1024), 256*1024)
	return scanner
}

// parseHistoryLineForPrompt extracts just the prompt text from a JSONL line.
// Used by IngestEngine's simpler parsing path.
func parseHistoryLineForPrompt(line []byte) (string, error) {
	entry, err := ParseHistoryLine(line)
	if err != nil {
		return "", err
	}
	return entry.Prompt, nil
}
