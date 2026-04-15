package knowledge

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Meeting represents a processed meeting record.
type Meeting struct {
	ID             string       `json:"id"`
	Title          string       `json:"title"`
	Date           time.Time    `json:"date"`
	Duration       string       `json:"duration,omitempty"`
	Participants   []string     `json:"participants,omitempty"`
	Summary        string       `json:"summary,omitempty"`
	Decisions      []string     `json:"decisions,omitempty"`
	ActionItems    []ActionItem `json:"action_items,omitempty"`
	AudioPath      string       `json:"audio_path,omitempty"`
	TranscriptPath string       `json:"transcript_path,omitempty"`
	Transcript     string       `json:"transcript,omitempty"`
	Status         string       `json:"status"` // pending, processing, completed, failed
	Error          string       `json:"error,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
}

// ActionItem is a task extracted from a meeting.
type ActionItem struct {
	Description string `json:"description"`
	Assignee    string `json:"assignee,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	Done        bool   `json:"done"`
}

// MeetingStore manages meetings with JSON file persistence (~/.naozhi/meetings.json).
type MeetingStore struct {
	mu       sync.RWMutex
	meetings []*Meeting
	filePath string
}

// NewMeetingStore creates a MeetingStore, loading existing data from disk.
func NewMeetingStore(filePath string) *MeetingStore {
	ms := &MeetingStore{
		filePath: filePath,
		meetings: make([]*Meeting, 0),
	}
	if err := ms.load(); err != nil {
		slog.Warn("load meeting store failed", "err", err, "path", filePath)
	}
	return ms
}

// Add inserts a new meeting. An ID and CreatedAt are assigned if empty.
func (ms *MeetingStore) Add(m *Meeting) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if m.ID == "" {
		id, err := randomHexID(8)
		if err != nil {
			return fmt.Errorf("generate meeting id: %w", err)
		}
		m.ID = id
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.Status == "" {
		m.Status = "pending"
	}

	ms.meetings = append(ms.meetings, m)
	return ms.saveLocked()
}

// Update replaces a meeting in-place by ID.
func (ms *MeetingStore) Update(m *Meeting) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for i, existing := range ms.meetings {
		if existing.ID == m.ID {
			ms.meetings[i] = m
			return ms.saveLocked()
		}
	}
	return fmt.Errorf("meeting %s not found", m.ID)
}

// Get returns a meeting by ID.
func (ms *MeetingStore) Get(id string) (*Meeting, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	for _, m := range ms.meetings {
		if m.ID == id {
			cp := *m
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("meeting %s not found", id)
}

// List returns all meetings sorted by date descending (newest first).
func (ms *MeetingStore) List() []Meeting {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	result := make([]Meeting, 0, len(ms.meetings))
	for _, m := range ms.meetings {
		result = append(result, *m)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date.After(result[j].Date)
	})
	return result
}

// load reads meetings from the JSON file on disk.
func (ms *MeetingStore) load() error {
	if ms.filePath == "" {
		return nil
	}
	data, err := os.ReadFile(ms.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read meeting store: %w", err)
	}
	var entries []*Meeting
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse meeting store: %w", err)
	}
	ms.meetings = entries
	slog.Info("loaded meeting store", "count", len(entries), "path", ms.filePath)
	return nil
}

// saveLocked persists meetings to disk atomically. Must be called with mu held.
func (ms *MeetingStore) saveLocked() error {
	if ms.filePath == "" {
		return nil
	}
	if dir := filepath.Dir(ms.filePath); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create meeting store directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(ms.meetings, "", "  ")
	if err != nil {
		return err
	}
	tmp := ms.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, ms.filePath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
