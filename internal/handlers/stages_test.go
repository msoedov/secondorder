package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/msoedov/secondorder/internal/models"
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
		"stages": stages,
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
