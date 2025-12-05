package specs

import (
	"testing"
)

func TestParseSpecContent_WithFrontmatter(t *testing.T) {
	content := []byte(`---
id: auth-001
title: Implement JWT Auth
status: approved
owner: ttzrs
type: feature
context:
  - internal/auth/jwt.go
  - cmd/server/main.go
---

# Overview
Implement JWT authentication for the API.

# Requirements
- [ ] Create GenerateToken function
- [x] Setup middleware structure
- [ ] Handle token expiration

# Plan
1. Define JWT structures
2. Implement signing logic
3. Create middleware
`)

	spec, err := ParseSpecContent(content)
	if err != nil {
		t.Fatalf("ParseSpecContent failed: %v", err)
	}

	// Check frontmatter
	if spec.ID != "auth-001" {
		t.Errorf("ID = %q, want %q", spec.ID, "auth-001")
	}
	if spec.Title != "Implement JWT Auth" {
		t.Errorf("Title = %q, want %q", spec.Title, "Implement JWT Auth")
	}
	if spec.Status != "approved" {
		t.Errorf("Status = %q, want %q", spec.Status, "approved")
	}
	if spec.Owner != "ttzrs" {
		t.Errorf("Owner = %q, want %q", spec.Owner, "ttzrs")
	}
	if spec.Type != "feature" {
		t.Errorf("Type = %q, want %q", spec.Type, "feature")
	}
	if len(spec.Context) != 2 {
		t.Errorf("Context len = %d, want 2", len(spec.Context))
	}

	// Check requirements
	if len(spec.Requirements) != 3 {
		t.Errorf("Requirements len = %d, want 3", len(spec.Requirements))
	}
	if spec.Requirements[0].Complete {
		t.Error("First requirement should not be complete")
	}
	if !spec.Requirements[1].Complete {
		t.Error("Second requirement should be complete")
	}

	// Check plan
	if len(spec.Plan) != 3 {
		t.Errorf("Plan len = %d, want 3", len(spec.Plan))
	}
	if spec.Plan[0] != "Define JWT structures" {
		t.Errorf("Plan[0] = %q, want %q", spec.Plan[0], "Define JWT structures")
	}

	// Check progress
	progress := spec.Progress()
	expected := 100.0 / 3.0 // 1 out of 3 complete
	if progress < expected-0.1 || progress > expected+0.1 {
		t.Errorf("Progress = %.1f, want ~%.1f", progress, expected)
	}
}

func TestParseSpecContent_NoFrontmatter(t *testing.T) {
	content := []byte(`# Simple Spec

Just a plain markdown file.

- [ ] Task one
- [ ] Task two
`)

	spec, err := ParseSpecContent(content)
	if err != nil {
		t.Fatalf("ParseSpecContent failed: %v", err)
	}

	if spec.ID != "" {
		t.Errorf("ID should be empty, got %q", spec.ID)
	}

	if len(spec.Requirements) != 2 {
		t.Errorf("Requirements len = %d, want 2", len(spec.Requirements))
	}
}

func TestSpec_FormatRequirements(t *testing.T) {
	spec := &Spec{
		Requirements: []Requirement{
			{Text: "Task one", Complete: false},
			{Text: "Task two", Complete: true},
		},
	}

	formatted := spec.FormatRequirements()
	if formatted == "" {
		t.Error("FormatRequirements should not be empty")
	}
	if !contains(formatted, "[ ] Task one") {
		t.Error("Should contain unchecked task one")
	}
	if !contains(formatted, "[x] Task two") {
		t.Error("Should contain checked task two")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
