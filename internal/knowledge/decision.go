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

// Decision represents an Architecture Decision Record (ADR).
type Decision struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Context      string   `json:"context"`                // background / why this decision was needed
	Decision     string   `json:"decision"`               // what was decided
	Consequences string   `json:"consequences"`           // impact / trade-offs
	Tags         []string `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
	Source       string   `json:"source,omitempty"`       // e.g. session key or "manual"
}

// DecisionStore manages ADR CRUD with JSON file persistence at ~/.naozhi/decisions.json.
type DecisionStore struct {
	mu        sync.RWMutex
	decisions []Decision
	path      string
}

// NewDecisionStore creates a DecisionStore and loads existing data from path.
func NewDecisionStore(path string) *DecisionStore {
	ds := &DecisionStore{
		path:      path,
		decisions: make([]Decision, 0),
	}
	if err := ds.load(); err != nil {
		slog.Warn("load decision store failed", "err", err, "path", path)
	}
	return ds
}

// load reads decisions from JSON file into memory.
func (ds *DecisionStore) load() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.path == "" {
		return nil
	}
	data, err := os.ReadFile(ds.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read decision store: %w", err)
	}
	var entries []Decision
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse decision store: %w", err)
	}
	ds.decisions = entries
	slog.Info("loaded decision store", "count", len(entries), "path", ds.path)
	return nil
}

// save persists decisions to disk via atomic write.
func (ds *DecisionStore) save() error {
	ds.mu.RLock()
	entries := make([]Decision, len(ds.decisions))
	copy(entries, ds.decisions)
	ds.mu.RUnlock()

	if ds.path == "" {
		return nil
	}
	if dir := filepath.Dir(ds.path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create decision store directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal decisions: %w", err)
	}

	tmp := ds.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write decision tmp file: %w", err)
	}
	if err := os.Rename(tmp, ds.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename decision tmp file: %w", err)
	}
	return nil
}

// Add appends a decision. If d.ID is empty, a random hex ID is generated.
func (ds *DecisionStore) Add(d Decision) (Decision, error) {
	if d.ID == "" {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return Decision{}, fmt.Errorf("generate decision id: %w", err)
		}
		d.ID = hex.EncodeToString(b)
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}

	ds.mu.Lock()
	ds.decisions = append(ds.decisions, d)
	ds.mu.Unlock()

	if err := ds.save(); err != nil {
		return Decision{}, err
	}
	return d, nil
}

// List returns all decisions sorted by CreatedAt descending.
func (ds *DecisionStore) List() []Decision {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	result := make([]Decision, len(ds.decisions))
	copy(result, ds.decisions)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// Get returns a single decision by ID, or an error if not found.
func (ds *DecisionStore) Get(id string) (Decision, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	for _, d := range ds.decisions {
		if d.ID == id {
			return d, nil
		}
	}
	return Decision{}, fmt.Errorf("decision not found: %s", id)
}

// Search returns decisions matching query in Title, Context, Decision, or Tags
// (case-insensitive substring). Results sorted newest first.
func (ds *DecisionStore) Search(query string) []Decision {
	if query == "" {
		return ds.List()
	}
	q := strings.ToLower(query)

	ds.mu.RLock()
	defer ds.mu.RUnlock()

	var result []Decision
	for _, d := range ds.decisions {
		if strings.Contains(strings.ToLower(d.Title), q) ||
			strings.Contains(strings.ToLower(d.Context), q) ||
			strings.Contains(strings.ToLower(d.Decision), q) ||
			strings.Contains(strings.ToLower(d.Consequences), q) {
			result = append(result, d)
			continue
		}
		for _, tag := range d.Tags {
			if strings.Contains(strings.ToLower(tag), q) {
				result = append(result, d)
				break
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}
