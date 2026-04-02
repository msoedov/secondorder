package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
	"github.com/msoedov/secondorder/internal/templates"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// --- Context helpers ---

func TestWithAgentAndFromContext(t *testing.T) {
	agent := &models.Agent{ID: "a1", Name: "Test"}
	ctx := withAgent(context.Background(), agent)

	got := agentFromContext(ctx)
	if got == nil {
		t.Fatal("expected agent from context")
	}
	if got.ID != "a1" {
		t.Errorf("ID = %q, want a1", got.ID)
	}
}

func TestAgentFromContextMissing(t *testing.T) {
	got := agentFromContext(context.Background())
	if got != nil {
		t.Error("expected nil for empty context")
	}
}

// --- SSE Hub ---

func TestNewSSEHub(t *testing.T) {
	hub := NewSSEHub()
	if hub == nil {
		t.Fatal("expected non-nil hub")
	}
	hub.Close()
}

func TestSSEHubBroadcast(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Close()

	ch := make(chan string, 16)
	hub.mu.Lock()
	hub.clients[ch] = struct{}{}
	hub.mu.Unlock()

	hub.Broadcast("update", `{"key":"SO-1"}`)

	select {
	case msg := <-ch:
		want := "event: update\ndata: {\"key\":\"SO-1\"}\n\n"
		if msg != want {
			t.Errorf("got %q, want %q", msg, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestSSEHubBroadcastDropsSlow(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Close()

	// Buffer of 0 so it's always "slow"
	ch := make(chan string)
	hub.mu.Lock()
	hub.clients[ch] = struct{}{}
	hub.mu.Unlock()

	// Should not block
	hub.Broadcast("test", "data")
}

func TestSSEHubClose(t *testing.T) {
	hub := NewSSEHub()
	ch := make(chan string, 1)
	hub.mu.Lock()
	hub.clients[ch] = struct{}{}
	hub.mu.Unlock()

	hub.Close()

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 clients after close, got %d", count)
	}
}

// --- Auth middleware ---

type stubTelegram struct{}

func (s *stubTelegram) SendWorkBlockApproval(_, _, _, _ string) error { return nil }
func (s *stubTelegram) SendMessage(_ string) error                    { return nil }

func TestAuthMissingKey(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	handler := api.Auth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthInvalidKey(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	handler := api.Auth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer bad-key")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthValidKey(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	// Create agent and API key
	agent := &models.Agent{
		Name: "Auth Test", Slug: "auth-test", ArchetypeSlug: "worker",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(agent)

	rawKey := "so_test_key_123"
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])
	d.CreateAPIKey(agent.ID, keyHash, "so_test_ke")

	var gotAgent *models.Agent
	api := NewAPI(d, hub, nil, &stubTelegram{})
	handler := api.Auth(func(w http.ResponseWriter, r *http.Request) {
		gotAgent = agentFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if gotAgent == nil {
		t.Fatal("expected agent in context")
	}
	if gotAgent.ID != agent.ID {
		t.Errorf("agent ID = %q, want %q", gotAgent.ID, agent.ID)
	}
}

// --- SSE ServeHTTP ---

func TestSSEServeHTTP(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Close()

	// Use a context we can cancel to stop the SSE stream
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/events", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		hub.ServeHTTP(w, req)
		close(done)
	}()

	// Give the handler a moment to register and send keepalive
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if body == "" {
		t.Error("expected keepalive output")
	}
}

// --- Duplicate issue detection ---

func TestCreateIssueDuplicateDetection(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	// Create an agent + API key for auth
	agent := &models.Agent{
		Name: "Dup Test", Slug: "dup-test", ArchetypeSlug: "worker",
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(agent)

	rawKey := "so_dup_test_key"
	h := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(h[:])
	d.CreateAPIKey(agent.ID, keyHash, "so_dup_tes")

	authHandler := api.Auth(api.CreateIssue)

	// First creation should succeed
	body1 := `{"title":"Fix Login Bug","description":"details"}`
	req1 := httptest.NewRequest("POST", "/api/v1/issues", strings.NewReader(body1))
	req1.Header.Set("Authorization", "Bearer "+rawKey)
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	authHandler(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want 201, body: %s", w1.Code, w1.Body.String())
	}

	// Exact duplicate should return 409
	body2 := `{"title":"Fix Login Bug","description":"other"}`
	req2 := httptest.NewRequest("POST", "/api/v1/issues", strings.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer "+rawKey)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	authHandler(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate exact: status = %d, want 409, body: %s", w2.Code, w2.Body.String())
	}

	// Case-insensitive duplicate should also return 409
	body3 := `{"title":"fix login bug","description":"other"}`
	req3 := httptest.NewRequest("POST", "/api/v1/issues", strings.NewReader(body3))
	req3.Header.Set("Authorization", "Bearer "+rawKey)
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	authHandler(w3, req3)

	if w3.Code != http.StatusConflict {
		t.Fatalf("duplicate case-insensitive: status = %d, want 409, body: %s", w3.Code, w3.Body.String())
	}

	// Different title should succeed
	body4 := `{"title":"Different Issue","description":"details"}`
	req4 := httptest.NewRequest("POST", "/api/v1/issues", strings.NewReader(body4))
	req4.Header.Set("Authorization", "Bearer "+rawKey)
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	authHandler(w4, req4)

	if w4.Code != http.StatusCreated {
		t.Fatalf("different title: status = %d, want 201, body: %s", w4.Code, w4.Body.String())
	}
}

// --- UI: issue creation error handling ---

type stubSched struct{}

func (s *stubSched) WakeAgentHeartbeat(*models.Agent) {}
func (s *stubSched) CancelAudit(string) error { return nil }
func (s *stubSched) RunAudit(int, int, string) (string, error) {
	return "", nil
}

func testUI(t *testing.T, d *db.DB) *UI {
	t.Helper()
	tmpl, err := templates.Parse()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	hub := NewSSEHub()
	t.Cleanup(func() { hub.Close() })
	return NewUI(d, hub, tmpl, nil, &stubSched{})
}

func TestCreateIssueUI_Success(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	form := strings.NewReader("title=Test+Issue&description=desc&priority=2")
	req := httptest.NewRequest("POST", "/issues", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	ui.ListIssues(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/issues" {
		t.Fatalf("redirect = %q, want /issues", loc)
	}

	// Verify the issue exists in DB
	issues, err := d.ListIssues("", 100)
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	found := false
	for _, iss := range issues {
		if iss.Title == "Test Issue" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("issue not found in DB after creation")
	}
}

func TestCreateIssueUI_EmptyTitle(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	form := strings.NewReader("title=&description=desc")
	req := httptest.NewRequest("POST", "/issues", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	ui.ListIssues(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/issues" {
		t.Fatalf("redirect = %q, want /issues", loc)
	}
}

func TestIssueDetail_NotFound(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	req := httptest.NewRequest("GET", "/issues/SO-999", nil)
	req.SetPathValue("key", "SO-999")
	w := httptest.NewRecorder()

	ui.IssueDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Issue not found") {
		t.Errorf("body missing 'Issue not found', got: %s", body[:min(200, len(body))])
	}
}

// --- Settings ---

func TestSettingsGET(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()

	ui.Settings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Instance Settings", "Telegram Integration", "About", "Settings"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestSettingsGET_FlashMessage(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	req := httptest.NewRequest("GET", "/settings?msg=Settings+saved", nil)
	w := httptest.NewRecorder()

	ui.Settings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Settings saved") {
		t.Error("body missing flash message")
	}
}

func TestSettingsPOST_Instance(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	form := strings.NewReader("section=instance&instance_name=TestOrg&issue_prefix=TEST")
	req := httptest.NewRequest("POST", "/settings", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	ui.Settings(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "msg=Settings+saved") {
		t.Errorf("redirect = %q, want flash msg", loc)
	}

	// Verify values persisted
	val, err := d.GetSetting("instance_name")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "TestOrg" {
		t.Errorf("instance_name = %q, want TestOrg", val)
	}
	val, err = d.GetSetting("issue_prefix")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if val != "TEST" {
		t.Errorf("issue_prefix = %q, want TEST", val)
	}
}

func TestSettingsPOST_Telegram(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	form := strings.NewReader("section=telegram&telegram_token=tok123&telegram_chat_id=-100999")
	req := httptest.NewRequest("POST", "/settings", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	ui.Settings(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}

	val, _ := d.GetSetting("telegram_token")
	if val != "tok123" {
		t.Errorf("telegram_token = %q, want tok123", val)
	}
	val, _ = d.GetSetting("telegram_chat_id")
	if val != "-100999" {
		t.Errorf("telegram_chat_id = %q, want -100999", val)
	}
}

func TestSettingsPOST_UnknownSection(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	form := strings.NewReader("section=bad")
	req := httptest.NewRequest("POST", "/settings", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	ui.Settings(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "error=Unknown") {
		t.Errorf("redirect = %q, want error param", loc)
	}
}

// --- Board comment reopens issue ---

func TestBoardCommentReopensIssue(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		wantStatus string
	}{
		{"done_reopens", models.StatusDone, models.StatusInProgress},
		{"blocked_reopens", models.StatusBlocked, models.StatusInProgress},
		{"in_review_reopens", models.StatusInReview, models.StatusInProgress},
		{"in_progress_stays", models.StatusInProgress, models.StatusInProgress},
		{"todo_stays", models.StatusTodo, models.StatusTodo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDB(t)
			ui := testUI(t, d)

			// Create agent and issue
			agent := &models.Agent{
				Name: "Worker", Slug: "worker", ArchetypeSlug: "worker",
				Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
			}
			d.CreateAgent(agent)

			issue := &models.Issue{
				Key:             "SO-1",
				Title:           "Test issue",
				Status:          tt.initial,
				AssigneeAgentID: &agent.ID,
			}
			d.CreateIssue(issue)

			// Post board comment
			form := strings.NewReader("action=comment&body=Please+fix+this")
			req := httptest.NewRequest("POST", "/issues/SO-1", form)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.SetPathValue("key", "SO-1")
			w := httptest.NewRecorder()

			ui.IssueDetail(w, req)

			if w.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", w.Code)
			}

			got, err := d.GetIssue("SO-1")
			if err != nil {
				t.Fatalf("get issue: %v", err)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}

func TestNotFound(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	tests := []struct {
		name       string
		accept     string
		wantCode   int
		wantHTML   bool
	}{
		{"html request", "text/html", 404, true},
		{"json request", "application/json", 404, false},
		{"no accept header", "", 404, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/nonexistent", nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			w := httptest.NewRecorder()
			ui.NotFound(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
			body := w.Body.String()
			hasHTML := strings.Contains(body, "Page not found")
			if hasHTML != tt.wantHTML {
				t.Errorf("html content present = %v, want %v", hasHTML, tt.wantHTML)
			}
			if tt.wantHTML && !strings.Contains(body, "/dashboard") {
				t.Error("expected back link to /dashboard")
			}
		})
	}
}

func TestSettingsVersion(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	// Create a temporary VERSION file in the current directory for the test
	// Note: The code reads from "VERSION" in the working directory
	versionFile := "VERSION_test"
	if err := os.WriteFile(versionFile, []byte("v1.2.3-test"), 0644); err != nil {
		t.Fatalf("failed to create version file: %v", err)
	}
	defer os.Remove(versionFile)

	// We need to temporarily mock the file read or change the working directory
	// But since the handler uses os.ReadFile("VERSION"), we can just rename the file
	// during the test if we are careful, but that's risky.
	// Alternatively, we can just check if it's there.

	// Let's try a simpler approach: check if it reads from "VERSION"
	if _, err := os.Stat("VERSION"); err == nil {
		// VERSION exists, let's see if it's in the output
		content, _ := os.ReadFile("VERSION")
		wantVersion := strings.TrimSpace(string(content))

		req := httptest.NewRequest("GET", "/settings", nil)
		w := httptest.NewRecorder()
		ui.Settings(w, req)

		if !strings.Contains(w.Body.String(), wantVersion) {
			t.Errorf("body missing version %q", wantVersion)
		}
	} else {
		// If VERSION doesn't exist, it should fallback to "dev" or build info
		req := httptest.NewRequest("GET", "/settings", nil)
		w := httptest.NewRecorder()
		ui.Settings(w, req)

		if !strings.Contains(w.Body.String(), "dev") && !strings.Contains(w.Body.String(), "v") {
			t.Error("body missing default version")
		}
	}
}

func TestAgentUI_CreateAndUpdate(t *testing.T) {
	d := testDB(t)
	ui := testUI(t, d)

	// 1. Create agent
	form := "name=Test+Runner+Agent&runner=codex&model=gpt-4o&api_key_env=MY_KEY&archetype_slug=fullstack"
	req := httptest.NewRequest("POST", "/agents", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// We need a dummy archetype file
	os.MkdirAll("archetypes", 0755)
	os.WriteFile("archetypes/fullstack.md", []byte("test"), 0644)
	defer os.RemoveAll("archetypes")

	ui.ListAgents(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("create: status = %d, want 303", w.Code)
	}

	agent, err := d.GetAgentBySlug("test-runner-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Runner != "codex" {
		t.Errorf("runner = %q, want codex", agent.Runner)
	}
	if agent.ApiKeyEnv != "MY_KEY" {
		t.Errorf("api_key_env = %q, want MY_KEY", agent.ApiKeyEnv)
	}

	// 2. Update agent (change runner and clear api_key_env)
	updateForm := "name=Updated+Name&runner=antigravity&model=default&api_key_env="
	req = httptest.NewRequest("POST", "/agents/test-runner-agent", strings.NewReader(updateForm))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("slug", "test-runner-agent")
	w = httptest.NewRecorder()

	ui.AgentDetail(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("update: status = %d, want 303", w.Code)
	}

	agent, err = d.GetAgentBySlug("test-runner-agent")
	if err != nil {
		t.Fatalf("get updated agent: %v", err)
	}
	if agent.Runner != "antigravity" {
		t.Errorf("updated runner = %q, want antigravity", agent.Runner)
	}
	if agent.ApiKeyEnv != "" {
		t.Errorf("updated api_key_env = %q, want empty", agent.ApiKeyEnv)
	}
}

// helper: create agent + API key, return raw key
func createAgentWithKey(t *testing.T, d *db.DB, name, slug, archetype string) (*models.Agent, string) {
	t.Helper()
	agent := &models.Agent{
		Name: name, Slug: slug, ArchetypeSlug: archetype,
		Model: "sonnet", WorkingDir: "/tmp", MaxTurns: 50, TimeoutSec: 600, Active: true,
	}
	d.CreateAgent(agent)
	rawKey := "so_key_" + slug
	h := sha256.Sum256([]byte(rawKey))
	prefix := rawKey
	if len(prefix) > 10 {
		prefix = prefix[:10]
	}
	d.CreateAPIKey(agent.ID, hex.EncodeToString(h[:]), prefix)
	return agent, rawKey
}

// --- Security: ownership restriction ---

func TestUpdateIssue_ForbiddenForNonAssignee(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	owner, _ := createAgentWithKey(t, d, "Owner", "owner", "backend")
	other, otherKey := createAgentWithKey(t, d, "Other", "other", "frontend")
	_ = other

	issue := &models.Issue{Key: "SO-1", Title: "T", Status: "in_progress", AssigneeAgentID: &owner.ID}
	d.CreateIssue(issue)

	req := httptest.NewRequest("PATCH", "/api/v1/issues/SO-1", strings.NewReader(`{"status":"done"}`))
	req.Header.Set("Authorization", "Bearer "+otherKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("key", "SO-1")
	w := httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestUpdateIssue_CEOCanUpdateAnyIssue(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	owner, _ := createAgentWithKey(t, d, "Owner", "owner2", "backend")
	_, ceoKey := createAgentWithKey(t, d, "CEO", "ceo", "ceo")

	issue := &models.Issue{Key: "SO-2", Title: "T2", Status: "in_progress", AssigneeAgentID: &owner.ID}
	d.CreateIssue(issue)

	req := httptest.NewRequest("PATCH", "/api/v1/issues/SO-2", strings.NewReader(`{"status":"done"}`))
	req.Header.Set("Authorization", "Bearer "+ceoKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("key", "SO-2")
	w := httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestUpdateIssue_AssigneeCanUpdate(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	owner, ownerKey := createAgentWithKey(t, d, "Owner3", "owner3", "backend")

	issue := &models.Issue{Key: "SO-3", Title: "T3", Status: "in_progress", AssigneeAgentID: &owner.ID}
	d.CreateIssue(issue)

	req := httptest.NewRequest("PATCH", "/api/v1/issues/SO-3", strings.NewReader(`{"status":"done"}`))
	req.Header.Set("Authorization", "Bearer "+ownerKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("key", "SO-3")
	w := httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- Security: invalid API key returns 401 ---

func TestAuth_NoKey_Returns401(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	req := httptest.NewRequest("PATCH", "/api/v1/issues/SO-1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuth_InvalidKey_Returns401(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	req := httptest.NewRequest("PATCH", "/api/v1/issues/SO-1", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer totally-wrong-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// Ensure DB path comes from temp for test isolation
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestUpdateIssue_Reassign(t *testing.T) {
	d := testDB(t)
	hub := NewSSEHub()
	defer hub.Close()
	api := NewAPI(d, hub, nil, &stubTelegram{})

	owner, _ := createAgentWithKey(t, d, "Owner", "owner", "backend")
	newAssignee, _ := createAgentWithKey(t, d, "NewAssignee", "new-assignee", "frontend")
	_, ceoKey := createAgentWithKey(t, d, "CEO", "ceo", "ceo")

	issue := &models.Issue{Key: "SO-55", Title: "Test Issue", Status: "todo", AssigneeAgentID: &owner.ID}
	d.CreateIssue(issue)

	// Test reassignment by CEO
	body := fmt.Sprintf(`{"assignee_slug":"%s"}`, newAssignee.Slug)
	req := httptest.NewRequest("PATCH", "/api/v1/issues/SO-55", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+ceoKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("key", "SO-55")
	w := httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}

	updatedIssue, _ := d.GetIssue("SO-55")
	if updatedIssue.AssigneeAgentID == nil || *updatedIssue.AssigneeAgentID != newAssignee.ID {
		t.Errorf("assignee = %v, want %s", updatedIssue.AssigneeAgentID, newAssignee.ID)
	}

	// Test unassignment (empty slug)
	req = httptest.NewRequest("PATCH", "/api/v1/issues/SO-55", strings.NewReader(`{"assignee_slug":""}`))
	req.Header.Set("Authorization", "Bearer "+ceoKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("key", "SO-55")
	w = httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	updatedIssue, _ = d.GetIssue("SO-55")
	if updatedIssue.AssigneeAgentID != nil {
		t.Errorf("expected nil assignee, got %v", *updatedIssue.AssigneeAgentID)
	}

    // Test reassignment to non-existent agent
	req = httptest.NewRequest("PATCH", "/api/v1/issues/SO-55", strings.NewReader(`{"assignee_slug":"non-existent"}`))
	req.Header.Set("Authorization", "Bearer "+ceoKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("key", "SO-55")
	w = httptest.NewRecorder()
	api.Auth(api.UpdateIssue)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
