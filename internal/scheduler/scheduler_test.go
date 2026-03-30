package scheduler

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

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
	s := New(d, 9001, "/tmp/archetypes")
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if s.port != 9001 {
		t.Errorf("port = %d, want 9001", s.port)
	}
}

func TestSetCallbacks(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001, "/tmp")

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
	s := New(d, 9001, "/tmp")
	s.Stop()
	if !s.stopped {
		t.Error("expected stopped=true")
	}
}

func TestProvisionAPIKey(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001, "/tmp")

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
	s := New(d, 9001, "/tmp")

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
	s := New(d, 9001, "/tmp")

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
	s := New(d, 9001, "/tmp")

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
	s := New(d, 9001, "/tmp")

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
	s := New(d, 9001, "/tmp")

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
	s := New(d, 9001, "/tmp")
	s.Stop()

	agent := &models.Agent{ID: "a1", Name: "Test"}
	issue := &models.Issue{Key: "SO-1", Title: "Test"}

	// Should not panic, just return
	s.WakeAgent(agent, issue)
	s.WakeAgentHeartbeat(agent)
}

func TestWakeReviewerSelfReviewSkipped(t *testing.T) {
	d := testDB(t)
	s := New(d, 9001, "/tmp")

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
	_ = New(d, 9001, "/tmp")

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
