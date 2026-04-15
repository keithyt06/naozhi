package knowledge

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDecisionAddAndGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.json")
	ds := NewDecisionStore(path)

	d1 := Decision{
		Title:        "Use CDK over Terraform",
		Context:      "Need IaC tool for new AWS project",
		Decision:     "Chose CDK for better AWS integration",
		Consequences: "Team needs TypeScript skills",
		Tags:         []string{"infrastructure", "aws"},
		Source:       "session-123",
	}
	added, err := ds.Add(d1)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if added.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(added.ID) != 16 {
		t.Errorf("expected 16-char hex ID, got %q (len %d)", added.ID, len(added.ID))
	}
	if added.CreatedAt.IsZero() {
		t.Error("expected auto-set CreatedAt")
	}

	// Get by ID.
	got, err := ds.Get(added.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "Use CDK over Terraform" {
		t.Errorf("Title = %q, want 'Use CDK over Terraform'", got.Title)
	}
	if got.Source != "session-123" {
		t.Errorf("Source = %q, want 'session-123'", got.Source)
	}
}

func TestDecisionGetNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.json")
	ds := NewDecisionStore(path)

	_, err := ds.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent decision")
	}
}

func TestDecisionList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.json")
	ds := NewDecisionStore(path)

	// Add two decisions with known times.
	d1 := Decision{
		Title:     "Decision A",
		CreatedAt: time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC),
	}
	d2 := Decision{
		Title:     "Decision B",
		CreatedAt: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
	}
	if _, err := ds.Add(d1); err != nil {
		t.Fatalf("Add d1: %v", err)
	}
	if _, err := ds.Add(d2); err != nil {
		t.Fatalf("Add d2: %v", err)
	}

	list := ds.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(list))
	}
	// Newest first.
	if list[0].Title != "Decision B" {
		t.Errorf("first item should be newest: got %q", list[0].Title)
	}
	if list[1].Title != "Decision A" {
		t.Errorf("second item should be oldest: got %q", list[1].Title)
	}
}

func TestDecisionSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.json")
	ds := NewDecisionStore(path)

	decisions := []Decision{
		{Title: "Use CDK over Terraform", Context: "IaC decision", Tags: []string{"aws", "cdk"}},
		{Title: "Adopt EKS for containers", Context: "Container orchestration", Tags: []string{"kubernetes", "eks"}},
		{Title: "WAF rule strategy", Context: "Security layer", Tags: []string{"security", "waf"}},
	}
	for _, d := range decisions {
		if _, err := ds.Add(d); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// Search by title.
	results := ds.Search("CDK")
	if len(results) != 1 {
		t.Errorf("search 'CDK': expected 1 result, got %d", len(results))
	}

	// Search by context.
	results = ds.Search("container")
	if len(results) != 1 {
		t.Errorf("search 'container': expected 1 result, got %d", len(results))
	}

	// Search by tag.
	results = ds.Search("security")
	if len(results) != 1 {
		t.Errorf("search 'security': expected 1 result, got %d", len(results))
	}

	// Case-insensitive.
	results = ds.Search("waf")
	if len(results) != 1 {
		t.Errorf("search 'waf': expected 1 result, got %d", len(results))
	}

	// No match.
	results = ds.Search("nonexistent-xyz")
	if len(results) != 0 {
		t.Errorf("search 'nonexistent-xyz': expected 0 results, got %d", len(results))
	}

	// Empty query returns all.
	results = ds.Search("")
	if len(results) != 3 {
		t.Errorf("search '': expected 3 results, got %d", len(results))
	}
}

func TestDecisionPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.json")

	ds1 := NewDecisionStore(path)
	d := Decision{
		Title:        "Persistent decision test",
		Context:      "Testing persistence",
		Decision:     "Use JSON file",
		Consequences: "Simple but limited",
		Tags:         []string{"test"},
		Source:       "manual",
		CreatedAt:    time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
	}
	added, err := ds1.Add(d)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Reload from same file.
	ds2 := NewDecisionStore(path)
	list := ds2.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 decision after reload, got %d", len(list))
	}

	got := list[0]
	if got.ID != added.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, added.ID)
	}
	if got.Title != "Persistent decision test" {
		t.Errorf("Title mismatch: %q", got.Title)
	}
	if got.Context != "Testing persistence" {
		t.Errorf("Context mismatch: %q", got.Context)
	}
	if got.Decision != "Use JSON file" {
		t.Errorf("Decision mismatch: %q", got.Decision)
	}
	if got.Source != "manual" {
		t.Errorf("Source mismatch: %q", got.Source)
	}
	if !got.CreatedAt.Equal(d.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", got.CreatedAt, d.CreatedAt)
	}
}

func TestDecisionEmptyStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.json")
	ds := NewDecisionStore(path)

	list := ds.List()
	if list == nil {
		t.Fatal("List() should return empty slice, not nil")
	}
	if len(list) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(list))
	}
}
