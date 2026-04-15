package knowledge

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestTwinManager(t *testing.T) *TwinManager {
	t.Helper()
	dir := t.TempDir()
	wikiDir := filepath.Join(dir, "wiki")
	os.MkdirAll(wikiDir, 0o755)
	wiki := NewWikiManager(wikiDir)
	return NewTwinManager(wiki, dir)
}

func TestDefaultTwinConfig(t *testing.T) {
	cfg := DefaultTwinConfig()
	if cfg.Enabled {
		t.Error("default config should be disabled")
	}
	if cfg.Name != "Keith" {
		t.Errorf("Name = %q, want %q", cfg.Name, "Keith")
	}
	if cfg.Style != "formal" {
		t.Errorf("Style = %q, want %q", cfg.Style, "formal")
	}
	if cfg.AutoReplyThreshold != 0.8 {
		t.Errorf("AutoReplyThreshold = %f, want 0.8", cfg.AutoReplyThreshold)
	}
}

func TestTwinManager_ConfigPersistence(t *testing.T) {
	dir := t.TempDir()
	wikiDir := filepath.Join(dir, "wiki")
	os.MkdirAll(wikiDir, 0o755)
	wiki := NewWikiManager(wikiDir)

	tm := NewTwinManager(wiki, dir)

	cfg := tm.Config()
	cfg.Enabled = true
	cfg.Style = "casual"
	cfg.Name = "Alice"
	if err := tm.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// Create a new manager from the same dir — should load persisted config.
	tm2 := NewTwinManager(wiki, dir)
	cfg2 := tm2.Config()
	if !cfg2.Enabled {
		t.Error("loaded config should be enabled")
	}
	if cfg2.Style != "casual" {
		t.Errorf("Style = %q, want %q", cfg2.Style, "casual")
	}
	if cfg2.Name != "Alice" {
		t.Errorf("Name = %q, want %q", cfg2.Name, "Alice")
	}
}

func TestBuildTwinPrompt_NoWiki(t *testing.T) {
	tm := newTestTwinManager(t)
	prompt := tm.BuildTwinPrompt("how do we deploy to prod?", nil)

	// Should always have [Role] and [Response Style] sections.
	if !contains(prompt, "[Role]") {
		t.Error("prompt missing [Role] section")
	}
	if !contains(prompt, "[Response Style]") {
		t.Error("prompt missing [Response Style] section")
	}
	if !contains(prompt, "Keith") {
		t.Error("prompt missing CTO name")
	}
}

func TestBuildTwinPrompt_WithWiki(t *testing.T) {
	dir := t.TempDir()
	wikiDir := filepath.Join(dir, "wiki")
	os.MkdirAll(wikiDir, 0o755)
	wiki := NewWikiManager(wikiDir)

	// Write a wiki page about deployment.
	wiki.WritePage("deployment", WikiPage{
		Title:      "Production Deployment",
		Entities:   []string{"ECS", "CDK"},
		CompiledAt: time.Now().Format("2006-01-02T15:04:05Z"),
		Sources:    3,
	}, "We deploy using ECS with CDK. Blue-green deployment strategy.")

	tm := NewTwinManager(wiki, dir)
	prompt := tm.BuildTwinPrompt("production deployment ECS", nil)

	if !contains(prompt, "[Knowledge Context]") {
		t.Error("prompt missing [Knowledge Context] section with matching wiki")
	}
	if !contains(prompt, "Production Deployment") {
		t.Error("prompt missing wiki page title")
	}
}

func TestBuildTwinPrompt_WithDecisions(t *testing.T) {
	tm := newTestTwinManager(t)
	decisions := []Decision{
		{Title: "Use CDK over Terraform", Decision: "CDK provides better L2 constructs", CreatedAt: time.Now()},
	}
	prompt := tm.BuildTwinPrompt("infrastructure", decisions)

	if !contains(prompt, "[Recent Decisions]") {
		t.Error("prompt missing [Recent Decisions] section")
	}
	if !contains(prompt, "Use CDK over Terraform") {
		t.Error("prompt missing decision title")
	}
}

func TestBuildTwinPrompt_CasualStyle(t *testing.T) {
	tm := newTestTwinManager(t)
	cfg := tm.Config()
	cfg.Style = "casual"
	tm.UpdateConfig(cfg)

	prompt := tm.BuildTwinPrompt("test", nil)
	if !contains(prompt, "casual") {
		t.Error("casual style not reflected in prompt")
	}
}

func TestScoreConfidence_NoWiki(t *testing.T) {
	tm := newTestTwinManager(t)
	score := tm.ScoreConfidence("how to deploy?", nil)

	if score.Coverage != 0 {
		t.Errorf("Coverage = %f, want 0", score.Coverage)
	}
	if score.Recency != 0 {
		t.Errorf("Recency = %f, want 0", score.Recency)
	}
	// Overall should be very low (only specificity * 0.2).
	if score.Overall > 0.3 {
		t.Errorf("Overall = %f, want < 0.3", score.Overall)
	}
}

func TestScoreConfidence_FullCoverage(t *testing.T) {
	tm := newTestTwinManager(t)
	pages := []WikiPage{
		{
			Title:      "ECS Deployment",
			Entities:   []string{"ECS", "deployment"},
			CompiledAt: time.Now().Format("2006-01-02T15:04:05Z"),
		},
	}
	score := tm.ScoreConfidence("ECS deployment", pages)

	if score.Coverage < 0.8 {
		t.Errorf("Coverage = %f, want >= 0.8", score.Coverage)
	}
	if score.Recency < 0.9 {
		t.Errorf("Recency = %f, want >= 0.9 (just compiled)", score.Recency)
	}
	if score.Overall < 0.5 {
		t.Errorf("Overall = %f, want >= 0.5", score.Overall)
	}
}

func TestScoreConfidence_StaleWiki(t *testing.T) {
	tm := newTestTwinManager(t)
	staleTime := time.Now().Add(-90 * 24 * time.Hour) // 90 days ago
	pages := []WikiPage{
		{
			Title:      "Old Architecture",
			Entities:   []string{"architecture"},
			CompiledAt: staleTime.Format("2006-01-02T15:04:05Z"),
		},
	}
	score := tm.ScoreConfidence("architecture", pages)

	// Recency should be low due to staleness.
	if score.Recency > 0.15 {
		t.Errorf("Recency = %f, want < 0.15 for 90-day-old page", score.Recency)
	}
}

func TestScoreConfidence_EmptyQuery(t *testing.T) {
	tm := newTestTwinManager(t)
	score := tm.ScoreConfidence("", nil)
	if score.Overall != 0 {
		t.Errorf("Overall = %f, want 0 for empty query", score.Overall)
	}
}

func TestScoringSpecificity(t *testing.T) {
	// Short vague query.
	s1 := scoringSpecificity("help")
	// Long specific query with AWS service.
	s2 := scoringSpecificity("how do we configure CloudFront distribution with WAF rules for the production environment")

	if s2 <= s1 {
		t.Errorf("specific query (%f) should score higher than vague (%f)", s2, s1)
	}
}

func TestScoringRecency_RecentPage(t *testing.T) {
	pages := []WikiPage{
		{CompiledAt: time.Now().Format("2006-01-02T15:04:05Z")},
	}
	r := scoringRecency(pages)
	if r < 0.95 {
		t.Errorf("recency of just-compiled page = %f, want >= 0.95", r)
	}
}

func TestScoringRecency_OldPage(t *testing.T) {
	old := time.Now().Add(-60 * 24 * time.Hour)
	pages := []WikiPage{
		{CompiledAt: old.Format("2006-01-02T15:04:05Z")},
	}
	r := scoringRecency(pages)
	expected := math.Exp(-60.0 / 30.0) // ~0.135
	if math.Abs(r-expected) > 0.05 {
		t.Errorf("recency of 60-day-old page = %f, want ~%f", r, expected)
	}
}

func TestScoringCoverage(t *testing.T) {
	pages := []WikiPage{
		{Title: "ECS Deployment Guide", Entities: []string{"ECS", "CDK", "Docker"}},
	}
	// All terms match.
	c := scoringCoverage("ECS CDK deployment", pages)
	if c < 0.8 {
		t.Errorf("full coverage = %f, want >= 0.8", c)
	}

	// No terms match.
	c = scoringCoverage("kubernetes helm chart", pages)
	if c != 0 {
		t.Errorf("no coverage = %f, want 0", c)
	}

	// Partial match.
	c = scoringCoverage("ECS monitoring alerts", pages)
	if c < 0.3 || c > 0.6 {
		t.Errorf("partial coverage = %f, want between 0.3-0.6", c)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && containsStr(s, sub)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
