package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/msoedov/secondorder/internal/models"
)

// --- Agents ---

func (d *DB) CreateAgent(a *models.Agent) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now
	_, err := d.Exec(`INSERT INTO agents (id, name, slug, archetype_slug, model, working_dir, max_turns, timeout_sec,
		heartbeat_enabled, heartbeat_cron, chrome_enabled, reports_to, review_agent_id, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Slug, a.ArchetypeSlug, a.Model, a.WorkingDir, a.MaxTurns, a.TimeoutSec,
		a.HeartbeatEnabled, a.HeartbeatCron, a.ChromeEnabled, a.ReportsTo, a.ReviewAgentID, a.Active, a.CreatedAt, a.UpdatedAt)
	return err
}

func (d *DB) GetAgent(id string) (*models.Agent, error) {
	a := &models.Agent{}
	err := d.QueryRow(`SELECT id, name, slug, archetype_slug, model, working_dir, max_turns, timeout_sec,
		heartbeat_enabled, heartbeat_cron, chrome_enabled, reports_to, review_agent_id, active, created_at, updated_at
		FROM agents WHERE id = ?`, id).Scan(
		&a.ID, &a.Name, &a.Slug, &a.ArchetypeSlug, &a.Model, &a.WorkingDir, &a.MaxTurns, &a.TimeoutSec,
		&a.HeartbeatEnabled, &a.HeartbeatCron, &a.ChromeEnabled, &a.ReportsTo, &a.ReviewAgentID, &a.Active, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) GetAgentBySlug(slug string) (*models.Agent, error) {
	a := &models.Agent{}
	err := d.QueryRow(`SELECT id, name, slug, archetype_slug, model, working_dir, max_turns, timeout_sec,
		heartbeat_enabled, heartbeat_cron, chrome_enabled, reports_to, review_agent_id, active, created_at, updated_at
		FROM agents WHERE slug = ?`, slug).Scan(
		&a.ID, &a.Name, &a.Slug, &a.ArchetypeSlug, &a.Model, &a.WorkingDir, &a.MaxTurns, &a.TimeoutSec,
		&a.HeartbeatEnabled, &a.HeartbeatCron, &a.ChromeEnabled, &a.ReportsTo, &a.ReviewAgentID, &a.Active, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) ListAgents() ([]models.Agent, error) {
	rows, err := d.Query(`SELECT id, name, slug, archetype_slug, model, working_dir, max_turns, timeout_sec,
		heartbeat_enabled, heartbeat_cron, chrome_enabled, reports_to, review_agent_id, active, created_at, updated_at
		FROM agents ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []models.Agent
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Slug, &a.ArchetypeSlug, &a.Model, &a.WorkingDir, &a.MaxTurns, &a.TimeoutSec,
			&a.HeartbeatEnabled, &a.HeartbeatCron, &a.ChromeEnabled, &a.ReportsTo, &a.ReviewAgentID, &a.Active, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (d *DB) UpdateAgent(a *models.Agent) error {
	a.UpdatedAt = time.Now()
	_, err := d.Exec(`UPDATE agents SET name=?, slug=?, archetype_slug=?, model=?, working_dir=?, max_turns=?, timeout_sec=?,
		heartbeat_enabled=?, heartbeat_cron=?, chrome_enabled=?, reports_to=?, review_agent_id=?, active=?, updated_at=?
		WHERE id=?`,
		a.Name, a.Slug, a.ArchetypeSlug, a.Model, a.WorkingDir, a.MaxTurns, a.TimeoutSec,
		a.HeartbeatEnabled, a.HeartbeatCron, a.ChromeEnabled, a.ReportsTo, a.ReviewAgentID, a.Active, a.UpdatedAt, a.ID)
	return err
}

func (d *DB) GetCEOAgent() (*models.Agent, error) {
	a := &models.Agent{}
	err := d.QueryRow(`SELECT id, name, slug, archetype_slug, model, working_dir, max_turns, timeout_sec,
		heartbeat_enabled, heartbeat_cron, chrome_enabled, reports_to, review_agent_id, active, created_at, updated_at
		FROM agents WHERE archetype_slug = 'ceo' LIMIT 1`).Scan(
		&a.ID, &a.Name, &a.Slug, &a.ArchetypeSlug, &a.Model, &a.WorkingDir, &a.MaxTurns, &a.TimeoutSec,
		&a.HeartbeatEnabled, &a.HeartbeatCron, &a.ChromeEnabled, &a.ReportsTo, &a.ReviewAgentID, &a.Active, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) GetReviewer(agentID string) (*models.Agent, error) {
	agent, err := d.GetAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	// 1. Explicit review agent
	if agent.ReviewAgentID != nil {
		reviewer, err := d.GetAgent(*agent.ReviewAgentID)
		if err == nil {
			return reviewer, nil
		}
	}

	// 2. Walk reports-to chain (up to 10 levels to prevent cycles)
	current := agent
	for i := 0; i < 10 && current.ReportsTo != nil; i++ {
		manager, err := d.GetAgent(*current.ReportsTo)
		if err != nil {
			break
		}
		return manager, nil
	}

	// 3. CEO fallback
	return d.GetCEOAgent()
}

func (d *DB) CountAgents() (int, int, error) {
	var total, active int
	err := d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN active=1 THEN 1 ELSE 0 END), 0) FROM agents`).Scan(&total, &active)
	return total, active, err
}

// --- Issues ---

func (d *DB) GetIssueByTitle(title string) (*models.Issue, error) {
	i := &models.Issue{}
	err := d.QueryRow(`SELECT id, key, title, description, status, priority, assignee_agent_id,
		parent_issue_key, work_block_id, started_at, completed_at, created_at, updated_at
		FROM issues WHERE LOWER(title) = LOWER(?)`, title).Scan(
		&i.ID, &i.Key, &i.Title, &i.Description, &i.Status, &i.Priority, &i.AssigneeAgentID,
		&i.ParentIssueKey, &i.WorkBlockID, &i.StartedAt, &i.CompletedAt, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return i, nil
}

func (d *DB) CreateIssue(i *models.Issue) error {
	if i.ID == "" {
		i.ID = uuid.NewString()
	}
	now := time.Now()
	i.CreatedAt = now
	i.UpdatedAt = now
	_, err := d.Exec(`INSERT INTO issues (id, key, title, description, status, priority, assignee_agent_id,
		parent_issue_key, work_block_id, started_at, completed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.ID, i.Key, i.Title, i.Description, i.Status, i.Priority, i.AssigneeAgentID,
		i.ParentIssueKey, i.WorkBlockID, i.StartedAt, i.CompletedAt, i.CreatedAt, i.UpdatedAt)
	return err
}

func (d *DB) GetIssue(key string) (*models.Issue, error) {
	i := &models.Issue{}
	err := d.QueryRow(`SELECT i.id, i.key, i.title, i.description, i.status, i.priority, i.assignee_agent_id,
		i.parent_issue_key, i.work_block_id, i.started_at, i.completed_at, i.created_at, i.updated_at,
		COALESCE(a.name, '')
		FROM issues i LEFT JOIN agents a ON i.assignee_agent_id = a.id
		WHERE i.key = ?`, key).Scan(
		&i.ID, &i.Key, &i.Title, &i.Description, &i.Status, &i.Priority, &i.AssigneeAgentID,
		&i.ParentIssueKey, &i.WorkBlockID, &i.StartedAt, &i.CompletedAt, &i.CreatedAt, &i.UpdatedAt,
		&i.AssigneeName)
	if err != nil {
		return nil, err
	}
	return i, nil
}

func (d *DB) ListIssues(status string, limit int) ([]models.Issue, error) {
	query := `SELECT i.id, i.key, i.title, i.description, i.status, i.priority, i.assignee_agent_id,
		i.parent_issue_key, i.work_block_id, i.started_at, i.completed_at, i.created_at, i.updated_at,
		COALESCE(a.name, '')
		FROM issues i LEFT JOIN agents a ON i.assignee_agent_id = a.id`

	var args []any
	if status != "" {
		query += " WHERE i.status = ?"
		args = append(args, status)
	}
	query += " ORDER BY i.priority DESC, i.created_at DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.Issue
	for rows.Next() {
		var i models.Issue
		if err := rows.Scan(&i.ID, &i.Key, &i.Title, &i.Description, &i.Status, &i.Priority, &i.AssigneeAgentID,
			&i.ParentIssueKey, &i.WorkBlockID, &i.StartedAt, &i.CompletedAt, &i.CreatedAt, &i.UpdatedAt,
			&i.AssigneeName); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

func (d *DB) GetRecentIssues(limit int) ([]models.Issue, error) {
	query := `SELECT i.id, i.key, i.title, i.description, i.status, i.priority, i.assignee_agent_id,
		i.parent_issue_key, i.work_block_id, i.started_at, i.completed_at, i.created_at, i.updated_at,
		COALESCE(a.name, '')
		FROM issues i LEFT JOIN agents a ON i.assignee_agent_id = a.id
		ORDER BY i.updated_at DESC`

	var args []any
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.Issue
	for rows.Next() {
		var i models.Issue
		if err := rows.Scan(&i.ID, &i.Key, &i.Title, &i.Description, &i.Status, &i.Priority, &i.AssigneeAgentID,
			&i.ParentIssueKey, &i.WorkBlockID, &i.StartedAt, &i.CompletedAt, &i.CreatedAt, &i.UpdatedAt,
			&i.AssigneeName); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

func (d *DB) UpdateIssue(i *models.Issue) error {
	i.UpdatedAt = time.Now()
	_, err := d.Exec(`UPDATE issues SET title=?, description=?, status=?, priority=?, assignee_agent_id=?,
		parent_issue_key=?, work_block_id=?, started_at=?, completed_at=?, updated_at=?
		WHERE key=?`,
		i.Title, i.Description, i.Status, i.Priority, i.AssigneeAgentID,
		i.ParentIssueKey, i.WorkBlockID, i.StartedAt, i.CompletedAt, i.UpdatedAt, i.Key)
	return err
}

func (d *DB) CheckoutIssue(key, agentID string, expectedStatuses []string) error {
	if len(expectedStatuses) == 0 {
		return fmt.Errorf("expectedStatuses cannot be empty")
	}
	placeholders := strings.Repeat("?,", len(expectedStatuses))
	placeholders = placeholders[:len(placeholders)-1]

	now := time.Now()
	query := fmt.Sprintf(`UPDATE issues SET status='in_progress', assignee_agent_id=?, started_at=?, updated_at=?
		WHERE key=? AND status IN (%s)`, placeholders)

	args := []any{agentID, now, now, key}
	for _, s := range expectedStatuses {
		args = append(args, s)
	}

	res, err := d.Exec(query, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("issue %s not in expected status", key)
	}
	return nil
}

func (d *DB) GetAgentInbox(agentID string) ([]models.Issue, error) {
	rows, err := d.Query(`SELECT id, key, title, description, status, priority, assignee_agent_id,
		parent_issue_key, work_block_id, started_at, completed_at, created_at, updated_at
		FROM issues WHERE assignee_agent_id=? AND status NOT IN ('done','cancelled','wont_do')
		ORDER BY priority DESC, created_at`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.Issue
	for rows.Next() {
		var i models.Issue
		if err := rows.Scan(&i.ID, &i.Key, &i.Title, &i.Description, &i.Status, &i.Priority, &i.AssigneeAgentID,
			&i.ParentIssueKey, &i.WorkBlockID, &i.StartedAt, &i.CompletedAt, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

func (d *DB) CountIssues() (int, int, error) {
	var total, open int
	err := d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status NOT IN ('done','cancelled','wont_do') THEN 1 ELSE 0 END), 0) FROM issues`).Scan(&total, &open)
	return total, open, err
}

func (d *DB) GetChildIssues(parentKey string) ([]models.Issue, error) {
	rows, err := d.Query(`SELECT id, key, title, description, status, priority, assignee_agent_id,
		parent_issue_key, work_block_id, started_at, completed_at, created_at, updated_at
		FROM issues WHERE parent_issue_key=? ORDER BY created_at`, parentKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.Issue
	for rows.Next() {
		var i models.Issue
		if err := rows.Scan(&i.ID, &i.Key, &i.Title, &i.Description, &i.Status, &i.Priority, &i.AssigneeAgentID,
			&i.ParentIssueKey, &i.WorkBlockID, &i.StartedAt, &i.CompletedAt, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

func (d *DB) NextIssueKey() (string, error) {
	var maxNum sql.NullInt64
	err := d.QueryRow(`SELECT MAX(CAST(SUBSTR(key, 4) AS INTEGER)) FROM issues WHERE key LIKE 'SO-%'`).Scan(&maxNum)
	if err != nil {
		return "", err
	}
	next := 1
	if maxNum.Valid {
		next = int(maxNum.Int64) + 1
	}
	return fmt.Sprintf("SO-%d", next), nil
}

// --- Runs ---

func (d *DB) CreateRun(r *models.Run) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	now := time.Now()
	r.StartedAt = now
	r.CreatedAt = now
	_, err := d.Exec(`INSERT INTO runs (id, agent_id, issue_key, mode, status, stdout, diff,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens, total_cost_usd,
		started_at, completed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.AgentID, r.IssueKey, r.Mode, r.Status, r.Stdout, r.Diff,
		r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheCreateTokens, r.TotalCostUSD,
		r.StartedAt, r.CompletedAt, r.CreatedAt)
	return err
}

func (d *DB) GetRun(id string) (*models.Run, error) {
	r := &models.Run{}
	err := d.QueryRow(`SELECT id, agent_id, issue_key, mode, status, stdout, diff,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens, total_cost_usd,
		started_at, completed_at, created_at
		FROM runs WHERE id=?`, id).Scan(
		&r.ID, &r.AgentID, &r.IssueKey, &r.Mode, &r.Status, &r.Stdout, &r.Diff,
		&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheCreateTokens, &r.TotalCostUSD,
		&r.StartedAt, &r.CompletedAt, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (d *DB) ListRunsForAgent(agentID string, limit int) ([]models.Run, error) {
	query := `SELECT id, agent_id, issue_key, mode, status, stdout, diff,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens, total_cost_usd,
		started_at, completed_at, created_at
		FROM runs WHERE agent_id=? ORDER BY created_at DESC`
	var args []any
	args = append(args, agentID)
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

func (d *DB) ListRunsForIssue(issueKey string) ([]models.Run, error) {
	rows, err := d.Query(`SELECT id, agent_id, issue_key, mode, status, stdout, diff,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens, total_cost_usd,
		started_at, completed_at, created_at
		FROM runs WHERE issue_key=? ORDER BY created_at DESC`, issueKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

func (d *DB) UpdateRunStdout(id, stdout string) error {
	_, err := d.Exec(`UPDATE runs SET stdout=? WHERE id=?`, stdout, id)
	return err
}

func (d *DB) CompleteRun(id, status, stdout, diff string, tokens models.Run) error {
	now := time.Now()
	_, err := d.Exec(`UPDATE runs SET status=?, stdout=?, diff=?, input_tokens=?, output_tokens=?,
		cache_read_tokens=?, cache_create_tokens=?, total_cost_usd=?, completed_at=?
		WHERE id=?`,
		status, stdout, diff, tokens.InputTokens, tokens.OutputTokens,
		tokens.CacheReadTokens, tokens.CacheCreateTokens, tokens.TotalCostUSD, now, id)
	return err
}

func (d *DB) CountRunningRuns() (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM runs WHERE status='running'`).Scan(&count)
	return count, err
}

func scanRuns(rows *sql.Rows) ([]models.Run, error) {
	var runs []models.Run
	for rows.Next() {
		var r models.Run
		if err := rows.Scan(&r.ID, &r.AgentID, &r.IssueKey, &r.Mode, &r.Status, &r.Stdout, &r.Diff,
			&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheCreateTokens, &r.TotalCostUSD,
			&r.StartedAt, &r.CompletedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// --- Comments ---

func (d *DB) CreateComment(c *models.Comment) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	c.CreatedAt = time.Now()
	_, err := d.Exec(`INSERT INTO comments (id, issue_key, agent_id, author, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.IssueKey, c.AgentID, c.Author, c.Body, c.CreatedAt)
	return err
}

func (d *DB) ListComments(issueKey string) ([]models.Comment, error) {
	rows, err := d.Query(`SELECT id, issue_key, agent_id, author, body, created_at
		FROM comments WHERE issue_key=? ORDER BY created_at`, issueKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.IssueKey, &c.AgentID, &c.Author, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// --- API Keys ---

func (d *DB) CreateAPIKey(agentID, keyHash, prefix string) error {
	_, err := d.Exec(`INSERT INTO api_keys (id, agent_id, key_hash, prefix, created_at) VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), agentID, keyHash, prefix, time.Now())
	return err
}

func (d *DB) GetAgentByAPIKey(keyHash string) (*models.Agent, error) {
	a := &models.Agent{}
	err := d.QueryRow(`SELECT a.id, a.name, a.slug, a.archetype_slug, a.model, a.working_dir, a.max_turns, a.timeout_sec,
		a.heartbeat_enabled, a.heartbeat_cron, a.chrome_enabled, a.reports_to, a.review_agent_id, a.active, a.created_at, a.updated_at
		FROM api_keys k JOIN agents a ON k.agent_id = a.id
		WHERE k.key_hash=? AND k.revoked_at IS NULL`, keyHash).Scan(
		&a.ID, &a.Name, &a.Slug, &a.ArchetypeSlug, &a.Model, &a.WorkingDir, &a.MaxTurns, &a.TimeoutSec,
		&a.HeartbeatEnabled, &a.HeartbeatCron, &a.ChromeEnabled, &a.ReportsTo, &a.ReviewAgentID, &a.Active, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) RevokeAPIKeys(agentID string) error {
	_, err := d.Exec(`UPDATE api_keys SET revoked_at=? WHERE agent_id=? AND revoked_at IS NULL`, time.Now(), agentID)
	return err
}

// --- Approvals ---

func (d *DB) CreateApproval(a *models.Approval) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	a.CreatedAt = time.Now()
	_, err := d.Exec(`INSERT INTO approvals (id, issue_key, requested_by, reviewer_id, status, comment, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.IssueKey, a.RequestedBy, a.ReviewerID, a.Status, a.Comment, a.CreatedAt)
	return err
}

func (d *DB) GetApproval(id string) (*models.Approval, error) {
	a := &models.Approval{}
	err := d.QueryRow(`SELECT id, issue_key, requested_by, reviewer_id, status, comment, resolved_at, created_at
		FROM approvals WHERE id=?`, id).Scan(
		&a.ID, &a.IssueKey, &a.RequestedBy, &a.ReviewerID, &a.Status, &a.Comment, &a.ResolvedAt, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) ListPendingApprovals() ([]models.Approval, error) {
	rows, err := d.Query(`SELECT id, issue_key, requested_by, reviewer_id, status, comment, resolved_at, created_at
		FROM approvals WHERE status='pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []models.Approval
	for rows.Next() {
		var a models.Approval
		if err := rows.Scan(&a.ID, &a.IssueKey, &a.RequestedBy, &a.ReviewerID, &a.Status, &a.Comment, &a.ResolvedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		approvals = append(approvals, a)
	}
	return approvals, rows.Err()
}

func (d *DB) ResolveApproval(id, status, comment string) error {
	now := time.Now()
	_, err := d.Exec(`UPDATE approvals SET status=?, comment=?, resolved_at=? WHERE id=?`, status, comment, now, id)
	return err
}

// --- Cost / Budget ---

func (d *DB) CreateCostEvent(e *models.CostEvent) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	e.CreatedAt = time.Now()
	_, err := d.Exec(`INSERT INTO cost_events (id, run_id, agent_id, input_tokens, output_tokens, total_cost_usd, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.RunID, e.AgentID, e.InputTokens, e.OutputTokens, e.TotalCostUSD, e.CreatedAt)
	return err
}

func (d *DB) GetAgentUsage(agentID string) (todayTokens int64, todayCost float64, totalTokens int64, totalCost float64, err error) {
	err = d.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN DATE(created_at)=DATE('now') THEN input_tokens+output_tokens ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN DATE(created_at)=DATE('now') THEN total_cost_usd ELSE 0 END), 0),
		COALESCE(SUM(input_tokens+output_tokens), 0),
		COALESCE(SUM(total_cost_usd), 0)
		FROM cost_events WHERE agent_id=?`, agentID).Scan(&todayTokens, &todayCost, &totalTokens, &totalCost)
	return
}

func (d *DB) GetTotalCostToday() (float64, error) {
	var cost float64
	err := d.QueryRow(`SELECT COALESCE(SUM(total_cost_usd), 0) FROM cost_events WHERE DATE(created_at)=DATE('now')`).Scan(&cost)
	return cost, err
}

func (d *DB) GetBudgetPolicy(agentID string) (*models.BudgetPolicy, error) {
	b := &models.BudgetPolicy{}
	err := d.QueryRow(`SELECT id, agent_id, daily_token_limit, daily_cost_limit, active, created_at
		FROM budget_policies WHERE agent_id=? AND active=1`, agentID).Scan(
		&b.ID, &b.AgentID, &b.DailyTokenLimit, &b.DailyCostLimit, &b.Active, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (d *DB) IsAgentOverBudget(agentID string) (bool, error) {
	policy, err := d.GetBudgetPolicy(agentID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	todayTokens, todayCost, _, _, err := d.GetAgentUsage(agentID)
	if err != nil {
		return false, err
	}

	if policy.DailyTokenLimit > 0 && todayTokens >= policy.DailyTokenLimit {
		return true, nil
	}
	if policy.DailyCostLimit > 0 && todayCost >= policy.DailyCostLimit {
		return true, nil
	}
	return false, nil
}

// --- Activity ---

func (d *DB) LogActivity(action, entityType, entityID string, agentID *string, details string) error {
	_, err := d.Exec(`INSERT INTO activity_log (id, action, entity_type, entity_id, agent_id, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), action, entityType, entityID, agentID, details, time.Now())
	return err
}

func (d *DB) ListActivity(limit int) ([]models.ActivityLog, error) {
	query := `SELECT a.id, a.action, a.entity_type, a.entity_id, a.agent_id, a.details, a.created_at
		FROM activity_log a ORDER BY a.created_at DESC`
	var args []any
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.ActivityLog
	for rows.Next() {
		var l models.ActivityLog
		if err := rows.Scan(&l.ID, &l.Action, &l.EntityType, &l.EntityID, &l.AgentID, &l.Details, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// --- Dashboard ---

func (d *DB) GetDashboardStats() (*models.DashboardStats, error) {
	s := &models.DashboardStats{}
	var err error

	s.TotalAgents, s.ActiveAgents, err = d.CountAgents()
	if err != nil {
		return nil, fmt.Errorf("count agents: %w", err)
	}

	s.TotalIssues, s.OpenIssues, err = d.CountIssues()
	if err != nil {
		return nil, fmt.Errorf("count issues: %w", err)
	}

	s.RunningRuns, err = d.CountRunningRuns()
	if err != nil {
		return nil, fmt.Errorf("count runs: %w", err)
	}

	s.TotalCostToday, err = d.GetTotalCostToday()
	if err != nil {
		return nil, fmt.Errorf("cost today: %w", err)
	}

	return s, nil
}

// --- Labels ---

func (d *DB) CreateLabel(l *models.Label) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	_, err := d.Exec(`INSERT INTO labels (id, name, color) VALUES (?, ?, ?)`, l.ID, l.Name, l.Color)
	return err
}

func (d *DB) ListLabels() ([]models.Label, error) {
	rows, err := d.Query(`SELECT id, name, color FROM labels ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []models.Label
	for rows.Next() {
		var l models.Label
		if err := rows.Scan(&l.ID, &l.Name, &l.Color); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

func (d *DB) AddLabelToIssue(issueID, labelID string) error {
	_, err := d.Exec(`INSERT OR IGNORE INTO issue_labels (issue_id, label_id) VALUES (?, ?)`, issueID, labelID)
	return err
}

// --- Work Blocks ---

func (d *DB) CreateWorkBlock(wb *models.WorkBlock) error {
	// Enforce one active/proposed at a time
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM work_blocks WHERE status IN ('proposed','active')`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot create work block: an active or proposed block already exists")
	}

	if wb.ID == "" {
		wb.ID = uuid.NewString()
	}
	now := time.Now()
	wb.CreatedAt = now
	wb.UpdatedAt = now
	if wb.Status == "" {
		wb.Status = models.WBStatusProposed
	}
	_, err := d.Exec(`INSERT INTO work_blocks (id, title, goal, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		wb.ID, wb.Title, wb.Goal, wb.Status, wb.CreatedAt, wb.UpdatedAt)
	return err
}

func (d *DB) GetWorkBlock(id string) (*models.WorkBlock, error) {
	wb := &models.WorkBlock{}
	err := d.QueryRow(`SELECT id, title, goal, status, created_at, updated_at, completed_at FROM work_blocks WHERE id=?`, id).Scan(
		&wb.ID, &wb.Title, &wb.Goal, &wb.Status, &wb.CreatedAt, &wb.UpdatedAt, &wb.CompletedAt)
	if err != nil {
		return nil, err
	}
	return wb, nil
}

func (d *DB) GetActiveWorkBlock() (*models.WorkBlock, error) {
	wb := &models.WorkBlock{}
	err := d.QueryRow(`SELECT id, title, goal, status, created_at, updated_at, completed_at FROM work_blocks WHERE status='active' LIMIT 1`).Scan(
		&wb.ID, &wb.Title, &wb.Goal, &wb.Status, &wb.CreatedAt, &wb.UpdatedAt, &wb.CompletedAt)
	if err != nil {
		return nil, err
	}
	return wb, nil
}

func (d *DB) ListWorkBlocks() ([]models.WorkBlock, error) {
	rows, err := d.Query(`SELECT id, title, goal, status, created_at, updated_at, completed_at FROM work_blocks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []models.WorkBlock
	for rows.Next() {
		var wb models.WorkBlock
		if err := rows.Scan(&wb.ID, &wb.Title, &wb.Goal, &wb.Status, &wb.CreatedAt, &wb.UpdatedAt, &wb.CompletedAt); err != nil {
			return nil, err
		}
		blocks = append(blocks, wb)
	}
	return blocks, rows.Err()
}

func (d *DB) UpdateWorkBlockStatus(id, newStatus string) error {
	wb, err := d.GetWorkBlock(id)
	if err != nil {
		return fmt.Errorf("work block not found: %w", err)
	}

	// Validate transitions
	allowed := map[string][]string{
		models.WBStatusProposed: {models.WBStatusActive, models.WBStatusCancelled},
		models.WBStatusActive:   {models.WBStatusReady, models.WBStatusCancelled},
		models.WBStatusReady:    {models.WBStatusShipped, models.WBStatusActive, models.WBStatusCancelled},
	}
	valid := false
	for _, s := range allowed[wb.Status] {
		if s == newStatus {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid transition: %s -> %s", wb.Status, newStatus)
	}

	// Sequential enforcement for activation
	if newStatus == models.WBStatusActive {
		var count int
		d.QueryRow(`SELECT COUNT(*) FROM work_blocks WHERE status='active' AND id != ?`, id).Scan(&count)
		if count > 0 {
			return fmt.Errorf("another work block is already active")
		}
	}

	now := time.Now()
	if newStatus == models.WBStatusShipped || newStatus == models.WBStatusCancelled {
		_, err = d.Exec(`UPDATE work_blocks SET status=?, updated_at=?, completed_at=? WHERE id=?`, newStatus, now, now, id)
	} else if newStatus == models.WBStatusActive && wb.Status == models.WBStatusReady {
		// Reactivation: clear completed_at
		_, err = d.Exec(`UPDATE work_blocks SET status=?, updated_at=?, completed_at=NULL WHERE id=?`, newStatus, now, id)
	} else {
		_, err = d.Exec(`UPDATE work_blocks SET status=?, updated_at=? WHERE id=?`, newStatus, now, id)
	}
	return err
}

func (d *DB) AssignIssueToWorkBlock(issueKey, blockID string) error {
	wb, err := d.GetWorkBlock(blockID)
	if err != nil {
		return fmt.Errorf("work block not found: %w", err)
	}
	if wb.Status != models.WBStatusActive {
		return fmt.Errorf("can only assign issues to an active work block")
	}
	now := time.Now()
	_, err = d.Exec(`UPDATE issues SET work_block_id=?, updated_at=? WHERE key=?`, blockID, now, issueKey)
	return err
}

func (d *DB) UnassignIssueFromWorkBlock(issueKey string) error {
	now := time.Now()
	_, err := d.Exec(`UPDATE issues SET work_block_id=NULL, updated_at=? WHERE key=?`, now, issueKey)
	return err
}

func (d *DB) ListWorkBlockIssues(blockID string) ([]models.Issue, error) {
	rows, err := d.Query(`SELECT i.id, i.key, i.title, i.description, i.status, i.priority, i.assignee_agent_id,
		i.parent_issue_key, i.work_block_id, i.started_at, i.completed_at, i.created_at, i.updated_at,
		COALESCE(a.name, '')
		FROM issues i LEFT JOIN agents a ON i.assignee_agent_id = a.id
		WHERE i.work_block_id=? ORDER BY i.priority DESC, i.created_at`, blockID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.Issue
	for rows.Next() {
		var i models.Issue
		if err := rows.Scan(&i.ID, &i.Key, &i.Title, &i.Description, &i.Status, &i.Priority, &i.AssigneeAgentID,
			&i.ParentIssueKey, &i.WorkBlockID, &i.StartedAt, &i.CompletedAt, &i.CreatedAt, &i.UpdatedAt,
			&i.AssigneeName); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

func (d *DB) CheckWorkBlockAutoReady(blockID string) (bool, error) {
	var total, notDone int
	if err := d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status NOT IN ('done','cancelled','wont_do') THEN 1 ELSE 0 END), 0)
		FROM issues WHERE work_block_id=?`, blockID).Scan(&total, &notDone); err != nil {
		return false, err
	}
	return total > 0 && notDone == 0, nil
}

func (d *DB) GetWorkBlockStats(blockID string) (*models.WorkBlockStats, error) {
	s := &models.WorkBlockStats{}

	// Issue counts
	d.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN status='done' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status IN ('cancelled','wont_do') THEN 1 ELSE 0 END), 0)
		FROM issues WHERE work_block_id=?`, blockID).Scan(&s.IssuesPlanned, &s.IssuesCompleted, &s.IssuesCancelled)

	// Cost and run count
	d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_cost_usd), 0)
		FROM runs WHERE issue_key IN (SELECT key FROM issues WHERE work_block_id=?)`, blockID).Scan(&s.RunCount, &s.TotalCost)

	// Cycle time
	wb, err := d.GetWorkBlock(blockID)
	if err == nil && wb.CompletedAt != nil {
		s.CycleTimeHours = wb.CompletedAt.Sub(wb.CreatedAt).Hours()
	}

	return s, nil
}

func (d *DB) CountRunsForIssue(issueKey string) (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM runs WHERE issue_key=?`, issueKey).Scan(&count)
	return count, err
}

// --- Audit ---

func (d *DB) CreateAuditRun(ar *models.AuditRun) error {
	if ar.ID == "" {
		ar.ID = uuid.NewString()
	}
	ar.CreatedAt = time.Now()
	ar.Status = "running"
	_, err := d.Exec(`INSERT INTO audit_runs (id, run_id, status, issues_reviewed, blocks_reviewed, findings, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ar.ID, ar.RunID, ar.Status, ar.IssuesReviewed, ar.BlocksReviewed, ar.Findings, ar.CreatedAt)
	return err
}

func (d *DB) GetAuditRun(id string) (*models.AuditRun, error) {
	ar := &models.AuditRun{}
	err := d.QueryRow(`SELECT id, run_id, status, issues_reviewed, blocks_reviewed, findings, created_at, completed_at
		FROM audit_runs WHERE id=?`, id).Scan(
		&ar.ID, &ar.RunID, &ar.Status, &ar.IssuesReviewed, &ar.BlocksReviewed, &ar.Findings, &ar.CreatedAt, &ar.CompletedAt)
	if err != nil {
		return nil, err
	}
	return ar, nil
}

func (d *DB) ListAuditRuns(limit int) ([]models.AuditRun, error) {
	query := `SELECT id, run_id, status, issues_reviewed, blocks_reviewed, findings, created_at, completed_at
		FROM audit_runs ORDER BY created_at DESC`
	var args []any
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []models.AuditRun
	for rows.Next() {
		var ar models.AuditRun
		if err := rows.Scan(&ar.ID, &ar.RunID, &ar.Status, &ar.IssuesReviewed, &ar.BlocksReviewed, &ar.Findings, &ar.CreatedAt, &ar.CompletedAt); err != nil {
			return nil, err
		}
		runs = append(runs, ar)
	}
	return runs, rows.Err()
}

func (d *DB) CreateArchetypePatch(p *models.ArchetypePatch) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	p.CreatedAt = time.Now()
	if p.Status == "" {
		p.Status = "pending"
	}
	_, err := d.Exec(`INSERT INTO archetype_patches (id, audit_run_id, agent_slug, current_content, proposed_content, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.AuditRunID, p.AgentSlug, p.CurrentContent, p.ProposedContent, p.Status, p.CreatedAt)
	return err
}

func (d *DB) ListPendingPatches() ([]models.ArchetypePatch, error) {
	rows, err := d.Query(`SELECT id, audit_run_id, agent_slug, current_content, proposed_content, status, reviewed_at, created_at
		FROM archetype_patches WHERE status='pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPatches(rows)
}

func (d *DB) GetArchetypePatch(id string) (*models.ArchetypePatch, error) {
	p := &models.ArchetypePatch{}
	err := d.QueryRow(`SELECT id, audit_run_id, agent_slug, current_content, proposed_content, status, reviewed_at, created_at
		FROM archetype_patches WHERE id=?`, id).Scan(
		&p.ID, &p.AuditRunID, &p.AgentSlug, &p.CurrentContent, &p.ProposedContent, &p.Status, &p.ReviewedAt, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (d *DB) ResolvePatch(id, status string) error {
	now := time.Now()
	_, err := d.Exec(`UPDATE archetype_patches SET status=?, reviewed_at=? WHERE id=?`, status, now, id)
	return err
}

func scanPatches(rows *sql.Rows) ([]models.ArchetypePatch, error) {
	var patches []models.ArchetypePatch
	for rows.Next() {
		var p models.ArchetypePatch
		if err := rows.Scan(&p.ID, &p.AuditRunID, &p.AgentSlug, &p.CurrentContent, &p.ProposedContent, &p.Status, &p.ReviewedAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		patches = append(patches, p)
	}
	return patches, rows.Err()
}

// --- Board Policies ---

func (d *DB) CreateBoardPolicy(bp *models.BoardPolicy) error {
	if bp.ID == "" {
		bp.ID = uuid.NewString()
	}
	now := time.Now()
	bp.CreatedAt = now
	bp.UpdatedAt = now
	bp.Active = false
	_, err := d.Exec(`INSERT INTO board_policies (id, directive, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		bp.ID, bp.Directive, bp.Active, bp.CreatedAt, bp.UpdatedAt)
	return err
}

func (d *DB) ListBoardPolicies() ([]models.BoardPolicy, error) {
	rows, err := d.Query(`SELECT id, directive, active, created_at, updated_at FROM board_policies ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []models.BoardPolicy
	for rows.Next() {
		var bp models.BoardPolicy
		if err := rows.Scan(&bp.ID, &bp.Directive, &bp.Active, &bp.CreatedAt, &bp.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, bp)
	}
	return policies, rows.Err()
}

func (d *DB) ToggleBoardPolicy(id string) error {
	now := time.Now()
	_, err := d.Exec(`UPDATE board_policies SET active = NOT active, updated_at=? WHERE id=?`, now, id)
	return err
}

func (d *DB) DeleteBoardPolicy(id string) error {
	_, err := d.Exec(`DELETE FROM board_policies WHERE id=?`, id)
	return err
}

func (d *DB) GetActiveBoardPolicies() ([]models.BoardPolicy, error) {
	rows, err := d.Query(`SELECT id, directive, active, created_at, updated_at FROM board_policies WHERE active=1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []models.BoardPolicy
	for rows.Next() {
		var bp models.BoardPolicy
		if err := rows.Scan(&bp.ID, &bp.Directive, &bp.Active, &bp.CreatedAt, &bp.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, bp)
	}
	return policies, rows.Err()
}

func (d *DB) GetRecentCompletedIssues(limit int) ([]models.Issue, error) {
	rows, err := d.Query(`SELECT i.id, i.key, i.title, i.description, i.status, i.priority, i.assignee_agent_id,
		i.parent_issue_key, i.work_block_id, i.started_at, i.completed_at, i.created_at, i.updated_at,
		COALESCE(a.name, '')
		FROM issues i LEFT JOIN agents a ON i.assignee_agent_id = a.id
		WHERE i.status IN ('done','cancelled','wont_do')
		ORDER BY i.completed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.Issue
	for rows.Next() {
		var i models.Issue
		if err := rows.Scan(&i.ID, &i.Key, &i.Title, &i.Description, &i.Status, &i.Priority, &i.AssigneeAgentID,
			&i.ParentIssueKey, &i.WorkBlockID, &i.StartedAt, &i.CompletedAt, &i.CreatedAt, &i.UpdatedAt,
			&i.AssigneeName); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

// --------------- Settings ---------------

func (d *DB) GetSetting(key string) (string, error) {
	var value string
	err := d.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value, err
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

func (d *DB) GetAllSettings() (map[string]string, error) {
	rows, err := d.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, rows.Err()
}
