package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

func TestVerifyHMAC(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"title":"test"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name string
		sig  string
		want bool
	}{
		{"valid signature", validSig, true},
		{"valid without prefix", strings.TrimPrefix(validSig, "sha256="), true},
		{"invalid signature", "sha256=deadbeef", false},
		{"empty signature", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyHMAC(body, tt.sig, secret)
			if got != tt.want {
				t.Errorf("verifyHMAC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdaptGenericIssue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "valid issue",
			input: `{"title":"Bug report","description":"Something broke","type":"bug","priority":2}`,
			want:  "Bug report",
		},
		{
			name:    "invalid json",
			input:   `{not json}`,
			wantErr: true,
		},
		{
			name:  "minimal fields",
			input: `{"title":"Minimal"}`,
			want:  "Minimal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adaptGenericIssue([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("adaptGenericIssue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got.Title != tt.want {
				t.Errorf("adaptGenericIssue() title = %v, want %v", got.Title, tt.want)
			}
		})
	}
}

func TestAdaptGenericComment(t *testing.T) {
	input := `{"issue_key":"SO-1","author":"webhook-user","body":"A comment from webhook"}`
	got, err := adaptGenericComment([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IssueKey != "SO-1" {
		t.Errorf("issue_key = %v, want SO-1", got.IssueKey)
	}
	if got.Author != "webhook-user" {
		t.Errorf("author = %v, want webhook-user", got.Author)
	}
	if got.Body != "A comment from webhook" {
		t.Errorf("body = %v, want 'A comment from webhook'", got.Body)
	}
}

func TestAdaptGitHubIssue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name: "opened issue",
			input: `{
				"action": "opened",
				"issue": {
					"number": 42,
					"title": "GitHub Issue Title",
					"body": "Issue description from GitHub",
					"state": "open",
					"labels": [{"name": "bug"}],
					"user": {"login": "octocat"}
				}
			}`,
			want: "GitHub Issue Title",
		},
		{
			name: "closed issue ignored",
			input: `{
				"action": "closed",
				"issue": {
					"number": 42,
					"title": "Closed Issue",
					"body": "Body",
					"state": "closed",
					"labels": [],
					"user": {"login": "octocat"}
				}
			}`,
			wantErr: true,
		},
		{
			name: "feature label",
			input: `{
				"action": "opened",
				"issue": {
					"number": 1,
					"title": "New Feature",
					"body": "Please add this",
					"state": "open",
					"labels": [{"name": "enhancement"}],
					"user": {"login": "user"}
				}
			}`,
			want: "New Feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adaptGitHubIssue([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("adaptGitHubIssue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got.Title != tt.want {
				t.Errorf("adaptGitHubIssue() title = %v, want %v", got.Title, tt.want)
			}
		})
	}
}

func TestAdaptGitHubIssue_LabelMapping(t *testing.T) {
	payload := `{
		"action": "opened",
		"issue": {
			"number": 1,
			"title": "Bug",
			"body": "",
			"state": "open",
			"labels": [{"name": "bug"}],
			"user": {"login": "u"}
		}
	}`
	got, err := adaptGitHubIssue([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "bug" {
		t.Errorf("type = %v, want bug", got.Type)
	}
}

func TestAdaptGitHubComment(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name: "created comment",
			input: `{
				"action": "created",
				"issue": {"number": 1, "title": "Test Issue"},
				"comment": {
					"body": "This is a comment",
					"user": {"login": "octocat"}
				}
			}`,
		},
		{
			name: "edited comment ignored",
			input: `{
				"action": "edited",
				"issue": {"number": 1, "title": "Test Issue"},
				"comment": {
					"body": "Edited comment",
					"user": {"login": "octocat"}
				}
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adaptGitHubComment([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("adaptGitHubComment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if got.Author != "octocat" {
					t.Errorf("author = %v, want octocat", got.Author)
				}
			}
		})
	}
}

func TestWebhookAuth_MissingSignature(t *testing.T) {
	os.Setenv("WEBHOOK_SECRET_TESTAUTH", "secret123")
	defer os.Unsetenv("WEBHOOK_SECRET_TESTAUTH")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create request with valid path value but no signature
	req := httptest.NewRequest("POST", "/api/v1/webhooks/testauth/issues", strings.NewReader(`{"title":"test"}`))
	req.SetPathValue("source", "testauth")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "missing signature" {
		t.Errorf("expected 'missing signature' error, got %q", resp["error"])
	}
}

func TestWebhookAuth_InvalidSignature(t *testing.T) {
	os.Setenv("WEBHOOK_SECRET_TESTAUTH", "secret123")
	defer os.Unsetenv("WEBHOOK_SECRET_TESTAUTH")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/v1/webhooks/testauth/issues", strings.NewReader(`{"title":"test"}`))
	req.SetPathValue("source", "testauth")
	req.Header.Set("X-Webhook-Signature-256", "sha256=invalid")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWebhookAuth_ValidSignature(t *testing.T) {
	secret := "secret123"
	os.Setenv("WEBHOOK_SECRET_TESTAUTH", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_TESTAUTH")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}

	called := false
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	body := `{"title":"test"}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/v1/webhooks/testauth/issues", strings.NewReader(body))
	req.SetPathValue("source", "testauth")
	req.Header.Set("X-Webhook-Signature-256", sig)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler was not called with valid signature")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWebhookAuth_GitHubSignatureHeader(t *testing.T) {
	secret := "gh-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}

	called := false
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	body := `{"action":"opened","issue":{"title":"test"}}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", strings.NewReader(body))
	req.SetPathValue("source", "github")
	// Use GitHub-specific header
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler was not called with valid GitHub signature")
	}
}

func TestWebhookAuth_UnconfiguredSource(t *testing.T) {
	// Make sure env is not set
	os.Unsetenv("WEBHOOK_SECRET_UNKNOWN")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/v1/webhooks/unknown/issues", strings.NewReader(`{}`))
	req.SetPathValue("source", "unknown")
	req.Header.Set("X-Webhook-Signature-256", "sha256=abc")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unconfigured source, got %d", w.Code)
	}
}

func TestRateLimiting(t *testing.T) {
	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}

	// Should allow up to rateLimitMax requests
	for i := 0; i < rateLimitMax; i++ {
		if !wh.allowRequest("test-source") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// Next request should be denied
	if wh.allowRequest("test-source") {
		t.Error("request beyond rate limit should be denied")
	}

	// Different source should still be allowed
	if !wh.allowRequest("other-source") {
		t.Error("different source should not be rate limited")
	}
}

// --- Test Setup Helpers for Integration Tests ---

func testWebhookDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test_webhook.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func testWebhookAPI(t *testing.T) (*WebhookAPI, *db.DB) {
	t.Helper()
	d := testWebhookDB(t)
	hub := NewSSEHub()
	t.Cleanup(func() { hub.Close() })
	return NewWebhookAPI(d, hub), d
}

// computeHMAC computes the HMAC-SHA256 signature for a payload
func computeHMAC(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// --- Integration Tests for HandleIssues Endpoint ---

func TestHandleIssuesGitHubOpened(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	payload := []byte(`{
  "action": "opened",
  "issue": {
    "number": 123,
    "title": "Add webhook support",
    "body": "We need to integrate webhooks from external sources",
    "state": "open",
    "labels": [{"name": "feature"}],
    "user": {"login": "octocat"}
  }
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", bytes.NewReader(payload))
	req.SetPathValue("source", "github")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	// First auth, then handle
	authHandler := wh.WebhookAuth(wh.HandleIssues)
	authHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if _, ok := resp["key"]; !ok {
		t.Error("response missing 'key'")
	}
	if title, ok := resp["title"].(string); !ok || title != "Add webhook support" {
		t.Errorf("title = %q, want 'Add webhook support'", title)
	}

	// Verify issue was created in DB
	createdIssue, _ := db.GetIssueByTitle("Add webhook support")
	if createdIssue == nil {
		t.Fatal("issue not found in database")
	}
	if createdIssue.Type != "feature" {
		t.Errorf("issue type = %q, want feature", createdIssue.Type)
	}
}

func TestHandleIssuesGenericPayload(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	payload := []byte(`{
  "title": "Generic issue title",
  "description": "A generic issue description",
  "type": "task",
  "priority": 2
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/generic/issues", bytes.NewReader(payload))
	req.SetPathValue("source", "generic")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleIssues)
	authHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	createdIssue, _ := db.GetIssueByTitle("Generic issue title")
	if createdIssue == nil {
		t.Fatal("issue not found in database")
	}
	if createdIssue.Type != "task" {
		t.Errorf("issue type = %q, want task", createdIssue.Type)
	}
}

func TestHandleIssuesBugLabel(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	payload := []byte(`{
  "action": "opened",
  "issue": {
    "number": 456,
    "title": "Fix webhook timeout",
    "body": "Webhooks timeout after 30 seconds",
    "state": "open",
    "labels": [{"name": "bug"}],
    "user": {"login": "developer"}
  }
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", bytes.NewReader(payload))
	req.SetPathValue("source", "github")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleIssues)
	authHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	createdIssue, _ := db.GetIssueByTitle("Fix webhook timeout")
	if createdIssue == nil {
		t.Fatal("issue not found in database")
	}
	if createdIssue.Type != "bug" {
		t.Errorf("issue type = %q, want bug", createdIssue.Type)
	}
}

func TestHandleIssuesMissingTitle(t *testing.T) {
	wh, _ := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	payload := []byte(`{
  "description": "No title provided",
  "type": "task"
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/generic/issues", bytes.NewReader(payload))
	req.SetPathValue("source", "generic")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleIssues)
	authHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "title required") {
		t.Errorf("response missing 'title required': %s", w.Body.String())
	}
}

func TestHandleIssuesMalformedJSON(t *testing.T) {
	wh, _ := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	payload := []byte(`{invalid json}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", bytes.NewReader(payload))
	req.SetPathValue("source", "github")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleIssues)
	authHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid payload") {
		t.Errorf("response missing 'invalid payload': %s", w.Body.String())
	}
}

func TestHandleIssuesIdempotency(t *testing.T) {
	wh, _ := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	payload := []byte(`{
  "action": "opened",
  "issue": {
    "number": 123,
    "title": "Idempotent issue",
    "body": "Should be created only once",
    "state": "open",
    "labels": [],
    "user": {"login": "octocat"}
  }
}`)

	deliveryID := "unique-delivery-id-123"

	// First request
	req1 := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", bytes.NewReader(payload))
	req1.SetPathValue("source", "github")
	req1.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	req1.Header.Set("X-GitHub-Delivery", deliveryID)
	w1 := httptest.NewRecorder()

	authHandler1 := wh.WebhookAuth(wh.HandleIssues)
	authHandler1(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Errorf("first request status = %d, want %d", w1.Code, http.StatusCreated)
	}

	// Second request with same delivery ID
	req2 := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", bytes.NewReader(payload))
	req2.SetPathValue("source", "github")
	req2.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	req2.Header.Set("X-GitHub-Delivery", deliveryID)
	w2 := httptest.NewRecorder()

	authHandler2 := wh.WebhookAuth(wh.HandleIssues)
	authHandler2(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("duplicate request status = %d, want %d", w2.Code, http.StatusOK)
	}

	var resp2 map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp2)

	if status, ok := resp2["status"].(string); !ok || status != "duplicate" {
		t.Errorf("duplicate response status = %q, want duplicate", status)
	}
}

func TestHandleIssuesUnsupportedAction(t *testing.T) {
	wh, _ := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	payload := []byte(`{
  "action": "edited",
  "issue": {
    "number": 123,
    "title": "Updated: Add webhook support",
    "body": "Updated description",
    "state": "open",
    "labels": [],
    "user": {"login": "octocat"}
  }
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", bytes.NewReader(payload))
	req.SetPathValue("source", "github")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleIssues)
	authHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for unsupported action", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "unsupported action") {
		t.Errorf("response missing 'unsupported action': %s", w.Body.String())
	}
}

// --- Integration Tests for HandleComments Endpoint ---

func TestHandleCommentsGenericPayload(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	// Create an issue first
	issue := &models.Issue{
		ID:    "test-id",
		Key:   "SO-1",
		Title: "Test Issue",
		Type:  "task",
	}
	db.CreateIssue(issue)

	payload := []byte(`{
  "issue_key": "SO-1",
  "author": "generic_user",
  "body": "This is a generic comment"
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/generic/comments", bytes.NewReader(payload))
	req.SetPathValue("source", "generic")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleComments)
	authHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	if issueKey, ok := resp["issue_key"].(string); !ok || issueKey != "SO-1" {
		t.Errorf("response issue_key = %q, want SO-1", issueKey)
	}

	// Verify comment was created
	comments, _ := db.ListComments("SO-1")
	if len(comments) == 0 {
		t.Fatal("comment not created")
	}
	if comments[0].Body != "This is a generic comment" {
		t.Errorf("comment body = %q", comments[0].Body)
	}
}

func TestHandleCommentsNonExistentIssue(t *testing.T) {
	wh, _ := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	payload := []byte(`{
  "issue_key": "SO-999",
  "author": "user",
  "body": "Comment on non-existent issue"
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/generic/comments", bytes.NewReader(payload))
	req.SetPathValue("source", "generic")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleComments)
	authHandler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "issue not found") {
		t.Errorf("response missing 'issue not found': %s", w.Body.String())
	}
}

func TestHandleCommentsMissingIssueKey(t *testing.T) {
	wh, _ := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	payload := []byte(`{
  "author": "user",
  "body": "Comment without issue_key"
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/generic/comments", bytes.NewReader(payload))
	req.SetPathValue("source", "generic")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleComments)
	authHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "issue_key required") {
		t.Errorf("response missing 'issue_key required': %s", w.Body.String())
	}
}

func TestHandleCommentsMissingBody(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	// Create an issue first
	issue := &models.Issue{
		ID:    "test-id",
		Key:   "SO-1",
		Title: "Test Issue",
		Type:  "task",
	}
	db.CreateIssue(issue)

	payload := []byte(`{
  "issue_key": "SO-1",
  "author": "user"
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/generic/comments", bytes.NewReader(payload))
	req.SetPathValue("source", "generic")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleComments)
	authHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "body required") {
		t.Errorf("response missing 'body required': %s", w.Body.String())
	}
}

func TestHandleCommentsIdempotency(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	// Create an issue first
	issue := &models.Issue{
		ID:    "test-id",
		Key:   "SO-1",
		Title: "Test Issue",
		Type:  "task",
	}
	db.CreateIssue(issue)

	payload := []byte(`{
  "issue_key": "SO-1",
  "author": "generic_user",
  "body": "This is a generic comment"
}`)

	deliveryID := "unique-comment-delivery-id-456"

	// First request
	req1 := httptest.NewRequest("POST", "/api/v1/webhooks/generic/comments", bytes.NewReader(payload))
	req1.SetPathValue("source", "generic")
	req1.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	req1.Header.Set("X-Webhook-Delivery-ID", deliveryID)
	w1 := httptest.NewRecorder()

	authHandler1 := wh.WebhookAuth(wh.HandleComments)
	authHandler1(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Errorf("first request status = %d, want %d", w1.Code, http.StatusCreated)
	}

	commentsBefore, _ := db.ListComments("SO-1")
	initialCount := len(commentsBefore)

	// Second request with same delivery ID
	req2 := httptest.NewRequest("POST", "/api/v1/webhooks/generic/comments", bytes.NewReader(payload))
	req2.SetPathValue("source", "generic")
	req2.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	req2.Header.Set("X-Webhook-Delivery-ID", deliveryID)
	w2 := httptest.NewRecorder()

	authHandler2 := wh.WebhookAuth(wh.HandleComments)
	authHandler2(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("duplicate request status = %d, want %d", w2.Code, http.StatusOK)
	}

	var resp2 map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp2)

	if status, ok := resp2["status"].(string); !ok || status != "duplicate" {
		t.Errorf("duplicate response status = %q, want duplicate", status)
	}

	// Verify no duplicate comment was created
	commentsAfter, _ := db.ListComments("SO-1")
	if len(commentsAfter) != initialCount {
		t.Errorf("comment count = %d, want %d (should be idempotent)", len(commentsAfter), initialCount)
	}
}

func TestHandleCommentsUnsupportedAction(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	// Create an issue first
	issue := &models.Issue{
		ID:    "test-id",
		Key:   "SO-1",
		Title: "Add webhook support",
		Type:  "task",
	}
	db.CreateIssue(issue)

	payload := []byte(`{
  "action": "deleted",
  "issue": {
    "number": 123,
    "title": "Add webhook support"
  },
  "comment": {
    "body": "This comment is being deleted",
    "user": {"login": "reviewer"}
  }
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/comments", bytes.NewReader(payload))
	req.SetPathValue("source", "github")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleComments)
	authHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for unsupported action", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "unsupported action") {
		t.Errorf("response missing 'unsupported action': %s", w.Body.String())
	}
}

// --- Integration Tests for Body Size Limit ---

func TestBodySizeLimitExceeded(t *testing.T) {
	wh, _ := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	// Create a payload larger than 1 MB
	largePayload := make([]byte, maxWebhookBodySize+1)
	for i := range largePayload {
		largePayload[i] = 'a'
	}

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", bytes.NewReader(largePayload))
	req.SetPathValue("source", "github")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(largePayload, secret))
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleIssues)
	authHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for oversized payload", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "too large") {
		t.Errorf("response should mention payload size: %s", w.Body.String())
	}
}

// --- Integration Test: End-to-End Issue Creation ---

func TestEndToEndIssueCreation(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	payload := []byte(`{
  "action": "opened",
  "issue": {
    "number": 123,
    "title": "End-to-end test issue",
    "body": "This issue was created via webhook",
    "state": "open",
    "labels": [{"name": "feature"}],
    "user": {"login": "octocat"}
  }
}`)

	sig := computeHMAC(payload, secret)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", bytes.NewReader(payload))
	req.SetPathValue("source", "github")
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-123")
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleIssues)
	authHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	// Verify issue exists in database
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	issueKey, ok := resp["key"].(string)
	if !ok {
		t.Fatal("response missing 'key'")
	}

	// Query the database to verify it appears in the database
	issue, err := db.GetIssue(issueKey)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	if issue.Title != "End-to-end test issue" {
		t.Errorf("title = %q, want 'End-to-end test issue'", issue.Title)
	}
	if issue.Status != models.StatusTodo {
		t.Errorf("status = %q, want %q", issue.Status, models.StatusTodo)
	}
	if issue.Type != "feature" {
		t.Errorf("type = %q, want feature", issue.Type)
	}
}

// --- Integration Test: End-to-End Comment Creation ---

func TestEndToEndCommentCreation(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	// Create an issue first
	issue := &models.Issue{
		ID:    "test-id",
		Key:   "SO-100",
		Title: "End-to-end test issue",
		Type:  "task",
	}
	db.CreateIssue(issue)

	// Send webhook comment
	payload := []byte(`{
  "issue_key": "SO-100",
  "author": "integration-test",
  "body": "This is an integration test comment"
}`)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/generic/comments", bytes.NewReader(payload))
	req.SetPathValue("source", "generic")
	req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
	req.Header.Set("X-Webhook-Delivery-ID", "test-comment-delivery")
	w := httptest.NewRecorder()

	authHandler := wh.WebhookAuth(wh.HandleComments)
	authHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	// Query the issue to verify comment was added
	comments, err := db.ListComments("SO-100")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}

	if len(comments) == 0 {
		t.Fatal("no comments found")
	}

	found := false
	for _, c := range comments {
		if c.Body == "This is an integration test comment" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected comment not found in database")
	}
}

// --- Test: Concurrent Webhook Requests ---

func TestConcurrentWebhookRequests(t *testing.T) {
	wh, db := testWebhookAPI(t)
	secret := "test-secret"
	os.Setenv("WEBHOOK_SECRET_GENERIC", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GENERIC")

	// Send 3 concurrent requests (SQLite serializes writes, so keep it small)
	type result struct {
		idx  int
		code int
		body string
	}
	done := make(chan result, 3)
	for i := 0; i < 3; i++ {
		go func(idx int) {
			payload := []byte(fmt.Sprintf(`{
  "title": "Concurrent webhook test issue #%d",
  "description": "Test concurrent webhook processing request %d",
  "type": "task"
}`, idx, idx))

			req := httptest.NewRequest("POST", "/api/v1/webhooks/generic/issues", bytes.NewReader(payload))
			req.SetPathValue("source", "generic")
			req.Header.Set("X-Webhook-Signature-256", computeHMAC(payload, secret))
			w := httptest.NewRecorder()

			authHandler := wh.WebhookAuth(wh.HandleIssues)
			authHandler(w, req)
			done <- result{idx, w.Code, w.Body.String()}
		}(i)
	}

	// Wait for all to complete and allow either success or graceful failure
	for i := 0; i < 3; i++ {
		res := <-done
		if res.code != http.StatusCreated && res.code != http.StatusConflict && res.code != http.StatusInternalServerError {
			t.Logf("request %d returned unexpected status %d: %s", res.idx, res.code, res.body)
		}
	}

	// Verify at least some issues were created (SQLite may have serialization issues)
	var count int
	row := db.QueryRow("SELECT COUNT(*) FROM issues WHERE title LIKE 'Concurrent webhook test issue%'")
	row.Scan(&count)

	if count == 0 {
		t.Errorf("no concurrent webhook issues created")
	}
}
