package patrol

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApprovalLifecycle(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "approvals.json")
	store := NewApprovalStore(storePath)

	// Create an approval
	a := &Approval{
		PatrolName: "cost-alert",
		Agent:      "general",
		Action:     "terraform apply",
		Summary:    "Apply infra changes",
		Detail:     "Terraform plan shows 3 resources to add",
		Impact:     "Production infrastructure change",
		Urgency:    UrgencyUrgent,
	}
	if err := store.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected non-empty ID after Create")
	}
	if a.Status != ApprovalPending {
		t.Fatalf("expected status pending, got %s", a.Status)
	}

	// Verify it appears in list
	pending := store.ListPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].ID != a.ID {
		t.Fatalf("expected ID %s, got %s", a.ID, pending[0].ID)
	}

	all := store.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 total, got %d", len(all))
	}

	// Approve it
	if err := store.Approve(a.ID, "keith"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, err := store.Get(a.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != ApprovalApproved {
		t.Fatalf("expected approved, got %s", got.Status)
	}
	if got.ResolvedBy != "keith" {
		t.Fatalf("expected resolved by keith, got %s", got.ResolvedBy)
	}
	if got.ResolvedAt == nil {
		t.Fatal("expected non-nil ResolvedAt")
	}

	// Pending list should be empty now
	pending = store.ListPending()
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending after approve, got %d", len(pending))
	}

	// Trying to approve again should fail
	if err := store.Approve(a.ID, "keith"); err == nil {
		t.Fatal("expected error approving non-pending approval")
	}

	// Verify persistence: reload from disk
	store2 := NewApprovalStore(storePath)
	all2 := store2.ListAll()
	if len(all2) != 1 {
		t.Fatalf("reloaded store: expected 1 total, got %d", len(all2))
	}
	if all2[0].Status != ApprovalApproved {
		t.Fatalf("reloaded store: expected approved, got %s", all2[0].Status)
	}
}

func TestApprovalReject(t *testing.T) {
	dir := t.TempDir()
	store := NewApprovalStore(filepath.Join(dir, "approvals.json"))

	a := &Approval{
		PatrolName: "infra-health",
		Agent:      "general",
		Action:     "kubectl delete",
		Summary:    "Delete pods",
		Urgency:    UrgencyNormal,
	}
	if err := store.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Reject(a.ID, "alice"); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	got, err := store.Get(a.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != ApprovalRejected {
		t.Fatalf("expected rejected, got %s", got.Status)
	}
	if got.ResolvedBy != "alice" {
		t.Fatalf("expected resolved by alice, got %s", got.ResolvedBy)
	}
}

func TestApprovalExpiry(t *testing.T) {
	dir := t.TempDir()
	store := NewApprovalStore(filepath.Join(dir, "approvals.json"))

	// Create an approval with a manually backdated CreatedAt
	a := &Approval{
		PatrolName: "test",
		Agent:      "general",
		Action:     "rm -rf /",
		Summary:    "Dangerous delete",
		Urgency:    UrgencyUrgent,
	}
	if err := store.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually backdate it past the expiry window
	store.mu.Lock()
	for _, ap := range store.approvals {
		if ap.ID == a.ID {
			ap.CreatedAt = time.Now().Add(-approvalExpiry - time.Minute)
		}
	}
	store.mu.Unlock()

	// ListPending triggers auto-expire
	pending := store.ListPending()
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending after expiry, got %d", len(pending))
	}

	got, err := store.Get(a.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != ApprovalExpired {
		t.Fatalf("expected expired, got %s", got.Status)
	}
	if got.ResolvedBy != "system:auto-expire" {
		t.Fatalf("expected system:auto-expire, got %s", got.ResolvedBy)
	}
}

func TestApprovalNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewApprovalStore(filepath.Join(dir, "approvals.json"))

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent approval")
	}

	if err := store.Approve("nonexistent", "keith"); err == nil {
		t.Fatal("expected error approving nonexistent")
	}

	if err := store.Reject("nonexistent", "keith"); err == nil {
		t.Fatal("expected error rejecting nonexistent")
	}
}

func TestApprovalStoreEmptyPath(t *testing.T) {
	// Store with empty path should work in-memory without errors.
	store := NewApprovalStore("")
	a := &Approval{
		PatrolName: "test",
		Agent:      "general",
		Action:     "test",
		Summary:    "test",
		Urgency:    UrgencyNormal,
	}
	if err := store.Create(a); err != nil {
		t.Fatalf("Create with empty path: %v", err)
	}
	if len(store.ListAll()) != 1 {
		t.Fatal("expected 1 approval in memory")
	}
}

func TestApprovalPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "approvals.json")

	store := NewApprovalStore(path)
	a := &Approval{
		PatrolName: "test",
		Agent:      "general",
		Action:     "test action",
		Summary:    "test summary",
		Urgency:    UrgencyNormal,
	}
	if err := store.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected approvals.json to exist on disk")
	}

	// Load from a fresh store
	store2 := NewApprovalStore(path)
	all := store2.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 approval after reload, got %d", len(all))
	}
	if all[0].Action != "test action" {
		t.Fatalf("expected action 'test action', got %s", all[0].Action)
	}
}
