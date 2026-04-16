package validator

import (
	"fmt"
	"strings"

	"github.com/msoedov/mesa/internal/models"
)

// ValidateStages enforces the linear checkpoint model used by issue stages.
func ValidateStages(stages []models.IssueStage, currentStageID int) error {
	if len(stages) == 0 {
		if currentStageID != 0 {
			return fmt.Errorf("current_stage_id must be 0 when stages are empty")
		}
		return nil
	}

	if currentStageID < 1 || currentStageID > len(stages) {
		return fmt.Errorf("current_stage_id must be between 1 and %d", len(stages))
	}

	expectedCurrent := len(stages)
	seenTodo := false

	for i, stage := range stages {
		expectedID := i + 1
		if stage.ID != expectedID {
			return fmt.Errorf("stage ids must be sequential starting at 1")
		}
		if strings.TrimSpace(stage.Title) == "" {
			return fmt.Errorf("stage %d title is required", stage.ID)
		}

		switch stage.Status {
		case "done":
			if seenTodo {
				return fmt.Errorf("stage %d cannot be done after an incomplete stage", stage.ID)
			}
		case "todo":
			if !seenTodo {
				expectedCurrent = stage.ID
				seenTodo = true
			}
		default:
			return fmt.Errorf("stage %d has invalid status %q", stage.ID, stage.Status)
		}
	}

	if currentStageID != expectedCurrent {
		return fmt.Errorf("current_stage_id must point to the first incomplete stage")
	}

	return nil
}

// ApplyStageToggle keeps manual UI toggles consistent with linear stage progression.
func ApplyStageToggle(stages []models.IssueStage, stageID int, status string) ([]models.IssueStage, int, error) {
	if len(stages) == 0 {
		return nil, 0, fmt.Errorf("cannot toggle stages on an issue without stages")
	}
	if stageID < 1 || stageID > len(stages) {
		return nil, 0, fmt.Errorf("stage_id must be between 1 and %d", len(stages))
	}
	if status != "todo" && status != "done" {
		return nil, 0, fmt.Errorf("status must be \"todo\" or \"done\"")
	}

	next := append([]models.IssueStage(nil), stages...)

	switch status {
	case "done":
		for i := 0; i < stageID; i++ {
			next[i].Status = "done"
		}
	case "todo":
		for i := stageID - 1; i < len(next); i++ {
			next[i].Status = "todo"
		}
	}

	currentStageID := len(next)
	seenTodo := false
	for i := range next {
		if next[i].Status == "todo" && !seenTodo {
			currentStageID = i + 1
			seenTodo = true
		}
		if next[i].Status == "done" && seenTodo {
			next[i].Status = "todo"
		}
	}

	if err := ValidateStages(next, currentStageID); err != nil {
		return nil, 0, err
	}

	return next, currentStageID, nil
}
