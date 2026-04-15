package patrol

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ApprovalStatus represents the state of an approval request.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
	ApprovalExpired  ApprovalStatus = "expired"
)

// ApprovalUrgency represents the urgency level of an approval.
type ApprovalUrgency string

const (
	UrgencyUrgent ApprovalUrgency = "urgent"
	UrgencyNormal ApprovalUrgency = "normal"
)

// approvalExpiry is the duration after which a pending approval expires.
const approvalExpiry = 30 * time.Minute

// Approval represents a single approval request created when an agent
// encounters a high-risk operation that requires human confirmation.
type Approval struct {
	ID         string          `json:"id"`
	PatrolName string          `json:"patrol_name"`
	Agent      string          `json:"agent"`
	Action     string          `json:"action"`
	Summary    string          `json:"summary"`
	Detail     string          `json:"detail"`
	Impact     string          `json:"impact"`
	Urgency    ApprovalUrgency `json:"urgency"`
	Status     ApprovalStatus  `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	ResolvedAt *time.Time      `json:"resolved_at,omitempty"`
	ResolvedBy string          `json:"resolved_by,omitempty"`
}

// generateApprovalID returns a prefixed 16-char hex ID.
func generateApprovalID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("appr-%x", b)
}

// ApprovalStore manages approval requests with JSON file persistence.
type ApprovalStore struct {
	mu        sync.RWMutex
	approvals []*Approval
	filePath  string
}

// NewApprovalStore creates a new store, loading any existing data from disk.
func NewApprovalStore(filePath string) *ApprovalStore {
	s := &ApprovalStore{filePath: filePath}
	s.approvals = loadApprovals(filePath)
	if s.approvals == nil {
		s.approvals = make([]*Approval, 0)
	}
	return s
}

// Create adds a new approval request. It assigns an ID, sets the status
// to Pending, and persists the change to disk.
func (s *ApprovalStore) Create(a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	a.ID = generateApprovalID()
	a.Status = ApprovalPending
	a.CreatedAt = time.Now()
	s.approvals = append(s.approvals, a)
	return s.saveLocked()
}

// Approve marks the approval with the given ID as approved.
func (s *ApprovalStore) Approve(id, by string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	a, err := s.findLocked(id)
	if err != nil {
		return err
	}
	if a.Status != ApprovalPending {
		return fmt.Errorf("approval %s is not pending (status: %s)", id, a.Status)
	}
	now := time.Now()
	a.Status = ApprovalApproved
	a.ResolvedAt = &now
	a.ResolvedBy = by
	return s.saveLocked()
}

// Reject marks the approval with the given ID as rejected.
func (s *ApprovalStore) Reject(id, by string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	a, err := s.findLocked(id)
	if err != nil {
		return err
	}
	if a.Status != ApprovalPending {
		return fmt.Errorf("approval %s is not pending (status: %s)", id, a.Status)
	}
	now := time.Now()
	a.Status = ApprovalRejected
	a.ResolvedAt = &now
	a.ResolvedBy = by
	return s.saveLocked()
}

// Get returns a copy of the approval with the given ID.
func (s *ApprovalStore) Get(id string) (Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.approvals {
		if a.ID == id {
			return *a, nil
		}
	}
	return Approval{}, fmt.Errorf("approval %s not found", id)
}

// ListPending returns all pending approvals, auto-expiring stale ones first.
func (s *ApprovalStore) ListPending() []Approval {
	s.mu.Lock()
	changed := s.expireLocked()
	if changed {
		_ = s.saveLocked()
	}
	s.mu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Approval
	for _, a := range s.approvals {
		if a.Status == ApprovalPending {
			result = append(result, *a)
		}
	}
	return result
}

// ListAll returns copies of all approvals.
func (s *ApprovalStore) ListAll() []Approval {
	s.mu.Lock()
	changed := s.expireLocked()
	if changed {
		_ = s.saveLocked()
	}
	s.mu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Approval, 0, len(s.approvals))
	for _, a := range s.approvals {
		result = append(result, *a)
	}
	return result
}

// expireLocked marks pending approvals older than approvalExpiry as expired.
// Must be called with s.mu held (write lock). Returns true if any changed.
func (s *ApprovalStore) expireLocked() bool {
	cutoff := time.Now().Add(-approvalExpiry)
	changed := false
	for _, a := range s.approvals {
		if a.Status == ApprovalPending && a.CreatedAt.Before(cutoff) {
			now := time.Now()
			a.Status = ApprovalExpired
			a.ResolvedAt = &now
			a.ResolvedBy = "system:auto-expire"
			changed = true
		}
	}
	return changed
}

// findLocked returns the approval with the given ID. Must be called with mu held.
func (s *ApprovalStore) findLocked(id string) (*Approval, error) {
	for _, a := range s.approvals {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("approval %s not found", id)
}

// saveLocked persists approvals to disk atomically. Must be called with mu held.
func (s *ApprovalStore) saveLocked() error {
	return saveApprovals(s.filePath, s.approvals)
}

// --- Atomic JSON persistence ---

func saveApprovals(path string, approvals []*Approval) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create approval store directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(approvals, "", "  ")
	if err != nil {
		return err
	}
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

func loadApprovals(path string) []*Approval {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("load approval store failed", "err", err)
		}
		return nil
	}
	var approvals []*Approval
	if err := json.Unmarshal(data, &approvals); err != nil {
		slog.Warn("parse approval store failed", "err", err)
		return nil
	}
	slog.Info("loaded approval store", "count", len(approvals), "path", path)
	return approvals
}
