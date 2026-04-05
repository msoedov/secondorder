package scheduler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/msoedov/secondorder/internal/archetypes"
	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
	log "github.com/sirupsen/logrus"
)

type Scheduler struct {
	db           *db.DB
	port         int
	mu           sync.Mutex
	running      map[string]context.CancelFunc // runID -> cancel
	wg           sync.WaitGroup
	stopped      bool
	onRunStart    func(run *models.Run)
	onRunComplete func(run *models.Run)
	onComment     func(issueKey, author, body string)
	onActivity    func(action, entityType, entityID string, agentID *string, details string)
}

func New(database *db.DB, port int) *Scheduler {
	return &Scheduler{
		db:      database,
		port:    port,
		running: make(map[string]context.CancelFunc),
	}
}

func (s *Scheduler) SetOnRunStart(fn func(run *models.Run)) {
	s.onRunStart = fn
}

func (s *Scheduler) SetOnRunComplete(fn func(run *models.Run)) {
	s.onRunComplete = fn
}

func (s *Scheduler) SetOnComment(fn func(issueKey, author, body string)) {
	s.onComment = fn
}

func (s *Scheduler) SetOnActivity(fn func(action, entityType, entityID string, agentID *string, details string)) {
	s.onActivity = fn
}

func (s *Scheduler) logActivity(action, entityType, entityID string, agentID *string, details string) {
	if s.onActivity != nil {
		s.onActivity(action, entityType, entityID, agentID, details)
	} else {
		s.db.LogActivity(action, entityType, entityID, agentID, details)
	}
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	s.stopped = true
	for _, cancel := range s.running {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	log.Info("scheduler: all agents stopped")
}

// WakeAgent spawns an agent for a specific issue (event-driven)
func (s *Scheduler) WakeAgent(agent *models.Agent, issue *models.Issue) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	prompt := s.buildTaskPrompt(agent, issue)
	s.spawnAgent(agent, issue.Key, "task", prompt)
}

// WakeAgentHeartbeat spawns a heartbeat run for the agent
func (s *Scheduler) WakeAgentHeartbeat(agent *models.Agent) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	prompt := s.buildHeartbeatPrompt(agent)
	s.spawnAgent(agent, "", "heartbeat", prompt)
}

// WakeReviewer finds and wakes the appropriate reviewer for an agent's completed issue
func (s *Scheduler) WakeReviewer(agentID, issueKey string) {
	reviewer, err := s.db.GetReviewer(agentID)
	if err != nil {
		log.WithError(err).Error("scheduler: failed to find reviewer")
		return
	}
	if reviewer.ID == agentID {
		log.WithField("agent", agentID).Debug("scheduler: skipping self-review")
		return
	}
	issue, err := s.db.GetIssue(issueKey)
	if err != nil {
		log.WithError(err).Error("scheduler: failed to get issue for reviewer")
		return
	}
	s.WakeAgent(reviewer, issue)
}

func (s *Scheduler) spawnAgent(agent *models.Agent, issueKey, mode, prompt string) string {
	runID := uuid.New().String()

	run := &models.Run{
		ID:        runID,
		AgentID:   agent.ID,
		Mode:      mode,
		Status:    models.RunStatusRunning,
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	if issueKey != "" {
		run.IssueKey = &issueKey
	}

	if err := s.db.CreateRun(run); err != nil {
		log.WithError(err).Error("scheduler: failed to create run")
		return ""
	}

	if s.onRunStart != nil {
		s.onRunStart(run)
	}

	// Provision API key
	rawKey, err := s.provisionAPIKey(agent.ID)
	if err != nil {
		log.WithError(err).Error("scheduler: failed to provision API key")
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(agent.TimeoutSec)*time.Second)

	s.mu.Lock()
	s.running[runID] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.running, runID)
			s.mu.Unlock()
			cancel()
		}()

		logEntry := log.WithFields(log.Fields{
			"agent":     agent.Name,
			"archetype": agent.ArchetypeSlug,
			"run_id":    runID,
			"model":     agent.Model,
			"mode":      mode,
			"issue_key": issueKey,
		})
		logEntry.Info("scheduler: spawning agent")

		startTime := time.Now()
		var stdout string
		var err error
		switch agent.Runner {
		case "codex":
			stdout, err = s.execCodex(ctx, agent, rawKey, runID, issueKey, prompt)
		case "gemini":
			stdout, err = s.execGemini(ctx, agent, rawKey, runID, issueKey, prompt)
		case "antigravity":
			stdout, err = s.execAntigravity(ctx, agent, rawKey, runID, issueKey, prompt)
		case "claude_code":
			stdout, err = s.execClaudeCode(ctx, agent, rawKey, runID, issueKey, prompt)
		default:
			if agent.Runner == "" {
				stdout, err = s.execClaudeCode(ctx, agent, rawKey, runID, issueKey, prompt)
			} else {
				err = fmt.Errorf("unsupported runner: %s", agent.Runner)
			}
		}
		elapsed := time.Since(startTime)

		status := models.RunStatusCompleted
		if err != nil {
			status = models.RunStatusFailed
			if ctx.Err() == context.DeadlineExceeded {
				status = models.RunStatusFailed
				logEntry.Warn("scheduler: agent timed out")
			} else if ctx.Err() == context.Canceled {
				status = models.RunStatusCancelled
			} else {
				// Include the last few lines of stdout in the log for easier debugging
				lastOutput := stdout
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				if len(lines) > 10 {
					lastOutput = "... (truncated)\n" + strings.Join(lines[len(lines)-10:], "\n")
				}
				logEntry.WithError(err).WithField("output", lastOutput).Error("scheduler: agent failed")
			}
		}

		// Parse token usage from stream-json output
		tokens := parseTokenUsage(stdout)
		if agent.Runner != "claude_code" && agent.Runner != "gemini" && agent.Runner != "codex" && agent.Runner != "" {
			tokens = tokenUsage{}
		}

		// Capture git diff
		diff := captureGitDiff(agent.WorkingDir)

		completedAt := time.Now()
		completedRun := models.Run{
			InputTokens:       tokens.InputTokens,
			OutputTokens:      tokens.OutputTokens,
			CacheReadTokens:   tokens.CacheReadTokens,
			CacheCreateTokens: tokens.CacheCreateTokens,
			TotalCostUSD:      tokens.TotalCostUSD,
		}
		if err := s.db.CompleteRun(runID, status, stdout, diff, completedRun); err != nil {
			logEntry.WithError(err).Error("scheduler: failed to complete run")
		}

		// Record cost event
		if tokens.TotalCostUSD > 0 {
			s.db.CreateCostEvent(&models.CostEvent{
				ID:           uuid.New().String(),
				RunID:        runID,
				AgentID:      agent.ID,
				InputTokens:  tokens.InputTokens,
				OutputTokens: tokens.OutputTokens,
				TotalCostUSD: tokens.TotalCostUSD,
				CreatedAt:    time.Now(),
			})
		}

		logEntry.WithFields(log.Fields{
			"status":        status,
			"elapsed":       elapsed.Round(time.Second),
			"cost_usd":      fmt.Sprintf("%.4f", tokens.TotalCostUSD),
			"input_tokens":  tokens.InputTokens,
			"output_tokens": tokens.OutputTokens,
		}).Info("scheduler: agent completed")

		if s.onRunComplete != nil {
			finalRun := &models.Run{
				ID:          runID,
				AgentID:     agent.ID,
				IssueKey:    run.IssueKey,
				Mode:        mode,
				Status:      status,
				CompletedAt: &completedAt,
			}
			s.onRunComplete(finalRun)
		}
	}()

	return runID
}

func (s *Scheduler) execClaudeCode(ctx context.Context, agent *models.Agent, apiKey, runID, issueKey, prompt string) (string, error) {
	args := []string{
		"--print",
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
		"--max-turns", fmt.Sprintf("%d", agent.MaxTurns),
		"--model", agent.Model,
	}

	if agent.ArchetypeSlug != "" {
		if tmpFile, cleanup, err := archetypes.WriteToTemp(agent.ArchetypeSlug); err == nil {
			defer cleanup()
			args = append(args, "--append-system-prompt-file", tmpFile)
		}
	}

	artifactDir := filepath.Join(agent.WorkingDir, "artifact-docs")
	if info, err := os.Stat(artifactDir); err == nil && info.IsDir() {
		args = append(args, "--add-dir", artifactDir)
	}

	if agent.ChromeEnabled {
		args = append(args, "--chrome")
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = agent.WorkingDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SECONDORDER_AGENT_ID=%s", agent.ID),
		fmt.Sprintf("SECONDORDER_AGENT_NAME=%s", agent.Name),
		fmt.Sprintf("SECONDORDER_RUN_ID=%s", runID),
		fmt.Sprintf("SECONDORDER_API_URL=http://localhost:%d", s.port),
		fmt.Sprintf("SECONDORDER_ISSUE_KEY=%s", issueKey),
		fmt.Sprintf("SECONDORDER_ARTIFACT_DOCS=%s", filepath.Join(agent.WorkingDir, "artifact-docs")),
		fmt.Sprintf("SECONDORDER_API_KEY=%s", apiKey),
	)

	// Use liveWriter to stream stdout to DB
	lw := &liveWriter{
		db:       s.db,
		runID:    runID,
		interval: 2 * time.Second,
	}
	cmd.Stdout = lw
	cmd.Stderr = lw

	err := cmd.Run()
	lw.Flush()
	return lw.String(), err
}

func (s *Scheduler) execCodex(ctx context.Context, agent *models.Agent, apiKey, runID, issueKey, prompt string) (string, error) {
	args := []string{
		"exec",
		"--full-auto",
		"--json",
	}
	if agent.Model != "" && agent.Model != "default" {
		args = append(args, "--model", agent.Model)
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, "codex", args...)
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

	// Handle API key env override
	if agent.ApiKeyEnv != "" {
		if val := os.Getenv(agent.ApiKeyEnv); val != "" {
			env = append(env, fmt.Sprintf("OPENAI_API_KEY=%s", val))
		}
	}

	// Codex system prompt via env
	if data, err := archetypes.Read(agent.ArchetypeSlug); err == nil {
		env = append(env, fmt.Sprintf("CODEX_SYSTEM_PROMPT=%s", string(data)))
	}

	cmd.Env = env

	lw := &liveWriter{
		db:       s.db,
		runID:    runID,
		interval: 2 * time.Second,
	}
	cmd.Stdout = lw
	cmd.Stderr = lw

	err := cmd.Run()
	lw.Flush()
	return lw.String(), err
}

func (s *Scheduler) execGemini(ctx context.Context, agent *models.Agent, apiKey, runID, issueKey, prompt string) (string, error) {
	// Prepend archetype to prompt since gemini CLI doesn't have a system prompt file flag
	fullPrompt := prompt
	if data, err := archetypes.Read(agent.ArchetypeSlug); err == nil {
		fullPrompt = fmt.Sprintf("SYSTEM PROMPT:\n%s\n\nUSER PROMPT:\n%s", string(data), prompt)
	}

	args := []string{
		"-p", fullPrompt,
		"--yolo",
		"--output-format", "stream-json",
	}
	if agent.Model != "" {
		args = append(args, "-m", agent.Model)
	}

	cmd := exec.CommandContext(ctx, "gemini", args...)
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

	// Handle API key env override
	if agent.ApiKeyEnv != "" {
		if val := os.Getenv(agent.ApiKeyEnv); val != "" {
			env = append(env, fmt.Sprintf("GEMINI_API_KEY=%s", val))
		}
	}

	cmd.Env = env

	lw := &liveWriter{
		db:       s.db,
		runID:    runID,
		interval: 2 * time.Second,
	}
	cmd.Stdout = lw
	cmd.Stderr = lw

	err := cmd.Run()
	lw.Flush()
	return lw.String(), err
}

func (s *Scheduler) execAntigravity(ctx context.Context, agent *models.Agent, apiKey, runID, issueKey, prompt string) (string, error) {
	args := []string{
		"run",
		"--non-interactive",
		"--prompt", prompt,
		"--max-turns", fmt.Sprintf("%d", agent.MaxTurns),
	}
	if agent.Model != "" && agent.Model != "default" {
		args = append(args, "--model", agent.Model)
	}

	if agent.ArchetypeSlug != "" {
		if tmpFile, cleanup, err := archetypes.WriteToTemp(agent.ArchetypeSlug); err == nil {
			defer cleanup()
			args = append(args, "--system-prompt-file", tmpFile)
		}
	}

	cmd := exec.CommandContext(ctx, "antigravity", args...)
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

	// Handle API key env override
	if agent.ApiKeyEnv != "" {
		if val := os.Getenv(agent.ApiKeyEnv); val != "" {
			env = append(env, fmt.Sprintf("ANTIGRAVITY_API_KEY=%s", val))
		}
	}

	cmd.Env = env

	lw := &liveWriter{
		db:       s.db,
		runID:    runID,
		interval: 2 * time.Second,
	}
	cmd.Stdout = lw
	cmd.Stderr = lw

	err := cmd.Run()
	lw.Flush()
	return lw.String(), err
}

func (s *Scheduler) provisionAPIKey(agentID string) (string, error) {
	// Revoke existing keys
	s.db.RevokeAPIKeys(agentID)

	// Generate new key
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	rawKey := "so_" + hex.EncodeToString(raw)

	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])
	prefix := rawKey[:12]

	if err := s.db.CreateAPIKey(agentID, keyHash, prefix); err != nil {
		return "", err
	}

	return rawKey, nil
}

func (s *Scheduler) buildTaskPrompt(agent *models.Agent, issue *models.Issue) string {
	comments, _ := s.db.ListComments(issue.Key)

	var commentBlock string
	start := 0
	if len(comments) > 5 {
		start = len(comments) - 5
	}
	for _, c := range comments[start:] {
		commentBlock += fmt.Sprintf("- %s (%s): %s\n", c.Author, c.CreatedAt.Format("Jan 2 15:04"), c.Body)
	}

	apiRef := workerAPIRef
	rules := workerRules
	wbContext := ""
	backlog := ""
	if agent.ArchetypeSlug == "ceo" {
		apiRef = s.buildCEOAPIRef()
		rules = ceoRules
		wbContext = s.buildWorkBlockContext()
		backlog = s.readBacklogFile(agent.WorkingDir)
	}

	return fmt.Sprintf(`ISSUE: %s
TITLE: %s
DESCRIPTION:
%s
%s%s
RECENT COMMENTS:
%s
%s

%s

BASE_URL: http://localhost:%d
`, issue.Key, issue.Title, issue.Description, wbContext, backlog, commentBlock, rules, apiRef, s.port)
}

func (s *Scheduler) buildHeartbeatPrompt(agent *models.Agent) string {
	inbox, _ := s.db.GetAgentInbox(agent.ID)

	var issueBlock string
	for _, i := range inbox {
		issueBlock += fmt.Sprintf("- [%s] %s (status: %s, priority: %d)\n", i.Key, i.Title, i.Status, i.Priority)
	}

	apiRef := workerAPIRef
	rules := workerRules
	if agent.ArchetypeSlug == "ceo" {
		apiRef = s.buildCEOAPIRef()
		rules = ceoRules

		approvals, _ := s.db.ListPendingApprovals()
		if len(approvals) > 0 {
			issueBlock += "\nPENDING REVIEWS:\n"
			for _, a := range approvals {
				issueBlock += fmt.Sprintf("- Approval %s for issue %s (requested by: %s)\n", a.ID, a.IssueKey, a.RequestedBy)
			}
		}

		issueBlock += s.buildWorkBlockContext()
		issueBlock += s.readBacklogFile(agent.WorkingDir)
	}

	return fmt.Sprintf(`HEARTBEAT CHECK - Review your inbox and take action on any pending items.

YOUR INBOX:
%s

%s

%s

BASE_URL: http://localhost:%d
`, issueBlock, rules, apiRef, s.port)
}

func (s *Scheduler) buildWorkBlockContext() string {
	wb, err := s.db.GetActiveWorkBlock()
	if err != nil {
		return ""
	}
	issues, _ := s.db.ListWorkBlockIssues(wb.ID)
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("\nACTIVE WORK BLOCK: %s (id: %s)\nGoal: %s\n", wb.Title, wb.ID, wb.Goal))
	if len(issues) > 0 {
		buf.WriteString("Block Issues:\n")
		for _, i := range issues {
			buf.WriteString(fmt.Sprintf("  - [%s] %s (status: %s)\n", i.Key, i.Title, i.Status))
		}
	}
	return buf.String()
}

func (s *Scheduler) readBacklogFile(workingDir string) string {
	backlogPath := filepath.Join(workingDir, "artifact-docs", "backlog.md")
	data, err := os.ReadFile(backlogPath)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return ""
	}
	return fmt.Sprintf(`
BACKLOG FILE (artifact-docs/backlog.md):
%s

INSTRUCTIONS FOR BACKLOG:
- Create issues from each item in the backlog file using the API.
- Assign each issue to the appropriate agent based on the item's scope.
- After creating all issues, delete the backlog file to avoid duplicates.
`, strings.TrimSpace(string(data)))
}

func (s *Scheduler) buildCEOAPIRef() string {
	agents, _ := s.db.ListAgents()
	var roster string
	for _, a := range agents {
		if a.ArchetypeSlug == "ceo" {
			continue
		}
		roster += fmt.Sprintf("  %s (slug: %s, role: %s)\n", a.Name, a.Slug, a.ArchetypeSlug)
	}
	return fmt.Sprintf(ceoAPIRef, roster)
}

// CancelAudit cancels a running audit by its audit run ID
func (s *Scheduler) CancelAudit(auditRunID string) error {
	ar, err := s.db.GetAuditRun(auditRunID)
	if err != nil {
		return fmt.Errorf("audit run not found: %w", err)
	}
	if ar.Status != "running" {
		return fmt.Errorf("audit run %s is not running (status: %s)", auditRunID, ar.Status)
	}
	if ar.RunID != nil {
		s.mu.Lock()
		if cancel, ok := s.running[*ar.RunID]; ok {
			cancel()
		}
		s.mu.Unlock()
	}
	now := time.Now()
	_, err = s.db.Exec(`UPDATE audit_runs SET status='cancelled', completed_at=? WHERE id=?`, now, auditRunID)
	return err
}

// RunAudit spawns an auditor agent to review recent work
func (s *Scheduler) RunAudit(maxBlocks, maxIssues int, focus, runner, model string) (string, error) {
	agents, _ := s.db.ListAgents()
	var auditor *models.Agent
	for i := range agents {
		if agents[i].ArchetypeSlug == "auditor" {
			auditor = &agents[i]
			break
		}
	}
	if auditor == nil {
		return "", fmt.Errorf("no auditor agent found -- create one with archetype 'auditor'")
	}

	// Configuration file support (.secondorder.json or .secondorder.yml)
	if runner == "" || model == "" {
		var data []byte
		var err error
		if data, err = os.ReadFile(".secondorder.json"); err != nil {
			data, err = os.ReadFile(".secondorder.yml")
		}

		if err == nil {
			var config struct {
				Audit struct {
					Runner string `json:"runner"`
					Model  string `json:"model"`
				} `json:"audit"`
			}
			// Try as JSON (common for both .json and often used in .yml in this project)
			if err := json.Unmarshal(data, &config); err == nil {
				if runner == "" && config.Audit.Runner != "" {
					runner = config.Audit.Runner
				}
				if model == "" && config.Audit.Model != "" {
					model = config.Audit.Model
				}
			}
		}
	}

	ar := &models.AuditRun{
		Runner: runner,
		Model:  model,
	}
	if ar.Runner == "" {
		ar.Runner = auditor.Runner
	}
	// If runner was specified (via UI or config) but model was not,
	// we must ensure the model is valid for THAT runner.
	// If we just use auditor.Model, it might be incompatible.
	if ar.Model == "" {
		if ar.Runner == auditor.Runner {
			ar.Model = auditor.Model
		} else {
			// Pick first valid model for the runner
			if m, ok := models.RunnerModels[ar.Runner]; ok && len(m) > 0 {
				ar.Model = m[0]
			}
		}
	}

	// Final validation
	if !models.IsValidModelForRunner(ar.Runner, ar.Model) {
		// Fallback to auditor defaults if invalid
		ar.Runner = auditor.Runner
		ar.Model = auditor.Model
	}

	if err := s.db.CreateAuditRun(ar); err != nil {
		return "", err
	}

	prompt := s.buildAuditPrompt(maxBlocks, maxIssues, ar.ID, focus)

	// Restrict auditor to artifact-docs only (no codebase access)
	auditAgent := *auditor
	if ar.Runner != "" {
		auditAgent.Runner = ar.Runner
	}
	if ar.Model != "" {
		auditAgent.Model = ar.Model
	}

	artifactDir := filepath.Join(auditor.WorkingDir, "artifact-docs")
	if info, err := os.Stat(artifactDir); err == nil && info.IsDir() {
		auditAgent.WorkingDir = artifactDir
	}
	runID := s.spawnAgent(&auditAgent, "", "audit", prompt)

	if runID != "" {
		ar.RunID = &runID
		s.db.Exec(`UPDATE audit_runs SET run_id=? WHERE id=?`, runID, ar.ID)
	}

	return ar.ID, nil
}

func (s *Scheduler) buildAuditPrompt(maxBlocks, maxIssues int, auditRunID, focus string) string {
	var buf strings.Builder

	buf.WriteString("AUDIT RUN - Review recent work and improve agent performance.\n\n")
	buf.WriteString(fmt.Sprintf("AUDIT_RUN_ID: %s\n\n", auditRunID))

	// Recent shipped blocks
	blocks, _ := s.db.ListWorkBlocks()
	shippedCount := 0
	for _, wb := range blocks {
		if wb.Status != "shipped" || shippedCount >= maxBlocks {
			continue
		}
		shippedCount++
		issues, _ := s.db.ListWorkBlockIssues(wb.ID)
		stats, _ := s.db.GetWorkBlockStats(wb.ID)
		buf.WriteString(fmt.Sprintf("SHIPPED BLOCK: %s\n  Goal: %s\n", wb.Title, wb.Goal))
		if stats != nil {
			buf.WriteString(fmt.Sprintf("  Issues: %d planned, %d completed, %d cancelled\n  Runs: %d, Cost: $%.4f\n",
				stats.IssuesPlanned, stats.IssuesCompleted, stats.IssuesCancelled, stats.RunCount, stats.TotalCost))
		}
		for _, i := range issues {
			runCount, _ := s.db.CountRunsForIssue(i.Key)
			buf.WriteString(fmt.Sprintf("  - [%s] %s (status: %s, assignee: %s, runs: %d)\n",
				i.Key, i.Title, i.Status, i.AssigneeName, runCount))
		}
		buf.WriteString("\n")
	}

	// Recent completed issues
	recentIssues, _ := s.db.GetRecentCompletedIssues(maxIssues)
	if len(recentIssues) > 0 {
		buf.WriteString("RECENT COMPLETED ISSUES:\n")
		for _, i := range recentIssues {
			runCount, _ := s.db.CountRunsForIssue(i.Key)
			buf.WriteString(fmt.Sprintf("  - [%s] %s (status: %s, assignee: %s, runs: %d)\n",
				i.Key, i.Title, i.Status, i.AssigneeName, runCount))
		}
		buf.WriteString("\n")
	}

	// Current archetypes
	agents, _ := s.db.ListAgents()
	buf.WriteString("CURRENT AGENT ARCHETYPES:\n")
	for _, a := range agents {
		content, err := archetypes.Read(a.ArchetypeSlug)
		if err != nil {
			continue
		}
		buf.WriteString(fmt.Sprintf("\n--- %s (slug: %s, archetype: %s) ---\n%s\n",
			a.Name, a.Slug, a.ArchetypeSlug, string(content)))
	}

	// Board policies from DB
	boardPolicies, _ := s.db.GetActiveBoardPolicies()
	if len(boardPolicies) > 0 {
		buf.WriteString("\nBOARD DIRECTIVES (active, from human board members):\n")
		for _, bp := range boardPolicies {
			buf.WriteString(fmt.Sprintf("  - %s\n", bp.Directive))
		}
		buf.WriteString("\n")
	}

	// Accepted policies (already approved, for reconciliation)
	acceptedDir := filepath.Join("artifact-docs", "policies", "accepted")
	if entries, err := os.ReadDir(acceptedDir); err == nil && len(entries) > 0 {
		buf.WriteString("ACCEPTED POLICIES (currently active, review for reconciliation):\n")
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			content, err := os.ReadFile(filepath.Join(acceptedDir, e.Name()))
			if err != nil {
				continue
			}
			buf.WriteString(fmt.Sprintf("  [%s]: %s\n", e.Name(), strings.TrimSpace(string(content))))
		}
		buf.WriteString("\n")
	}

	if focus != "" {
		buf.WriteString(fmt.Sprintf("\nAUDIT FOCUS:\n%s\n", focus))
	}

	buf.WriteString(fmt.Sprintf(`
IMPORTANT: You have NO access to the project source code. You operate only on issues, agent archetypes, policies, and artifact-docs. Do NOT attempt to read or modify any source code files.

INSTRUCTIONS:
1. Review the completed work above. Look for patterns:
   - Agents with high retry counts (many runs per issue)
   - Agents that frequently get rejected
   - Missing guidance in archetypes that caused problems
   - Board directives that are not being followed

2. Review accepted policies for reconciliation -- are they still relevant? Do any contradict each other or the board directives?

3. Write short new policies (1-2 sentences each) to policies/.
   Write background/rationale separately to decisions/.

4. For each archetype that needs improvement, propose a patch via:
   POST $SECONDORDER_API_URL/api/v1/archetype-patches
   Headers: Authorization: Bearer $SECONDORDER_API_KEY, X-Audit-Run-ID: %s
   Body: {"agent_slug": "...", "proposed_content": "...full new archetype content..."}

5. Create feature requests for workflow improvements you identify. Use:
   POST $SECONDORDER_API_URL/api/v1/issues
   Body: {"title": "Feature: ...", "description": "...", "priority": 2}
   These are improvements to the SecondOrder system itself, not project work.

6. Review artifact-docs for stale or contradictory documentation. Clean up.

SO API (Authorization: Bearer $SECONDORDER_API_KEY):
  POST   $SECONDORDER_API_URL/api/v1/archetype-patches  - propose archetype change
  POST   $SECONDORDER_API_URL/api/v1/issues              - create feature request
  GET    $SECONDORDER_API_URL/api/v1/agents              - list team
  GET    $SECONDORDER_API_URL/api/v1/work-blocks         - list work blocks

BASE_URL: http://localhost:%d
`, auditRunID, s.port))

	return buf.String()
}

// RecoverStuckIssues finds issues stuck in in_progress/todo after a restart and re-wakes their agents.
func (s *Scheduler) RecoverStuckIssues() int {
	// Mark any stale "running" runs as failed since the process restarted
	staleCount, err := s.db.MarkStaleRunsFailed()
	if err != nil {
		log.WithError(err).Error("scheduler: failed to mark stale runs")
	} else if staleCount > 0 {
		log.WithField("count", staleCount).Info("scheduler: marked stale runs as failed")
	}

	issues, err := s.db.GetStuckIssues()
	if err != nil {
		log.WithError(err).Error("scheduler: failed to get stuck issues")
		return 0
	}
	if len(issues) == 0 {
		return 0
	}

	// Deduplicate by agent: only wake each agent once (for their highest-priority issue)
	woken := map[string]bool{}
	recovered := 0
	for i := range issues {
		issue := &issues[i]
		if issue.AssigneeAgentID == nil || woken[*issue.AssigneeAgentID] {
			continue
		}

		agent, err := s.db.GetAgent(*issue.AssigneeAgentID)
		if err != nil || !agent.Active {
			continue
		}

		// Check budget before waking
		over, _ := s.db.IsAgentOverBudget(agent.ID)
		if over {
			log.WithFields(log.Fields{"agent": agent.Name, "issue": issue.Key}).
				Warn("scheduler: agent over budget, skipping recovery")
			continue
		}

		log.WithFields(log.Fields{"agent": agent.Name, "issue": issue.Key, "status": issue.Status}).
			Info("scheduler: recovering stuck issue")
		s.WakeAgent(agent, issue)
		woken[agent.ID] = true
		recovered++
	}

	if recovered > 0 {
		details := fmt.Sprintf("Recovered %d stuck issues on startup (%d stale runs marked failed)", recovered, staleCount)
		s.logActivity("recovery", "system", "startup", nil, details)
	}

	return recovered
}

// StartHeartbeatLoop runs heartbeat checks on a timer (safety net)
func (s *Scheduler) StartHeartbeatLoop(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.mu.Lock()
				if s.stopped {
					s.mu.Unlock()
					return
				}
				s.mu.Unlock()
				s.runHeartbeats()
			}
		}
	}()
}

func (s *Scheduler) runHeartbeats() {
	agents, err := s.db.ListAgents()
	if err != nil {
		log.WithError(err).Error("scheduler: failed to list agents for heartbeat")
		return
	}
	for i := range agents {
		a := &agents[i]
		if !a.Active || !a.HeartbeatEnabled {
			continue
		}
		// Check if agent is over budget
		over, _ := s.db.IsAgentOverBudget(a.ID)
		if over {
			log.WithField("agent", a.Name).Warn("scheduler: agent over budget, skipping heartbeat")
			continue
		}
		s.WakeAgentHeartbeat(a)
	}
}

const workerRules = `RULES:
- You are fully autonomous. Do NOT ask questions interactively. Do NOT wait for human input.
- If you have a question or need clarification, post it as a comment on the ticket and mark the issue "blocked".
- Do NOT request approvals. Just do the work and mark done.
- Always checkout the issue first, then do the work, then update status.
- Write any documentation to the artifact-docs/ folder.`

const workerAPIRef = `SO API (Authorization: Bearer $SECONDORDER_API_KEY):
  GET    $SECONDORDER_API_URL/api/v1/inbox                              - your assigned issues
  GET    $SECONDORDER_API_URL/api/v1/issues/{key}                       - issue detail + comments
  POST   $SECONDORDER_API_URL/api/v1/issues/{key}/checkout              - claim issue
  PATCH  $SECONDORDER_API_URL/api/v1/issues/{key}                       - update status, comment, or reassignment ({"status":"...","comment":"...","assignee_slug":"..."})
  POST   $SECONDORDER_API_URL/api/v1/issues/{key}/comments              - add comment
  POST   $SECONDORDER_API_URL/api/v1/issues                             - create sub-issue
  GET    $SECONDORDER_API_URL/api/v1/usage                              - your token/cost usage`

const ceoRules = `RULES:
- You are fully autonomous. Do NOT ask questions interactively.
- Do NOT do implementation work yourself. Always delegate by creating sub-issues with assignee_slug and parent_issue_key.
- Break complex tasks into clear sub-issues with acceptance criteria.
- After delegating, mark the parent as "in_progress" and comment your plan.
- When reviewing completed work: approve, request changes via comment, or reassign.
- If blocked, post a comment and mark "blocked".
- If there is an active work block, focus your work on its goal. Assign relevant issues to the block.
- When all issues in a block are done, mark the block as "ready" via PATCH.
- To start new work, propose a work block first. A human must approve it before it becomes active.
- Only one work block can be active or proposed at a time.`

const ceoAPIRef = `SO API (Authorization: Bearer $SECONDORDER_API_KEY):
  GET    $SECONDORDER_API_URL/api/v1/inbox                              - your assigned issues
  GET    $SECONDORDER_API_URL/api/v1/issues/{key}                       - issue detail + comments
  PATCH  $SECONDORDER_API_URL/api/v1/issues/{key}                       - update status, comment, or reassignment ({"status":"...","comment":"...","assignee_slug":"..."})
  POST   $SECONDORDER_API_URL/api/v1/issues/{key}/comments              - add comment
  POST   $SECONDORDER_API_URL/api/v1/issues                             - create & assign: {"title":"...","assignee_slug":"...","parent_issue_key":"..."}
  GET    $SECONDORDER_API_URL/api/v1/agents                             - list team (slug, name, archetype)
  POST   $SECONDORDER_API_URL/api/v1/approvals/{id}/resolve             - review: {"status":"approved","comment":"..."}
  GET    $SECONDORDER_API_URL/api/v1/work-blocks                        - list work blocks
  GET    $SECONDORDER_API_URL/api/v1/work-blocks/{id}                   - block detail + issues + metrics
  POST   $SECONDORDER_API_URL/api/v1/work-blocks                        - propose block: {"title":"...","goal":"..."}
  PATCH  $SECONDORDER_API_URL/api/v1/work-blocks/{id}                   - update status: {"status":"ready"}
  POST   $SECONDORDER_API_URL/api/v1/work-blocks/{id}/issues            - assign issue: {"issue_key":"SO-5"}
  DELETE $SECONDORDER_API_URL/api/v1/work-blocks/{id}/issues/{key}      - unassign issue

Your team:
%s`

// liveWriter buffers stdout and flushes to DB periodically
type liveWriter struct {
	db       *db.DB
	runID    string
	interval time.Duration
	mu       sync.Mutex
	buf      strings.Builder
	lastFlush time.Time
}

func (w *liveWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if time.Since(w.lastFlush) >= w.interval {
		w.db.UpdateRunStdout(w.runID, w.buf.String())
		w.lastFlush = time.Now()
	}
	return n, err
}

func (w *liveWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.db.UpdateRunStdout(w.runID, w.buf.String())
}

func (w *liveWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

type tokenUsage struct {
	InputTokens       int64
	OutputTokens      int64
	CacheReadTokens   int64
	CacheCreateTokens int64
	TotalCostUSD      float64
}

func parseTokenUsage(stdout string) tokenUsage {
	var usage tokenUsage
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var msg struct {
			Type   string `json:"type"`
			Result struct {
				InputTokens              int64   `json:"input_tokens"`
				OutputTokens             int64   `json:"output_tokens"`
				CacheReadInputTokens     int64   `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int64   `json:"cache_creation_input_tokens"`
				TotalCostUSD             float64 `json:"total_cost_usd"`
			} `json:"result"`
			Stats struct {
				InputTokens  int64 `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
				TotalTokens  int64 `json:"total_tokens"`
				Models       map[string]struct {
					InputTokens  int64 `json:"input_tokens"`
					OutputTokens int64 `json:"output_tokens"`
				} `json:"models"`
			} `json:"stats"`
			Usage struct {
				InputTokens       int64 `json:"input_tokens"`
				CachedInputTokens int64 `json:"cached_input_tokens"`
				OutputTokens      int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Type == "result" {
			// Prefer explicit result fields
			if msg.Result.InputTokens > 0 || msg.Result.OutputTokens > 0 {
				usage.InputTokens = msg.Result.InputTokens
				usage.OutputTokens = msg.Result.OutputTokens
				usage.CacheReadTokens = msg.Result.CacheReadInputTokens
				usage.CacheCreateTokens = msg.Result.CacheCreationInputTokens
				usage.TotalCostUSD = msg.Result.TotalCostUSD
			} else if msg.Stats.InputTokens > 0 || msg.Stats.OutputTokens > 0 {
				// Fallback to stats top-level
				usage.InputTokens = msg.Stats.InputTokens
				usage.OutputTokens = msg.Stats.OutputTokens
			} else if len(msg.Stats.Models) > 0 {
				// Sum tokens from models map (common in some Gemini versions)
				for _, m := range msg.Stats.Models {
					usage.InputTokens += m.InputTokens
					usage.OutputTokens += m.OutputTokens
				}
			}
		} else if msg.Type == "turn.completed" {
			// Handle codex-cli usage format
			usage.InputTokens = msg.Usage.InputTokens
			usage.CacheReadTokens = msg.Usage.CachedInputTokens
			usage.OutputTokens = msg.Usage.OutputTokens
		}
	}
	return usage
}

func captureGitDiff(workingDir string) string {
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	diff := string(out)
	// Cap at 100KB
	if len(diff) > 100*1024 {
		diff = diff[:100*1024] + "\n... (truncated at 100KB)"
	}
	return diff
}
