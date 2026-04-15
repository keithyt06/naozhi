package server

import (
	"testing"
	"time"

	"github.com/naozhi/naozhi/internal/cli"
)

func TestBuildTimeline_Empty(t *testing.T) {
	tl := BuildTimeline("test:key", nil)
	if tl.SessionKey != "test:key" {
		t.Errorf("SessionKey = %q, want %q", tl.SessionKey, "test:key")
	}
	if len(tl.Events) != 0 {
		t.Errorf("Events = %d, want 0", len(tl.Events))
	}
	if tl.TotalDuration != 0 {
		t.Errorf("TotalDuration = %d, want 0", tl.TotalDuration)
	}
}

func TestBuildTimeline_SingleMessage(t *testing.T) {
	events := []cli.EventEntry{
		{Time: 1000, Type: "text", Summary: "hello", Detail: "hello world"},
	}
	tl := BuildTimeline("key:1", events)
	if len(tl.Events) != 1 {
		t.Fatalf("Events = %d, want 1", len(tl.Events))
	}
	if tl.Events[0].DeltaMs != 0 {
		t.Errorf("DeltaMs = %d, want 0", tl.Events[0].DeltaMs)
	}
	if tl.Events[0].Content != "hello world" {
		t.Errorf("Content = %q, want %q", tl.Events[0].Content, "hello world")
	}
	if tl.Stats.EventCount != 1 {
		t.Errorf("EventCount = %d, want 1", tl.Stats.EventCount)
	}
}

func TestBuildTimeline_MultipleEvents(t *testing.T) {
	events := []cli.EventEntry{
		{Time: 1000, Type: "text", Summary: "start"},
		{Time: 2000, Type: "tool_use", Summary: "reading file", Tool: "Read"},
		{Time: 2500, Type: "result", Summary: "done", Cost: 0.05},
		{Time: 3000, Type: "text", Summary: "end"},
	}
	tl := BuildTimeline("key:2", events)
	if len(tl.Events) != 4 {
		t.Fatalf("Events = %d, want 4", len(tl.Events))
	}
	if tl.TotalDuration != 2000 {
		t.Errorf("TotalDuration = %d, want 2000", tl.TotalDuration)
	}
	if tl.Stats.ToolCallCount != 1 {
		t.Errorf("ToolCallCount = %d, want 1", tl.Stats.ToolCallCount)
	}
	if tl.Stats.TotalCost != 0.05 {
		t.Errorf("TotalCost = %f, want 0.05", tl.Stats.TotalCost)
	}
	// Check delta ordering.
	if tl.Events[1].DeltaMs != 1000 {
		t.Errorf("Events[1].DeltaMs = %d, want 1000", tl.Events[1].DeltaMs)
	}
	if tl.Events[1].ToolName != "Read" {
		t.Errorf("Events[1].ToolName = %q, want %q", tl.Events[1].ToolName, "Read")
	}
}

func TestBuildTimeline_OutOfOrder(t *testing.T) {
	// Events arrive out of order; BuildTimeline should sort them.
	events := []cli.EventEntry{
		{Time: 3000, Type: "text", Summary: "end"},
		{Time: 1000, Type: "text", Summary: "start"},
		{Time: 2000, Type: "tool_use", Summary: "tool", Tool: "Bash"},
	}
	tl := BuildTimeline("key:3", events)
	if tl.Events[0].Content != "start" {
		t.Errorf("Events[0].Content = %q, want %q", tl.Events[0].Content, "start")
	}
	if tl.Events[2].Content != "end" {
		t.Errorf("Events[2].Content = %q, want %q", tl.Events[2].Content, "end")
	}
}

func TestShareStore_GenerateAndLookup(t *testing.T) {
	ss := NewShareStore("")

	token, err := ss.GenerateShareToken("session:abc")
	if err != nil {
		t.Fatalf("GenerateShareToken: %v", err)
	}
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64", len(token))
	}

	entry := ss.Lookup(token)
	if entry == nil {
		t.Fatal("Lookup returned nil for valid token")
	}
	if entry.SessionKey != "session:abc" {
		t.Errorf("SessionKey = %q, want %q", entry.SessionKey, "session:abc")
	}
}

func TestShareStore_ExpiredToken(t *testing.T) {
	ss := NewShareStore("")

	token, _ := ss.GenerateShareToken("session:abc")

	// Manually expire the entry by setting ExpiresAt to the past.
	ss.mu.Lock()
	ss.shares[token].ExpiresAt = time.Now().Add(-1 * time.Hour)
	ss.mu.Unlock()

	entry := ss.Lookup(token)
	if entry != nil {
		t.Error("Lookup returned non-nil for expired token")
	}
}

func TestShareStore_Revoke(t *testing.T) {
	ss := NewShareStore("")

	token, _ := ss.GenerateShareToken("session:abc")
	if !ss.Revoke(token) {
		t.Error("Revoke returned false for existing token")
	}
	if ss.Revoke(token) {
		t.Error("Revoke returned true for already-revoked token")
	}
	if ss.Lookup(token) != nil {
		t.Error("Lookup returned non-nil after Revoke")
	}
}

func TestShareStore_ListForSession(t *testing.T) {
	ss := NewShareStore("")

	ss.GenerateShareToken("session:abc")
	ss.GenerateShareToken("session:abc")
	ss.GenerateShareToken("session:xyz")

	list := ss.ListForSession("session:abc")
	if len(list) != 2 {
		t.Errorf("ListForSession = %d, want 2", len(list))
	}

	list = ss.ListForSession("session:xyz")
	if len(list) != 1 {
		t.Errorf("ListForSession = %d, want 1", len(list))
	}

	list = ss.ListForSession("session:none")
	if len(list) != 0 {
		t.Errorf("ListForSession = %d, want 0", len(list))
	}
}

func TestShareStore_Cleanup(t *testing.T) {
	ss := NewShareStore("")

	t1, _ := ss.GenerateShareToken("session:abc")
	ss.GenerateShareToken("session:def")

	// Expire one by setting ExpiresAt to the past.
	ss.mu.Lock()
	ss.shares[t1].ExpiresAt = time.Now().Add(-1 * time.Hour)
	ss.mu.Unlock()

	removed := ss.Cleanup()
	if removed != 1 {
		t.Errorf("Cleanup removed = %d, want 1", removed)
	}

	ss.mu.RLock()
	remaining := len(ss.shares)
	ss.mu.RUnlock()
	if remaining != 1 {
		t.Errorf("remaining shares = %d, want 1", remaining)
	}
}
