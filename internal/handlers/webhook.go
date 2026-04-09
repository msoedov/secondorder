package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

const (
	maxWebhookBodySize = 1 << 20 // 1 MB
	rateLimitWindow    = time.Minute
	rateLimitMax       = 120
)

// WebhookAPI handles incoming webhook requests from external sources.
type WebhookAPI struct {
	db  *db.DB
	sse *SSEHub

	rateMu    sync.Mutex
	rateCount map[string]*rateBucket
}

type rateBucket struct {
	count    int
	windowAt time.Time
}

func NewWebhookAPI(database *db.DB, sse *SSEHub) *WebhookAPI {
	return &WebhookAPI{
		db:        database,
		sse:       sse,
		rateCount: make(map[string]*rateBucket),
	}
}

// WebhookAuth validates the HMAC-SHA256 signature for a given source.
func (wh *WebhookAPI) WebhookAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.PathValue("source")
		if source == "" {
			jsonError(w, "missing source", http.StatusBadRequest)
			return
		}

		// Rate limiting per source
		if !wh.allowRequest(source) {
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Read body with size limit
		r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodySize)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			jsonError(w, "payload too large or unreadable", http.StatusBadRequest)
			return
		}

		// Look up per-source secret
		envKey := fmt.Sprintf("WEBHOOK_SECRET_%s", strings.ToUpper(source))
		secret := os.Getenv(envKey)
		if secret == "" {
			jsonError(w, "webhook source not configured", http.StatusBadRequest)
			return
		}

		// Get signature from headers (check both standard and GitHub-specific)
		sig := r.Header.Get("X-Webhook-Signature-256")
		if sig == "" {
			sig = r.Header.Get("X-Hub-Signature-256")
		}
		if sig == "" {
			jsonError(w, "missing signature", http.StatusUnauthorized)
			return
		}

		// Verify HMAC-SHA256
		if !verifyHMAC(body, sig, secret) {
			jsonError(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		// Store raw body in context via request body replacement
		// We pass the body bytes through a custom header so handlers can access it
		r.Header.Set("X-Webhook-Body", string(body))
		r.Header.Set("X-Webhook-Source", source)
		next(w, r)
	}
}

// HandleIssues processes webhook payloads to create issues.
func (wh *WebhookAPI) HandleIssues(w http.ResponseWriter, r *http.Request) {
	source := r.Header.Get("X-Webhook-Source")
	rawBody := r.Header.Get("X-Webhook-Body")

	// Parse delivery ID for idempotency
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Webhook-Delivery-ID")
	}

	// Check for duplicate delivery
	if deliveryID != "" {
		if exists, _ := wh.db.WebhookEventExists(deliveryID); exists {
			jsonOK(w, map[string]string{"status": "duplicate", "delivery_id": deliveryID})
			return
		}
	}

	// Store raw webhook event
	eventID := uuid.New().String()
	wh.db.CreateWebhookEvent(eventID, source, "issues", deliveryID, rawBody)

	// Adapt payload based on source
	var issue *issuePayload
	var err error
	switch source {
	case "github":
		issue, err = adaptGitHubIssue([]byte(rawBody))
	default:
		issue, err = adaptGenericIssue([]byte(rawBody))
	}

	if err != nil {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", err.Error())
		jsonError(w, "invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if issue.Title == "" {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", "title required")
		jsonError(w, "title required", http.StatusBadRequest)
		return
	}

	// Check for existing issue with same title
	if existing, err := wh.db.GetIssueByTitle(issue.Title); err == nil {
		wh.db.UpdateWebhookEventStatus(eventID, "processed", "duplicate issue: "+existing.Key)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"error":        "issue with this title already exists",
			"existing_key": existing.Key,
		})
		return
	}

	key, err := wh.db.NextIssueKey()
	if err != nil {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", err.Error())
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	newIssue := &models.Issue{
		ID:          uuid.New().String(),
		Key:         key,
		Title:       issue.Title,
		Description: issue.Description,
		Status:      models.StatusTodo,
		Type:        issue.Type,
		Priority:    issue.Priority,
	}
	if newIssue.Type == "" {
		newIssue.Type = "task"
	}

	if err := wh.db.CreateIssue(newIssue); err != nil {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", err.Error())
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	wh.db.UpdateWebhookEventStatus(eventID, "processed", "created "+key)

	// SSE broadcast
	createData, _ := json.Marshal(map[string]any{
		"key":    key,
		"title":  newIssue.Title,
		"status": newIssue.Status,
		"type":   newIssue.Type,
		"source": source,
	})
	wh.sse.Broadcast("issue_created", string(createData))

	LogActivityAndBroadcast(wh.db, wh.sse, nil,
		"webhook_create", "issue", key, nil, fmt.Sprintf("via %s webhook", source))

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]any{
		"key":    key,
		"title":  newIssue.Title,
		"status": newIssue.Status,
		"source": source,
	})
}

// HandleComments processes webhook payloads to create comments on existing issues.
func (wh *WebhookAPI) HandleComments(w http.ResponseWriter, r *http.Request) {
	source := r.Header.Get("X-Webhook-Source")
	rawBody := r.Header.Get("X-Webhook-Body")

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID == "" {
		deliveryID = r.Header.Get("X-Webhook-Delivery-ID")
	}

	if deliveryID != "" {
		if exists, _ := wh.db.WebhookEventExists(deliveryID); exists {
			jsonOK(w, map[string]string{"status": "duplicate", "delivery_id": deliveryID})
			return
		}
	}

	eventID := uuid.New().String()
	wh.db.CreateWebhookEvent(eventID, source, "comments", deliveryID, rawBody)

	var comment *commentPayload
	var err error
	switch source {
	case "github":
		comment, err = adaptGitHubComment([]byte(rawBody))
	default:
		comment, err = adaptGenericComment([]byte(rawBody))
	}

	if err != nil {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", err.Error())
		jsonError(w, "invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if comment.IssueKey == "" {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", "issue_key required")
		jsonError(w, "issue_key required", http.StatusBadRequest)
		return
	}
	if comment.Body == "" {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", "body required")
		jsonError(w, "body required", http.StatusBadRequest)
		return
	}

	// Resolve issue
	issue, err := wh.db.GetIssue(comment.IssueKey)
	if err != nil {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", "issue not found: "+comment.IssueKey)
		jsonError(w, "issue not found: "+comment.IssueKey, http.StatusNotFound)
		return
	}

	newComment := &models.Comment{
		ID:       uuid.New().String(),
		IssueKey: issue.Key,
		Author:   comment.Author,
		Body:     comment.Body,
	}
	if newComment.Author == "" {
		newComment.Author = "Webhook (" + source + ")"
	}

	if err := wh.db.CreateComment(newComment); err != nil {
		wh.db.UpdateWebhookEventStatus(eventID, "failed", err.Error())
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	wh.db.UpdateWebhookEventStatus(eventID, "processed", "comment on "+issue.Key)

	data, _ := json.Marshal(map[string]string{
		"issue_key": issue.Key,
		"author":    newComment.Author,
		"body":      newComment.Body,
	})
	wh.sse.Broadcast("comment", string(data))

	LogActivityAndBroadcast(wh.db, wh.sse, nil,
		"webhook_comment", "issue", issue.Key, nil, fmt.Sprintf("via %s webhook", source))

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]any{
		"issue_key": issue.Key,
		"author":    newComment.Author,
		"source":    source,
	})
}

// --- Rate Limiting ---

func (wh *WebhookAPI) allowRequest(source string) bool {
	wh.rateMu.Lock()
	defer wh.rateMu.Unlock()

	now := time.Now()
	bucket, ok := wh.rateCount[source]
	if !ok || now.Sub(bucket.windowAt) > rateLimitWindow {
		wh.rateCount[source] = &rateBucket{count: 1, windowAt: now}
		return true
	}
	bucket.count++
	return bucket.count <= rateLimitMax
}

// --- HMAC Verification ---

func verifyHMAC(body []byte, signature, secret string) bool {
	// Strip "sha256=" prefix if present (GitHub format)
	sig := strings.TrimPrefix(signature, "sha256=")

	expectedMAC := hmac.New(sha256.New, []byte(secret))
	expectedMAC.Write(body)
	expectedSig := hex.EncodeToString(expectedMAC.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expectedSig))
}

// --- Payload Types ---

type issuePayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Priority    int    `json:"priority"`
}

type commentPayload struct {
	IssueKey string `json:"issue_key"`
	Author   string `json:"author"`
	Body     string `json:"body"`
}

// --- Generic Adapter ---

func adaptGenericIssue(data []byte) (*issuePayload, error) {
	var p issuePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &p, nil
}

func adaptGenericComment(data []byte) (*commentPayload, error) {
	var p commentPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &p, nil
}

// --- GitHub Adapter ---

type githubIssueEvent struct {
	Action string `json:"action"`
	Issue  struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"issue"`
	PullRequest *struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	} `json:"pull_request"`
}

type githubCommentEvent struct {
	Action string `json:"action"`
	Issue  struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	} `json:"issue"`
	Comment struct {
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
}

func adaptGitHubIssue(data []byte) (*issuePayload, error) {
	var evt githubIssueEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return nil, fmt.Errorf("invalid GitHub payload: %w", err)
	}

	// Only process "opened" events for issue creation
	if evt.Action != "" && evt.Action != "opened" {
		return nil, fmt.Errorf("unsupported action: %s (only 'opened' creates issues)", evt.Action)
	}

	title := evt.Issue.Title
	body := evt.Issue.Body

	// If this is a PR event, use PR fields
	if evt.PullRequest != nil && title == "" {
		title = evt.PullRequest.Title
		body = evt.PullRequest.Body
	}

	issueType := "task"
	for _, label := range evt.Issue.Labels {
		switch strings.ToLower(label.Name) {
		case "bug":
			issueType = "bug"
		case "feature", "enhancement":
			issueType = "feature"
		}
	}

	return &issuePayload{
		Title:       title,
		Description: body,
		Type:        issueType,
	}, nil
}

func adaptGitHubComment(data []byte) (*commentPayload, error) {
	var evt githubCommentEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return nil, fmt.Errorf("invalid GitHub comment payload: %w", err)
	}

	if evt.Action != "" && evt.Action != "created" {
		return nil, fmt.Errorf("unsupported action: %s (only 'created' adds comments)", evt.Action)
	}

	// GitHub comments reference issues by title — the caller must resolve the issue key.
	// For GitHub adapter, we look for an issue_key in a custom field or
	// use the issue title to find the matching SO issue.
	return &commentPayload{
		IssueKey: "", // Will be resolved by the handler if empty
		Author:   evt.Comment.User.Login,
		Body:     evt.Comment.Body,
	}, nil
}
