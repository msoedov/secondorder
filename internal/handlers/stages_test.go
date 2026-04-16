package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/msoedov/mesa/internal/models"
)

func TestCreateComment_StagesParser(t *testing.T) {
	d := testDB(t)
	api := &API{db: d, sse: NewSSEHub()}
	defer api.sse.Close()

	agent := &models.Agent{ID: uuid.NewString(), Name: "Test Agent", Slug: "test-agent"}
	d.CreateAgent(agent)

	issue := &models.Issue{
		Key:             "SO-1",
		Title:           "Test Issue",
		Status:          models.StatusInProgress,
		AssigneeAgentID: &agent.ID,
	}
	d.CreateIssue(issue)

	// 1. Initialize stages
	body := map[string]string{
		"body": "Stages: [Setup], [Logic], [UI]",
	}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/issues/SO-1/comments", bytes.NewReader(jsonBody))
	req.SetPathValue("key", "SO-1")
	req = req.WithContext(withAgent(context.Background(), agent))

	w := httptest.NewRecorder()
	api.CreateComment(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	updatedIssue, _ := d.GetIssue("SO-1")
	if len(updatedIssue.Stages) != 3 {
		t.Errorf("expected 3 stages, got %d", len(updatedIssue.Stages))
	}
	if updatedIssue.CurrentStageID != 1 {
		t.Errorf("expected current_stage_id 1, got %d", updatedIssue.CurrentStageID)
	}
	if updatedIssue.Stages[0].Title != "Setup" {
		t.Errorf("expected stage 1 title 'Setup', got %q", updatedIssue.Stages[0].Title)
	}

	// 2. Mark stage 1 complete
	body = map[string]string{
		"body": "Stage 1: [Setup] - Complete",
	}
	jsonBody, _ = json.Marshal(body)
	req = httptest.NewRequest("POST", "/api/v1/issues/SO-1/comments", bytes.NewReader(jsonBody))
	req.SetPathValue("key", "SO-1")
	req = req.WithContext(withAgent(context.Background(), agent))

	w = httptest.NewRecorder()
	api.CreateComment(w, req)

	updatedIssue, _ = d.GetIssue("SO-1")
	if updatedIssue.Stages[0].Status != "done" {
		t.Errorf("expected stage 1 status 'done', got %q", updatedIssue.Stages[0].Status)
	}
	if updatedIssue.CurrentStageID != 2 {
		t.Errorf("expected current_stage_id 2, got %d", updatedIssue.CurrentStageID)
	}

	// Check for acknowledgment comment
	comments, _ := d.ListComments("SO-1")
	foundAck := false
	for _, c := range comments {
		if c.Author == "System" && bytes.Contains([]byte(c.Body), []byte("Stage 1 acknowledged")) {
			foundAck = true
			break
		}
	}
	if !foundAck {
		t.Error("expected system acknowledgment comment")
	}

	// 3. Mark stage 3 complete (skipping 2)
	body = map[string]string{
		"body": "Stage 3: [UI] - Complete",
	}
	jsonBody, _ = json.Marshal(body)
	req = httptest.NewRequest("POST", "/api/v1/issues/SO-1/comments", bytes.NewReader(jsonBody))
	req.SetPathValue("key", "SO-1")
	req = req.WithContext(withAgent(context.Background(), agent))

	w = httptest.NewRecorder()
	api.CreateComment(w, req)

	updatedIssue, _ = d.GetIssue("SO-1")
	if updatedIssue.Stages[1].Status != "done" {
		t.Errorf("expected stage 2 status 'done' (auto-marked), got %q", updatedIssue.Stages[1].Status)
	}
	if updatedIssue.Stages[2].Status != "done" {
		t.Errorf("expected stage 3 status 'done', got %q", updatedIssue.Stages[2].Status)
	}
	if updatedIssue.CurrentStageID != 3 {
		t.Errorf("expected current_stage_id 3, got %d", updatedIssue.CurrentStageID)
	}
}

func TestUpdateIssue_Stages(t *testing.T) {
	d := testDB(t)
	api := &API{db: d, sse: NewSSEHub()}
	defer api.sse.Close()

	agent := &models.Agent{ID: uuid.NewString(), Name: "Test Agent", Slug: "test-agent"}
	d.CreateAgent(agent)

	issue := &models.Issue{
		Key:             "SO-1",
		Title:           "Test Issue",
		Status:          models.StatusInProgress,
		AssigneeAgentID: &agent.ID,
	}
	d.CreateIssue(issue)

	// Update stages via API
	stages := []models.IssueStage{
		{ID: 1, Title: "Stage 1", Status: "todo"},
	}
	body := map[string]any{
		"stages":           stages,
		"current_stage_id": 1,
	}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("PATCH", "/api/v1/issues/SO-1", bytes.NewReader(jsonBody))
	req.SetPathValue("key", "SO-1")
	req = req.WithContext(withAgent(context.Background(), agent))

	w := httptest.NewRecorder()
	api.UpdateIssue(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	updatedIssue, _ := d.GetIssue("SO-1")
	if len(updatedIssue.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(updatedIssue.Stages))
	}
	if updatedIssue.Stages[0].Title != "Stage 1" {
		t.Errorf("expected stage title 'Stage 1', got %q", updatedIssue.Stages[0].Title)
	}
}

func TestUpdateIssue_StagesValidation(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, nil, &stubTelegram{}, nil)

	agent, agentKey := createAgentWithKey(t, d, "Stage Owner", "stage-owner", "qa")

	issue := &models.Issue{
		Key:             "SO-2",
		Title:           "Test Issue",
		Status:          models.StatusInProgress,
		AssigneeAgentID: &agent.ID,
		Stages: []models.IssueStage{
			{ID: 1, Title: "Setup", Status: "done"},
			{ID: 2, Title: "Logic", Status: "todo"},
		},
		CurrentStageID: 2,
	}
	d.CreateIssue(issue)

	req := httptest.NewRequest("PATCH", "/api/v1/issues/SO-2", strings.NewReader(`{
		"stages":[
			{"id":1,"title":"Setup","status":"todo"},
			{"id":2,"title":"Logic","status":"done"}
		],
		"current_stage_id":1
	}`))
	req.Header.Set("Authorization", "Bearer "+agentKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("key", "SO-2")

	w := httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}

	updatedIssue, _ := d.GetIssue("SO-2")
	if updatedIssue.CurrentStageID != 2 {
		t.Fatalf("current_stage_id = %d, want 2", updatedIssue.CurrentStageID)
	}
	if updatedIssue.Stages[0].Status != "done" || updatedIssue.Stages[1].Status != "todo" {
		t.Fatalf("issue stages were mutated on invalid update: %+v", updatedIssue.Stages)
	}
}

func TestIssueDetail_ToggleStageNormalizesProgress(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	ui := NewUI(d, hub, nil, nil, nil)

	issue := &models.Issue{
		Key:    "SO-3",
		Title:  "Test Issue",
		Status: models.StatusInProgress,
		Stages: []models.IssueStage{
			{ID: 1, Title: "Setup", Status: "done"},
			{ID: 2, Title: "Logic", Status: "done"},
			{ID: 3, Title: "UI", Status: "done"},
			{ID: 4, Title: "QA", Status: "todo"},
		},
		CurrentStageID: 4,
	}
	d.CreateIssue(issue)

	form := "action=toggle_stage&stage_id=2&status=todo"
	req := httptest.NewRequest("POST", "/issues/SO-3", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	ui.updateIssueUI(w, req, "SO-3")

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect status 303, got %d", w.Code)
	}

	updatedIssue, _ := d.GetIssue("SO-3")
	if updatedIssue.CurrentStageID != 2 {
		t.Fatalf("current_stage_id = %d, want 2", updatedIssue.CurrentStageID)
	}

	got := []string{
		updatedIssue.Stages[0].Status,
		updatedIssue.Stages[1].Status,
		updatedIssue.Stages[2].Status,
		updatedIssue.Stages[3].Status,
	}
	want := []string{"done", "todo", "todo", "todo"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stage %d status = %s, want %s; stages=%+v", i+1, got[i], want[i], updatedIssue.Stages)
		}
	}
}

func TestIssueDetail_ToggleStageHTMXReturnsNoContent(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	ui := NewUI(d, hub, nil, nil, nil)

	issue := &models.Issue{
		Key:    "SO-4",
		Title:  "HTMX Toggle",
		Status: models.StatusInProgress,
		Stages: []models.IssueStage{
			{ID: 1, Title: "Setup", Status: "done"},
			{ID: 2, Title: "Logic", Status: "todo"},
		},
		CurrentStageID: 2,
	}
	d.CreateIssue(issue)

	form := "action=toggle_stage&stage_id=2&status=done"
	req := httptest.NewRequest("POST", "/issues/SO-4", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	ui.updateIssueUI(w, req, "SO-4")

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	updatedIssue, _ := d.GetIssue("SO-4")
	if updatedIssue.Stages[1].Status != "done" {
		t.Fatalf("stage 2 status = %s, want done", updatedIssue.Stages[1].Status)
	}
}
