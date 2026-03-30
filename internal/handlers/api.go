package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

type TelegramNotifier interface {
	SendWorkBlockApproval(blockID, title, goal, transition string) error
	SendMessage(text string) error
}

type API struct {
	db       *db.DB
	sse      *SSEHub
	wake     func(agent *models.Agent, issue *models.Issue)
	telegram TelegramNotifier
}

func NewAPI(database *db.DB, sse *SSEHub, wake func(*models.Agent, *models.Issue), tg TelegramNotifier) *API {
	return &API{db: database, sse: sse, wake: wake, telegram: tg}
}

// Auth middleware extracts agent from API key
func (a *API) Auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			http.Error(w, `{"error":"missing api key"}`, http.StatusUnauthorized)
			return
		}
		hash := sha256.Sum256([]byte(token))
		keyHash := hex.EncodeToString(hash[:])
		agent, err := a.db.GetAgentByAPIKey(keyHash)
		if err != nil {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}
		r = r.WithContext(withAgent(r.Context(), agent))
		next(w, r)
	}
}

func (a *API) Inbox(w http.ResponseWriter, r *http.Request) {
	agent := agentFromContext(r.Context())
	issues, err := a.db.GetAgentInbox(agent.ID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, issues)
}

func (a *API) GetIssue(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	issue, err := a.db.GetIssue(key)
	if err != nil {
		jsonError(w, "issue not found", http.StatusNotFound)
		return
	}
	comments, _ := a.db.ListComments(key)
	children, _ := a.db.GetChildIssues(key)

	jsonOK(w, map[string]any{
		"issue":    issue,
		"comments": comments,
		"children": children,
	})
}

func (a *API) CheckoutIssue(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	agent := agentFromContext(r.Context())

	var body struct {
		AgentID          string   `json:"agentId"`
		ExpectedStatuses []string `json:"expectedStatuses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(body.ExpectedStatuses) == 0 {
		body.ExpectedStatuses = []string{"todo", "backlog"}
	}

	if err := a.db.CheckoutIssue(key, agent.ID, body.ExpectedStatuses); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}

	a.db.LogActivity("checkout", "issue", key, &agent.ID, "")
	jsonOK(w, map[string]string{"status": "checked_out"})
}

func (a *API) UpdateIssue(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	issue, err := a.db.GetIssue(key)
	if err != nil {
		jsonError(w, "issue not found", http.StatusNotFound)
		return
	}

	var body struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
		Title   string `json:"title"`
		Description string `json:"description"`
		Priority *int   `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	agent := agentFromContext(r.Context())

	if body.Status != "" {
		issue.Status = body.Status
	}
	if body.Title != "" {
		issue.Title = body.Title
	}
	if body.Description != "" {
		issue.Description = body.Description
	}
	if body.Priority != nil {
		issue.Priority = *body.Priority
	}

	if err := a.db.UpdateIssue(issue); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add comment if provided
	if body.Comment != "" {
		agentName := "Board"
		if agent != nil {
			agentName = agent.Name
		}
		comment := &models.Comment{
			ID:        uuid.New().String(),
			IssueKey:  key,
			AgentID:   ptrStr(agent),
			Author:    agentName,
			Body:      body.Comment,
		}
		a.db.CreateComment(comment)

		// SSE broadcast
		data, _ := json.Marshal(map[string]string{
			"issue_key": key,
			"author":    agentName,
			"body":      body.Comment,
		})
		a.sse.Broadcast("comment", string(data))
		if a.wake != nil && agent != nil {
			go a.notifyOnComment(key, agentName, body.Comment)
		}
	}

	a.db.LogActivity("update", "issue", key, ptrStr(agent), body.Status)

	// Wake chain on status change
	if body.Status == models.StatusDone || body.Status == models.StatusBlocked || body.Status == models.StatusInReview {
		if agent != nil && a.wake != nil {
			go a.wakeReviewerForIssue(agent.ID, key)
		}
	}

	// Wake assignee when reviewer sends work back to in_progress
	if body.Status == models.StatusInProgress && agent != nil && issue.AssigneeAgentID != nil && *issue.AssigneeAgentID != agent.ID {
		// Check retry limit
		runCount, _ := a.db.CountRunsForIssue(key)
		if runCount > 6 {
			issue.Status = models.StatusBlocked
			a.db.UpdateIssue(issue)
			a.db.CreateComment(&models.Comment{
				ID:       uuid.New().String(),
				IssueKey: key,
				Author:   "System",
				Body:     "Max retry limit reached. Needs human intervention.",
			})
			if a.telegram != nil {
				go a.telegram.SendMessage(fmt.Sprintf("Issue %s stuck after %d runs. Needs human intervention.", key, runCount))
			}
		} else if a.wake != nil {
			if assignee, err := a.db.GetAgent(*issue.AssigneeAgentID); err == nil {
				go a.wake(assignee, issue)
			}
		}
	}

	// Auto-ready check for work blocks
	if (body.Status == models.StatusDone || body.Status == models.StatusCancelled || body.Status == models.StatusWontDo) && issue.WorkBlockID != nil {
		if allDone, _ := a.db.CheckWorkBlockAutoReady(*issue.WorkBlockID); allDone {
			a.db.UpdateWorkBlockStatus(*issue.WorkBlockID, models.WBStatusReady)
			if a.telegram != nil {
				if wb, err := a.db.GetWorkBlock(*issue.WorkBlockID); err == nil {
					go a.telegram.SendWorkBlockApproval(wb.ID, wb.Title, wb.Goal, "ready_to_ship")
				}
			}
		}
	}

	jsonOK(w, issue)
}

func (a *API) CreateComment(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	agent := agentFromContext(r.Context())

	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		jsonError(w, "body required", http.StatusBadRequest)
		return
	}

	agentName := "Board"
	if agent != nil {
		agentName = agent.Name
	}

	comment := &models.Comment{
		ID:       uuid.New().String(),
		IssueKey: key,
		AgentID:  ptrStr(agent),
		Author:   agentName,
		Body:     body.Body,
	}
	if err := a.db.CreateComment(comment); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data, _ := json.Marshal(map[string]string{
		"issue_key": key,
		"author":    agentName,
		"body":      body.Body,
	})
	a.sse.Broadcast("comment", string(data))

	jsonOK(w, comment)
}

func (a *API) CreateIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title          string `json:"title"`
		Description    string `json:"description"`
		AssigneeSlug   string `json:"assignee_slug"`
		ParentIssueKey string `json:"parent_issue_key"`
		Priority       int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		jsonError(w, "title required", http.StatusBadRequest)
		return
	}

	if existing, err := a.db.GetIssueByTitle(body.Title); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"error":        "issue with this title already exists",
			"existing_key": existing.Key,
		})
		return
	}

	key, err := a.db.NextIssueKey()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	issue := &models.Issue{
		ID:          uuid.New().String(),
		Key:         key,
		Title:       body.Title,
		Description: body.Description,
		Status:      models.StatusTodo,
		Priority:    body.Priority,
	}

	if body.ParentIssueKey != "" {
		issue.ParentIssueKey = &body.ParentIssueKey
	}

	// Resolve assignee
	var assignee *models.Agent
	if body.AssigneeSlug != "" {
		assignee, err = a.db.GetAgentBySlug(body.AssigneeSlug)
		if err != nil {
			jsonError(w, "agent not found: "+body.AssigneeSlug, http.StatusBadRequest)
			return
		}
		issue.AssigneeAgentID = &assignee.ID
	} else {
		// Auto-assign to CEO
		ceo, err := a.db.GetCEOAgent()
		if err == nil {
			issue.AssigneeAgentID = &ceo.ID
			assignee = ceo
		}
	}

	if err := a.db.CreateIssue(issue); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	agent := agentFromContext(r.Context())
	a.db.LogActivity("create", "issue", key, ptrStr(agent), body.Title)

	// Wake assigned agent
	if assignee != nil && a.wake != nil {
		go a.wake(assignee, issue)
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, issue)
}

func (a *API) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := a.db.ListAgents()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Return slim view for agent API
	type slim struct {
		ID            string `json:"id"`
		Slug          string `json:"slug"`
		Name          string `json:"name"`
		ArchetypeSlug string `json:"archetype_slug"`
	}
	result := make([]slim, len(agents))
	for i, ag := range agents {
		result[i] = slim{ag.ID, ag.Slug, ag.Name, ag.ArchetypeSlug}
	}
	jsonOK(w, result)
}

func (a *API) AgentMe(w http.ResponseWriter, r *http.Request) {
	agent := agentFromContext(r.Context())
	jsonOK(w, agent)
}

func (a *API) Usage(w http.ResponseWriter, r *http.Request) {
	agent := agentFromContext(r.Context())
	todayTokens, todayCost, totalTokens, totalCost, err := a.db.GetAgentUsage(agent.ID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{
		"today_tokens": todayTokens,
		"today_cost":   todayCost,
		"total_tokens": totalTokens,
		"total_cost":   totalCost,
	})
}

func (a *API) ResolveApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Status != "approved" && body.Status != "rejected" {
		jsonError(w, "status must be approved or rejected", http.StatusBadRequest)
		return
	}
	if err := a.db.ResolveApproval(id, body.Status, body.Comment); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": body.Status})
}

func (a *API) wakeReviewerForIssue(agentID, issueKey string) {
	reviewer, err := a.db.GetReviewer(agentID)
	if err != nil {
		return
	}
	issue, err := a.db.GetIssue(issueKey)
	if err != nil {
		return
	}
	a.wake(reviewer, issue)
}

func (a *API) notifyOnComment(issueKey, author, body string) {
	if a.sse != nil {
		data, _ := json.Marshal(map[string]string{
			"issue_key": issueKey,
			"author":    author,
			"body":      body,
		})
		a.sse.Broadcast("comment", string(data))
	}
}

// helpers

func ptrStr(agent *models.Agent) *string {
	if agent == nil {
		return nil
	}
	return &agent.ID
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// --- Work Blocks API ---

func (a *API) ListWorkBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := a.db.ListWorkBlocks()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, blocks)
}

func (a *API) GetWorkBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wb, err := a.db.GetWorkBlock(id)
	if err != nil {
		jsonError(w, "work block not found", http.StatusNotFound)
		return
	}
	wb.Issues, _ = a.db.ListWorkBlockIssues(id)
	wb.Stats, _ = a.db.GetWorkBlockStats(id)
	jsonOK(w, wb)
}

func (a *API) CreateWorkBlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Goal  string `json:"goal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		jsonError(w, "title required", http.StatusBadRequest)
		return
	}

	wb := &models.WorkBlock{
		Title:  body.Title,
		Goal:   body.Goal,
		Status: models.WBStatusProposed,
	}
	if err := a.db.CreateWorkBlock(wb); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}

	agent := agentFromContext(r.Context())
	a.db.LogActivity("create", "work_block", wb.ID, ptrStr(agent), body.Title)

	if a.telegram != nil {
		go a.telegram.SendWorkBlockApproval(wb.ID, wb.Title, wb.Goal, "proposed")
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, wb)
}

func (a *API) UpdateWorkBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
		jsonError(w, "status required", http.StatusBadRequest)
		return
	}

	if err := a.db.UpdateWorkBlockStatus(id, body.Status); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	agent := agentFromContext(r.Context())
	a.db.LogActivity("update", "work_block", id, ptrStr(agent), body.Status)

	jsonOK(w, map[string]string{"status": body.Status})
}

func (a *API) AssignIssueToBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		IssueKey string `json:"issue_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IssueKey == "" {
		jsonError(w, "issue_key required", http.StatusBadRequest)
		return
	}

	if err := a.db.AssignIssueToWorkBlock(body.IssueKey, id); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	agent := agentFromContext(r.Context())
	a.db.LogActivity("assign_to_block", "issue", body.IssueKey, ptrStr(agent), id)

	jsonOK(w, map[string]string{"status": "assigned"})
}

func (a *API) CreateArchetypePatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentSlug       string `json:"agent_slug"`
		ProposedContent string `json:"proposed_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AgentSlug == "" || body.ProposedContent == "" {
		jsonError(w, "agent_slug and proposed_content required", http.StatusBadRequest)
		return
	}

	auditRunID := r.Header.Get("X-Audit-Run-ID")
	if auditRunID == "" {
		auditRunID = "manual"
	}

	// Snapshot current archetype content
	currentContent := ""
	if data, err := os.ReadFile(filepath.Join("archetypes", body.AgentSlug+".md")); err == nil {
		currentContent = string(data)
	}

	patch := &models.ArchetypePatch{
		AuditRunID:      auditRunID,
		AgentSlug:       body.AgentSlug,
		CurrentContent:  currentContent,
		ProposedContent: body.ProposedContent,
	}
	if err := a.db.CreateArchetypePatch(patch); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	agent := agentFromContext(r.Context())
	a.db.LogActivity("propose_patch", "archetype", body.AgentSlug, ptrStr(agent), "")

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, patch)
}

func (a *API) UnassignIssueFromBlock(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if err := a.db.UnassignIssueFromWorkBlock(key); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	agent := agentFromContext(r.Context())
	a.db.LogActivity("unassign_from_block", "issue", key, ptrStr(agent), "")

	jsonOK(w, map[string]string{"status": "unassigned"})
}
