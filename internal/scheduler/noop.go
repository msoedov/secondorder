package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/msoedov/mesa/internal/models"
)

// execNoop is a no-op runner for end-to-end tests. It returns immediately
// with a fixed stream-json result line so the scheduler completes normally.
func (s *Scheduler) execNoop(ctx context.Context, agent *models.Agent, apiKey, runID, issueKey, prompt string) (string, error) {
	slog.Info("scheduler: noop runner invoked", "run_id", runID, "agent", agent.Name, "issue_key", issueKey)

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}

	output := fmt.Sprintf(`{"type":"result","result":{"input_tokens":0,"output_tokens":0,"total_cost_usd":0}}
{"type":"noop","run_id":%q,"issue_key":%q,"note":"noop runner — no work performed"}
`, runID, issueKey)

	s.db.UpdateRunStdout(runID, output)
	return output, nil
}
