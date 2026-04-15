package patrol

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	robfigcron "github.com/robfig/cron/v3"

	"github.com/naozhi/naozhi/internal/platform"
	"github.com/naozhi/naozhi/internal/session"
)

// HubBroadcaster is an optional interface for pushing real-time events.
type HubBroadcaster interface {
	Broadcast(v any)
}

// ManagerConfig holds configuration for the patrol manager.
type ManagerConfig struct {
	Router    *session.Router
	Platforms map[string]platform.Platform
	Agents    map[string]session.AgentOpts
	StorePath string
	LogDir    string
}

// Manager is the patrol subsystem coordinator.
// It owns the patrol registry, cron scheduler, and execution engine.
type Manager struct {
	mu        sync.RWMutex
	patrols   map[string]*Patrol
	router    *session.Router
	agents    map[string]session.AgentOpts
	platforms map[string]platform.Platform
	hub       HubBroadcaster
	logDir    string
	cron      *robfigcron.Cron
	storePath string
	stopCtx   context.Context
	stopFn    context.CancelFunc
}

// NewManager creates a patrol manager, loads config, and restores persisted state.
func NewManager(cfg ManagerConfig, configPatrols map[string]*Patrol) *Manager {
	stopCtx, stopFn := context.WithCancel(context.Background())
	m := &Manager{
		patrols:   make(map[string]*Patrol),
		router:    cfg.Router,
		agents:    cfg.Agents,
		platforms: cfg.Platforms,
		logDir:    cfg.LogDir,
		storePath: cfg.StorePath,
		cron:      robfigcron.New(robfigcron.WithChain(robfigcron.SkipIfStillRunning(robfigcron.DefaultLogger))),
		stopCtx:   stopCtx,
		stopFn:    stopFn,
	}

	// Merge config with persisted state
	stored := loadPatrols(cfg.StorePath)
	m.patrols = mergeConfig(configPatrols, stored)

	return m
}

// SetHub sets the WebSocket broadcaster for real-time event push.
func (m *Manager) SetHub(hub HubBroadcaster) {
	m.mu.Lock()
	m.hub = hub
	m.mu.Unlock()
}

// Start registers scheduled patrols with the cron scheduler and starts it.
func (m *Manager) Start() {
	m.mu.Lock()
	for name, p := range m.patrols {
		if p.Schedule != "" && p.State == StateActive {
			if err := m.registerSchedule(name, p); err != nil {
				slog.Warn("skip invalid patrol schedule", "name", name, "schedule", p.Schedule, "err", err)
			}
		}
	}
	m.mu.Unlock()
	m.cron.Start()
	slog.Info("patrol manager started", "patrols", len(m.patrols))
}

// Stop halts the cron scheduler and saves state.
func (m *Manager) Stop() {
	m.stopFn()
	ctx := m.cron.Stop()
	<-ctx.Done()
	m.mu.Lock()
	snap := m.snapshotPatrols()
	m.mu.Unlock()
	if err := savePatrols(m.storePath, snap); err != nil {
		slog.Error("save patrol store on shutdown", "err", err)
	}
}

// registerSchedule adds a patrol's cron schedule to the scheduler.
func (m *Manager) registerSchedule(name string, p *Patrol) error {
	patrolName := name
	entryID, err := m.cron.AddFunc(p.Schedule, func() {
		m.Execute(m.stopCtx, patrolName, nil)
	})
	if err != nil {
		return err
	}
	p.entryID = entryID
	return nil
}

// Execute runs a patrol: validates state, spawns session, sends prompt, records result.
func (m *Manager) Execute(ctx context.Context, name string, eventPayload json.RawMessage) error {
	m.mu.Lock()
	p, ok := m.patrols[name]
	if !ok {
		m.mu.Unlock()
		return errPatrolNotFound(name)
	}
	if !p.CanRun() {
		m.mu.Unlock()
		return errPatrolNotRunnable(name, p.State)
	}
	p.State = StateRunning
	snap := m.snapshotPatrols()
	m.mu.Unlock()
	_ = savePatrols(m.storePath, snap)

	log := slog.With("patrol", name)
	log.Info("patrol executing")

	start := time.Now()

	// Build execution prompt (inject event data if webhook-triggered)
	prompt := p.Prompt
	if len(eventPayload) > 0 {
		prompt = buildEventPrompt(p, eventPayload)
	}

	// Resolve timeout
	timeout := p.ParseTimeout()
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Session key: patrol:{name}, exempt from maxProcs
	key := "patrol:" + name
	opts := m.agents[p.Agent]
	opts.Exempt = true
	if p.Model != "" {
		opts.Model = p.Model
	}
	if p.WorkDir != "" {
		opts.Workspace = p.WorkDir
	}

	sess, _, err := m.router.GetOrCreate(execCtx, key, opts)
	if err != nil {
		rl := m.recordError(p, start, "session error: "+err.Error(), eventPayload)
		m.writeLog(name, rl)
		m.restoreActive(p)
		return err
	}

	result, err := sess.Send(execCtx, prompt, nil, nil)
	duration := time.Since(start)
	if err != nil {
		rl := m.recordError(p, start, "send error: "+err.Error(), eventPayload)
		m.writeLog(name, rl)
		m.restoreActive(p)
		return err
	}

	// Classify and record result
	status := classifyResult(result.Text)
	summary := extractSummary(result.Text)

	rl := &RunLog{
		ID:         generateID(),
		PatrolName: name,
		Timestamp:  start,
		Duration:   duration,
		Cost:       result.CostUSD,
		Status:     status,
		Summary:    summary,
		Detail:     result.Text,
	}
	if len(eventPayload) > 0 {
		rl.EventData = string(eventPayload)
	}

	m.mu.Lock()
	p.State = StateActive
	p.LastRun = rl
	p.TotalRuns++
	p.TotalCost += result.CostUSD
	if status == RunError {
		p.TotalErrors++
	}
	snap = m.snapshotPatrols()
	m.mu.Unlock()
	_ = savePatrols(m.storePath, snap)

	m.writeLog(name, rl)

	// Broadcast patrol event via WebSocket
	m.mu.RLock()
	hub := m.hub
	m.mu.RUnlock()
	if hub != nil {
		hub.Broadcast(map[string]any{
			"type":    "patrol_event",
			"patrol":  name,
			"status":  string(status),
			"summary": summary,
			"run_id":  rl.ID,
			"time":    time.Now().UnixMilli(),
		})
	}

	log.Info("patrol completed", "status", status, "duration", duration)
	return nil
}

// recordError creates a RunLog for a failed execution and updates patrol stats.
func (m *Manager) recordError(p *Patrol, start time.Time, errMsg string, eventPayload json.RawMessage) *RunLog {
	rl := &RunLog{
		ID:         generateID(),
		PatrolName: p.Name,
		Timestamp:  start,
		Duration:   time.Since(start),
		Status:     RunError,
		Summary:    errMsg,
		Error:      errMsg,
	}
	if len(eventPayload) > 0 {
		rl.EventData = string(eventPayload)
	}

	m.mu.Lock()
	p.LastRun = rl
	p.TotalRuns++
	p.TotalErrors++
	m.mu.Unlock()

	return rl
}

// restoreActive sets the patrol back to Active state after execution.
func (m *Manager) restoreActive(p *Patrol) {
	m.mu.Lock()
	if p.State == StateRunning {
		p.State = StateActive
	}
	snap := m.snapshotPatrols()
	m.mu.Unlock()
	_ = savePatrols(m.storePath, snap)
}

// writeLog appends a run log entry to the patrol's JSONL log file.
func (m *Manager) writeLog(name string, rl *RunLog) {
	if m.logDir == "" {
		return
	}
	lw, err := NewLogWriter(m.logDir, name)
	if err != nil {
		slog.Warn("patrol log writer", "patrol", name, "err", err)
		return
	}
	defer lw.Close()
	if err := lw.Append(rl); err != nil {
		slog.Warn("patrol log append", "patrol", name, "err", err)
	}
}

// buildEventPrompt prepends webhook event data to the patrol prompt.
func buildEventPrompt(p *Patrol, payload json.RawMessage) string {
	var b strings.Builder
	b.WriteString("[Event: ")
	b.WriteString(p.Trigger)
	b.WriteString("]\nPayload:\n```json\n")
	b.Write(payload)
	b.WriteString("\n```\n\n")
	b.WriteString(p.Prompt)
	return b.String()
}

// classifyResult determines the RunStatus based on keywords in the result text.
func classifyResult(text string) RunStatus {
	lower := strings.ToLower(text)
	for _, kw := range []string{"error", "fail", "critical", "exception", "panic"} {
		if strings.Contains(lower, kw) {
			return RunError
		}
	}
	for _, kw := range []string{"warn", "alert", "异常", "attention", "caution"} {
		if strings.Contains(lower, kw) {
			return RunWarn
		}
	}
	return RunOK
}

// extractSummary returns the first 200 characters of text as a summary.
func extractSummary(text string) string {
	// Trim leading whitespace and take first line or 200 chars
	text = strings.TrimSpace(text)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 && idx < 200 {
		return text[:idx]
	}
	if len(text) > 200 {
		return text[:200]
	}
	return text
}

// GetPatrol returns a patrol by name (read-only copy).
func (m *Manager) GetPatrol(name string) (*Patrol, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.patrols[name]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

// ListPatrols returns all patrols as value copies.
func (m *Manager) ListPatrols() map[string]Patrol {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]Patrol, len(m.patrols))
	for k, v := range m.patrols {
		result[k] = *v
	}
	return result
}

// SetState changes a patrol's state with validation.
func (m *Manager) SetState(name string, newState State) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.patrols[name]
	if !ok {
		return errPatrolNotFound(name)
	}

	if err := p.ValidateTransition(newState); err != nil {
		return err
	}

	oldState := p.State
	p.State = newState

	// Handle schedule registration/removal
	if newState == StateActive && p.Schedule != "" && p.entryID == 0 {
		if err := m.registerSchedule(name, p); err != nil {
			p.State = oldState // rollback
			return err
		}
	} else if (newState == StatePaused || newState == StateDisabled) && p.entryID != 0 {
		m.cron.Remove(p.entryID)
		p.entryID = 0
	}

	snap := m.snapshotPatrols()
	go func() { _ = savePatrols(m.storePath, snap) }()
	return nil
}

// snapshotPatrols returns a deep copy of the patrol map. Caller must hold mu.
func (m *Manager) snapshotPatrols() map[string]*Patrol {
	snap := make(map[string]*Patrol, len(m.patrols))
	for k, v := range m.patrols {
		cp := *v
		cp.entryID = 0
		snap[k] = &cp
	}
	return snap
}

// ReadLogs reads the last N log entries for a patrol.
func (m *Manager) ReadLogs(name string, limit int) ([]*RunLog, error) {
	if m.logDir == "" {
		return nil, nil
	}
	lw, err := NewLogWriter(m.logDir, name)
	if err != nil {
		return nil, err
	}
	defer lw.Close()
	return lw.ReadTail(limit)
}

// ReadLogByID finds a specific log entry by run ID.
func (m *Manager) ReadLogByID(name, runID string) (*RunLog, error) {
	logs, err := m.ReadLogs(name, 100) // scan recent logs
	if err != nil {
		return nil, err
	}
	for _, rl := range logs {
		if rl.ID == runID {
			return rl, nil
		}
	}
	return nil, nil
}
