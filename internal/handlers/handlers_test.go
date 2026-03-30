package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// Ensure DB path comes from temp for test isolation
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
