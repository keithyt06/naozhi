package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFromWiki_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	g, err := ExtractFromWiki(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(g.Edges))
	}
}

func TestExtractFromWiki_NonExistentDir(t *testing.T) {
	g, err := ExtractFromWiki("/tmp/nonexistent-wiki-dir-test-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes for nonexistent dir, got %d", len(g.Nodes))
	}
}

func TestExtractFromWiki_SinglePageNoLinks(t *testing.T) {
	dir := t.TempDir()
	content := `---
title: AWS Overview
entities:
  - CloudFront
  - S3
---

# AWS Overview

This page describes AWS services.
`
	if err := os.WriteFile(filepath.Join(dir, "aws-overview.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	g, err := ExtractFromWiki(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: page node + 2 entity nodes = 3 nodes.
	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Nodes))
		for _, n := range g.Nodes {
			t.Logf("  node: %s (type=%s)", n.Label, n.Type)
		}
	}

	// Should have 2 edges: page->CloudFront, page->S3.
	if len(g.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(g.Edges))
	}

	// Verify stats match.
	if g.Stats.TotalNodes != len(g.Nodes) {
		t.Errorf("stats.TotalNodes=%d, but len(nodes)=%d", g.Stats.TotalNodes, len(g.Nodes))
	}
	if g.Stats.TotalEdges != len(g.Edges) {
		t.Errorf("stats.TotalEdges=%d, but len(edges)=%d", g.Stats.TotalEdges, len(g.Edges))
	}
}

func TestExtractFromWiki_CrossPageLinks(t *testing.T) {
	dir := t.TempDir()

	page1 := `---
title: EKS Setup
entities: []
---

# EKS Setup

See [[terraform-guide]] for infrastructure code.
Also check [[security-baseline]].
`
	page2 := `---
title: Terraform Guide
entities: []
---

# Terraform Guide

Used by [[eks-setup]] and [[cloudfront-config]].
`
	if err := os.WriteFile(filepath.Join(dir, "eks-setup.md"), []byte(page1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform-guide.md"), []byte(page2), 0644); err != nil {
		t.Fatal(err)
	}

	g, err := ExtractFromWiki(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Nodes: eks-setup, terraform-guide (from files), security-baseline, cloudfront-config (from wikilinks).
	if len(g.Nodes) < 4 {
		t.Errorf("expected at least 4 nodes, got %d", len(g.Nodes))
		for _, n := range g.Nodes {
			t.Logf("  node: %s (id=%s)", n.Label, n.ID)
		}
	}

	// Edges: eks-setup->terraform-guide, eks-setup->security-baseline,
	//        terraform-guide->eks-setup (merged with reverse), terraform-guide->cloudfront-config.
	if len(g.Edges) < 3 {
		t.Errorf("expected at least 3 edges, got %d", len(g.Edges))
	}
}

func TestExtractFromWiki_DuplicateEntitiesMerged(t *testing.T) {
	dir := t.TempDir()

	page1 := `---
title: Page One
entities:
  - S3
  - CloudFront
---

Content with [[S3]] link.
`
	page2 := `---
title: Page Two
entities:
  - S3
---

More about [[CloudFront]].
`
	if err := os.WriteFile(filepath.Join(dir, "page-one.md"), []byte(page1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "page-two.md"), []byte(page2), 0644); err != nil {
		t.Fatal(err)
	}

	g, err := ExtractFromWiki(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count unique node IDs.
	ids := make(map[string]bool)
	for _, n := range g.Nodes {
		if ids[n.ID] {
			t.Errorf("duplicate node ID: %s", n.ID)
		}
		ids[n.ID] = true
	}

	// S3 and CloudFront should each appear exactly once despite being in both pages.
	if !ids["s3"] {
		t.Error("missing node: s3")
	}
	if !ids["cloudfront"] {
		t.Error("missing node: cloudfront")
	}
}

func TestExtractFromWiki_SkipsCLAUDEMD(t *testing.T) {
	dir := t.TempDir()

	// CLAUDE.md should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# CLAUDE\nInstructions."), 0644); err != nil {
		t.Fatal(err)
	}
	// A real page.
	if err := os.WriteFile(filepath.Join(dir, "real-page.md"), []byte("---\ntitle: Real\n---\nContent."), 0644); err != nil {
		t.Fatal(err)
	}

	g, err := ExtractFromWiki(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, n := range g.Nodes {
		if n.ID == "claude" {
			t.Error("CLAUDE.md should be skipped but was included as a node")
		}
	}
	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node (real-page), got %d", len(g.Nodes))
	}
}

func TestInferType(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{"aws service", "eks cluster setup", NodeTypeService},
		{"terraform infra", "terraform helm docker deploy", NodeTypeInfra},
		{"security topic", "waf security rules", NodeTypeSecurity},
		{"project type", "customer migration project", NodeTypeProject},
		{"generic hub", "knowledge overview", NodeTypeHub},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyText(tt.text)
			if got != tt.expected {
				t.Errorf("classifyText(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestNormalizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CloudFront", "cloudfront"},
		{"EKS Setup", "eks-setup"},
		{"terraform_guide", "terraform-guide"},
		{"  spaces  ", "spaces"},
	}
	for _, tt := range tests {
		got := normalizeID(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestColorForType(t *testing.T) {
	if c := ColorForType(NodeTypeHub); c != "#8B5CF6" {
		t.Errorf("hub color = %q, want #8B5CF6", c)
	}
	if c := ColorForType("unknown"); c != "#6B7280" {
		t.Errorf("unknown color = %q, want #6B7280", c)
	}
}
