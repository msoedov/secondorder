package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/msoedov/secondorder/internal/models"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpen(t *testing.T) {
	d := testDB(t)
	if err := d.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestOpenInvalidPath(t *testing.T) {
	_, err := Open("/nonexistent/dir/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	d := testDB(t)
	// Running migrations again should be a no-op
	if err := d.RunMigrations(); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		filename string
		want     int
		wantErr  bool
	}{
		{"001_init.sql", 1, false},
		{"042_add_table.sql", 42, false},
		{"abc_bad.sql", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := parseVersion(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseVersion(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseVersion(%q) = %d, want %d", tt.filename, got, tt.want)
			}
		})
	}
}

// --- Agent CRUD ---

func makeAgent(slug string) *models.Agent {
	return &models.Agent{
		Name:          "Agent " + slug,
		Slug:          slug,
		ArchetypeSlug: "worker",
		Model:         "sonnet",
		WorkingDir:    "/tmp",
		MaxTurns:      50,
		TimeoutSec:    600,
		Active:        true,
	}
}

func TestCreateAndGetAgent(t *testing.T) {
	d := testDB(t)
	a := makeAgent("alice")
	if err := d.CreateAgent(a); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected ID to be set")
	}

	got, err := d.GetAgent(a.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Slug != "alice" {
		t.Errorf("slug = %q, want alice", got.Slug)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	d := testDB(t)
	_, err := d.GetAgent("nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestGetAgentBySlug(t *testing.T) {
	d := testDB(t)
	a := makeAgent("bob")
	d.CreateAgent(a)

	got, err := d.GetAgentBySlug("bob")
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	if got.ID != a.ID {
		t.Errorf("ID mismatch")
	}
}

func TestListAgents(t *testing.T) {
	d := testDB(t)
	d.CreateAgent(makeAgent("a1"))
	d.CreateAgent(makeAgent("a2"))

	agents, err := d.ListAgents()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("got %d agents, want 2", len(agents))
	}
}

func TestUpdateAgent(t *testing.T) {
	d := testDB(t)
	a := makeAgent("update-me")
	d.CreateAgent(a)

	a.Name = "Updated"
	if err := d.UpdateAgent(a); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := d.GetAgent(a.ID)
	if got.Name != "Updated" {
		t.Errorf("name = %q, want Updated", got.Name)
	}
}

func TestGetCEOAgent(t *testing.T) {
	d := testDB(t)
	ceo := makeAgent("ceo")
	ceo.ArchetypeSlug = "ceo"
	d.CreateAgent(ceo)

	got, err := d.GetCEOAgent()
	if err != nil {
		t.Fatalf("get ceo: %v", err)
	}
	if got.Slug != "ceo" {
		t.Errorf("slug = %q, want ceo", got.Slug)
	}
}

func TestGetReviewer(t *testing.T) {
	d := testDB(t)

	// Create CEO
	ceo := makeAgent("ceo-reviewer")
	ceo.ArchetypeSlug = "ceo"
	d.CreateAgent(ceo)

	// Agent with explicit reviewer
	reviewer := makeAgent("reviewer")
	d.CreateAgent(reviewer)

	worker := makeAgent("worker")
	worker.ReviewAgentID = &reviewer.ID
	d.CreateAgent(worker)

	got, err := d.GetReviewer(worker.ID)
	if err != nil {
		t.Fatalf("get reviewer: %v", err)
	}
	if got.ID != reviewer.ID {
		t.Errorf("expected explicit reviewer, got %s", got.Slug)
	}

	// Agent with reports_to chain
	worker2 := makeAgent("worker2")
	worker2.ReportsTo = &ceo.ID
	d.CreateAgent(worker2)

	got2, err := d.GetReviewer(worker2.ID)
	if err != nil {
		t.Fatalf("get reviewer via reports_to: %v", err)
	}
	if got2.ID != ceo.ID {
		t.Errorf("expected CEO via reports_to, got %s", got2.Slug)
	}

	// Agent with no reviewer falls back to CEO
	worker3 := makeAgent("worker3")
	d.CreateAgent(worker3)

	got3, err := d.GetReviewer(worker3.ID)
	if err != nil {
		t.Fatalf("get reviewer fallback: %v", err)
	}
	if got3.ArchetypeSlug != "ceo" {
		t.Errorf("expected CEO fallback, got %s", got3.Slug)
	}
}

func TestCountAgents(t *testing.T) {
	d := testDB(t)
	d.CreateAgent(makeAgent("c1"))
	inactive := makeAgent("c2")
	inactive.Active = false
	d.CreateAgent(inactive)

	total, active, err := d.CountAgents()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if active != 1 {
		t.Errorf("active = %d, want 1", active)
	}
}

// --- Issue CRUD ---

func makeIssue(key string) *models.Issue {
	return &models.Issue{
		Key:         key,
		Title:       "Issue " + key,
		Description: "desc",
		Status:      models.StatusTodo,
		Priority:    1,
	}
}

func TestCreateAndGetIssue(t *testing.T) {
	d := testDB(t)
	i := makeIssue("SO-1")
	if err := d.CreateIssue(i); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := d.GetIssue("SO-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Issue SO-1" {
		t.Errorf("title = %q", got.Title)
	}
}

func TestGetIssueNotFound(t *testing.T) {
	d := testDB(t)
	_, err := d.GetIssue("SO-999")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestListIssues(t *testing.T) {
	d := testDB(t)
	d.CreateIssue(makeIssue("SO-1"))
	done := makeIssue("SO-2")
	done.Status = models.StatusDone
	d.CreateIssue(done)

	// All issues
	all, err := d.ListIssues("", 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("got %d, want 2", len(all))
	}

	// Filter by status
	todos, err := d.ListIssues("todo", 0)
	if err != nil {
		t.Fatalf("list todo: %v", err)
	}
	if len(todos) != 1 {
		t.Errorf("got %d todos, want 1", len(todos))
	}

	// Limit
	limited, err := d.ListIssues("", 1)
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("got %d, want 1", len(limited))
	}
}

func TestUpdateIssue(t *testing.T) {
	d := testDB(t)
	i := makeIssue("SO-1")
	d.CreateIssue(i)

	i.Status = models.StatusDone
	i.Title = "Updated"
	if err := d.UpdateIssue(i); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := d.GetIssue("SO-1")
	if got.Status != models.StatusDone {
		t.Errorf("status = %q", got.Status)
	}
	if got.Title != "Updated" {
		t.Errorf("title = %q", got.Title)
	}
}

func TestCheckoutIssue(t *testing.T) {
	d := testDB(t)
	a := makeAgent("checkout-agent")
	d.CreateAgent(a)
	i := makeIssue("SO-1")
	d.CreateIssue(i)

	// Successful checkout
	err := d.CheckoutIssue("SO-1", a.ID, []string{models.StatusTodo})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	got, _ := d.GetIssue("SO-1")
	if got.Status != models.StatusInProgress {
		t.Errorf("status = %q, want in_progress", got.Status)
	}

	// Checkout again should fail (already in_progress, not in expected)
	err = d.CheckoutIssue("SO-1", a.ID, []string{models.StatusTodo})
	if err == nil {
		t.Fatal("expected error for double checkout")
	}
}

func TestCheckoutIssueEmptyStatuses(t *testing.T) {
	d := testDB(t)
	err := d.CheckoutIssue("SO-1", "agent", nil)
	if err == nil {
		t.Fatal("expected error for empty statuses")
	}
}

func TestGetAgentInbox(t *testing.T) {
	d := testDB(t)
	a := makeAgent("inbox-agent")
	d.CreateAgent(a)

	todo := makeIssue("SO-1")
	todo.AssigneeAgentID = &a.ID
	d.CreateIssue(todo)

	done := makeIssue("SO-2")
	done.AssigneeAgentID = &a.ID
	done.Status = models.StatusDone
	d.CreateIssue(done)

	inbox, err := d.GetAgentInbox(a.ID)
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Errorf("got %d, want 1 (done excluded)", len(inbox))
	}
}

func TestCountIssues(t *testing.T) {
	d := testDB(t)
	d.CreateIssue(makeIssue("SO-1"))
	done := makeIssue("SO-2")
	done.Status = models.StatusDone
	d.CreateIssue(done)

	total, open, err := d.CountIssues()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if open != 1 {
		t.Errorf("open = %d, want 1", open)
	}
}

func TestGetChildIssues(t *testing.T) {
	d := testDB(t)
	parent := makeIssue("SO-1")
	d.CreateIssue(parent)

	parentKey := "SO-1"
	child := makeIssue("SO-2")
	child.ParentIssueKey = &parentKey
	d.CreateIssue(child)

	children, err := d.GetChildIssues("SO-1")
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(children) != 1 {
		t.Errorf("got %d, want 1", len(children))
	}
}

func TestNextIssueKey(t *testing.T) {
	d := testDB(t)

	// Empty db
	key, err := d.NextIssueKey()
	if err != nil {
		t.Fatalf("next key: %v", err)
	}
	if key != "SO-1" {
		t.Errorf("got %q, want SO-1", key)
	}

	d.CreateIssue(makeIssue("SO-1"))
	d.CreateIssue(makeIssue("SO-5"))

	key2, err := d.NextIssueKey()
	if err != nil {
		t.Fatalf("next key: %v", err)
	}
	if key2 != "SO-6" {
		t.Errorf("got %q, want SO-6", key2)
	}
}

// --- Runs ---

func TestCreateAndGetRun(t *testing.T) {
	d := testDB(t)
	a := makeAgent("run-agent")
	d.CreateAgent(a)
	i := makeIssue("SO-1")
	d.CreateIssue(i)

	issueKey := "SO-1"
	r := &models.Run{
		AgentID:  a.ID,
		IssueKey: &issueKey,
		Mode:     "task",
		Status:   models.RunStatusRunning,
	}
	if err := d.CreateRun(r); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if r.ID == "" {
		t.Fatal("expected ID")
	}

	got, err := d.GetRun(r.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != models.RunStatusRunning {
		t.Errorf("status = %q", got.Status)
	}
}

func TestCompleteRun(t *testing.T) {
	d := testDB(t)
	a := makeAgent("complete-agent")
	d.CreateAgent(a)

	r := &models.Run{AgentID: a.ID, Mode: "task", Status: models.RunStatusRunning}
	d.CreateRun(r)

	tokens := models.Run{InputTokens: 100, OutputTokens: 50, TotalCostUSD: 0.01}
	if err := d.CompleteRun(r.ID, models.RunStatusCompleted, "output", "diff", tokens); err != nil {
		t.Fatalf("complete: %v", err)
	}

	got, _ := d.GetRun(r.ID)
	if got.Status != models.RunStatusCompleted {
		t.Errorf("status = %q", got.Status)
	}
	if got.InputTokens != 100 {
		t.Errorf("input tokens = %d", got.InputTokens)
	}
	if got.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestUpdateRunStdout(t *testing.T) {
	d := testDB(t)
	a := makeAgent("stdout-agent")
	d.CreateAgent(a)
	r := &models.Run{AgentID: a.ID, Mode: "task", Status: models.RunStatusRunning}
	d.CreateRun(r)

	d.UpdateRunStdout(r.ID, "hello world")
	got, _ := d.GetRun(r.ID)
	if got.Stdout != "hello world" {
		t.Errorf("stdout = %q", got.Stdout)
	}
}

func TestListRunsForAgent(t *testing.T) {
	d := testDB(t)
	a := makeAgent("list-runs")
	d.CreateAgent(a)
	d.CreateRun(&models.Run{AgentID: a.ID, Mode: "task", Status: models.RunStatusRunning})
	d.CreateRun(&models.Run{AgentID: a.ID, Mode: "heartbeat", Status: models.RunStatusCompleted})

	runs, err := d.ListRunsForAgent(a.ID, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("got %d, want 2", len(runs))
	}

	// With limit
	limited, _ := d.ListRunsForAgent(a.ID, 1)
	if len(limited) != 1 {
		t.Errorf("got %d, want 1", len(limited))
	}
}

func TestListRunsForIssue(t *testing.T) {
	d := testDB(t)
	a := makeAgent("issue-runs")
	d.CreateAgent(a)
	d.CreateIssue(makeIssue("SO-1"))
	issueKey := "SO-1"
	d.CreateRun(&models.Run{AgentID: a.ID, IssueKey: &issueKey, Mode: "task", Status: models.RunStatusRunning})

	runs, err := d.ListRunsForIssue("SO-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("got %d, want 1", len(runs))
	}
}

func TestCountRunningRuns(t *testing.T) {
	d := testDB(t)
	a := makeAgent("count-runs")
	d.CreateAgent(a)
	d.CreateRun(&models.Run{AgentID: a.ID, Mode: "task", Status: models.RunStatusRunning})
	d.CreateRun(&models.Run{AgentID: a.ID, Mode: "task", Status: models.RunStatusCompleted})

	count, err := d.CountRunningRuns()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d, want 1", count)
	}
}

func TestCountRunsForIssue(t *testing.T) {
	d := testDB(t)
	a := makeAgent("count-issue-runs")
	d.CreateAgent(a)
	d.CreateIssue(makeIssue("SO-1"))
	issueKey := "SO-1"
	d.CreateRun(&models.Run{AgentID: a.ID, IssueKey: &issueKey, Mode: "task", Status: models.RunStatusRunning})

	count, err := d.CountRunsForIssue("SO-1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d, want 1", count)
	}
}

// --- Comments ---

func TestCreateAndListComments(t *testing.T) {
	d := testDB(t)
	d.CreateIssue(makeIssue("SO-1"))

	c := &models.Comment{
		IssueKey: "SO-1",
		Author:   "system",
		Body:     "hello",
	}
	if err := d.CreateComment(c); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	comments, err := d.ListComments("SO-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 1 {
		t.Errorf("got %d, want 1", len(comments))
	}
	if comments[0].Body != "hello" {
		t.Errorf("body = %q", comments[0].Body)
	}
}

// --- API Keys ---

func TestAPIKeyLifecycle(t *testing.T) {
	d := testDB(t)
	a := makeAgent("api-key-agent")
	d.CreateAgent(a)

	if err := d.CreateAPIKey(a.ID, "hash123", "so_abc"); err != nil {
		t.Fatalf("create key: %v", err)
	}

	got, err := d.GetAgentByAPIKey("hash123")
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if got.ID != a.ID {
		t.Errorf("agent ID mismatch")
	}

	// Revoke
	if err := d.RevokeAPIKeys(a.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, err = d.GetAgentByAPIKey("hash123")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows after revoke, got %v", err)
	}
}

// --- Approvals ---

func TestApprovalLifecycle(t *testing.T) {
	d := testDB(t)
	d.CreateIssue(makeIssue("SO-1"))
	a := makeAgent("approver")
	d.CreateAgent(a)

	approval := &models.Approval{
		IssueKey:    "SO-1",
		RequestedBy: a.ID,
		Status:      "pending",
	}
	if err := d.CreateApproval(approval); err != nil {
		t.Fatalf("create: %v", err)
	}

	pending, err := d.ListPendingApprovals()
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("got %d pending, want 1", len(pending))
	}

	if err := d.ResolveApproval(approval.ID, "approved", "lgtm"); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	got, _ := d.GetApproval(approval.ID)
	if got.Status != "approved" {
		t.Errorf("status = %q", got.Status)
	}
	if got.ResolvedAt == nil {
		t.Error("expected ResolvedAt to be set")
	}

	// No longer pending
	pending2, _ := d.ListPendingApprovals()
	if len(pending2) != 0 {
		t.Errorf("got %d pending after resolve", len(pending2))
	}
}

// --- Cost / Budget ---

func TestCostAndUsage(t *testing.T) {
	d := testDB(t)
	a := makeAgent("cost-agent")
	d.CreateAgent(a)
	r := &models.Run{AgentID: a.ID, Mode: "task", Status: models.RunStatusCompleted}
	d.CreateRun(r)

	// Insert with UTC timestamp so DATE('now') matches
	d.Exec(`INSERT INTO cost_events (id, run_id, agent_id, input_tokens, output_tokens, total_cost_usd, created_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
		"ce1", r.ID, a.ID, 1000, 500, 0.05)

	todayTokens, todayCost, totalTokens, totalCost, err := d.GetAgentUsage(a.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if totalTokens != 1500 {
		t.Errorf("total tokens = %d, want 1500", totalTokens)
	}
	if totalCost != 0.05 {
		t.Errorf("total cost = %f, want 0.05", totalCost)
	}
	if todayTokens != 1500 {
		t.Errorf("today tokens = %d, want 1500", todayTokens)
	}
	if todayCost != 0.05 {
		t.Errorf("today cost = %f, want 0.05", todayCost)
	}
}

func TestIsAgentOverBudget(t *testing.T) {
	d := testDB(t)
	a := makeAgent("budget-agent")
	d.CreateAgent(a)

	// No policy = not over budget
	over, err := d.IsAgentOverBudget(a.ID)
	if err != nil {
		t.Fatalf("budget check: %v", err)
	}
	if over {
		t.Error("expected not over budget with no policy")
	}

	// Add policy and cost event with UTC timestamps
	d.Exec(`INSERT INTO budget_policies (id, agent_id, daily_token_limit, daily_cost_limit, active, created_at)
		VALUES (?, ?, ?, ?, 1, datetime('now'))`, "bp1", a.ID, 1000, 0.10)

	r := &models.Run{AgentID: a.ID, Mode: "task", Status: models.RunStatusCompleted}
	d.CreateRun(r)
	d.Exec(`INSERT INTO cost_events (id, run_id, agent_id, input_tokens, output_tokens, total_cost_usd, created_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
		"ce-budget", r.ID, a.ID, 600, 500, 0.05)

	over2, err := d.IsAgentOverBudget(a.ID)
	if err != nil {
		t.Fatalf("budget check: %v", err)
	}
	if !over2 {
		t.Error("expected over budget (1100 tokens >= 1000 limit)")
	}
}

func TestGetTotalCostToday(t *testing.T) {
	d := testDB(t)
	a := makeAgent("today-cost")
	d.CreateAgent(a)
	r := &models.Run{AgentID: a.ID, Mode: "task", Status: models.RunStatusCompleted}
	d.CreateRun(r)
	// Insert with UTC timestamp so DATE('now') matches
	d.Exec(`INSERT INTO cost_events (id, run_id, agent_id, input_tokens, output_tokens, total_cost_usd, created_at)
		VALUES (?, ?, ?, 0, 0, ?, datetime('now'))`,
		"ce-today", r.ID, a.ID, 1.23)

	cost, err := d.GetTotalCostToday()
	if err != nil {
		t.Fatalf("cost today: %v", err)
	}
	if cost != 1.23 {
		t.Errorf("got %f, want 1.23", cost)
	}
}

// --- Activity ---

func TestLogActivity(t *testing.T) {
	d := testDB(t)
	err := d.LogActivity("created", "issue", "SO-1", nil, "test")
	if err != nil {
		t.Fatalf("log activity: %v", err)
	}
}

// --- Dashboard ---

func TestGetDashboardStats(t *testing.T) {
	d := testDB(t)
	d.CreateAgent(makeAgent("dash-agent"))
	d.CreateIssue(makeIssue("SO-1"))

	stats, err := d.GetDashboardStats()
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if stats.TotalAgents != 1 {
		t.Errorf("agents = %d", stats.TotalAgents)
	}
	if stats.TotalIssues != 1 {
		t.Errorf("issues = %d", stats.TotalIssues)
	}
}

// --- Labels ---

func TestLabels(t *testing.T) {
	d := testDB(t)
	l := &models.Label{Name: "bug", Color: "red"}
	if err := d.CreateLabel(l); err != nil {
		t.Fatalf("create label: %v", err)
	}

	labels, err := d.ListLabels()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(labels) != 1 {
		t.Errorf("got %d, want 1", len(labels))
	}

	// Add to issue
	d.CreateIssue(makeIssue("SO-1"))
	issue, _ := d.GetIssue("SO-1")
	if err := d.AddLabelToIssue(issue.ID, l.ID); err != nil {
		t.Fatalf("add label: %v", err)
	}
	// Idempotent
	if err := d.AddLabelToIssue(issue.ID, l.ID); err != nil {
		t.Fatalf("add label again: %v", err)
	}
}

// --- Work Blocks ---

func TestWorkBlockLifecycle(t *testing.T) {
	d := testDB(t)

	wb := &models.WorkBlock{Title: "Sprint 1", Goal: "Ship MVP"}
	if err := d.CreateWorkBlock(wb); err != nil {
		t.Fatalf("create: %v", err)
	}
	if wb.Status != models.WBStatusProposed {
		t.Errorf("status = %q, want proposed", wb.Status)
	}

	// Can't create another while proposed exists
	wb2 := &models.WorkBlock{Title: "Sprint 2", Goal: "More"}
	if err := d.CreateWorkBlock(wb2); err == nil {
		t.Fatal("expected error: active/proposed already exists")
	}

	// Activate
	if err := d.UpdateWorkBlockStatus(wb.ID, models.WBStatusActive); err != nil {
		t.Fatalf("activate: %v", err)
	}

	got, _ := d.GetWorkBlock(wb.ID)
	if got.Status != models.WBStatusActive {
		t.Errorf("status = %q", got.Status)
	}

	// Get active
	active, err := d.GetActiveWorkBlock()
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active.ID != wb.ID {
		t.Error("wrong active block")
	}

	// Ready
	if err := d.UpdateWorkBlockStatus(wb.ID, models.WBStatusReady); err != nil {
		t.Fatalf("ready: %v", err)
	}

	// Ship
	if err := d.UpdateWorkBlockStatus(wb.ID, models.WBStatusShipped); err != nil {
		t.Fatalf("ship: %v", err)
	}

	shipped, _ := d.GetWorkBlock(wb.ID)
	if shipped.CompletedAt == nil {
		t.Error("expected CompletedAt to be set on ship")
	}
}

func TestWorkBlockInvalidTransition(t *testing.T) {
	d := testDB(t)
	wb := &models.WorkBlock{Title: "T", Goal: "G"}
	d.CreateWorkBlock(wb)

	tests := []struct {
		name      string
		newStatus string
	}{
		{"proposed_to_ready", models.WBStatusReady},
		{"proposed_to_shipped", models.WBStatusShipped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.UpdateWorkBlockStatus(wb.ID, tt.newStatus); err == nil {
				t.Error("expected error for invalid transition")
			}
		})
	}
}

func TestAssignIssueToWorkBlock(t *testing.T) {
	d := testDB(t)
	wb := &models.WorkBlock{Title: "T", Goal: "G"}
	d.CreateWorkBlock(wb)

	d.CreateIssue(makeIssue("SO-1"))

	// Can't assign to proposed block
	if err := d.AssignIssueToWorkBlock("SO-1", wb.ID); err == nil {
		t.Fatal("expected error: block not active")
	}

	// Activate and assign
	d.UpdateWorkBlockStatus(wb.ID, models.WBStatusActive)
	if err := d.AssignIssueToWorkBlock("SO-1", wb.ID); err != nil {
		t.Fatalf("assign: %v", err)
	}

	issues, _ := d.ListWorkBlockIssues(wb.ID)
	if len(issues) != 1 {
		t.Errorf("got %d issues, want 1", len(issues))
	}

	// Unassign
	d.UnassignIssueFromWorkBlock("SO-1")
	issues2, _ := d.ListWorkBlockIssues(wb.ID)
	if len(issues2) != 0 {
		t.Errorf("got %d issues after unassign", len(issues2))
	}
}

func TestCheckWorkBlockAutoReady(t *testing.T) {
	d := testDB(t)
	wb := &models.WorkBlock{Title: "T", Goal: "G"}
	d.CreateWorkBlock(wb)
	d.UpdateWorkBlockStatus(wb.ID, models.WBStatusActive)

	// No issues = not ready (total == 0)
	ready, _ := d.CheckWorkBlockAutoReady(wb.ID)
	if ready {
		t.Error("expected not ready with no issues")
	}

	// Add done issue
	done := makeIssue("SO-1")
	done.Status = models.StatusDone
	done.WorkBlockID = &wb.ID
	d.CreateIssue(done)

	ready2, _ := d.CheckWorkBlockAutoReady(wb.ID)
	if !ready2 {
		t.Error("expected ready with all done")
	}

	// Add in_progress issue
	ip := makeIssue("SO-2")
	ip.Status = models.StatusInProgress
	ip.WorkBlockID = &wb.ID
	d.CreateIssue(ip)

	ready3, _ := d.CheckWorkBlockAutoReady(wb.ID)
	if ready3 {
		t.Error("expected not ready with in_progress issue")
	}
}

func TestGetWorkBlockStats(t *testing.T) {
	d := testDB(t)
	wb := &models.WorkBlock{Title: "T", Goal: "G"}
	d.CreateWorkBlock(wb)
	d.UpdateWorkBlockStatus(wb.ID, models.WBStatusActive)

	done := makeIssue("SO-1")
	done.Status = models.StatusDone
	done.WorkBlockID = &wb.ID
	d.CreateIssue(done)

	stats, err := d.GetWorkBlockStats(wb.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.IssuesPlanned != 1 {
		t.Errorf("planned = %d", stats.IssuesPlanned)
	}
	if stats.IssuesCompleted != 1 {
		t.Errorf("completed = %d", stats.IssuesCompleted)
	}
}

func TestListWorkBlocks(t *testing.T) {
	d := testDB(t)
	wb := &models.WorkBlock{Title: "T", Goal: "G"}
	d.CreateWorkBlock(wb)

	blocks, err := d.ListWorkBlocks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(blocks) != 1 {
		t.Errorf("got %d, want 1", len(blocks))
	}
}

// TestOpenWithEnvOverride verifies the DB can be opened from a temp file
func TestOpenWithTempFile(t *testing.T) {
	f, err := os.CreateTemp("", "so-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	d, err := Open(f.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	d.Close()
}

// --- GetIssueByTitle ---

func TestGetIssueByTitle(t *testing.T) {
	d := testDB(t)

	issue := &models.Issue{
		Key:    "SO-1",
		Title:  "Fix Login Bug",
		Status: "todo",
	}
	if err := d.CreateIssue(issue); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	tests := []struct {
		name    string
		title   string
		wantKey string
		wantErr bool
	}{
		{"exact match", "Fix Login Bug", "SO-1", false},
		{"case insensitive lower", "fix login bug", "SO-1", false},
		{"case insensitive upper", "FIX LOGIN BUG", "SO-1", false},
		{"case insensitive mixed", "fix Login BUG", "SO-1", false},
		{"no match", "Different Title", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.GetIssueByTitle(tt.title)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Key != tt.wantKey {
				t.Errorf("key = %q, want %q", got.Key, tt.wantKey)
			}
		})
	}
}

func TestSettings(t *testing.T) {
	d := testDB(t)

	t.Run("defaults seeded by migration", func(t *testing.T) {
		val, err := d.GetSetting("issue_prefix")
		if err != nil {
			t.Fatalf("get issue_prefix: %v", err)
		}
		if val != "SO" {
			t.Errorf("issue_prefix = %q, want %q", val, "SO")
		}
	})

	t.Run("get all settings returns seeded keys", func(t *testing.T) {
		all, err := d.GetAllSettings()
		if err != nil {
			t.Fatalf("get all settings: %v", err)
		}
		expected := []string{"issue_prefix", "telegram_token", "telegram_chat_id", "github_url", "instance_name"}
		for _, k := range expected {
			if _, ok := all[k]; !ok {
				t.Errorf("missing expected key %q", k)
			}
		}
		if len(all) != len(expected) {
			t.Errorf("got %d settings, want %d", len(all), len(expected))
		}
	})

	t.Run("set existing setting", func(t *testing.T) {
		if err := d.SetSetting("issue_prefix", "PRJ"); err != nil {
			t.Fatalf("set: %v", err)
		}
		val, err := d.GetSetting("issue_prefix")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if val != "PRJ" {
			t.Errorf("got %q, want %q", val, "PRJ")
		}
	})

	t.Run("set new setting", func(t *testing.T) {
		if err := d.SetSetting("custom_key", "custom_value"); err != nil {
			t.Fatalf("set: %v", err)
		}
		val, err := d.GetSetting("custom_key")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if val != "custom_value" {
			t.Errorf("got %q, want %q", val, "custom_value")
		}
	})

	t.Run("get nonexistent setting returns error", func(t *testing.T) {
		_, err := d.GetSetting("nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent key")
		}
	})
}

// --- BoardPolicy CRUD ---

func TestBoardPolicyDefaultsToInactive(t *testing.T) {
	d := testDB(t)
	bp := &models.BoardPolicy{Directive: "test directive"}
	if err := d.CreateBoardPolicy(bp); err != nil {
		t.Fatalf("create: %v", err)
	}
	if bp.Active {
		t.Error("new policy should default to inactive, got active=true")
	}
	if bp.ID == "" {
		t.Error("ID should be set after create")
	}
}

func TestGetActiveBoardPoliciesExcludesInactive(t *testing.T) {
	d := testDB(t)
	d.CreateBoardPolicy(&models.BoardPolicy{Directive: "inactive one"})

	active, err := d.GetActiveBoardPolicies()
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active policies, got %d", len(active))
	}
}

func TestToggleBoardPolicyActivatesAndDeactivates(t *testing.T) {
	d := testDB(t)
	bp := &models.BoardPolicy{Directive: "toggle me"}
	d.CreateBoardPolicy(bp)

	// toggle on
	if err := d.ToggleBoardPolicy(bp.ID); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	active, _ := d.GetActiveBoardPolicies()
	if len(active) != 1 || active[0].ID != bp.ID {
		t.Errorf("expected 1 active policy after toggle, got %d", len(active))
	}

	// toggle off
	if err := d.ToggleBoardPolicy(bp.ID); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	active, _ = d.GetActiveBoardPolicies()
	if len(active) != 0 {
		t.Errorf("expected 0 active policies after second toggle, got %d", len(active))
	}
}

func TestExistingActivePoliciesUnaffectedByCreate(t *testing.T) {
	d := testDB(t)

	// create and manually activate one policy
	existing := &models.BoardPolicy{Directive: "existing active"}
	d.CreateBoardPolicy(existing)
	d.ToggleBoardPolicy(existing.ID)

	// create a new policy (should be inactive)
	d.CreateBoardPolicy(&models.BoardPolicy{Directive: "new one"})

	active, _ := d.GetActiveBoardPolicies()
	if len(active) != 1 {
		t.Errorf("expected 1 active policy, got %d", len(active))
	}
	if active[0].ID != existing.ID {
		t.Errorf("expected original policy to remain active")
	}
}

func TestGetActiveBoardPoliciesOnlyReturnsActive(t *testing.T) {
	d := testDB(t)
	cases := []struct {
		directive string
		activate  bool
	}{
		{"policy A", true},
		{"policy B", false},
		{"policy C", true},
	}
	ids := map[string]string{}
	for _, c := range cases {
		bp := &models.BoardPolicy{Directive: c.directive}
		d.CreateBoardPolicy(bp)
		ids[c.directive] = bp.ID
		if c.activate {
			d.ToggleBoardPolicy(bp.ID)
		}
	}

	active, err := d.GetActiveBoardPolicies()
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active policies, got %d", len(active))
	}
	for _, p := range active {
		if !p.Active {
			t.Errorf("policy %q returned by GetActiveBoardPolicies but active=false", p.Directive)
		}
	}
}
