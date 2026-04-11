package scheduler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/msoedov/secondorder/internal/models"
)

func TestCheckBinaries_GitAlwaysPresent(t *testing.T) {
	results := CheckBinaries()
	var gitResult *BinaryStatus
	for i := range results {
		if results[i].Binary == "git" {
			gitResult = &results[i]
			break
		}
	}
	if gitResult == nil {
		t.Fatal("expected git in CheckBinaries results")
	}
	if !gitResult.Found {
		t.Error("git should be found in CI/dev environments")
	}
	if gitResult.Path == "" {
		t.Error("expected non-empty path for git")
	}
}

func TestCheckBinaries_CoversAllRunners(t *testing.T) {
	results := CheckBinaries()
	seen := make(map[string]bool)
	for _, r := range results {
		seen[r.Runner] = true
	}
	for runner := range RunnerBinaries {
		if !seen[runner] {
			t.Errorf("runner %q not covered by CheckBinaries", runner)
		}
	}
	if !seen["*"] {
		t.Error("expected git entry with runner='*'")
	}
}

func TestCheckRunnerBinary_Found(t *testing.T) {
	binDir := t.TempDir()
	makeStub(t, binDir, "fakecli")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	origBinaries := RunnerBinaries["claude_code"]
	RunnerBinaries["claude_code"] = "fakecli"
	defer func() { RunnerBinaries["claude_code"] = origBinaries }()

	if err := CheckRunnerBinary("claude_code"); err != nil {
		t.Errorf("expected nil error for found binary, got: %v", err)
	}
}

func TestCheckRunnerBinary_NotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if err := CheckRunnerBinary("claude_code"); err == nil {
		t.Error("expected error for missing binary, got nil")
	}
}

func TestCheckRunnerBinary_UnknownRunner(t *testing.T) {
	if err := CheckRunnerBinary("nonexistent_runner"); err != nil {
		t.Errorf("expected nil for unknown runner, got: %v", err)
	}
}

func TestRunnerBinariesMapping(t *testing.T) {
	expected := map[string]string{
		"claude_code": "claude",
		"codex":       "codex",
		"gemini":      "gemini",
		"copilot":     "gh",
		"opencode":    "opencode",
	}
	for runner, binary := range expected {
		got, ok := RunnerBinaries[runner]
		if !ok {
			t.Errorf("runner %q missing from RunnerBinaries", runner)
			continue
		}
		if got != binary {
			t.Errorf("RunnerBinaries[%q] = %q, want %q", runner, got, binary)
		}
	}
}

func TestSpawnAgent_MissingBinary(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	agent := &models.Agent{
		Name: "NoBin", Slug: "nobin", ArchetypeSlug: "worker",
		Runner: "gemini", Model: "default",
		WorkingDir: t.TempDir(), MaxTurns: 5, TimeoutSec: 10, Active: true,
	}
	d.CreateAgent(agent)

	issue := &models.Issue{Key: "SO-50", Title: "Binary check test", Description: "test", Status: "todo", Priority: 1}
	d.CreateIssue(issue)

	// Remove all runner binaries from PATH so the check fails.
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))

	s.WakeAgent(agent, issue)
	s.wg.Wait()

	runs, err := d.ListRunsForAgent(agent.ID, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected a run to be created")
	}
	if runs[0].Status != "failed" {
		t.Errorf("expected run status 'failed', got %q", runs[0].Status)
	}
}
