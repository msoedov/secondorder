package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/msoedov/thelastorg/internal/db"
	"github.com/msoedov/thelastorg/internal/models"
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

	hub.Broadcast("update", `{"key":"TLO-1"}`)

	select {
	case msg := <-ch:
		want := "event: update\ndata: {\"key\":\"TLO-1\"}\n\n"
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

	rawKey := "tlo_test_key_123"
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])
	d.CreateAPIKey(agent.ID, keyHash, "tlo_test_ke")

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

// Ensure DB path comes from temp for test isolation
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
