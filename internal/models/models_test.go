package models

import "testing"

func TestIssueStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"todo", StatusTodo, "todo"},
		{"in_progress", StatusInProgress, "in_progress"},
		{"in_review", StatusInReview, "in_review"},
		{"done", StatusDone, "done"},
		{"blocked", StatusBlocked, "blocked"},
		{"cancelled", StatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.want {
				t.Errorf("got %q, want %q", tt.constant, tt.want)
			}
		})
	}
}

func TestRunStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"running", RunStatusRunning, "running"},
		{"completed", RunStatusCompleted, "completed"},
		{"failed", RunStatusFailed, "failed"},
		{"cancelled", RunStatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.want {
				t.Errorf("got %q, want %q", tt.constant, tt.want)
			}
		})
	}
}

func TestWorkBlockStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"proposed", WBStatusProposed, "proposed"},
		{"active", WBStatusActive, "active"},
		{"ready", WBStatusReady, "ready"},
		{"shipped", WBStatusShipped, "shipped"},
		{"cancelled", WBStatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.want {
				t.Errorf("got %q, want %q", tt.constant, tt.want)
			}
		})
	}
}

func TestAgentStruct(t *testing.T) {
	a := Agent{
		ID:   "test-id",
		Name: "Test Agent",
		Slug: "test-agent",
	}
	if a.ID != "test-id" {
		t.Errorf("expected ID test-id, got %s", a.ID)
	}
	if a.Name != "Test Agent" {
		t.Errorf("expected Name Test Agent, got %s", a.Name)
	}

	// nil pointer fields
	if a.ReportsTo != nil {
		t.Error("expected ReportsTo to be nil")
	}
	if a.ReviewAgentID != nil {
		t.Error("expected ReviewAgentID to be nil")
	}
}

func TestIssueStruct(t *testing.T) {
	i := Issue{
		Key:    "TLO-1",
		Title:  "Test Issue",
		Status: StatusTodo,
	}
	if i.Key != "TLO-1" {
		t.Errorf("expected Key TLO-1, got %s", i.Key)
	}
	if i.AssigneeAgentID != nil {
		t.Error("expected AssigneeAgentID to be nil")
	}
	if i.StartedAt != nil {
		t.Error("expected StartedAt to be nil")
	}
	if i.CompletedAt != nil {
		t.Error("expected CompletedAt to be nil")
	}
}

func TestWorkBlockStruct(t *testing.T) {
	wb := WorkBlock{
		ID:     "wb-1",
		Title:  "Sprint 1",
		Goal:   "Ship MVP",
		Status: WBStatusProposed,
	}
	if wb.CompletedAt != nil {
		t.Error("expected CompletedAt to be nil")
	}
	if wb.Issues != nil {
		t.Error("expected Issues to be nil")
	}
	if wb.Stats != nil {
		t.Error("expected Stats to be nil")
	}
}
