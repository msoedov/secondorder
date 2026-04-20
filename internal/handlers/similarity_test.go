package handlers

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/msoedov/mesa/internal/models"
	"github.com/msoedov/mesa/internal/templates"
)

// --- unit tests for titleSimilarity / levenshtein ---

func TestLevenshteinExact(t *testing.T) {
	if d := levenshtein("hello", "hello"); d != 0 {
		t.Fatalf("want 0, got %d", d)
	}
}

func TestLevenshteinEmpty(t *testing.T) {
	if d := levenshtein("", "abc"); d != 3 {
		t.Fatalf("want 3, got %d", d)
	}
	if d := levenshtein("abc", ""); d != 3 {
		t.Fatalf("want 3, got %d", d)
	}
}

func TestLevenshteinSingleEdit(t *testing.T) {
	if d := levenshtein("cat", "bat"); d != 1 {
		t.Fatalf("want 1, got %d", d)
	}
}

func TestTitleSimilarityExact(t *testing.T) {
	s := titleSimilarity("Add login page", "Add login page")
	if s != 1.0 {
		t.Fatalf("exact match: want 1.0, got %v", s)
	}
}

func TestTitleSimilarityNearMatch(t *testing.T) {
	// One word different — should be well above 0.70 threshold
	s := titleSimilarity("Add user login page", "Add user login form")
	if s < SimilarityThreshold {
		t.Fatalf("near-match: want >= %.2f, got %.4f", SimilarityThreshold, s)
	}
}

func TestTitleSimilarityBelowThreshold(t *testing.T) {
	s := titleSimilarity("Add login page", "Fix database migration issue")
	if s >= SimilarityThreshold {
		t.Fatalf("unrelated: want < %.2f, got %.4f", SimilarityThreshold, s)
	}
}

func TestTitleSimilarityEmptyBoth(t *testing.T) {
	s := titleSimilarity("", "")
	if s != 1.0 {
		t.Fatalf("both empty: want 1.0, got %v", s)
	}
}

func TestTitleSimilarityCaseInsensitive(t *testing.T) {
	a := titleSimilarity("Add Login Page", "add login page")
	if a != 1.0 {
		t.Fatalf("case fold: want 1.0, got %v", a)
	}
}

// --- HTTP handler tests ---

func makeAPIWithIssues(t *testing.T, issues []models.Issue) *API {
	t.Helper()
	d := testDB(t)
	for i := range issues {
		if err := d.CreateIssue(&issues[i]); err != nil {
			t.Fatalf("seed issue: %v", err)
		}
	}
	hub := NewSSEHub()
	t.Cleanup(hub.Close)
	tmpl, _ := templates.Parse()
	return NewAPI(d, hub, tmpl, nil, nil, nil)
}

func TestSimilarIssues_ExactMatch(t *testing.T) {
	api := makeAPIWithIssues(t, []models.Issue{
		{Key: "SO-1", Title: "Add login page", Status: models.StatusTodo, Type: "task"},
		{Key: "SO-2", Title: "Fix database bug", Status: models.StatusInProgress, Type: "task"},
	})

	body, _ := json.Marshal(map[string]string{"title": "Add login page"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/similarity", bytes.NewReader(body))
	req = req.WithContext(withAgent(req.Context(), &models.Agent{ID: "a1", Slug: "bot", ArchetypeSlug: "bot"}))
	w := httptest.NewRecorder()

	api.SimilarIssues(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", w.Code, w.Body)
	}
	var results []SimilarIssueResult
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for exact match")
	}
	if results[0].Key != "SO-1" {
		t.Errorf("top result key = %q, want SO-1", results[0].Key)
	}
	if math.Abs(results[0].SimilarityScore-1.0) > 0.001 {
		t.Errorf("exact match score = %v, want ~1.0", results[0].SimilarityScore)
	}
}

func TestSimilarIssues_NearMatch(t *testing.T) {
	api := makeAPIWithIssues(t, []models.Issue{
		{Key: "SO-1", Title: "Add user login page", Status: models.StatusTodo, Type: "task"},
		{Key: "SO-2", Title: "Fix unrelated database migration", Status: models.StatusTodo, Type: "task"},
	})

	body, _ := json.Marshal(map[string]string{"title": "Add user login form"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/similarity", bytes.NewReader(body))
	req = req.WithContext(withAgent(req.Context(), &models.Agent{ID: "a1", Slug: "bot", ArchetypeSlug: "bot"}))
	w := httptest.NewRecorder()

	api.SimilarIssues(w, req)

	var results []SimilarIssueResult
	json.Unmarshal(w.Body.Bytes(), &results)

	found := false
	for _, r := range results {
		if r.Key == "SO-1" {
			found = true
			if r.SimilarityScore < SimilarityThreshold {
				t.Errorf("near-match score %.4f below threshold %.2f", r.SimilarityScore, SimilarityThreshold)
			}
		}
	}
	if !found {
		t.Errorf("near-match SO-1 not returned; results: %+v", results)
	}
}

func TestSimilarIssues_BelowThreshold(t *testing.T) {
	api := makeAPIWithIssues(t, []models.Issue{
		{Key: "SO-1", Title: "Fix database migration bug", Status: models.StatusTodo, Type: "task"},
	})

	body, _ := json.Marshal(map[string]string{"title": "Implement OAuth2 provider integration"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/similarity", bytes.NewReader(body))
	req = req.WithContext(withAgent(req.Context(), &models.Agent{ID: "a1", Slug: "bot", ArchetypeSlug: "bot"}))
	w := httptest.NewRecorder()

	api.SimilarIssues(w, req)

	var results []SimilarIssueResult
	json.Unmarshal(w.Body.Bytes(), &results)
	if len(results) != 0 {
		t.Errorf("expected empty results for unrelated title, got %+v", results)
	}
}

func TestSimilarIssues_EmptyDatabase(t *testing.T) {
	api := makeAPIWithIssues(t, nil)

	body, _ := json.Marshal(map[string]string{"title": "Add login page"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/similarity", bytes.NewReader(body))
	req = req.WithContext(withAgent(req.Context(), &models.Agent{ID: "a1", Slug: "bot", ArchetypeSlug: "bot"}))
	w := httptest.NewRecorder()

	api.SimilarIssues(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var results []SimilarIssueResult
	json.Unmarshal(w.Body.Bytes(), &results)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %+v", results)
	}
}

func TestSimilarIssues_SkipsTerminalIssues(t *testing.T) {
	api := makeAPIWithIssues(t, []models.Issue{
		{Key: "SO-1", Title: "Add login page", Status: models.StatusDone, Type: "task"},
		{Key: "SO-2", Title: "Add login page", Status: models.StatusCancelled, Type: "task"},
	})

	body, _ := json.Marshal(map[string]string{"title": "Add login page"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/similarity", bytes.NewReader(body))
	req = req.WithContext(withAgent(req.Context(), &models.Agent{ID: "a1", Slug: "bot", ArchetypeSlug: "bot"}))
	w := httptest.NewRecorder()

	api.SimilarIssues(w, req)

	var results []SimilarIssueResult
	json.Unmarshal(w.Body.Bytes(), &results)
	if len(results) != 0 {
		t.Errorf("done/cancelled issues should not appear in similarity results, got %+v", results)
	}
}

func TestSimilarIssues_SortedByScore(t *testing.T) {
	api := makeAPIWithIssues(t, []models.Issue{
		{Key: "SO-1", Title: "Add user login form", Status: models.StatusTodo, Type: "task"},
		{Key: "SO-2", Title: "Add login page", Status: models.StatusTodo, Type: "task"},
	})

	body, _ := json.Marshal(map[string]string{"title": "Add user login page"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/similarity", bytes.NewReader(body))
	req = req.WithContext(withAgent(req.Context(), &models.Agent{ID: "a1", Slug: "bot", ArchetypeSlug: "bot"}))
	w := httptest.NewRecorder()

	api.SimilarIssues(w, req)

	var results []SimilarIssueResult
	json.Unmarshal(w.Body.Bytes(), &results)

	for i := 1; i < len(results); i++ {
		if results[i].SimilarityScore > results[i-1].SimilarityScore {
			t.Errorf("results not sorted desc at index %d: %v > %v", i, results[i].SimilarityScore, results[i-1].SimilarityScore)
		}
	}
}

func TestSimilarIssues_MissingTitle(t *testing.T) {
	api := makeAPIWithIssues(t, nil)

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues/similarity", bytes.NewReader(body))
	req = req.WithContext(withAgent(req.Context(), &models.Agent{ID: "a1", Slug: "bot", ArchetypeSlug: "bot"}))
	w := httptest.NewRecorder()

	api.SimilarIssues(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
