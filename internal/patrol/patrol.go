package patrol

import (
	"crypto/rand"
	"fmt"
	"time"

	robfigcron "github.com/robfig/cron/v3"
)

// State represents the lifecycle state of a patrol.
type State string

const (
	StateActive   State = "active"
	StatePaused   State = "paused"
	StateDisabled State = "disabled"
	StateRunning  State = "running"
)

// RunStatus classifies the outcome of a patrol execution.
type RunStatus string

const (
	RunOK    RunStatus = "ok"
	RunWarn  RunStatus = "warn"
	RunError RunStatus = "error"
)

// cronParser matches the cron package parser for schedule compatibility.
var cronParser = robfigcron.NewParser(
	robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow | robfigcron.Descriptor,
)

// Patrol defines a recurring autonomous agent task.
type Patrol struct {
	// Configuration fields (from config.yaml)
	Name             string   `json:"name" yaml:"name"`
	Agent            string   `json:"agent" yaml:"agent"`
	Model            string   `json:"model,omitempty" yaml:"model,omitempty"`
	Schedule         string   `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	Trigger          string   `json:"trigger,omitempty" yaml:"trigger,omitempty"`
	Prompt           string   `json:"prompt" yaml:"prompt"`
	Timeout          string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	NotifyTargets    []string `json:"notify,omitempty" yaml:"notify,omitempty"`
	ApprovalRequired bool     `json:"approval_required,omitempty" yaml:"approval_required,omitempty"`
	AutoFix          bool     `json:"auto_fix,omitempty" yaml:"auto_fix,omitempty"`
	Budget           float64  `json:"budget,omitempty" yaml:"budget,omitempty"`
	MCPServers       []string `json:"mcp_servers,omitempty" yaml:"mcp_servers,omitempty"`
	WorkDir          string   `json:"work_dir,omitempty" yaml:"work_dir,omitempty"`
	WebhookSecret    string   `json:"webhook_secret,omitempty" yaml:"webhook_secret,omitempty"`

	// Runtime fields (persisted to patrols.json)
	State       State     `json:"state"`
	LastRun     *RunLog   `json:"last_run,omitempty"`
	TotalRuns   int64     `json:"total_runs"`
	TotalErrors int64     `json:"total_errors"`
	TotalCost   float64   `json:"total_cost"`
	CreatedAt   time.Time `json:"created_at"`

	// Runtime only, not persisted
	entryID robfigcron.EntryID `json:"-" yaml:"-"`
}

// RunLog records the result of a single patrol execution.
type RunLog struct {
	ID        string        `json:"id"`
	PatrolName string       `json:"patrol_name"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
	Cost      float64       `json:"cost"`
	Status    RunStatus     `json:"status"`
	Summary   string        `json:"summary"`
	Detail    string        `json:"detail,omitempty"`
	Error     string        `json:"error,omitempty"`
	EventData string        `json:"event_data,omitempty"`
}

// MarshalDuration returns a human-readable duration for JSON display.
func (r *RunLog) DurationString() string {
	return r.Duration.String()
}

// generateID returns a 16-char hex string (8 random bytes).
func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("%x", b)
}

// ValidateTransition checks whether transitioning to newState is legal.
//
// Allowed transitions:
//
//	Active  -> Running  (trigger)
//	Running -> Active   (execution complete)
//	Active  -> Paused   (user pause)
//	Running -> Paused   (deferred pause)
//	Paused  -> Active   (user resume)
//	Any     -> Disabled (user disable)
func (p *Patrol) ValidateTransition(newState State) error {
	switch newState {
	case StateRunning:
		if p.State != StateActive {
			return fmt.Errorf("cannot transition from %s to running", p.State)
		}
	case StateActive:
		if p.State != StateRunning && p.State != StatePaused {
			return fmt.Errorf("cannot transition from %s to active", p.State)
		}
	case StatePaused:
		if p.State != StateActive && p.State != StateRunning {
			return fmt.Errorf("cannot transition from %s to paused", p.State)
		}
	case StateDisabled:
		// Any state can transition to disabled
	default:
		return fmt.Errorf("unknown state: %s", newState)
	}
	return nil
}

// CanRun returns true if the patrol is in a state that allows execution.
func (p *Patrol) CanRun() bool {
	return p.State == StateActive
}

// Pause transitions the patrol to the paused state.
func (p *Patrol) Pause() error {
	if err := p.ValidateTransition(StatePaused); err != nil {
		return err
	}
	p.State = StatePaused
	return nil
}

// Resume transitions the patrol from paused to active.
func (p *Patrol) Resume() error {
	if err := p.ValidateTransition(StateActive); err != nil {
		return err
	}
	p.State = StateActive
	return nil
}

// Disable transitions the patrol to the disabled state.
func (p *Patrol) Disable() error {
	if err := p.ValidateTransition(StateDisabled); err != nil {
		return err
	}
	p.State = StateDisabled
	return nil
}

// ParseTimeout returns the configured timeout duration, defaulting to 5 minutes.
func (p *Patrol) ParseTimeout() time.Duration {
	if p.Timeout == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(p.Timeout)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// ValidateSchedule checks if the patrol's schedule expression is valid.
func (p *Patrol) ValidateSchedule() error {
	if p.Schedule == "" {
		return nil
	}
	_, err := cronParser.Parse(p.Schedule)
	return err
}
