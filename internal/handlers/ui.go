package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/msoedov/secondorder/internal/archetypes"
	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
	acvalidator "github.com/msoedov/secondorder/internal/validator"
)

type UI struct {
	db    *db.DB
	sse   *SSEHub
	tmpl  *template.Template
	wake  func(agent *models.Agent, issue *models.Issue)
	sched interface {
		WakeAgentHeartbeat(agent *models.Agent)
		RunAudit(maxBlocks, maxIssues int, focus, runner, model string) (string, error)
		CancelAudit(auditRunID string) error
	}
}

func NewUI(database *db.DB, sse *SSEHub, tmpl *template.Template, wake func(*models.Agent, *models.Issue), sched interface {
	WakeAgentHeartbeat(*models.Agent)
	RunAudit(int, int, string, string, string) (string, error)
	CancelAudit(string) error
}) *UI {
	return &UI{db: database, sse: sse, tmpl: tmpl, wake: wake, sched: sched}
}

func (u *UI) Dashboard(w http.ResponseWriter, r *http.Request) {
	stats, _ := u.db.GetDashboardStats()
	issues, _ := u.db.GetRecentIssues(20)
	agents, _ := u.db.ListAgents()
	runningAgents, _ := u.db.GetRunningAgentIDs()

	// Calculate alignment score
	workBlocks, _ := u.db.ListWorkBlocks()
	totalWorkBlocks := len(workBlocks)
	alignedWorkBlocks := 0
	for _, wb := range workBlocks {
		if wb.ApexBlockID != nil && *wb.ApexBlockID != "" {
			alignedWorkBlocks++
		}
	}
	alignmentScore := 0
	if totalWorkBlocks > 0 {
		alignmentScore = (alignedWorkBlocks * 100) / totalWorkBlocks
	}

	data := map[string]any{
		"Stats":          stats,
		"Issues":         issues,
		"Agents":         agents,
		"RunningAgents":  runningAgents,
		"AlignmentScore": alignmentScore,
	}

	if activeBlock, err := u.db.GetActiveWorkBlock(); err == nil {
		data["ActiveBlock"] = activeBlock
		if blockStats, err := u.db.GetWorkBlockStats(activeBlock.ID); err == nil {
			data["ActiveBlockStats"] = blockStats
		}
	}

	u.render(w, "dashboard", data)
}

func (u *UI) ListIssues(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		if r.FormValue("backlog") == "1" {
			u.submitBacklog(w, r)
			return
		}
		u.createIssueUI(w, r)
		return
	}
	status := r.URL.Query().Get("status")
	dbStatus := status
	if status == "" {
		dbStatus = "todo,in_progress,in_review"
	} else if status == "all" {
		dbStatus = ""
	}
	issues, _ := u.db.ListIssues(dbStatus, 100)
	agents, _ := u.db.ListAgents()

	data := map[string]any{
		"Issues":        issues,
		"Agents":        agents,
		"CurrentStatus": status,
		"Error":         r.URL.Query().Get("error"),
		"Success":       r.URL.Query().Get("success"),
	}

	if r.Header.Get("HX-Request") != "" {
		u.render(w, "issue_list", data)
		return
	}
	u.render(w, "issues", data)
}

func (u *UI) createIssueUI(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	title := r.FormValue("title")
	if title == "" {
		http.Redirect(w, r, "/issues", http.StatusSeeOther)
		return
	}

	key, err := u.db.NextIssueKey()
	if err != nil {
		http.Redirect(w, r, "/issues?error=Failed+to+generate+issue+key", http.StatusSeeOther)
		return
	}

	issue := &models.Issue{
		ID:             uuid.New().String(),
		Key:            key,
		Title:          title,
		Description:    r.FormValue("description"),
		Type:           r.FormValue("type"),
		Status:         models.StatusTodo,
		ParentIssueKey: ptrStrOrNil(r.FormValue("parent_issue_key")),
	}

	if issue.Type == "" {
		issue.Type = "task"
	}

	if p, err := strconv.Atoi(r.FormValue("priority")); err == nil {
		issue.Priority = p
	}

	// Assign
	assigneeSlug := r.FormValue("assignee_slug")
	var assignee *models.Agent
	if assigneeSlug != "" {
		if a, err := u.db.GetAgentBySlug(assigneeSlug); err == nil {
			issue.AssigneeAgentID = &a.ID
			assignee = a
		}
	} else {
		if ceo, err := u.db.GetCEOAgent(); err == nil {
			issue.AssigneeAgentID = &ceo.ID
			assignee = ceo
		}
	}

	if err := u.db.CreateIssue(issue); err != nil {
		http.Redirect(w, r, "/issues?error=Failed+to+create+issue", http.StatusSeeOther)
		return
	}

	// Validate AC
	warnings := acvalidator.ValidateAC(issue.Type, issue.Description)
	warningParam := ""
	if len(warnings) > 0 {
		warningParam = "&warning=Heads+up:+This+" + issue.Type + "+issue+seems+to+be+missing+some+standard+acceptance+criteria.+This+might+block+your+agents."
	}

	// SSE broadcast
	createData, _ := json.Marshal(map[string]any{
		"key":      key,
		"title":    issue.Title,
		"status":   issue.Status,
		"type":     issue.Type,
		"warnings": warnings,
	})
	u.sse.Broadcast("issue_created", string(createData))

	LogActivityAndBroadcast(u.db, u.sse, u.tmpl, "create", "issue", key, nil, title)

	if assignee != nil && u.wake != nil {
		go u.wake(assignee, issue)
	}

	if issue.ParentIssueKey != nil {
		http.Redirect(w, r, "/issues/"+*issue.ParentIssueKey+"?success=Created"+warningParam, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/issues?success=Created"+warningParam, http.StatusSeeOther)
}

func ptrStrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (u *UI) submitBacklog(w http.ResponseWriter, r *http.Request) {
	text := strings.TrimSpace(r.FormValue("backlog_text"))
	if text == "" {
		http.Redirect(w, r, "/issues", http.StatusSeeOther)
		return
	}

	// Resolve CEO working dir so backlog.md is written where the CEO agent reads it.
	ceoWorkingDir := "artifact-docs"
	ceo, ceoErr := u.db.GetCEOAgent()
	if ceoErr == nil && ceo.WorkingDir != "" {
		ceoWorkingDir = ceo.WorkingDir
	}

	backlogDir := filepath.Join(ceoWorkingDir, "artifact-docs")
	backlogPath := filepath.Join(backlogDir, "backlog.md")
	if err := os.MkdirAll(backlogDir, 0o755); err != nil {
		http.Redirect(w, r, "/issues?error=Failed+to+create+backlog+directory", http.StatusSeeOther)
		return
	}

	entry := fmt.Sprintf("\n---\n\n### %s\n\n%s\n", time.Now().Format("2006-01-02 15:04:05"), text)

	f, err := os.OpenFile(backlogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		http.Redirect(w, r, "/issues?error=Failed+to+write+backlog", http.StatusSeeOther)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		http.Redirect(w, r, "/issues?error=Failed+to+write+backlog", http.StatusSeeOther)
		return
	}

	// Wake CEO to triage the backlog
	if ceoErr == nil && u.sched != nil {
		go u.sched.WakeAgentHeartbeat(ceo)
	}

	LogActivityAndBroadcast(u.db, u.sse, u.tmpl, "backlog", "issue", "backlog", nil, text)

	http.Redirect(w, r, "/issues?success=Submitted+to+backlog+for+triage", http.StatusSeeOther)
}

func (u *UI) IssueDetail(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	if r.Method == http.MethodPost || r.Method == http.MethodPatch {
		u.updateIssueUI(w, r, key)
		return
	}

	issue, err := u.db.GetIssue(key)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		u.render(w, "not_found", map[string]any{
			"Title":   "Issue not found",
			"Message": fmt.Sprintf("Issue %q does not exist.", key),
			"BackURL": "/issues",
		})
		return
	}
	comments, _ := u.db.ListComments(key)
	children, _ := u.db.GetChildIssues(key)
	runs, _ := u.db.ListRunsForIssue(key)
	agents, _ := u.db.ListAgents()

	var assignee *models.Agent
	if issue.AssigneeAgentID != nil {
		assignee, _ = u.db.GetAgent(*issue.AssigneeAgentID)
	}

	u.render(w, "issue_detail", map[string]any{
		"Issue":    issue,
		"Assignee": assignee,
		"Comments": comments,
		"Children": children,
		"Runs":     runs,
		"Agents":   agents,
		"Error":    r.URL.Query().Get("error"),
		"Success":  r.URL.Query().Get("success"),
		"Warning":  r.URL.Query().Get("warning"),
	})
}

func (u *UI) updateIssueUI(w http.ResponseWriter, r *http.Request, key string) {
	r.ParseForm()
	issue, err := u.db.GetIssue(key)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	action := r.FormValue("action")
	switch action {
	case "restart":
		issue.Status = models.StatusTodo
		issue.StartedAt = nil
		u.db.UpdateIssue(issue)
		if issue.AssigneeAgentID != nil {
			if a, err := u.db.GetAgent(*issue.AssigneeAgentID); err == nil && u.wake != nil {
				go u.wake(a, issue)
			}
		}
	case "cancel":
		issue.Status = models.StatusCancelled
		u.db.UpdateIssue(issue)
	case "comment":
		body := r.FormValue("body")
		if body != "" {
			comment := &models.Comment{
				ID:       uuid.New().String(),
				IssueKey: key,
				Author:   "Board",
				Body:     body,
			}
			u.db.CreateComment(comment)
			data, _ := json.Marshal(map[string]string{"issue_key": key, "author": "Board", "body": body})
			u.sse.Broadcast("comment", string(data))

			// Reopen ticket to in_progress when board comments on a completed/blocked/in_review issue
			if issue.Status == models.StatusDone || issue.Status == models.StatusBlocked || issue.Status == models.StatusInReview {
				issue.Status = models.StatusInProgress
				u.db.UpdateIssue(issue)
				if issue.AssigneeAgentID != nil {
					if a, err := u.db.GetAgent(*issue.AssigneeAgentID); err == nil && u.wake != nil {
						go u.wake(a, issue)
					}
				}
			}
		}
	case "assign":
		slug := r.FormValue("assignee_slug")
		if a, err := u.db.GetAgentBySlug(slug); err == nil {
			issue.AssigneeAgentID = &a.ID
			u.db.UpdateIssue(issue)
			if u.wake != nil {
				go u.wake(a, issue)
			}
		}
	default:
		if s := r.FormValue("status"); s != "" {
			issue.Status = s
		}
		if t := r.FormValue("type"); t != "" {
			issue.Type = t
		}
		if t := r.FormValue("title"); t != "" {
			issue.Title = t
		}
		if d := r.FormValue("description"); d != "" {
			issue.Description = d
		}
		if p, err := strconv.Atoi(r.FormValue("priority")); err == nil {
			issue.Priority = p
		}
		u.db.UpdateIssue(issue)
	case "toggle_stage":
		stageID, _ := strconv.Atoi(r.FormValue("stage_id"))
		status := r.FormValue("status")
		stages, currentStageID, err := acvalidator.ApplyStageToggle(issue.Stages, stageID, status)
		if err != nil {
			if r.Header.Get("HX-Request") != "" {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Redirect(w, r, "/issues/"+key+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		issue.Stages = stages
		issue.CurrentStageID = currentStageID
		u.db.UpdateIssue(issue)
	}

	// Validate AC for UI warning
	warnings := acvalidator.ValidateAC(issue.Type, issue.Description)

	// SSE broadcast
	var assigneeSlug string
	if issue.AssigneeAgentID != nil {
		if a, err := u.db.GetAgent(*issue.AssigneeAgentID); err == nil {
			assigneeSlug = a.Slug
		}
	}
	updateData, _ := json.Marshal(map[string]any{
		"key":              key,
		"status":           issue.Status,
		"type":             issue.Type,
		"title":            issue.Title,
		"assignee_slug":    assigneeSlug,
		"stages":           issue.Stages,
		"current_stage_id": issue.CurrentStageID,
		"warnings":         warnings,
	})
	u.sse.Broadcast("issue_updated", string(updateData))

	if r.Header.Get("HX-Request") != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	warningParam := ""
	if len(warnings) > 0 {
		warningParam = "&flash=warning&msg=Heads+up:+This+" + issue.Type + "+issue+seems+to+be+missing+some+standard+acceptance+criteria.+This+might+block+your+agents."
	}

	http.Redirect(w, r, "/issues/"+key+"?success=Issue+created"+warningParam, http.StatusSeeOther)
}

func (u *UI) ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		u.createAgentUI(w, r)
		return
	}
	agents, _ := u.db.ListAgents()
	u.render(w, "agents", map[string]any{"Agents": agents})
}

func (u *UI) createAgentUI(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("name")
	if name == "" {
		http.Redirect(w, r, "/agents", http.StatusSeeOther)
		return
	}

	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	// Check for slug collision
	if _, err := u.db.GetAgentBySlug(slug); err == nil {
		http.Error(w, fmt.Sprintf("An agent with the name %q already exists", name), http.StatusConflict)
		return
	}

	agent := &models.Agent{
		ID:            uuid.New().String(),
		Name:          name,
		Slug:          slug,
		ArchetypeSlug: r.FormValue("archetype_slug"),
		Model:         r.FormValue("model"),
		Runner:        r.FormValue("runner"),
		ApiKeyEnv:     r.FormValue("api_key_env"),
		WorkingDir:    r.FormValue("working_dir"),
		MaxTurns:      50,
		TimeoutSec:    models.DefaultAgentTimeoutSec,
		Active:        true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if agent.Runner == "" {
		agent.Runner = models.RunnerClaudeCode
	}
	if agent.Model == "" {
		if m, ok := models.RunnerModels[agent.Runner]; ok && len(m) > 0 {
			agent.Model = m[0]
		}
	}
	if !models.IsValidModelForRunner(agent.Runner, agent.Model) {
		http.Error(w, fmt.Sprintf("invalid model %q for runner %q", agent.Model, agent.Runner), http.StatusBadRequest)
		return
	}
	if agent.ArchetypeSlug == "" {
		agent.ArchetypeSlug = "other"
	}
	if !archetypes.Exists(agent.ArchetypeSlug) {
		http.Error(w, fmt.Sprintf("archetype not found: %s", agent.ArchetypeSlug), http.StatusBadRequest)
		return
	}
	if agent.WorkingDir == "" {
		agent.WorkingDir = "."
	}

	if mt, err := strconv.Atoi(r.FormValue("max_turns")); err == nil && mt > 0 {
		agent.MaxTurns = mt
	}
	if ts, err := strconv.Atoi(r.FormValue("timeout_sec")); err == nil && ts > 0 {
		agent.TimeoutSec = ts
	}
	agent.HeartbeatEnabled = r.FormValue("heartbeat_enabled") == "on"
	agent.ChromeEnabled = r.FormValue("chrome_enabled") == "on"

	u.db.CreateAgent(agent)

	// SSE broadcast
	agentData, _ := json.Marshal(map[string]any{
		"id":     agent.ID,
		"slug":   agent.Slug,
		"name":   agent.Name,
		"active": agent.Active,
	})
	u.sse.Broadcast("agent_created", string(agentData))

	http.Redirect(w, r, "/agents/"+slug, http.StatusSeeOther)
}

func (u *UI) AgentDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	if r.Method == http.MethodPost || r.Method == http.MethodPatch {
		u.updateAgentUI(w, r, slug)
		return
	}

	agent, err := u.db.GetAgentBySlug(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	runs, _ := u.db.ListRunsForAgent(agent.ID, 20)
	issues, _ := u.db.GetAgentInbox(agent.ID)
	availableIssues, _ := u.db.ListIssues("todo,in_progress,in_review", 100)
	todayTokens, todayCost, totalTokens, totalCost, _ := u.db.GetAgentUsage(agent.ID)

	u.render(w, "agent_detail", map[string]any{
		"Agent":           agent,
		"Runs":            runs,
		"Issues":          issues,
		"AvailableIssues": availableIssues,
		"TodayTokens":     todayTokens,
		"TodayCost":       todayCost,
		"TotalTokens":     totalTokens,
		"TotalCost":       totalCost,
	})
}

func (u *UI) updateAgentUI(w http.ResponseWriter, r *http.Request, slug string) {
	r.ParseForm()
	agent, err := u.db.GetAgentBySlug(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if name := r.FormValue("name"); name != "" {
		agent.Name = name
	}
	if as := r.FormValue("archetype_slug"); as != "" {
		agent.ArchetypeSlug = as
	}
	if m := r.FormValue("model"); m != "" {
		agent.Model = m
	}
	if rn := r.FormValue("runner"); rn != "" {
		agent.Runner = rn
	}
	if !models.IsValidModelForRunner(agent.Runner, agent.Model) {
		http.Error(w, fmt.Sprintf("invalid model %q for runner %q", agent.Model, agent.Runner), http.StatusBadRequest)
		return
	}
	agent.ApiKeyEnv = r.FormValue("api_key_env")
	if wd := r.FormValue("working_dir"); wd != "" {
		agent.WorkingDir = wd
	}
	if mt, err := strconv.Atoi(r.FormValue("max_turns")); err == nil && mt > 0 {
		agent.MaxTurns = mt
	}
	if ts, err := strconv.Atoi(r.FormValue("timeout_sec")); err == nil && ts > 0 {
		agent.TimeoutSec = ts
	}
	agent.HeartbeatEnabled = r.FormValue("heartbeat_enabled") == "on"
	agent.ChromeEnabled = r.FormValue("chrome_enabled") == "on"
	agent.Active = r.FormValue("active") != "off"

	u.db.UpdateAgent(agent)

	// SSE broadcast
	agentData, _ := json.Marshal(map[string]any{
		"id":     agent.ID,
		"slug":   agent.Slug,
		"active": agent.Active,
	})
	u.sse.Broadcast("agent_updated", string(agentData))

	http.Redirect(w, r, "/agents/"+slug, http.StatusSeeOther)
}

func (u *UI) AgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	agent, err := u.db.GetAgentBySlug(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u.sched.WakeAgentHeartbeat(agent)
	http.Redirect(w, r, "/agents/"+slug, http.StatusSeeOther)
}

func (u *UI) AgentAssign(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	r.ParseForm()
	issueKey := r.FormValue("issue_key")

	agent, err := u.db.GetAgentBySlug(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	issue, err := u.db.GetIssue(issueKey)
	if err != nil {
		http.Error(w, "issue not found", http.StatusBadRequest)
		return
	}

	issue.AssigneeAgentID = &agent.ID
	u.db.UpdateIssue(issue)

	// SSE broadcast
	updateData, _ := json.Marshal(map[string]string{
		"key":           issueKey,
		"status":        issue.Status,
		"title":         issue.Title,
		"assignee_slug": agent.Slug,
	})
	u.sse.Broadcast("issue_updated", string(updateData))

	if u.wake != nil {
		go u.wake(agent, issue)
	}

	http.Redirect(w, r, "/agents/"+slug, http.StatusSeeOther)
}

func (u *UI) RunDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := u.db.GetRun(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	agent, _ := u.db.GetAgent(run.AgentID)

	var parentIssueKey string
	if run.IssueKey != nil {
		issue, err := u.db.GetIssue(*run.IssueKey)
		if err == nil && issue.ParentIssueKey != nil {
			parentIssueKey = *issue.ParentIssueKey
		}
	}

	u.render(w, "run_detail", map[string]any{
		"Run":             run,
		"Agent":           agent,
		"ParentIssueKey":  parentIssueKey,
		"FormattedStdout": template.HTML(formatStreamJSON(run.Stdout, run.Status)),
	})
}

func (u *UI) RunStdout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := u.db.GetRun(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	// If run is finished, send 286 to tell HTMX to stop polling
	if run.Status != models.RunStatusRunning {
		w.Header().Set("HX-Trigger", "run-complete")
		w.WriteHeader(286)
	}
	fmt.Fprint(w, formatStreamJSON(run.Stdout, run.Status))
}

func (u *UI) SearchIssuesAndAgents(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	if q == "" {
		jsonOK(w, []any{})
		return
	}

	var results []map[string]string

	issues, _ := u.db.ListIssues("", 50)
	for _, i := range issues {
		if strings.Contains(strings.ToLower(i.Title), q) || strings.Contains(strings.ToLower(i.Key), q) {
			results = append(results, map[string]string{
				"type":  "issue",
				"key":   i.Key,
				"title": i.Title,
				"url":   "/issues/" + i.Key,
			})
		}
	}

	agents, _ := u.db.ListAgents()
	for _, a := range agents {
		if strings.Contains(strings.ToLower(a.Name), q) || strings.Contains(strings.ToLower(a.Slug), q) {
			results = append(results, map[string]string{
				"type":  "agent",
				"key":   a.Slug,
				"title": a.Name,
				"url":   "/agents/" + a.Slug,
			})
		}
	}

	jsonOK(w, results)
}

func (u *UI) ListWorkBlocks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		u.createWorkBlockUI(w, r)
		return
	}
	blocks, _ := u.db.ListWorkBlocks()
	apexBlocks, _ := u.db.ListApexBlocks()
	u.render(w, "work_blocks", map[string]any{
		"Blocks":     blocks,
		"ApexBlocks": apexBlocks,
	})
}

func (u *UI) createWorkBlockUI(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	title := r.FormValue("title")
	if title == "" {
		http.Redirect(w, r, "/work-blocks", http.StatusSeeOther)
		return
	}
	wb := &models.WorkBlock{
		Title:              title,
		Goal:               r.FormValue("goal"),
		AcceptanceCriteria: r.FormValue("acceptance_criteria"),
		NorthStarMetric:    r.FormValue("north_star_metric"),
		NorthStarTarget:    r.FormValue("north_star_target"),
		Status:             models.WBStatusProposed,
		ApexBlockID:        ptrStrOrNil(r.FormValue("apex_block_id")),
	}
	u.db.CreateWorkBlock(wb)
	http.Redirect(w, r, "/work-blocks/"+wb.ID, http.StatusSeeOther)
}

func (u *UI) WorkBlockDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if r.Method == http.MethodPost {
		u.updateWorkBlockUI(w, r, id)
		return
	}

	wb, err := u.db.GetWorkBlock(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	issues, _ := u.db.ListWorkBlockIssues(id)
	stats, _ := u.db.GetWorkBlockStats(id)
	apexBlocks, _ := u.db.ListApexBlocks()

	u.render(w, "work_block_detail", map[string]any{
		"Block":      wb,
		"Issues":     issues,
		"Stats":      stats,
		"ApexBlocks": apexBlocks,
	})
}

func (u *UI) updateWorkBlockUI(w http.ResponseWriter, r *http.Request, id string) {
	r.ParseForm()
	action := r.FormValue("action")

	wb, _ := u.db.GetWorkBlock(id)

	switch action {
	case "activate":
		u.db.UpdateWorkBlockStatus(id, models.WBStatusActive)
	case "ready":
		u.db.UpdateWorkBlockStatus(id, models.WBStatusReady)
	case "ship":
		u.db.UpdateWorkBlockStatus(id, models.WBStatusShipped)
	case "reactivate":
		u.db.UpdateWorkBlockStatus(id, models.WBStatusActive)
	case "cancel":
		u.db.UpdateWorkBlockStatus(id, models.WBStatusCancelled)
	case "update":
		if t := r.FormValue("title"); t != "" {
			wb.Title = t
		}
		if g := r.FormValue("goal"); g != "" {
			wb.Goal = g
		}
		if ac := r.FormValue("acceptance_criteria"); ac != "" {
			wb.AcceptanceCriteria = ac
		}
		wb.NorthStarMetric = r.FormValue("north_star_metric")
		wb.NorthStarTarget = r.FormValue("north_star_target")
		wb.ApexBlockID = ptrStrOrNil(r.FormValue("apex_block_id"))
		u.db.UpdateWorkBlock(wb)
	case "assign_issue":
		issueKey := r.FormValue("issue_key")
		if issueKey != "" {
			u.db.AssignIssueToWorkBlock(issueKey, id)
			LogActivityAndBroadcast(u.db, u.sse, u.tmpl, "assign_to_block", "issue", issueKey, nil, id)
			if issue, err := u.db.GetIssue(issueKey); err == nil {
				updateData, _ := json.Marshal(map[string]string{
					"key":    issueKey,
					"status": issue.Status,
					"title":  issue.Title,
				})
				u.sse.Broadcast("issue_updated", string(updateData))
			}
		}
	case "unassign_issue":
		issueKey := r.FormValue("issue_key")
		if issueKey != "" {
			u.db.UnassignIssueFromWorkBlock(issueKey)
			if issue, err := u.db.GetIssue(issueKey); err == nil {
				updateData, _ := json.Marshal(map[string]string{
					"key":    issueKey,
					"status": issue.Status,
					"title":  issue.Title,
				})
				u.sse.Broadcast("issue_updated", string(updateData))
			}
		}
	}

	http.Redirect(w, r, "/work-blocks/"+id, http.StatusSeeOther)
}

func (u *UI) ActivityPage(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	limit := 30
	offset := (page - 1) * limit

	logs, _ := u.db.ListActivity(limit, offset)
	total, _ := u.db.CountActivity()
	dailyStats, _ := u.db.GetDailyActivityStats(14)
	overview := buildActivityOverview(dailyStats)

	u.render(w, "activity", map[string]any{
		"Logs":       logs,
		"DailyStats": dailyStats,
		"Overview":   overview,
		"Page":       page,
		"Total":      total,
		"Limit":      limit,
		"HasNext":    total > page*limit,
		"HasPrev":    page > 1,
		"NextPage":   page + 1,
		"PrevPage":   page - 1,
	})
}

func buildActivityOverview(dailyStats []models.DailyStat) models.ActivityOverview {
	overview := models.ActivityOverview{
		WindowDays: len(dailyStats),
	}
	streak := 0

	for _, stat := range dailyStats {
		dayTotal := totalActivityForDay(stat)
		overview.TotalActions += dayTotal
		overview.Completed += stat.Completed

		if dayTotal > overview.BusiestDayCount {
			overview.BusiestDayCount = dayTotal
			overview.BusiestDayLabel = stat.Label
		}

		if dayTotal > 0 {
			overview.ActiveDays++
			streak++
			overview.CurrentStreak = streak
			if streak > overview.LongestStreak {
				overview.LongestStreak = streak
			}
			continue
		}

		streak = 0
		overview.CurrentStreak = 0
	}

	if overview.WindowDays > 0 {
		overview.AvgPerDay = float64(overview.TotalActions) / float64(overview.WindowDays)
	}
	if overview.BusiestDayLabel == "" {
		overview.BusiestDayLabel = "No activity yet"
	}

	return overview
}

func totalActivityForDay(stat models.DailyStat) int {
	return stat.Creations + stat.Updates + stat.Checkouts + stat.AssignToBlock +
		stat.Deletions + stat.Backlog + stat.Recovery + stat.Completed
}
func (u *UI) PoliciesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		u.handlePoliciesAction(w, r)
		return
	}

	boardPolicies, _ := u.db.ListBoardPolicies()
	pendingPatches, _ := u.db.ListPendingPatches()
	auditRuns, _ := u.db.ListAuditRuns(20)
	pendingPolicies := u.readPolicyDir("policies")
	acceptedPolicies := u.readPolicyDir(filepath.Join("policies", "accepted"))
	disabledPolicies := u.readPolicyDir(filepath.Join("policies", "disabled"))
	errMsg := r.URL.Query().Get("error")

	u.render(w, "policies", map[string]any{
		"BoardPolicies":    boardPolicies,
		"PendingPolicies":  pendingPolicies,
		"AcceptedPolicies": acceptedPolicies,
		"DisabledPolicies": disabledPolicies,
		"PendingPatches":   pendingPatches,
		"AuditRuns":        auditRuns,
		"Error":            errMsg,
	})
}

type policyFile struct {
	Name    string
	Content string
}

func (u *UI) readPolicyDir(dirname string) []policyFile {
	agents, _ := u.db.ListAgents()
	if len(agents) == 0 {
		return nil
	}
	dir := filepath.Join(agents[0].WorkingDir, "artifact-docs", dirname)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var policies []policyFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		policies = append(policies, policyFile{Name: e.Name(), Content: string(content)})
	}
	return policies
}

func (u *UI) handlePoliciesAction(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	action := r.FormValue("action")

	switch action {
	case "run_audit":
		maxBlocks := 3
		maxIssues := 50
		if v, err := strconv.Atoi(r.FormValue("max_blocks")); err == nil && v > 0 {
			maxBlocks = v
		}
		if v, err := strconv.Atoi(r.FormValue("max_issues")); err == nil && v > 0 {
			maxIssues = v
		}
		focus := r.FormValue("focus")
		runner := r.FormValue("runner")
		model := r.FormValue("model")
		if _, err := u.sched.RunAudit(maxBlocks, maxIssues, focus, runner, model); err != nil {
			http.Redirect(w, r, "/policies?error="+err.Error(), http.StatusSeeOther)
			return
		}

	case "add_directive":
		directive := strings.TrimSpace(r.FormValue("directive"))
		if directive != "" {
			u.db.CreateBoardPolicy(&models.BoardPolicy{Directive: directive})
		}

	case "toggle_directive":
		id := r.FormValue("policy_id")
		if id != "" {
			u.db.ToggleBoardPolicy(id)
		}

	case "delete_directive":
		id := r.FormValue("policy_id")
		if id != "" {
			u.db.DeleteBoardPolicy(id)
		}

	case "disable_policy":
		filename := r.FormValue("filename")
		if filename != "" {
			u.movePolicyFile(filename, filepath.Join("policies", "accepted"), filepath.Join("policies", "disabled"))
		}

	case "enable_policy":
		filename := r.FormValue("filename")
		if filename != "" {
			u.movePolicyFile(filename, filepath.Join("policies", "disabled"), filepath.Join("policies", "accepted"))
		}

	case "accept_policy":
		filename := r.FormValue("filename")
		if filename != "" {
			u.movePolicyFile(filename, "policies", filepath.Join("policies", "accepted"))
		}

	case "reject_policy":
		filename := r.FormValue("filename")
		if filename != "" {
			u.deletePolicyFile("policies", filename)
		}

	case "approve_patch":
		patchID := r.FormValue("patch_id")
		if patchID != "" {
			u.applyPatch(patchID)
		}

	case "reject_patch":
		patchID := r.FormValue("patch_id")
		if patchID != "" {
			u.db.ResolvePatch(patchID, "rejected")
		}

	case "cancel_audit":
		auditID := r.FormValue("audit_id")
		if auditID != "" {
			u.sched.CancelAudit(auditID)
		}
	}

	http.Redirect(w, r, "/policies", http.StatusSeeOther)
}

func (u *UI) applyPatch(patchID string) {
	patch, err := u.db.GetArchetypePatch(patchID)
	if err != nil {
		return
	}
	filePath := filepath.Join("archetypes", patch.AgentSlug+".md")
	current, _ := os.ReadFile(filePath)
	if len(current) > 0 && patch.CurrentContent == "" {
		patch.CurrentContent = string(current)
	}
	if err := os.WriteFile(filePath, []byte(patch.ProposedContent), 0644); err != nil {
		return
	}
	u.db.ResolvePatch(patchID, "approved")
}

func (u *UI) writePolicy(dirname, filename, content string) {
	agents, _ := u.db.ListAgents()
	if len(agents) == 0 {
		return
	}
	dir := filepath.Join(agents[0].WorkingDir, "artifact-docs", dirname)
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
}

func (u *UI) policyBaseDir() string {
	agents, _ := u.db.ListAgents()
	if len(agents) == 0 {
		return "artifact-docs"
	}
	return filepath.Join(agents[0].WorkingDir, "artifact-docs")
}

func (u *UI) movePolicyFile(filename, fromDir, toDir string) {
	base := u.policyBaseDir()
	src := filepath.Join(base, fromDir, filename)
	dstDir := filepath.Join(base, toDir)
	os.MkdirAll(dstDir, 0755)
	content, err := os.ReadFile(src)
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(dstDir, filename), content, 0644)
	os.Remove(src)
}

func (u *UI) deletePolicyFile(dir, filename string) {
	base := u.policyBaseDir()
	os.Remove(filepath.Join(base, dir, filename))
}

func (u *UI) Settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		u.saveSettings(w, r)
		return
	}

	settings, _ := u.db.GetAllSettings()

	version := models.Version

	goVersion := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		goVersion = info.GoVersion
	}

	var sqliteVersion string
	u.db.QueryRow("SELECT sqlite_version()").Scan(&sqliteVersion)

	gitHubURL := settings["github_url"]
	if gitHubURL == "" {
		gitHubURL = "https://github.com/msoedov/secondorder"
	}

	u.render(w, "settings", map[string]any{
		"Settings":      settings,
		"Version":       version,
		"GoVersion":     goVersion,
		"SQLiteVersion": sqliteVersion,
		"GitHubURL":     gitHubURL,
		"Flash":         r.URL.Query().Get("msg"),
		"Error":         r.URL.Query().Get("error"),
	})
}

func (u *UI) saveSettings(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	section := r.FormValue("section")

	var keys []string
	switch section {
	case "instance":
		keys = []string{"instance_name", "issue_prefix"}
	case "telegram":
		keys = []string{"telegram_token", "telegram_chat_id"}
	default:
		http.Redirect(w, r, "/settings?error=Unknown+section", http.StatusSeeOther)
		return
	}

	for _, k := range keys {
		if err := u.db.SetSetting(k, r.FormValue(k)); err != nil {
			http.Redirect(w, r, "/settings?error=Failed+to+save+settings", http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/settings?msg=Settings+saved", http.StatusSeeOther)
}

func (u *UI) ListCrons(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		u.createCronUI(w, r)
		return
	}
	crons, _ := u.db.ListCronJobs()
	agents, _ := u.db.ListAgents()
	u.render(w, "crons", map[string]any{
		"Crons":  crons,
		"Agents": agents,
		"Error":  r.URL.Query().Get("error"),
	})
}

func (u *UI) createCronUI(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	agentID := r.FormValue("agent_id")
	task := r.FormValue("task")
	frequency := r.FormValue("frequency")

	if agentID == "" || task == "" {
		http.Redirect(w, r, "/crons?error=Agent+and+task+are+required", http.StatusSeeOther)
		return
	}

	validFreqs := map[string]bool{"1m": true, "20m": true, "1h": true, "2h": true, "1d": true}
	if !validFreqs[frequency] {
		frequency = "1h"
	}

	cron := &models.CronJob{
		AgentID:   agentID,
		Task:      task,
		Frequency: frequency,
	}
	if err := u.db.CreateCronJob(cron); err != nil {
		http.Redirect(w, r, "/crons?error=Failed+to+create+cron", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/crons", http.StatusSeeOther)
}

func (u *UI) CronAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	action := r.FormValue("action")

	cron, err := u.db.GetCronJob(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch action {
	case "toggle":
		cron.Active = !cron.Active
		u.db.UpdateCronJob(cron)
	case "update":
		if agentID := r.FormValue("agent_id"); agentID != "" {
			cron.AgentID = agentID
		}
		if task := r.FormValue("task"); task != "" {
			cron.Task = task
		}
		if freq := r.FormValue("frequency"); freq != "" {
			cron.Frequency = freq
		}
		u.db.UpdateCronJob(cron)
	case "delete":
		u.db.DeleteCronJob(id)
	}

	http.Redirect(w, r, "/crons", http.StatusSeeOther)
}

func (u *UI) StrategyPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		u.createApexBlockUI(w, r)
		return
	}

	blocks, _ := u.db.ListApexBlocks()
	workBlocks, _ := u.db.ListWorkBlocks()

	// Calculate alignment score
	totalWorkBlocks := len(workBlocks)
	alignedWorkBlocks := 0
	apexWorkBlocks := make(map[string][]models.WorkBlock)

	for _, wb := range workBlocks {
		if wb.ApexBlockID != nil && *wb.ApexBlockID != "" {
			alignedWorkBlocks++
			apexWorkBlocks[*wb.ApexBlockID] = append(apexWorkBlocks[*wb.ApexBlockID], wb)
		}
	}

	alignmentScore := 0
	if totalWorkBlocks > 0 {
		alignmentScore = (alignedWorkBlocks * 100) / totalWorkBlocks
	}

	u.render(w, "strategy", map[string]any{
		"ApexBlocks":     blocks,
		"WorkBlocks":     workBlocks,
		"ApexWorkBlocks": apexWorkBlocks,
		"AlignmentScore": alignmentScore,
	})
}

func (u *UI) createApexBlockUI(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	title := r.FormValue("title")
	goal := r.FormValue("goal")
	if title == "" || goal == "" {
		http.Redirect(w, r, "/strategy?error=Title+and+Goal+are+required", http.StatusSeeOther)
		return
	}

	ab := &models.ApexBlock{
		ID:     uuid.New().String(),
		Title:  title,
		Goal:   goal,
		Status: "active",
	}

	if err := u.db.CreateApexBlock(ab); err != nil {
		http.Redirect(w, r, "/strategy?error=Failed+to+create+apex+block", http.StatusSeeOther)
		return
	}

	LogActivityAndBroadcast(u.db, u.sse, u.tmpl, "create", "apex_block", ab.ID, nil, title)

	http.Redirect(w, r, "/strategy", http.StatusSeeOther)
}

func (u *UI) UpdateApexBlockUI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	action := r.FormValue("action")

	ab, err := u.db.GetApexBlock(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch action {
	case "toggle_status":
		if ab.Status == "active" {
			ab.Status = "archived"
		} else {
			ab.Status = "active"
		}
		u.db.UpdateApexBlock(ab)
	case "update":
		if t := r.FormValue("title"); t != "" {
			ab.Title = t
		}
		if g := r.FormValue("goal"); g != "" {
			ab.Goal = g
		}
		u.db.UpdateApexBlock(ab)
	}

	http.Redirect(w, r, "/strategy", http.StatusSeeOther)
}

func (u *UI) NotFound(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.WriteHeader(http.StatusNotFound)
		u.render(w, "not_found", map[string]any{
			"Title":   "Page not found",
			"Message": "The page you're looking for doesn't exist or has been moved.",
			"BackURL": "/dashboard",
		})
		return
	}
	http.NotFound(w, r)
}

func (u *UI) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := u.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func formatStreamJSON(stdout, runStatus string) string {
	if stdout == "" {
		if runStatus == models.RunStatusRunning {
			return `<div class="flex items-center gap-2 text-zinc-400"><span class="animate-pulse">●</span> Agent is running...</div>`
		}
		return `<div class="text-zinc-500">No output yet.</div>`
	}

	var b strings.Builder
	b.WriteString(`<div class="space-y-2 font-mono text-sm" id="stdout-content">`)

	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") {
			b.WriteString(fmt.Sprintf(`<div class="text-zinc-300 whitespace-pre-wrap">%s</div>`, template.HTMLEscapeString(line)))
			continue
		}

		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			b.WriteString(fmt.Sprintf(`<div class="text-zinc-300 whitespace-pre-wrap">%s</div>`, template.HTMLEscapeString(line)))
			continue
		}

		msgType, _ := msg["type"].(string)
		switch msgType {
		case "assistant":
			if content, ok := msg["message"].(map[string]any); ok {
				if blocks, ok := content["content"].([]any); ok {
					for _, block := range blocks {
						if bm, ok := block.(map[string]any); ok {
							if bm["type"] == "text" {
								text, _ := bm["text"].(string)
								b.WriteString(fmt.Sprintf(`<div class="border-l-2 border-indigo-500 pl-3 py-1 text-zinc-200 whitespace-pre-wrap">%s</div>`, template.HTMLEscapeString(text)))
							} else if bm["type"] == "tool_use" {
								name, _ := bm["name"].(string)
								input, _ := json.MarshalIndent(bm["input"], "", "  ")
								b.WriteString(fmt.Sprintf(`<details class="border border-zinc-700 rounded p-2"><summary class="cursor-pointer text-blue-400 font-medium">Tool: %s</summary><pre class="mt-2 text-xs text-zinc-400 overflow-x-auto">%s</pre></details>`, template.HTMLEscapeString(name), template.HTMLEscapeString(string(input))))
							}
						}
					}
				}
			}
		case "tool_result":
			// skip verbose tool results in formatted view
		case "result":
			if result, ok := msg["result"].(map[string]any); ok {
				cost, _ := result["total_cost_usd"].(float64)
				inputT, _ := result["input_tokens"].(float64)
				outputT, _ := result["output_tokens"].(float64)
				dur, _ := result["duration_ms"].(float64)
				b.WriteString(fmt.Sprintf(`<div class="mt-4 pt-3 border-t border-zinc-700 text-xs text-zinc-500 flex gap-4"><span>Cost: $%.4f</span><span>In: %.0f</span><span>Out: %.0f</span><span>Duration: %.1fs</span></div>`, cost, inputT, outputT, dur/1000))
			}
		}
	}

	if runStatus == models.RunStatusRunning {
		b.WriteString(`<div class="flex items-center gap-2 text-emerald-400 mt-2"><span class="animate-pulse">●</span> Running...</div>`)
	}

	b.WriteString(`</div>`)
	return b.String()
}
