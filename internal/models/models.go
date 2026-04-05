package models

import "time"

// Issue statuses
const (
	StatusTodo        = "todo"
	StatusInProgress  = "in_progress"
	StatusInReview    = "in_review"
	StatusDone        = "done"
	StatusBlocked     = "blocked"
	StatusCancelled   = "cancelled"
	StatusWontDo      = "wont_do"
	StatusBoardReview = "board_review"
)

// Run statuses
const (
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
)

// Runner types
const (
	RunnerClaudeCode   = "claude_code"
	RunnerGemini       = "gemini"
	RunnerCodex        = "codex"
	RunnerAntigravity  = "antigravity"
)

var RunnerModels = map[string][]string{
	RunnerClaudeCode: {
		"claude-3-7-sonnet-20250219",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
		"sonnet",
		"opus",
		"haiku",
	},
	RunnerGemini:      {"gemini-2.0-flash", "gemini-2.0-flash-lite", "gemini-1.5-pro", "gemini-1.5-flash"},
	RunnerCodex:       {"gpt-4o", "o4-mini"},
	RunnerAntigravity: {"default"},
}

func IsValidModelForRunner(runner, model string) bool {
	models, ok := RunnerModels[runner]
	if !ok {
		return false
	}
	for _, m := range models {
		if m == model {
			return true
		}
	}
	return false
}

// WorkBlock statuses
const (
	WBStatusProposed  = "proposed"
	WBStatusActive    = "active"
	WBStatusReady     = "ready"
	WBStatusShipped   = "shipped"
	WBStatusCancelled = "cancelled"
)

type Agent struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	ArchetypeSlug    string    `json:"archetype_slug"`
	Model            string    `json:"model"`
	Runner           string    `json:"runner"`
	ApiKeyEnv        string    `json:"api_key_env"`
	WorkingDir       string    `json:"working_dir"`
	MaxTurns         int       `json:"max_turns"`
	TimeoutSec       int       `json:"timeout_sec"`
	HeartbeatEnabled bool      `json:"heartbeat_enabled"`
	HeartbeatCron    string    `json:"heartbeat_cron"`
	ChromeEnabled    bool      `json:"chrome_enabled"`
	ReportsTo        *string   `json:"reports_to"`
	ReviewAgentID    *string   `json:"review_agent_id"`
	Active           bool      `json:"active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Issue struct {
	ID              string     `json:"id"`
	Key             string     `json:"key"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	Priority        int        `json:"priority"`
	AssigneeAgentID *string    `json:"assignee_agent_id"`
	ParentIssueKey  *string    `json:"parent_issue_key"`
	WorkBlockID     *string    `json:"work_block_id"`
	AssigneeName    string     `json:"assignee_name,omitempty"`
	AssigneeSlug    string     `json:"assignee_slug,omitempty"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Run struct {
	ID                string     `json:"id"`
	AgentID           string     `json:"agent_id"`
	IssueKey          *string    `json:"issue_key"`
	Mode              string     `json:"mode"`
	Status            string     `json:"status"`
	Stdout            string     `json:"stdout"`
	Diff              string     `json:"diff"`
	InputTokens       int64      `json:"input_tokens"`
	OutputTokens      int64      `json:"output_tokens"`
	CacheReadTokens   int64      `json:"cache_read_tokens"`
	CacheCreateTokens int64      `json:"cache_create_tokens"`
	TotalCostUSD      float64    `json:"total_cost_usd"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

type Comment struct {
	ID        string    `json:"id"`
	IssueKey  string    `json:"issue_key"`
	AgentID   *string   `json:"agent_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Approval struct {
	ID          string     `json:"id"`
	IssueKey    string     `json:"issue_key"`
	RequestedBy string    `json:"requested_by"`
	ReviewerID  *string   `json:"reviewer_id"`
	Status      string    `json:"status"`
	Comment     string    `json:"comment"`
	ResolvedAt  *time.Time `json:"resolved_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type APIKey struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	KeyHash   string     `json:"-"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}

type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type IssueLabel struct {
	IssueID string `json:"issue_id"`
	LabelID string `json:"label_id"`
}

type ActivityLog struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	AgentID    *string   `json:"agent_id"`
	Details    string    `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}

type CostEvent struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	AgentID      string    `json:"agent_id"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalCostUSD float64   `json:"total_cost_usd"`
	CreatedAt    time.Time `json:"created_at"`
}

type BudgetPolicy struct {
	ID              string    `json:"id"`
	AgentID         string    `json:"agent_id"`
	DailyTokenLimit int64     `json:"daily_token_limit"`
	DailyCostLimit  float64   `json:"daily_cost_limit"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
}

type Secret struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	AgentID     string    `json:"agent_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type AgentConfigRevision struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Config    string    `json:"config"`
	ChangedBy string    `json:"changed_by"`
	CreatedAt time.Time `json:"created_at"`
}

type CronJob struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	AgentName string     `json:"agent_name,omitempty"`
	Task      string     `json:"task"`
	Frequency string     `json:"frequency"`
	Active    bool       `json:"active"`
	LastRunAt *time.Time `json:"last_run_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type RunEvent struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	EventType string    `json:"event_type"`
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

type WorkBlock struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Goal               string     `json:"goal"`
	AcceptanceCriteria string     `json:"acceptance_criteria"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ActivatedAt        *time.Time `json:"activated_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	Issues             []Issue         `json:"issues,omitempty"`
	Stats              *WorkBlockStats `json:"stats,omitempty"`
}

type WorkBlockStats struct {
	TotalCost       float64 `json:"total_cost"`
	RunCount        int     `json:"run_count"`
	IssuesPlanned   int     `json:"issues_planned"`
	IssuesCompleted int     `json:"issues_completed"`
	IssuesCancelled int     `json:"issues_cancelled"`
	CycleTimeHours  float64 `json:"cycle_time_hours"`
}

type AuditRun struct {
	ID             string     `json:"id"`
	RunID          *string    `json:"run_id"`
	Runner         string     `json:"runner"`
	Model          string     `json:"model"`
	Status         string     `json:"status"`
	IssuesReviewed int        `json:"issues_reviewed"`
	BlocksReviewed int        `json:"blocks_reviewed"`
	Findings       string     `json:"findings"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}

type ArchetypePatch struct {
	ID              string     `json:"id"`
	AuditRunID      string     `json:"audit_run_id"`
	AgentSlug       string     `json:"agent_slug"`
	CurrentContent  string     `json:"current_content"`
	ProposedContent string     `json:"proposed_content"`
	Status          string     `json:"status"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type BoardPolicy struct {
	ID        string    `json:"id"`
	Directive string    `json:"directive"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DashboardStats struct {
        TotalAgents    int     `json:"total_agents"`
        ActiveAgents   int     `json:"active_agents"`
        TotalIssues    int     `json:"total_issues"`
        OpenIssues     int     `json:"open_issues"`
        RunningRuns    int     `json:"running_runs"`
        TotalCostToday float64 `json:"total_cost_today"`
}

type DailyStat struct {
	Date          string `json:"date"`
	Label         string `json:"label"`
	Updates       int    `json:"updates"`
	Creations     int    `json:"creations"`
	Checkouts     int    `json:"checkouts"`
	AssignToBlock int    `json:"assign_to_block"`
	Deletions     int    `json:"deletions"`
	Backlog       int    `json:"backlog"`
	Recovery      int    `json:"recovery"`
}

