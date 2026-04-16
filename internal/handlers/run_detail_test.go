package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/msoedov/mesa/internal/models"
)

func TestRunDetail_Breadcrumb(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	// 1. Create Agent
	agent := &models.Agent{
		ID:   uuid.New().String(),
		Name: "Test Agent",
		Slug: "test-agent",
	}
	d.CreateAgent(agent)

	// 2. Create Parent Issue
	parent := &models.Issue{
		ID:    uuid.New().String(),
		Key:   "SO-100",
		Title: "Parent Issue",
	}
	d.CreateIssue(parent)

	// 3. Create Child Issue
	child := &models.Issue{
		ID:             uuid.New().String(),
		Key:            "SO-101",
		Title:          "Child Issue",
		ParentIssueKey: ptr("SO-100"),
	}
	d.CreateIssue(child)

	// 4. Create Run for Child Issue
	run := &models.Run{
		ID:       uuid.New().String(),
		AgentID:  agent.ID,
		IssueKey: ptr("SO-101"),
		Status:   "completed",
	}
	d.CreateRun(run)

	req := httptest.NewRequest("GET", "/runs/"+run.ID, nil)
	req.SetPathValue("id", run.ID)
	w := httptest.NewRecorder()

	ui.RunDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()

	// Verify Agent link
	if !strings.Contains(body, "/agents/test-agent") {
		t.Error("body missing agent link")
	}
	if !strings.Contains(body, "Test Agent") {
		t.Error("body missing agent name")
	}

	// Verify Parent Issue link
	if !strings.Contains(body, "/issues/SO-100") {
		t.Error("body missing parent issue link /issues/SO-100")
	}
	if !strings.Contains(body, "SO-100") {
		t.Error("body missing parent issue key SO-100")
	}

	// Verify Child Issue link
	if !strings.Contains(body, "/issues/SO-101") {
		t.Error("body missing child issue link /issues/SO-101")
	}
	if !strings.Contains(body, "SO-101") {
		t.Error("body missing child issue key SO-101")
	}

	// Verify Run text
	if !strings.Contains(body, "Run") {
		t.Error("body missing 'Run' text")
	}
	if !strings.Contains(body, run.ID[:8]) {
		t.Errorf("body missing run ID %s", run.ID[:8])
	}
}

func TestRunDetail_Breadcrumb_NoParent(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	// 1. Create Agent
	agent := &models.Agent{
		ID:   uuid.New().String(),
		Name: "Test Agent",
		Slug: "test-agent",
	}
	d.CreateAgent(agent)

	// 2. Create Issue with NO parent
	issue := &models.Issue{
		ID:    uuid.New().String(),
		Key:   "SO-200",
		Title: "Independent Issue",
	}
	d.CreateIssue(issue)

	// 3. Create Run for Issue
	run := &models.Run{
		ID:       uuid.New().String(),
		AgentID:  agent.ID,
		IssueKey: ptr("SO-200"),
		Status:   "completed",
	}
	d.CreateRun(run)

	req := httptest.NewRequest("GET", "/runs/"+run.ID, nil)
	req.SetPathValue("id", run.ID)
	w := httptest.NewRecorder()

	ui.RunDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()

	// Verify Agent link
	if !strings.Contains(body, "/agents/test-agent") {
		t.Error("body missing agent link")
	}

	// Verify Issue link
	if !strings.Contains(body, "/issues/SO-200") {
		t.Error("body missing issue link /issues/SO-200")
	}

	// Verify Parent Issue link is NOT present
	// We expect only one /issues/ link in this case (the one for SO-200)
	// Actually, there's also a link in the H1 tag, so we should be careful.
	// But /issues/SO-200 should be there.
	// Let's count how many times SO-200 appears or something.
	
	// Better: check that no other issue keys are present.
	if strings.Contains(body, "SO-100") || strings.Contains(body, "SO-101") {
		t.Error("body contains unrelated issue keys")
	}
}

func TestRunDetail_Breadcrumb_NoIssue(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	// 1. Create Agent
	agent := &models.Agent{
		ID:   uuid.New().String(),
		Name: "Test Agent",
		Slug: "test-agent",
	}
	d.CreateAgent(agent)

	// 2. Create Run with NO issue key
	run := &models.Run{
		ID:      uuid.New().String(),
		AgentID: agent.ID,
		Status:  "completed",
	}
	d.CreateRun(run)

	req := httptest.NewRequest("GET", "/runs/"+run.ID, nil)
	req.SetPathValue("id", run.ID)
	w := httptest.NewRecorder()

	ui.RunDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()

	// Verify Agent link
	if !strings.Contains(body, "/agents/test-agent") {
		t.Error("body missing agent link")
	}

	// Verify NO Issue link is present in the main content area (breadcrumbs or header)
	// Sidebar has a link to "/issues", so we look for "/issues/" followed by something
	if strings.Contains(body, "text-ink3/50 hover:text-ink2 transition-colors\">SO-") {
		t.Error("body should NOT contain issue links in breadcrumbs")
	}

	// Verify Run text
	if !strings.Contains(body, "Run") {
		t.Error("body missing 'Run' text")
	}
}
