package patrol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- TestPatrolLifecycle: state transitions ---

func TestPatrolLifecycle(t *testing.T) {
	p := &Patrol{Name: "test", State: StateActive}

	// Active -> Running
	if err := p.ValidateTransition(StateRunning); err != nil {
		t.Fatalf("Active->Running should be valid: %v", err)
	}
	p.State = StateRunning

	// Running -> Active
	if err := p.ValidateTransition(StateActive); err != nil {
		t.Fatalf("Running->Active should be valid: %v", err)
	}
	p.State = StateActive

	// Active -> Paused
	if err := p.Pause(); err != nil {
		t.Fatalf("Pause should succeed: %v", err)
	}
	if p.State != StatePaused {
		t.Fatalf("expected paused, got %s", p.State)
	}

	// Paused -> Active (Resume)
	if err := p.Resume(); err != nil {
		t.Fatalf("Resume should succeed: %v", err)
	}
	if p.State != StateActive {
		t.Fatalf("expected active, got %s", p.State)
	}

	// Active -> Disabled
	if err := p.Disable(); err != nil {
		t.Fatalf("Disable should succeed: %v", err)
	}
	if p.State != StateDisabled {
		t.Fatalf("expected disabled, got %s", p.State)
	}

	// Disabled -> Running should fail
	if err := p.ValidateTransition(StateRunning); err == nil {
		t.Fatal("Disabled->Running should be invalid")
	}

	// Disabled -> Active should fail
	if err := p.ValidateTransition(StateActive); err == nil {
		t.Fatal("Disabled->Active should be invalid")
	}

	// Paused -> Running should fail
	p.State = StatePaused
	if err := p.ValidateTransition(StateRunning); err == nil {
		t.Fatal("Paused->Running should be invalid")
	}
}

func TestPatrolCanRun(t *testing.T) {
	tests := []struct {
		state State
		want  bool
	}{
		{StateActive, true},
		{StatePaused, false},
		{StateDisabled, false},
		{StateRunning, false},
	}
	for _, tt := range tests {
		p := &Patrol{State: tt.state}
		if got := p.CanRun(); got != tt.want {
			t.Errorf("CanRun() with state %s: got %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestPatrolParseTimeout(t *testing.T) {
	p := &Patrol{}
	if d := p.ParseTimeout(); d != 5*time.Minute {
		t.Errorf("default timeout: got %v, want 5m", d)
	}
	p.Timeout = "10m"
	if d := p.ParseTimeout(); d != 10*time.Minute {
		t.Errorf("10m timeout: got %v, want 10m", d)
	}
	p.Timeout = "invalid"
	if d := p.ParseTimeout(); d != 5*time.Minute {
		t.Errorf("invalid timeout should default: got %v, want 5m", d)
	}
}

// --- TestPatrolStore: CRUD + persistence ---

func TestPatrolStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "patrols.json")

	// Create config patrols
	config := map[string]*Patrol{
		"cost-alert": {
			Agent:    "general",
			Schedule: "@every 1h",
			Prompt:   "check costs",
		},
		"pr-review": {
			Agent:   "reviewer",
			Trigger: "github:pull_request",
			Prompt:  "review this PR",
		},
	}

	// Initial merge with no stored state
	merged := mergeConfig(config, nil)
	if len(merged) != 2 {
		t.Fatalf("expected 2 patrols, got %d", len(merged))
	}
	for name, p := range merged {
		if p.State != StateActive {
			t.Errorf("patrol %s should be active, got %s", name, p.State)
		}
		if p.Name != name {
			t.Errorf("patrol name should be %s, got %s", name, p.Name)
		}
	}

	// Save and reload
	if err := savePatrols(path, merged); err != nil {
		t.Fatalf("save: %v", err)
	}
	stored := loadPatrols(path)
	if len(stored) != 2 {
		t.Fatalf("loaded %d states, expected 2", len(stored))
	}

	// Modify state and re-merge
	stored["cost-alert"].State = StatePaused
	stored["cost-alert"].TotalRuns = 42

	// Add a new patrol in config
	config["infra-health"] = &Patrol{
		Agent:  "general",
		Prompt: "check infra",
	}
	// Remove pr-review from config
	delete(config, "pr-review")

	merged2 := mergeConfig(config, stored)
	if len(merged2) != 2 { // cost-alert + infra-health
		t.Fatalf("expected 2 patrols after merge, got %d", len(merged2))
	}
	if _, ok := merged2["pr-review"]; ok {
		t.Error("pr-review should be removed")
	}
	if merged2["cost-alert"].State != StatePaused {
		t.Error("cost-alert should retain paused state")
	}
	if merged2["cost-alert"].TotalRuns != 42 {
		t.Error("cost-alert should retain TotalRuns")
	}
	if merged2["infra-health"].State != StateActive {
		t.Error("new patrol should be active")
	}
}

func TestPatrolStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "patrols.json")

	patrols := map[string]*Patrol{
		"test": {Name: "test", State: StateActive},
	}
	if err := savePatrols(path, patrols); err != nil {
		t.Fatalf("save with nested dir: %v", err)
	}
	// Verify no tmp file left
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file should be cleaned up")
	}
}

func TestLoadPatrolsMissing(t *testing.T) {
	result := loadPatrols("/nonexistent/path.json")
	if result != nil {
		t.Error("missing file should return nil")
	}
}

// --- TestPatrolLogger: append + read ---

func TestPatrolLogger(t *testing.T) {
	dir := t.TempDir()

	lw, err := NewLogWriter(dir, "test-patrol")
	if err != nil {
		t.Fatalf("new log writer: %v", err)
	}
	defer lw.Close()

	// Append 5 entries
	for i := 0; i < 5; i++ {
		rl := &RunLog{
			ID:         generateID(),
			PatrolName: "test-patrol",
			Timestamp:  time.Now(),
			Duration:   time.Duration(i) * time.Second,
			Status:     RunOK,
			Summary:    "run " + string(rune('A'+i)),
		}
		if err := lw.Append(rl); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// Read tail 3
	logs, err := lw.ReadTail(3)
	if err != nil {
		t.Fatalf("read tail: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
	// Newest first
	if logs[0].Summary != "run E" {
		t.Errorf("first log should be newest, got %q", logs[0].Summary)
	}
	if logs[2].Summary != "run C" {
		t.Errorf("last log should be oldest of 3, got %q", logs[2].Summary)
	}

	// Read all
	allLogs, err := lw.ReadTail(100)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(allLogs) != 5 {
		t.Fatalf("expected 5 logs, got %d", len(allLogs))
	}

	// ReadPage
	page, total, err := lw.ReadPage(0, 2)
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(page) != 2 {
		t.Errorf("expected page size 2, got %d", len(page))
	}
}

func TestPatrolLoggerEmpty(t *testing.T) {
	dir := t.TempDir()
	lw, err := NewLogWriter(dir, "empty-patrol")
	if err != nil {
		t.Fatalf("new log writer: %v", err)
	}
	defer lw.Close()

	logs, err := lw.ReadTail(10)
	if err != nil {
		t.Fatalf("read empty: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 logs from empty file, got %d", len(logs))
	}
}

// --- Test helpers ---

func TestGenerateID(t *testing.T) {
	id := generateID()
	if len(id) != 16 {
		t.Errorf("expected 16-char hex ID, got %d chars: %s", len(id), id)
	}
	// Must be unique
	id2 := generateID()
	if id == id2 {
		t.Error("two generated IDs should not be equal")
	}
}

func TestClassifyResult(t *testing.T) {
	tests := []struct {
		text string
		want RunStatus
	}{
		{"everything is fine", RunOK},
		{"Error: something broke", RunError},
		{"CRITICAL failure detected", RunError},
		{"Warning: high CPU usage", RunWarn},
		{"检测到异常情况", RunWarn},
		{"", RunOK},
	}
	for _, tt := range tests {
		if got := classifyResult(tt.text); got != tt.want {
			t.Errorf("classifyResult(%q) = %s, want %s", tt.text, got, tt.want)
		}
	}
}

func TestExtractSummary(t *testing.T) {
	// Short text
	if s := extractSummary("hello"); s != "hello" {
		t.Errorf("got %q", s)
	}
	// Multi-line
	if s := extractSummary("line1\nline2\nline3"); s != "line1" {
		t.Errorf("got %q", s)
	}
	// Long text
	long := ""
	for i := 0; i < 300; i++ {
		long += "x"
	}
	if s := extractSummary(long); len(s) != 200 {
		t.Errorf("expected 200 chars, got %d", len(s))
	}
}

func TestMatchTrigger(t *testing.T) {
	tests := []struct {
		pattern, key string
		want         bool
	}{
		{"github:pull_request", "github:pull_request", true},
		{"github:pull_request", "github:push", false},
		{"custom:*", "custom:deploy", true},
		{"custom:*", "github:push", false},
		{"github:*", "github:pr_opened", true},
	}
	for _, tt := range tests {
		if got := matchTrigger(tt.pattern, tt.key); got != tt.want {
			t.Errorf("matchTrigger(%q, %q) = %v, want %v", tt.pattern, tt.key, got, tt.want)
		}
	}
}

func TestMapGitHubEvent(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	if got := mapGitHubEvent("pull_request", body); got != "pr_opened" {
		t.Errorf("got %q, want pr_opened", got)
	}
	if got := mapGitHubEvent("push", body); got != "push" {
		t.Errorf("got %q, want push", got)
	}
	if got := mapGitHubEvent("issues", body); got != "issue_opened" {
		t.Errorf("got %q, want issue_opened", got)
	}
	emptyBody := []byte(`{}`)
	if got := mapGitHubEvent("pull_request", emptyBody); got != "pull_request" {
		t.Errorf("got %q, want pull_request", got)
	}
}

func TestWebhookPayloadParse(t *testing.T) {
	data := `{"event":"deploy","source":"custom","payload":{"env":"prod"}}`
	var wp WebhookPayload
	if err := json.Unmarshal([]byte(data), &wp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wp.Event != "deploy" {
		t.Errorf("event: got %q", wp.Event)
	}
	if wp.Source != "custom" {
		t.Errorf("source: got %q", wp.Source)
	}
}
