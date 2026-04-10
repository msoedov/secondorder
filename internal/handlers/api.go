package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/msoedov/secondorder/internal/archetypes"
	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
	acvalidator "github.com/msoedov/secondorder/internal/validator"
	"html/template"
)

type TelegramNotifier interface {
	SendWorkBlockApproval(blockID, title, goal, transition string) error
	SendMessage(text string) error
}

type DiscordNotifier interface {
	SendWorkBlockApproval(blockID, title, goal, transition string) error
	SendMessage(text string) error
}

type API struct {
	db       *db.DB
	sse      *SSEHub
	tmpl     *template.Template
	wake     func(agent *models.Agent, issue *models.Issue)
	telegram TelegramNotifier
	discord  DiscordNotifier
}

func NewAPI(database *db.DB, sse *SSEHub, tmpl *template.Template, wake func(*models.Agent, *models.Issue), tg TelegramNotifier, dc DiscordNotifier) *API {
	return &API{db: database, sse: sse, tmpl: tmpl, wake: wake, telegram: tg, discord: dc}
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

	issue, err := a.db.GetIssue(key)
	if err != nil {
		jsonError(w, "issue not found", http.StatusNotFound)
		return
	}

	if agent.ArchetypeSlug != "ceo" && issue.AssigneeAgentID != nil && *issue.AssigneeAgentID != agent.ID {
		jsonError(w, "forbidden: issue assigned to another agent", http.StatusForbidden)
		return
	}

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

	// SSE broadcast
	checkoutData, _ := json.Marshal(map[string]string{
		"key":      key,
		"status":   "in_progress",
		"assignee": agent.Name,
	})
	a.sse.Broadcast("issue_updated", string(checkoutData))

	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"checkout", "issue", key, &agent.ID, "")
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
		Status             string               `json:"status"`
		Type               string               `json:"type"`
		Comment            string               `json:"comment"`
		Title              string               `json:"title"`
		Description        string               `json:"description"`
		Priority           *int                 `json:"priority"`
		AssigneeSlug       *string              `json:"assignee_slug"`
		Stages             *[]models.IssueStage `json:"stages"`
		CurrentStageID     *int                 `json:"current_stage_id"`
		CancellationReason string               `json:"cancellation_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	agent := agentFromContext(r.Context())

	// Ownership check: only the assignee (or CEO) may update an issue
	if agent.ArchetypeSlug != "ceo" && (issue.AssigneeAgentID == nil || *issue.AssigneeAgentID != agent.ID) {
		jsonError(w, "forbidden: issue not assigned to you", http.StatusForbidden)
		return
	}

	// Pre-cancellation guard: if setting status to cancelled, check for completion comments
	if body.Status == models.StatusCancelled && body.CancellationReason == "" && issue.AssigneeAgentID != nil {
		found, excerpt, err := a.db.HasCompletionComment(key, *issue.AssigneeAgentID)
		if err == nil && found {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error":                      "Issue has completion comment from assignee. Provide cancellation_reason to proceed.",
				"completion_comment_excerpt": excerpt,
			})
			return
		}
	}

	oldAssigneeID := issue.AssigneeAgentID

	if body.Status != "" {
		issue.Status = body.Status
	}
	if body.Type != "" {
		issue.Type = body.Type
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
	if body.Stages != nil {
		issue.Stages = *body.Stages
	}
	if body.CurrentStageID != nil {
		issue.CurrentStageID = *body.CurrentStageID
	}
	if body.AssigneeSlug != nil {
		if *body.AssigneeSlug == "" {
			issue.AssigneeAgentID = nil
		} else {
			newAssignee, err := a.db.GetAgentBySlug(*body.AssigneeSlug)
			if err != nil {
				jsonError(w, "assignee not found", http.StatusNotFound)
				return
			}
			issue.AssigneeAgentID = &newAssignee.ID
		}
	}
	if err := acvalidator.ValidateStages(issue.Stages, issue.CurrentStageID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.db.UpdateIssue(issue); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Re-fetch to get joined fields (AssigneeName, AssigneeSlug)
	if updated, err := a.db.GetIssue(key); err == nil {
		issue = updated
	}

	// Validate AC and add warnings to response
	issue.Warnings = acvalidator.ValidateAC(issue.Type, issue.Description)

	// SSE broadcast
	updateData, _ := json.Marshal(map[string]any{
		"key":              key,
		"status":           issue.Status,
		"type":             issue.Type,
		"title":            issue.Title,
		"assignee_slug":    issue.AssigneeSlug,
		"stages":           issue.Stages,
		"current_stage_id": issue.CurrentStageID,
		"warnings":         issue.Warnings,
	})
	a.sse.Broadcast("issue_updated", string(updateData))

	// Add comment if provided
	if body.Comment != "" {
		agentName := "Board"
		if agent != nil {
			agentName = agent.Name
		}
		comment := &models.Comment{
			ID:       uuid.New().String(),
			IssueKey: key,
			AgentID:  ptrStr(agent),
			Author:   agentName,
			Body:     body.Comment,
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

	details := body.Status
	if body.AssigneeSlug != nil {
		msg := "reassigned to " + *body.AssigneeSlug
		if *body.AssigneeSlug == "" {
			msg = "unassigned"
		}
		if details != "" {
			details += ", " + msg
		} else {
			details = msg
		}
	}
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"update", "issue", key, ptrStr(agent), details)

	// Record cancellation_reason as a system comment
	if body.Status == models.StatusCancelled && body.CancellationReason != "" {
		sysComment := &models.Comment{
			ID:       uuid.New().String(),
			IssueKey: key,
			Author:   "System",
			Body:     fmt.Sprintf("Cancellation reason: %s", body.CancellationReason),
		}
		a.db.CreateComment(sysComment)
		sysData, _ := json.Marshal(map[string]string{
			"issue_key": key,
			"author":    "System",
			"body":      sysComment.Body,
		})
		a.sse.Broadcast("comment", string(sysData))
	}

	// Wake chain on status change
	if body.Status == models.StatusDone || body.Status == models.StatusBlocked || body.Status == models.StatusInReview {
		if agent != nil && a.wake != nil {
			go a.wakeReviewerForIssue(agent.ID, key)
		}
	}

	// Wake assignee when reviewer sends work back to in_progress OR when re-assigned in_progress issue
	isReassignedInProgress := body.AssigneeSlug != nil && issue.Status == models.StatusInProgress &&
		(oldAssigneeID == nil || (issue.AssigneeAgentID != nil && *oldAssigneeID != *issue.AssigneeAgentID))

	if (body.Status == models.StatusInProgress || isReassignedInProgress) && agent != nil && issue.AssigneeAgentID != nil && *issue.AssigneeAgentID != agent.ID {
		// Check retry limit
		runCount, _ := a.db.CountRunsForIssue(key)
		// Bypass retry limit if reassigned OR if updated by CEO
		isCEO := agent.ArchetypeSlug == "ceo"
		if runCount > 6 && !isReassignedInProgress && !isCEO {
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
			if a.discord != nil {
				go a.discord.SendMessage(fmt.Sprintf("Issue %s stuck after %d runs. Needs human intervention.", key, runCount))
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
			if a.discord != nil {
				if wb, err := a.db.GetWorkBlock(*issue.WorkBlockID); err == nil {
					go a.discord.SendWorkBlockApproval(wb.ID, wb.Title, wb.Goal, "ready_to_ship")
				}
			}
		}
	}

	jsonOK(w, issue)
}

func (a *API) DeleteIssue(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, err := a.db.GetIssue(key); err != nil {
		jsonError(w, "issue not found", http.StatusNotFound)
		return
	}
	if err := a.db.DeleteIssue(key); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// SSE broadcast
	deleteData, _ := json.Marshal(map[string]string{
		"key": key,
	})
	a.sse.Broadcast("issue_deleted", string(deleteData))

	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"delete", "issue", key, nil, "")
	jsonOK(w, map[string]string{"deleted": key})
}

func (a *API) CreateComment(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	agent := agentFromContext(r.Context())

	issue, err := a.db.GetIssue(key)
	if err != nil {
		jsonError(w, "issue not found", http.StatusNotFound)
		return
	}

	if agent.ArchetypeSlug != "ceo" && (issue.AssigneeAgentID == nil || *issue.AssigneeAgentID != agent.ID) {
		jsonError(w, "forbidden: issue not assigned to you", http.StatusForbidden)
		return
	}

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

	// Stage parsing logic
	stagesRegex := regexp.MustCompile(`Stages: (\[.*\](?:, \s*\[.*\])*)`)
	completeRegex := regexp.MustCompile(`Stage (\d+): .* - Complete`)
	updated := false

	if matches := stagesRegex.FindStringSubmatch(body.Body); len(matches) > 1 {
		rawStages := matches[1]
		stageTitles := regexp.MustCompile(`\[([^\]]+)\]`).FindAllStringSubmatch(rawStages, -1)

		var newStages []models.IssueStage
		for i, titleMatch := range stageTitles {
			newStages = append(newStages, models.IssueStage{
				ID:     i + 1,
				Title:  titleMatch[1],
				Status: "todo",
			})
		}
		if len(newStages) > 0 {
			issue.Stages = newStages
			issue.CurrentStageID = 1
			updated = true
		}
	}

	if matches := completeRegex.FindStringSubmatch(body.Body); len(matches) > 1 {
		stageID, _ := strconv.Atoi(matches[1])
		if stageID > 0 && stageID <= len(issue.Stages) {
			// Mark current and all previous stages as done
			for j := 0; j < stageID; j++ {
				issue.Stages[j].Status = "done"
			}

			if stageID < len(issue.Stages) {
				issue.CurrentStageID = stageID + 1
			} else {
				// last stage completed
				issue.CurrentStageID = stageID
			}
			updated = true

			// System acknowledgment comment
			progress := int(float64(stageID) / float64(len(issue.Stages)) * 100)
			ackBody := fmt.Sprintf("Stage %d acknowledged. Progress: %d%%", stageID, progress)
			ackComment := &models.Comment{
				ID:       uuid.New().String(),
				IssueKey: key,
				Author:   "System",
				Body:     ackBody,
			}
			a.db.CreateComment(ackComment)

			ackData, _ := json.Marshal(map[string]string{
				"issue_key": key,
				"author":    "System",
				"body":      ackBody,
			})
			a.sse.Broadcast("comment", string(ackData))
		}
	}

	if updated {
		if err := acvalidator.ValidateStages(issue.Stages, issue.CurrentStageID); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.db.UpdateIssue(issue)
		// Broadcast issue_updated
		updateData, _ := json.Marshal(map[string]any{
			"key":              key,
			"status":           issue.Status,
			"stages":           issue.Stages,
			"current_stage_id": issue.CurrentStageID,
		})
		a.sse.Broadcast("issue_updated", string(updateData))
	}

	jsonOK(w, comment)
}

func (a *API) ListWikiPages(w http.ResponseWriter, r *http.Request) {
	pages, err := a.db.ListWikiPages()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, pages)
}

func (a *API) CreateWikiPage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Slug = strings.TrimSpace(body.Slug)
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		jsonError(w, "title required", http.StatusBadRequest)
		return
	}

	// Auto-generate slug from title if not provided
	if body.Slug == "" {
		body.Slug = strings.ToLower(strings.ReplaceAll(body.Title, " ", "-"))
	}

	agent := agentFromContext(r.Context())
	page := &models.WikiPage{
		Slug:             body.Slug,
		Title:            body.Title,
		Content:          body.Content,
		CreatedByAgentID: ptrStr(agent),
		UpdatedByAgentID: ptrStr(agent),
	}
	if err := a.db.CreateWikiPage(page); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			jsonError(w, "wiki page with this slug already exists", http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, page)
}

func (a *API) GetWikiPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	page, err := a.db.GetWikiPageBySlug(slug)
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, "wiki page not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, page)
}

func (a *API) UpdateWikiPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	page, err := a.db.GetWikiPageBySlug(slug)
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, "wiki page not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var body struct {
		Title   *string `json:"title"`
		Content *string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	title := page.Title
	content := page.Content
	if body.Title != nil {
		title = strings.TrimSpace(*body.Title)
		if title == "" {
			jsonError(w, "title cannot be empty", http.StatusBadRequest)
			return
		}
	}
	if body.Content != nil {
		content = *body.Content
	}

	agent := agentFromContext(r.Context())
	page.Title = title
	page.Content = content
	page.UpdatedByAgentID = ptrStr(agent)
	if err := a.db.UpdateWikiPage(page); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	updated, err := a.db.GetWikiPageBySlug(slug)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, updated)
}

func (a *API) DeleteWikiPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	page, err := a.db.GetWikiPageBySlug(slug)
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, "wiki page not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.db.DeleteWikiPage(page.ID); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"deleted": slug})
}

func (a *API) CreateIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title          string `json:"title"`
		Description    string `json:"description"`
		Type           string `json:"type"`
		AssigneeSlug   string `json:"assignee_slug"`
		ParentIssueKey string `json:"parent_issue_key"`
		Priority       int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		jsonError(w, "title required", http.StatusBadRequest)
		return
	}

	if body.Type == "" {
		body.Type = "task"
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
		Type:        body.Type,
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

	// Validate AC and add warnings to response
	issue.Warnings = acvalidator.ValidateAC(issue.Type, issue.Description)

	// SSE broadcast
	createData, _ := json.Marshal(map[string]any{
		"key":              key,
		"title":            issue.Title,
		"status":           issue.Status,
		"type":             issue.Type,
		"warnings":         issue.Warnings,
		"stages":           issue.Stages,
		"current_stage_id": issue.CurrentStageID,
	})
	a.sse.Broadcast("issue_created", string(createData))

	agent := agentFromContext(r.Context())
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"create", "issue", key, ptrStr(agent), body.Title)

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
		Runner        string `json:"runner"`
	}
	result := make([]slim, len(agents))
	for i, ag := range agents {
		result[i] = slim{ag.ID, ag.Slug, ag.Name, ag.ArchetypeSlug, ag.Runner}
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

func ptrStrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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

// --- Apex Blocks API ---

func (a *API) ListApexBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := a.db.ListApexBlocks()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, blocks)
}

func (a *API) GetApexBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ab, err := a.db.GetApexBlock(id)
	if err != nil {
		jsonError(w, "apex block not found", http.StatusNotFound)
		return
	}
	jsonOK(w, ab)
}

func (a *API) CreateApexBlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Goal  string `json:"goal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		jsonError(w, "title required", http.StatusBadRequest)
		return
	}

	ab := &models.ApexBlock{
		ID:    uuid.New().String(),
		Title: body.Title,
		Goal:  body.Goal,
	}
	if err := a.db.CreateApexBlock(ab); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	agent := agentFromContext(r.Context())
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl, "create", "apex_block", ab.ID, ptrStr(agent), body.Title)

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, ab)
}

func (a *API) UpdateApexBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ab, err := a.db.GetApexBlock(id)
	if err != nil {
		jsonError(w, "apex block not found", http.StatusNotFound)
		return
	}

	var body struct {
		Title  string `json:"title"`
		Goal   string `json:"goal"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	if body.Title != "" {
		ab.Title = body.Title
	}
	if body.Goal != "" {
		ab.Goal = body.Goal
	}
	if body.Status != "" {
		ab.Status = body.Status
	}

	if err := a.db.UpdateApexBlock(ab); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	agent := agentFromContext(r.Context())
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl, "update", "apex_block", id, ptrStr(agent), ab.Title)

	jsonOK(w, ab)
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
		Title              string  `json:"title"`
		Goal               string  `json:"goal"`
		AcceptanceCriteria string  `json:"acceptance_criteria"`
		NorthStarMetric    string  `json:"north_star_metric"`
		NorthStarTarget    string  `json:"north_star_target"`
		ApexBlockID        *string `json:"apex_block_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		jsonError(w, "title required", http.StatusBadRequest)
		return
	}

	wb := &models.WorkBlock{
		Title:              body.Title,
		Goal:               body.Goal,
		AcceptanceCriteria: body.AcceptanceCriteria,
		Status:             models.WBStatusProposed,
		NorthStarMetric:    body.NorthStarMetric,
		NorthStarTarget:    body.NorthStarTarget,
		ApexBlockID:        body.ApexBlockID,
	}
	if err := a.db.CreateWorkBlock(wb); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}

	agent := agentFromContext(r.Context())
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"create", "work_block", wb.ID, ptrStr(agent), body.Title)

	if a.telegram != nil {
		go a.telegram.SendWorkBlockApproval(wb.ID, wb.Title, wb.Goal, "proposed")
	}
	if a.discord != nil {
		go a.discord.SendWorkBlockApproval(wb.ID, wb.Title, wb.Goal, "proposed")
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, wb)
}

func (a *API) UpdateWorkBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wb, err := a.db.GetWorkBlock(id)
	if err != nil {
		jsonError(w, "work block not found", http.StatusNotFound)
		return
	}

	var body struct {
		Title              string  `json:"title"`
		Goal               string  `json:"goal"`
		AcceptanceCriteria string  `json:"acceptance_criteria"`
		Status             string  `json:"status"`
		NorthStarMetric    string  `json:"north_star_metric"`
		NorthStarTarget    string  `json:"north_star_target"`
		ApexBlockID        *string `json:"apex_block_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	if body.Status != "" && body.Status != wb.Status {
		if err := a.db.UpdateWorkBlockStatus(id, body.Status); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		wb.Status = body.Status
	}

	if body.Title != "" {
		wb.Title = body.Title
	}
	if body.Goal != "" {
		wb.Goal = body.Goal
	}
	if body.AcceptanceCriteria != "" {
		wb.AcceptanceCriteria = body.AcceptanceCriteria
	}
	if body.NorthStarMetric != "" {
		wb.NorthStarMetric = body.NorthStarMetric
	}
	if body.NorthStarTarget != "" {
		wb.NorthStarTarget = body.NorthStarTarget
	}
	if body.ApexBlockID != nil {
		wb.ApexBlockID = body.ApexBlockID
	}

	if err := a.db.UpdateWorkBlock(wb); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	agent := agentFromContext(r.Context())
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"update", "work_block", id, ptrStr(agent), wb.Status)

	jsonOK(w, wb)
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

	// SSE broadcast
	if issue, err := a.db.GetIssue(body.IssueKey); err == nil {
		updateData, _ := json.Marshal(map[string]string{
			"key":    body.IssueKey,
			"status": issue.Status,
			"title":  issue.Title,
		})
		a.sse.Broadcast("issue_updated", string(updateData))
	}

	agent := agentFromContext(r.Context())
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"assign_to_block", "issue", body.IssueKey, ptrStr(agent), id)

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
	if data, err := os.ReadFile(filepath.Join(archetypes.GetOverridesDir(), body.AgentSlug+".md")); err == nil {
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
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"propose_patch", "archetype", body.AgentSlug, ptrStr(agent), "")

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, patch)
}

func (a *API) UnassignIssueFromBlock(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if err := a.db.UnassignIssueFromWorkBlock(key); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// SSE broadcast
	if issue, err := a.db.GetIssue(key); err == nil {
		updateData, _ := json.Marshal(map[string]string{
			"key":    key,
			"status": issue.Status,
			"title":  issue.Title,
		})
		a.sse.Broadcast("issue_updated", string(updateData))
	}

	agent := agentFromContext(r.Context())
	LogActivityAndBroadcast(a.db, a.sse, a.tmpl,
		"unassign_from_block", "issue", key, ptrStr(agent), "")

	jsonOK(w, map[string]string{"status": "unassigned"})
}
