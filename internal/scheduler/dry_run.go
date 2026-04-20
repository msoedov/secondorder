package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/msoedov/mesa/internal/models"
)

// execDryRun records the prompt that *would* be sent to a real runner, then
// returns immediately without invoking any CLI. Use it to iterate on
// archetypes and prompt construction without burning tokens.
func (s *Scheduler) execDryRun(ctx context.Context, agent *models.Agent, apiKey, runID, issueKey, prompt string) (string, error) {
	slog.Info("scheduler: dry_run runner invoked",
		"run_id", runID,
		"agent", agent.Name,
		"issue_key", issueKey,
		"prompt_chars", len(prompt),
	)

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}

	envelope, _ := json.Marshal(map[string]any{
		"type":      "dry_run",
		"run_id":    runID,
		"agent":    agent.Name,
		"model":     agent.Model,
		"issue_key": issueKey,
		"prompt":    prompt,
		"note":      "dry_run runner — prompt recorded, no CLI invoked",
	})

	output := fmt.Sprintf("%s\n%s\n",
		`{"type":"result","result":{"input_tokens":0,"output_tokens":0,"total_cost_usd":0}}`,
		string(envelope),
	)

	s.db.UpdateRunStdout(runID, output)
	return output, nil
}
