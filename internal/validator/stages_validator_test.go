package validator

import (
	"testing"

	"github.com/msoedov/mesa/internal/models"
)

func TestValidateStages(t *testing.T) {
	tests := []struct {
		name           string
		stages         []models.IssueStage
		currentStageID int
		wantErr        bool
	}{
		{
			name:           "empty stages require zero current stage",
			stages:         nil,
			currentStageID: 0,
		},
		{
			name: "valid in progress sequence",
			stages: []models.IssueStage{
				{ID: 1, Title: "Setup", Status: "done"},
				{ID: 2, Title: "Logic", Status: "todo"},
				{ID: 3, Title: "UI", Status: "todo"},
			},
			currentStageID: 2,
		},
		{
			name: "valid completed sequence",
			stages: []models.IssueStage{
				{ID: 1, Title: "Setup", Status: "done"},
				{ID: 2, Title: "Logic", Status: "done"},
			},
			currentStageID: 2,
		},
		{
			name: "rejects non sequential ids",
			stages: []models.IssueStage{
				{ID: 2, Title: "Setup", Status: "todo"},
			},
			currentStageID: 1,
			wantErr:        true,
		},
		{
			name: "rejects blank titles",
			stages: []models.IssueStage{
				{ID: 1, Title: " ", Status: "todo"},
			},
			currentStageID: 1,
			wantErr:        true,
		},
		{
			name: "rejects invalid status",
			stages: []models.IssueStage{
				{ID: 1, Title: "Setup", Status: "active"},
			},
			currentStageID: 1,
			wantErr:        true,
		},
		{
			name: "rejects done after todo",
			stages: []models.IssueStage{
				{ID: 1, Title: "Setup", Status: "todo"},
				{ID: 2, Title: "Logic", Status: "done"},
			},
			currentStageID: 1,
			wantErr:        true,
		},
		{
			name: "rejects inconsistent current stage",
			stages: []models.IssueStage{
				{ID: 1, Title: "Setup", Status: "done"},
				{ID: 2, Title: "Logic", Status: "todo"},
			},
			currentStageID: 1,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStages(tt.stages, tt.currentStageID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateStages() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyStageToggle(t *testing.T) {
	t.Run("reopening a stage resets downstream progress", func(t *testing.T) {
		stages := []models.IssueStage{
			{ID: 1, Title: "Setup", Status: "done"},
			{ID: 2, Title: "Logic", Status: "done"},
			{ID: 3, Title: "UI", Status: "done"},
			{ID: 4, Title: "QA", Status: "todo"},
		}

		gotStages, gotCurrent, err := ApplyStageToggle(stages, 2, "todo")
		if err != nil {
			t.Fatalf("ApplyStageToggle() error = %v", err)
		}

		if gotCurrent != 2 {
			t.Fatalf("current_stage_id = %d, want 2", gotCurrent)
		}
		if gotStages[0].Status != "done" || gotStages[1].Status != "todo" || gotStages[2].Status != "todo" || gotStages[3].Status != "todo" {
			t.Fatalf("unexpected stages after reopen: %+v", gotStages)
		}
	})

	t.Run("completing a stage marks prior stages done", func(t *testing.T) {
		stages := []models.IssueStage{
			{ID: 1, Title: "Setup", Status: "todo"},
			{ID: 2, Title: "Logic", Status: "todo"},
			{ID: 3, Title: "UI", Status: "todo"},
		}

		gotStages, gotCurrent, err := ApplyStageToggle(stages, 2, "done")
		if err != nil {
			t.Fatalf("ApplyStageToggle() error = %v", err)
		}

		if gotCurrent != 3 {
			t.Fatalf("current_stage_id = %d, want 3", gotCurrent)
		}
		if gotStages[0].Status != "done" || gotStages[1].Status != "done" || gotStages[2].Status != "todo" {
			t.Fatalf("unexpected stages after completion: %+v", gotStages)
		}
	})
}
