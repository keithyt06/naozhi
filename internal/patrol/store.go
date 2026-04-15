package patrol

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// PatrolState holds only the runtime fields persisted to patrols.json.
// Config fields (prompt, schedule, agent, etc.) come from config.yaml.
type PatrolState struct {
	State       State     `json:"state"`
	LastRun     *RunLog   `json:"last_run,omitempty"`
	TotalRuns   int64     `json:"total_runs"`
	TotalErrors int64     `json:"total_errors"`
	TotalCost   float64   `json:"total_cost"`
	CreatedAt   time.Time `json:"created_at"`
}

// savePatrols writes patrol runtime states atomically (write tmp + rename).
func savePatrols(path string, patrols map[string]*Patrol) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create patrol store directory: %w", err)
		}
	}

	// Extract only runtime state for persistence
	states := make(map[string]*PatrolState, len(patrols))
	for name, p := range patrols {
		states[name] = &PatrolState{
			State:       p.State,
			LastRun:     p.LastRun,
			TotalRuns:   p.TotalRuns,
			TotalErrors: p.TotalErrors,
			TotalCost:   p.TotalCost,
			CreatedAt:   p.CreatedAt,
		}
	}

	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: write to temp file, then rename
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// loadPatrols reads persisted runtime states from patrols.json.
func loadPatrols(path string) map[string]*PatrolState {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("load patrol store failed", "err", err)
		}
		return nil
	}

	var states map[string]*PatrolState
	if err := json.Unmarshal(data, &states); err != nil {
		slog.Warn("parse patrol store failed", "err", err)
		return nil
	}
	slog.Info("loaded patrol store", "count", len(states), "path", path)
	return states
}

// mergeConfig merges config-defined patrols with persisted runtime state.
// New patrols get initial Active state. Deleted patrols are dropped.
// Existing patrols keep their runtime fields (state, stats).
func mergeConfig(configPatrols map[string]*Patrol, stored map[string]*PatrolState) map[string]*Patrol {
	result := make(map[string]*Patrol, len(configPatrols))
	for name, p := range configPatrols {
		p.Name = name
		if st, ok := stored[name]; ok {
			// Restore runtime state
			p.State = st.State
			p.LastRun = st.LastRun
			p.TotalRuns = st.TotalRuns
			p.TotalErrors = st.TotalErrors
			p.TotalCost = st.TotalCost
			p.CreatedAt = st.CreatedAt
		} else {
			// New patrol
			p.State = StateActive
			p.CreatedAt = time.Now()
		}
		result[name] = p
	}
	return result
}
