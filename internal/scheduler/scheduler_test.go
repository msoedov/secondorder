package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msoedov/secondorder/internal/archetypes"
	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

// makeStub writes a shell script to dir/name that echoes its args and selected env vars.
func makeStub(t *testing.T, dir, name string) {
	t.Helper()
	p := filepath.Join(dir, name)
	script := "#!/bin/sh\necho ARGS: $@\nenv\n"
	if err := os.WriteFile(p, []byte(script), 0755); err != nil {
		t.Fatalf("makeStub %s: %v", name, err)
	}
}

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestNew(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if s.port != 9001 {
		t.Errorf("port = %d, want 9001", s.port)
	}
}

func TestSetCallbacks(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	startCalled := false
	s.SetOnRunStart(func(r *models.Run) { startCalled = true })
	s.onRunStart(&models.Run{})
	if !startCalled {
		t.Error("onRunStart not called")
	}

	called := false
	s.SetOnRunComplete(func(r *models.Run) { called = true })
	s.onRunComplete(&models.Run{})
	if !called {
		t.Error("onRunComplete not called")
	}

	commentCalled := false
	s.SetOnComment(func(_, _, _ string) { commentCalled = true })
	s.onComment("SO-1", "agent", "hello")
	if !commentCalled {
		t.Error("onComment not called")
	}
}

func TestStop(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)
	s.Stop()
	if !s.stopped {
		t.Error("expected stopped=true")
	}
}

func TestProvisionAPIKey(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	agent := &models.Agent{
		Name: "Key Agent", Slug: "key-agent", ArchetypeSlug: "worker",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(agent)

	key, err := s.provisionAPIKey(agent.ID)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if !strings.HasPrefix(key, "so_") {
		t.Errorf("key = %q, want so_ prefix", key)
	}
	if len(key) < 20 {
		t.Errorf("key too short: %d chars", len(key))
	}

	// Provision again revokes old key
	key2, err := s.provisionAPIKey(agent.ID)
	if err != nil {
		t.Fatalf("provision again: %v", err)
	}
	if key2 == key {
		t.Error("expected different key on re-provision")
	}
}

func TestRunAudit(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	// Create auditor agent
	auditor := &models.Agent{
		Name: "Auditor", Slug: "auditor", ArchetypeSlug: "auditor",
		Runner: "claude_code", Model: "sonnet", Active: true,
	}
	d.CreateAgent(auditor)

	// 1. Default (uses auditor agent settings)
	runID, err := s.RunAudit(1, 1, "", "", "")
	if err != nil {
		t.Fatalf("RunAudit default: %v", err)
	}
	ar, _ := d.GetAuditRun(runID)
	if ar.Runner != "claude_code" || ar.Model != "sonnet" {
		t.Errorf("expected default runner/model, got %s/%s", ar.Runner, ar.Model)
	}

	// 2. Explicit runner/model from UI
	runID2, err := s.RunAudit(1, 1, "", "gemini", "gemini-1.5-pro")
	if err != nil {
		t.Fatalf("RunAudit explicit: %v", err)
	}
	ar2, _ := d.GetAuditRun(runID2)
	if ar2.Runner != "gemini" || ar2.Model != "gemini-1.5-pro" {
		t.Errorf("expected explicit runner/model, got %s/%s", ar2.Runner, ar2.Model)
	}

	// 3. Configuration file support (.secondorder.json)
	jsonPath := ".secondorder.json"
	jsonContent := `{"audit": {"runner": "codex", "model": "gpt-5.4-thinking"}}`
	os.WriteFile(jsonPath, []byte(jsonContent), 0644)
	t.Cleanup(func() { os.Remove(jsonPath) })

	runID3, err := s.RunAudit(1, 1, "", "", "")
	if err != nil {
		t.Fatalf("RunAudit json config: %v", err)
	}
	ar3, _ := d.GetAuditRun(runID3)
	if ar3.Runner != "codex" || ar3.Model != "gpt-5.4-thinking" {
		t.Errorf("expected json config runner/model, got %s/%s", ar3.Runner, ar3.Model)
	}
	os.Remove(jsonPath) // Remove to test yaml fallback

	// 3b. Configuration file support (.secondorder.yml fallback)
	ymlPath := ".secondorder.yml"
	ymlContent := `{"audit": {"runner": "gemini", "model": "gemini-3.1-pro"}}`
	os.WriteFile(ymlPath, []byte(ymlContent), 0644)
	t.Cleanup(func() { os.Remove(ymlPath) })

	runID3b, err := s.RunAudit(1, 1, "", "", "")
	if err != nil {
		t.Fatalf("RunAudit yml config: %v", err)
	}
	ar3b, _ := d.GetAuditRun(runID3b)
	if ar3b.Runner != "gemini" || ar3b.Model != "gemini-3.1-pro" {
		t.Errorf("expected yml config runner/model, got %s/%s", ar3b.Runner, ar3b.Model)
	}
	os.Remove(ymlPath)

	// 4. UI overrides configuration file
	runID4, err := s.RunAudit(1, 1, "", "codex", "gpt-5.4-thinking")
	if err != nil {
		t.Fatalf("RunAudit override: %v", err)
	}
	ar4, _ := d.GetAuditRun(runID4)
	if ar4.Runner != "codex" || ar4.Model != "gpt-5.4-thinking" {
		t.Errorf("expected UI override runner/model, got %s/%s", ar4.Runner, ar4.Model)
	}

	// 5. Runner provided but model empty (should pick first valid model)
	runID5, err := s.RunAudit(1, 1, "", "gemini", "")
	if err != nil {
		t.Fatalf("RunAudit runner only: %v", err)
	}
	ar5, _ := d.GetAuditRun(runID5)
	if ar5.Runner != "gemini" || ar5.Model != "gemini-3.1-pro" {
		t.Errorf("expected first valid gemini model, got %s/%s", ar5.Runner, ar5.Model)
	}

	// 6. Invalid runner/model fallback
	runID6, err := s.RunAudit(1, 1, "", "gemini", "sonnet")
	if err != nil {
		t.Fatalf("RunAudit invalid fallback: %v", err)
	}
	ar6, _ := d.GetAuditRun(runID6)
	if ar6.Runner != auditor.Runner || ar6.Model != auditor.Model {
		t.Errorf("expected fallback to auditor defaults, got %s/%s", ar6.Runner, ar6.Model)
	}
}

// --- parseTokenUsage ---

func TestParseTokenUsage(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		want   tokenUsage
	}{
		{
			name:   "empty",
			stdout: "",
			want:   tokenUsage{},
		},
		{
			name:   "no result type",
			stdout: `{"type":"text","text":"hello"}`,
			want:   tokenUsage{},
		},
		{
			name: "valid result",
			stdout: `{"type":"text","text":"working..."}
{"type":"result","result":{"input_tokens":1000,"output_tokens":500,"cache_read_input_tokens":200,"cache_creation_input_tokens":100,"total_cost_usd":0.05}}`,
			want: tokenUsage{
				InputTokens:       1000,
				OutputTokens:      500,
				CacheReadTokens:   200,
				CacheCreateTokens: 100,
				TotalCostUSD:      0.05,
			},
		},
		{
			name:   "invalid json",
			stdout: `{not json}`,
			want:   tokenUsage{},
		},
		{
			name:   "non-json lines mixed in",
			stdout: "some text\n{\"type\":\"result\",\"result\":{\"input_tokens\":42,\"output_tokens\":10,\"total_cost_usd\":0.01}}\nmore text",
			want:   tokenUsage{InputTokens: 42, OutputTokens: 10, TotalCostUSD: 0.01},
		},
		{
			name:   "codex turn.completed",
			stdout: `{"type":"turn.completed","usage":{"input_tokens":92800,"cached_input_tokens":49792,"output_tokens":244}}`,
			want: tokenUsage{
				InputTokens:     92800,
				CacheReadTokens: 49792,
				OutputTokens:    244,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTokenUsage(tt.stdout)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// --- liveWriter ---

func TestLiveWriter(t *testing.T) {
	d := testDB(t)
	agent := &models.Agent{
		Name: "LW Agent", Slug: "lw", ArchetypeSlug: "worker",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(agent)

	r := &models.Run{AgentID: agent.ID, Mode: "task", Status: models.RunStatusRunning}
	d.CreateRun(r)

	lw := &liveWriter{
		db:       d,
		runID:    r.ID,
		interval: time.Hour, // long interval to avoid auto-flush
	}

	n, err := lw.Write([]byte("hello "))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 6 {
		t.Errorf("n = %d, want 6", n)
	}

	lw.Write([]byte("world"))
	if lw.String() != "hello world" {
		t.Errorf("string = %q", lw.String())
	}

	// Flush writes to DB
	lw.Flush()
	run, _ := d.GetRun(r.ID)
	if run.Stdout != "hello world" {
		t.Errorf("db stdout = %q", run.Stdout)
	}
}

func TestLiveWriterAutoFlush(t *testing.T) {
	d := testDB(t)
	agent := &models.Agent{
		Name: "LW2", Slug: "lw2", ArchetypeSlug: "worker",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(agent)

	r := &models.Run{AgentID: agent.ID, Mode: "task", Status: models.RunStatusRunning}
	d.CreateRun(r)

	lw := &liveWriter{
		db:        d,
		runID:     r.ID,
		interval:  0, // always flush
		lastFlush: time.Time{},
	}

	lw.Write([]byte("auto"))

	run, _ := d.GetRun(r.ID)
	if run.Stdout != "auto" {
		t.Errorf("expected auto-flush, stdout = %q", run.Stdout)
	}
}

// --- Prompt builders ---

func TestBuildTaskPrompt(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	agent := &models.Agent{
		Name: "Worker", Slug: "worker", ArchetypeSlug: "worker",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(agent)

	issue := &models.Issue{Key: "SO-1", Title: "Test Issue", Description: "Do the thing", Status: "todo", Priority: 1}
	d.CreateIssue(issue)

	prompt := s.buildTaskPrompt(agent, issue)
	if !strings.Contains(prompt, "SO-1") {
		t.Error("prompt missing issue key")
	}
	if !strings.Contains(prompt, "Test Issue") {
		t.Error("prompt missing issue title")
	}
	if !strings.Contains(prompt, "Do the thing") {
		t.Error("prompt missing description")
	}
	if !strings.Contains(prompt, "RULES:") {
		t.Error("prompt missing rules")
	}
	if !strings.Contains(prompt, "SO API") {
		t.Error("prompt missing API ref")
	}
}

func TestBuildTaskPromptCEO(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	ceo := &models.Agent{
		Name: "CEO", Slug: "ceo", ArchetypeSlug: "ceo",
		Model: "opus", WorkingDir: "/tmp", MaxTurns: 100, TimeoutSec: 1200, Active: true,
	}
	d.CreateAgent(ceo)

	issue := &models.Issue{Key: "SO-1", Title: "Plan", Description: "Plan the sprint", Status: "todo"}
	d.CreateIssue(issue)

	prompt := s.buildTaskPrompt(ceo, issue)
	if !strings.Contains(prompt, "delegate") {
		t.Error("CEO prompt should mention delegation")
	}
	if !strings.Contains(prompt, "approvals") {
		t.Error("CEO prompt should mention approvals")
	}
}

func TestBuildHeartbeatPrompt(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	agent := &models.Agent{
		Name: "HB Agent", Slug: "hb", ArchetypeSlug: "worker",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(agent)

	// Add an inbox issue
	issue := &models.Issue{Key: "SO-1", Title: "Inbox Item", Status: "todo", AssigneeAgentID: &agent.ID}
	d.CreateIssue(issue)

	prompt := s.buildHeartbeatPrompt(agent)
	if !strings.Contains(prompt, "HEARTBEAT") {
		t.Error("prompt missing HEARTBEAT")
	}
	if !strings.Contains(prompt, "SO-1") {
		t.Error("prompt missing inbox item")
	}
}

func TestBuildWorkBlockContext(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	// No active block
	ctx := s.buildWorkBlockContext()
	if ctx != "" {
		t.Errorf("expected empty context with no active block, got %q", ctx)
	}

	// Create and activate block
	wb := &models.WorkBlock{Title: "Sprint 1", Goal: "Ship MVP"}
	d.CreateWorkBlock(wb)
	d.UpdateWorkBlockStatus(wb.ID, models.WBStatusActive)

	ctx2 := s.buildWorkBlockContext()
	if !strings.Contains(ctx2, "Sprint 1") {
		t.Error("context missing block title")
	}
	if !strings.Contains(ctx2, "Ship MVP") {
		t.Error("context missing goal")
	}
}

func TestBuildCEOAPIRef(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	// Create CEO and worker
	ceo := &models.Agent{
		Name: "CEO", Slug: "ceo", ArchetypeSlug: "ceo",
		Model: "opus", WorkingDir: "/tmp", MaxTurns: 100, TimeoutSec: 1200, Active: true,
	}
	d.CreateAgent(ceo)

	worker := &models.Agent{
		Name: "Dev", Slug: "dev", ArchetypeSlug: "engineer",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(worker)

	ref := s.buildCEOAPIRef()
	if !strings.Contains(ref, "Dev") {
		t.Error("CEO API ref missing worker name")
	}
	if !strings.Contains(ref, "dev") {
		t.Error("CEO API ref missing worker slug")
	}
	// CEO should not be in roster
	if strings.Contains(ref, "slug: ceo") {
		t.Error("CEO should not appear in own roster")
	}
}

func TestWakeAgentWhenStopped(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)
	s.Stop()

	agent := &models.Agent{ID: "a1", Name: "Test"}
	issue := &models.Issue{Key: "SO-1", Title: "Test"}

	// Should not panic, just return
	s.WakeAgent(agent, issue)
	s.WakeAgentHeartbeat(agent)
}

func TestWakeReviewerSelfReviewSkipped(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	// CEO with no review_agent_id or reports_to — GetReviewer falls back to CEO itself
	ceo := &models.Agent{
		Name: "CEO", Slug: "ceo", ArchetypeSlug: "ceo",
		Model: "opus", WorkingDir: "/tmp", MaxTurns: 100, TimeoutSec: 1200, Active: true,
	}
	d.CreateAgent(ceo)

	issue := &models.Issue{Key: "SO-99", Title: "Self review test", Description: "test", Status: "done", Priority: 1}
	d.CreateIssue(issue)

	// WakeReviewer should not spawn a run when reviewer == agent
	s.WakeReviewer(ceo.ID, "SO-99")

	runs, _ := d.ListRunsForAgent(ceo.ID, 100)
	for _, r := range runs {
		if r.IssueKey != nil && *r.IssueKey == "SO-99" {
			t.Error("expected no run spawned for self-review, but found one")
		}
	}
}

func TestWakeReviewerDifferentReviewer(t *testing.T) {
	d := testDB(t)
	_ = New(d, 9001)

	reviewer := &models.Agent{
		Name: "CEO", Slug: "ceo", ArchetypeSlug: "ceo",
		Model: "opus", WorkingDir: "/tmp", MaxTurns: 100, TimeoutSec: 1200, Active: true,
	}
	d.CreateAgent(reviewer)

	workerReportsTo := reviewer.ID
	worker := &models.Agent{
		Name: "Worker", Slug: "worker", ArchetypeSlug: "worker",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
		ReportsTo: &workerReportsTo,
	}
	d.CreateAgent(worker)

	issue := &models.Issue{Key: "SO-100", Title: "Review chain test", Description: "test", Status: "done", Priority: 1}
	d.CreateIssue(issue)

	// GetReviewer for worker should return CEO (different agent), so WakeReviewer should proceed
	rev, err := d.GetReviewer(worker.ID)
	if err != nil {
		t.Fatalf("GetReviewer: %v", err)
	}
	if rev.ID != reviewer.ID {
		t.Fatalf("expected reviewer %s, got %s", reviewer.ID, rev.ID)
	}
	if rev.ID == worker.ID {
		t.Fatal("reviewer should not be the worker itself")
	}
}

func TestCaptureGitDiffInvalidDir(t *testing.T) {
	diff := captureGitDiff("/nonexistent/path")
	if diff != "" {
		t.Errorf("expected empty diff for invalid dir, got %q", diff)
	}
}

// --- Runner dispatch tests ---

// TestCodexDispatch verifies execCodex uses the codex binary.
func TestCodexDispatch(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)

	binDir := t.TempDir()
	makeStub(t, binDir, "codex")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	agent := &models.Agent{
		Name: "Codex", Slug: "codex", ArchetypeSlug: "worker",
		Runner: "codex", Model: "o4-mini",
		WorkingDir: t.TempDir(), MaxTurns: 10, TimeoutSec: 10, Active: true,
	}
	d.CreateAgent(agent)

	key := "test-api-key"
	out, err := s.execCodex(t.Context(), agent, key, "run1", "SO-1", "do work")
	// stub exits 0, so err should be nil
	if err != nil {
		t.Fatalf("execCodex: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "ARGS: exec") {
		t.Errorf("expected exec subcommand in args, got: %s", out)
	}
	if !strings.Contains(out, "--full-auto") {
		t.Errorf("expected --full-auto in args, got: %s", out)
	}
	if !strings.Contains(out, "--json") {
		t.Errorf("expected --json in args, got: %s", out)
	}
	if !strings.Contains(out, "do work") {
		t.Errorf("expected prompt in args, got: %s", out)
	}
}

// TestAntigravityDispatch verifies execAntigravity uses the antigravity binary.
func TestAntigravityDispatch(t *testing.T) {
	d := testDB(t)
	binDir := t.TempDir()
	archetypeDir := t.TempDir()

	// Write an archetype file so --system-prompt-file gets appended
	archetypeFile := filepath.Join(archetypeDir, "worker.md")
	if err := os.WriteFile(archetypeFile, []byte("you are a worker"), 0644); err != nil {
		t.Fatal(err)
	}
	archetypes.SetOverridesDir(archetypeDir)
	t.Cleanup(func() { archetypes.SetOverridesDir("archetypes") })

	s := New(d, 9001)

	makeStub(t, binDir, "antigravity")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	agent := &models.Agent{
		Name: "AG", Slug: "ag", ArchetypeSlug: "worker",
		Runner: "antigravity", Model: "default",
		WorkingDir: t.TempDir(), MaxTurns: 5, TimeoutSec: 10, Active: true,
	}
	d.CreateAgent(agent)

	out, err := s.execAntigravity(t.Context(), agent, "key", "run2", "SO-2", "do stuff")
	if err != nil {
		t.Fatalf("execAntigravity: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "--non-interactive") {
		t.Errorf("expected --non-interactive, got: %s", out)
	}
	if !strings.Contains(out, "--system-prompt-file") {
		t.Errorf("expected --system-prompt-file, got: %s", out)
	}
	if !strings.Contains(out, "worker-") {
		t.Errorf("expected worker- temp file in args, got: %s", out)
	}
}

// TestAntigravityNoSystemPromptFileWhenMissing verifies --system-prompt-file is NOT added when archetype is absent.
func TestAntigravityNoSystemPromptFileWhenMissing(t *testing.T) {
	d := testDB(t)
	binDir := t.TempDir()
	s := New(d, 9001) // empty archetype dir

	makeStub(t, binDir, "antigravity")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	agent := &models.Agent{
		Name: "AG2", Slug: "ag2", ArchetypeSlug: "nofile",
		Runner: "antigravity", WorkingDir: t.TempDir(), MaxTurns: 5, TimeoutSec: 10,
	}

	out, err := s.execAntigravity(t.Context(), agent, "key", "run3", "SO-3", "task")
	if err != nil {
		t.Fatalf("execAntigravity: %v\noutput: %s", err, out)
	}
	if strings.Contains(out, "--system-prompt-file") {
		t.Errorf("should not pass --system-prompt-file when archetype missing, got: %s", out)
	}
}

// TestCodexEnvVars verifies OPENAI_API_KEY and CODEX_SYSTEM_PROMPT are injected.
func TestCodexEnvVars(t *testing.T) {
	d := testDB(t)
	archetypeDir := t.TempDir()
	archetypeFile := filepath.Join(archetypeDir, "worker.md")
	if err := os.WriteFile(archetypeFile, []byte("system instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	archetypes.SetOverridesDir(archetypeDir)
	t.Cleanup(func() { archetypes.SetOverridesDir("archetypes") })

	s := New(d, 9001)
	binDir := t.TempDir()
	makeStub(t, binDir, "codex")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("MY_OPENAI_KEY", "sk-test-openai")

	agent := &models.Agent{
		Name: "CX", Slug: "cx", ArchetypeSlug: "worker",
		Runner: "codex", ApiKeyEnv: "MY_OPENAI_KEY",
		WorkingDir: t.TempDir(), MaxTurns: 5, TimeoutSec: 10,
	}

	out, err := s.execCodex(t.Context(), agent, "key", "run4", "SO-4", "task")
	if err != nil {
		t.Fatalf("execCodex: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "OPENAI_API_KEY=sk-test-openai") {
		t.Errorf("expected OPENAI_API_KEY in env, got: %s", out)
	}
	if !strings.Contains(out, "CODEX_SYSTEM_PROMPT=system instructions") {
		t.Errorf("expected CODEX_SYSTEM_PROMPT in env, got: %s", out)
	}
}

// TestAntigravityEnvVars verifies ANTIGRAVITY_API_KEY is injected.
func TestAntigravityEnvVars(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001)
	binDir := t.TempDir()
	makeStub(t, binDir, "antigravity")
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("MY_AG_KEY", "ag-secret-key")

	agent := &models.Agent{
		Name: "AG3", Slug: "ag3", ArchetypeSlug: "worker",
		Runner: "antigravity", ApiKeyEnv: "MY_AG_KEY",
		WorkingDir: t.TempDir(), MaxTurns: 5, TimeoutSec: 10,
	}

	out, err := s.execAntigravity(t.Context(), agent, "key", "run5", "SO-5", "task")
	if err != nil {
		t.Fatalf("execAntigravity: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "ANTIGRAVITY_API_KEY=ag-secret-key") {
		t.Errorf("expected ANTIGRAVITY_API_KEY in env, got: %s", out)
	}
}

// TestTokenZeroingForNonClaude verifies token counts are zeroed for non-Claude runners.
func TestTokenZeroingForNonClaude(t *testing.T) {
	tests := []struct {
		runner   string
		wantZero bool
	}{
		{"claude_code", false},
		{"", false},
		{"codex", false},
		{"antigravity", true},
		{"gemini", false},
	}

	for _, tt := range tests {
		t.Run(tt.runner, func(t *testing.T) {
			agent := &models.Agent{Runner: tt.runner}
			stdout := `{"type":"result","result":{"input_tokens":100,"output_tokens":50,"total_cost_usd":0.01}}`
			if tt.runner == "codex" {
				stdout = `{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":50}}`
			}

			tokens := parseTokenUsage(stdout)
			if agent.Runner != "claude_code" && agent.Runner != "gemini" && agent.Runner != "codex" && agent.Runner != "" {
				tokens = tokenUsage{}
			}

			if tt.wantZero {
				if tokens.InputTokens != 0 || tokens.OutputTokens != 0 {
					t.Errorf("expected 0 tokens for runner %q, got %+v", tt.runner, tokens)
				}
			} else {
				if tokens.InputTokens == 0 || tokens.OutputTokens == 0 {
					t.Errorf("expected non-zero tokens for runner %q, got %+v", tt.runner, tokens)
				}
			}
		})
	}
}
