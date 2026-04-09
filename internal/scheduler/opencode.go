package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"log/slog"

	"github.com/msoedov/secondorder/internal/archetypes"
	"github.com/msoedov/secondorder/internal/models"
)

func (s *Scheduler) execOpenCode(ctx context.Context, agent *models.Agent, apiKey, runID, issueKey, prompt string) (string, error) {
	fullPrompt := prompt
	if data, err := archetypes.Read(agent.ArchetypeSlug); err == nil {
		fullPrompt = fmt.Sprintf("SYSTEM PROMPT:\n%s\n\nUSER PROMPT:\n%s", string(data), prompt)
	}

	args := []string{
		"--pure",
		"run", fullPrompt,
		"--format", "json",
		"--dangerously-skip-permissions",
	}
	if agent.Model != "" {
		args = append(args, "-m", agent.Model)
	}

	cmd := exec.CommandContext(ctx, "opencode", args...)
	slog.Debug("scheduler: exec", "run_id", runID, "cmd", cmd.String())
	cmd.Dir = agent.WorkingDir

	env := os.Environ()
	env = append(env,
		fmt.Sprintf("SECONDORDER_AGENT_ID=%s", agent.ID),
		fmt.Sprintf("SECONDORDER_AGENT_NAME=%s", agent.Name),
		fmt.Sprintf("SECONDORDER_RUN_ID=%s", runID),
		fmt.Sprintf("SECONDORDER_API_URL=http://localhost:%d", s.port),
		fmt.Sprintf("SECONDORDER_ISSUE_KEY=%s", issueKey),
		fmt.Sprintf("SECONDORDER_ARTIFACT_DOCS=%s", filepath.Join(agent.WorkingDir, "artifact-docs")),
		fmt.Sprintf("SECONDORDER_API_KEY=%s", apiKey),
	)

	if agent.ApiKeyEnv != "" {
		if val := os.Getenv(agent.ApiKeyEnv); val != "" {
			env = append(env, fmt.Sprintf("GITHUB_TOKEN=%s", val))
		}
	}

	cmd.Env = env

	lw := &liveWriter{
		db:            s.db,
		runID:         runID,
		interval:      2 * time.Second,
		agentName:     agent.Name,
		archetypeSlug: agent.ArchetypeSlug,
	}
	cmd.Stdout = lw
	cmd.Stderr = lw

	err := cmd.Run()
	lw.Flush()

	stdout := lw.String()

	if err == nil && stdout == "" {
		return stdout, fmt.Errorf("opencode produced no output (possible silent failure)")
	}

	return stdout, err
}
