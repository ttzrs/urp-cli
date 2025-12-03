package query

import (
	"testing"
)

func TestImpactStruct(t *testing.T) {
	impact := Impact{
		Name:      "TestFunc",
		Path:      "/path/to/file.go",
		Distance:  2,
		Signature: "pkg.TestFunc",
	}

	if impact.Name != "TestFunc" {
		t.Errorf("expected TestFunc, got %s", impact.Name)
	}
	if impact.Distance != 2 {
		t.Errorf("expected distance 2, got %d", impact.Distance)
	}
}

func TestDependencyStruct(t *testing.T) {
	dep := Dependency{
		Name:      "Helper",
		Path:      "/lib/helper.go",
		Distance:  1,
		Signature: "lib.Helper",
	}

	if dep.Name != "Helper" {
		t.Error("name mismatch")
	}
	if dep.Distance != 1 {
		t.Error("distance mismatch")
	}
}

func TestDeadCodeStruct(t *testing.T) {
	dead := DeadCode{
		Name: "unusedFunc",
		Path: "/unused.go",
		Type: "function",
	}

	if dead.Type != "function" {
		t.Errorf("expected function, got %s", dead.Type)
	}
}

func TestCycleStruct(t *testing.T) {
	cycle := Cycle{
		Path: []string{"A", "B", "C", "A"},
	}

	if len(cycle.Path) != 4 {
		t.Errorf("expected 4 nodes in path, got %d", len(cycle.Path))
	}
}

func TestHotspotStruct(t *testing.T) {
	hotspot := Hotspot{
		Path:    "/main.go",
		Commits: 50,
		Authors: 3,
		Score:   75.5,
	}

	if hotspot.Commits != 50 {
		t.Errorf("expected 50 commits, got %d", hotspot.Commits)
	}
	if hotspot.Score != 75.5 {
		t.Errorf("expected score 75.5, got %f", hotspot.Score)
	}
}

func TestGraphStatsStruct(t *testing.T) {
	stats := GraphStats{
		Files:     100,
		Functions: 500,
		Structs:   50,
		Commits:   200,
		Authors:   5,
		Calls:     1000,
		Events:    10,
		Conflicts: 0,
	}

	if stats.Files != 100 {
		t.Error("files mismatch")
	}
	if stats.Calls != 1000 {
		t.Error("calls mismatch")
	}
}

func TestGetStringHelper(t *testing.T) {
	record := map[string]any{
		"name":  "test",
		"count": 42,
		"empty": "",
	}

	if getString(record, "name") != "test" {
		t.Error("expected test")
	}
	if getString(record, "count") != "" {
		t.Error("expected empty for non-string")
	}
	if getString(record, "missing") != "" {
		t.Error("expected empty for missing")
	}
	if getString(record, "empty") != "" {
		t.Error("expected empty string")
	}
}

func TestGetIntHelper(t *testing.T) {
	record := map[string]any{
		"int":     int(42),
		"int64":   int64(64),
		"float64": float64(99.9),
		"string":  "not a number",
	}

	if getInt(record, "int") != 42 {
		t.Error("expected 42")
	}
	if getInt(record, "int64") != 64 {
		t.Error("expected 64")
	}
	if getInt(record, "float64") != 99 {
		t.Error("expected 99")
	}
	if getInt(record, "string") != 0 {
		t.Error("expected 0 for string")
	}
	if getInt(record, "missing") != 0 {
		t.Error("expected 0 for missing")
	}
}

func TestGetFloatHelper(t *testing.T) {
	record := map[string]any{
		"float":  float64(3.14),
		"int":    int(10),
		"int64":  int64(20),
		"string": "not a number",
	}

	if getFloat(record, "float") != 3.14 {
		t.Error("expected 3.14")
	}
	if getFloat(record, "int") != 10.0 {
		t.Error("expected 10.0")
	}
	if getFloat(record, "int64") != 20.0 {
		t.Error("expected 20.0")
	}
	if getFloat(record, "string") != 0.0 {
		t.Error("expected 0 for string")
	}
}

