package handlers

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

// --- Wiki API Handler Tests ---

func createTestWikiAgent(t *testing.T, d *db.DB, slug string) *models.Agent {
	agent := &models.Agent{
		Name:          "Wiki Test Agent " + slug,
		Slug:          slug,
		ArchetypeSlug: "worker",
		Model:         "sonnet",
		WorkingDir:    "/tmp",
		MaxTurns:      50,
		TimeoutSec:    600,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create test agent: %v", err)
	}
	return agent
}

func createTestWikiAPIKey(t *testing.T, d *db.DB, agent *models.Agent) string {
	rawKey := "so_wiki_test_" + agent.Slug
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])
	if err := d.CreateAPIKey(agent.ID, "wiki-test-"+agent.Slug, keyHash, "so_wiki_test", 60*time.Minute); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return rawKey
}

func setupWikiTest(t *testing.T) (*db.DB, *API, *models.Agent, string) {
	d := testDB(t)
	hub := NewSSEHub()
	t.Cleanup(func() { hub.Close() })
	api := NewAPI(d, hub, nil, nil, &stubTelegram{}, nil)

	agent := createTestWikiAgent(t, d, "wiki-agent")
	apiKey := createTestWikiAPIKey(t, d, agent)

	return d, api, agent, apiKey
}

// --- Acceptance Criteria Tests ---

func TestListWikiPagesEmpty(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	req := httptest.NewRequest("GET", "/api/v1/wiki", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.ListWikiPages))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result []models.WikiPageSummary
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 pages, got %d", len(result))
	}
}

func TestListWikiPages(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	// Create some wiki pages
	page1 := &models.WikiPage{
		Slug:             "page1",
		Title:            "First Page",
		Content:          "Content 1",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	page2 := &models.WikiPage{
		Slug:             "page2",
		Title:            "Second Page",
		Content:          "Content 2",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page1)
	d.CreateWikiPage(page2)

	req := httptest.NewRequest("GET", "/api/v1/wiki", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.ListWikiPages))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result []models.WikiPageSummary
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 2 {
		t.Errorf("expected 2 pages, got %d", len(result))
	}
}

func TestCreateWikiPageSuccess(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"title":   "New Page",
		"content": "This is new content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Slug != "new-page" {
		t.Errorf("slug = %q, want new-page", result.Slug)
	}
	if result.Title != "New Page" {
		t.Errorf("title = %q, want New Page", result.Title)
	}
	if result.ID == "" {
		t.Error("expected ID to be set")
	}
}

func TestCreateWikiPageNoAuth(t *testing.T) {
	_, api, _, _ := setupWikiTest(t)

	body := map[string]string{
		"title": "No Auth",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestCreateWikiPageAutoGeneratesSlug(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"title": "No Slug",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestCreateWikiPageMissingTitle(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"content": "Missing title",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCreateWikiPageDuplicateSlug(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	// Create first page directly in DB
	firstPage := &models.WikiPage{
		Slug:             "duplicate",
		Title:            "First",
		Content:          "Content",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(firstPage)

	// Try to create second with same slug via API
	body := map[string]string{
		"title": "Duplicate",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409, body=%s", w.Code, w.Body.String())
	}
}

func TestGetWikiPageSuccess(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "get-test",
		Title:            "Get Test",
		Content:          "Test content",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	req := httptest.NewRequest("GET", "/api/v1/wiki/get-test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.SetPathValue("slug", "get-test")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.GetWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Slug != "get-test" {
		t.Errorf("slug = %q, want get-test", result.Slug)
	}
	if result.Title != "Get Test" {
		t.Errorf("title = %q, want Get Test", result.Title)
	}
}

func TestGetWikiPageNotFound(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	req := httptest.NewRequest("GET", "/api/v1/wiki/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.SetPathValue("slug", "nonexistent")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.GetWikiPage))(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestUpdateWikiPageSuccess(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "update-test",
		Title:            "Original Title",
		Content:          "Original Content",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	body := map[string]string{
		"title":   "Updated Title",
		"content": "Updated Content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/v1/wiki/update-test", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("slug", "update-test")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.UpdateWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Title != "Updated Title" {
		t.Errorf("title = %q, want Updated Title", result.Title)
	}
	if result.Content != "Updated Content" {
		t.Errorf("content = %q, want Updated Content", result.Content)
	}
}

func TestUpdateWikiPageNotFound(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"title": "New Title",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/v1/wiki/nonexistent", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("slug", "nonexistent")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.UpdateWikiPage))(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDeleteWikiPageSuccess(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "delete-test",
		Title:            "To Delete",
		Content:          "Delete me",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	// Verify it exists
	_, err := d.GetWikiPageBySlug("delete-test")
	if err != nil {
		t.Fatalf("page should exist before delete: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/wiki/delete-test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.SetPathValue("slug", "delete-test")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.DeleteWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Verify it's deleted
	_, err = d.GetWikiPageBySlug("delete-test")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows after delete, got %v", err)
	}
}

func TestDeleteWikiPageNotFound(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	req := httptest.NewRequest("DELETE", "/api/v1/wiki/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.SetPathValue("slug", "nonexistent")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.DeleteWikiPage))(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestCreateWikiPageSlugNormalization(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	cases := []struct {
		title    string
		wantSlug string
	}{
		{"What's New?", "what-s-new"},
		{"Hello   World", "hello-world"},
		{"  Leading Trailing  ", "leading-trailing"},
		{"API v2.0 Release!", "api-v2-0-release"},
	}

	for _, tc := range cases {
		body := map[string]string{"title": tc.title}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("title=%q: status = %d, want 201, body=%s", tc.title, w.Code, w.Body.String())
			continue
		}

		var result models.WikiPage
		json.NewDecoder(w.Body).Decode(&result)
		if result.Slug != tc.wantSlug {
			t.Errorf("title=%q: slug = %q, want %q", tc.title, result.Slug, tc.wantSlug)
		}
	}
}

func TestCreateWikiPageExplicitSlugNormalized(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"slug":  "My Custom Slug!",
		"title": "Test Page",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", w.Code, w.Body.String())
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Slug != "my-custom-slug" {
		t.Errorf("slug = %q, want my-custom-slug", result.Slug)
	}
}

func TestUpdateWikiPageSlug(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "old-slug",
		Title:            "Old Title",
		Content:          "Content",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	newSlug := "new-slug"
	body := map[string]*string{
		"slug": &newSlug,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/v1/wiki/old-slug", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("slug", "old-slug")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.UpdateWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Slug != "new-slug" {
		t.Errorf("slug = %q, want new-slug", result.Slug)
	}

	// Old slug should 404
	_, err := d.GetWikiPageBySlug("old-slug")
	if err != sql.ErrNoRows {
		t.Errorf("old slug should not exist, got %v", err)
	}
}

func TestListWikiPagesSummaryOmitsContent(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "summary-test",
		Title:            "Summary Test",
		Content:          "This content should NOT appear in list response",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	req := httptest.NewRequest("GET", "/api/v1/wiki", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.ListWikiPages))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Decode as raw JSON to verify content field is absent
	var raw []map[string]any
	json.NewDecoder(w.Body).Decode(&raw)
	if len(raw) != 1 {
		t.Fatalf("expected 1 page, got %d", len(raw))
	}

	if _, hasContent := raw[0]["content"]; hasContent {
		t.Error("list response should not include content field")
	}
	if _, hasSlug := raw[0]["slug"]; !hasSlug {
		t.Error("list response should include slug field")
	}
	if _, hasTitle := raw[0]["title"]; !hasTitle {
		t.Error("list response should include title field")
	}
}
