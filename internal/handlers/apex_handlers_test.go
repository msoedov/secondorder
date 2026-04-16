package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"github.com/msoedov/mesa/internal/models"
	"github.com/msoedov/mesa/internal/templates"
)

func TestApexBlockHandlers(t *testing.T) {
	d := testDB(t)
	sse := NewSSEHub()
	defer sse.Close()
	tmpl, _ := templates.Parse()
	api := NewAPI(d, sse, tmpl, nil, nil, nil)

	// Create an agent for auth
	agent := &models.Agent{
		ID:   "a1",
		Name: "Alice",
		Slug: "alice",
	}
	d.CreateAgent(agent)
	token := "test-token"
	d.CreateAPIKey(agent.ID, "run-apex", hashToken(token), "test", 60*time.Minute)

	// 1. Create Apex Block
	body := map[string]string{
		"title": "Strategy 2026",
		"goal":  "Dominate the market",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/apex-blocks", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	api.Auth(api.CreateApexBlock)(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusCreated)
	}

	var ab models.ApexBlock
	json.NewDecoder(rr.Body).Decode(&ab)
	if ab.Title != "Strategy 2026" {
		t.Errorf("got title %q, want %q", ab.Title, "Strategy 2026")
	}

	// 2. List Apex Blocks
	req = httptest.NewRequest("GET", "/api/v1/apex-blocks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	api.Auth(api.ListApexBlocks)(rr, req)

	var blocks []models.ApexBlock
	json.NewDecoder(rr.Body).Decode(&blocks)
	if len(blocks) != 1 {
		t.Errorf("got %d blocks, want 1", len(blocks))
	}

	// 3. Update Apex Block
	updateBody := map[string]string{
		"title": "Updated Strategy",
	}
	b, _ = json.Marshal(updateBody)
	req = httptest.NewRequest("PATCH", "/api/v1/apex-blocks/"+ab.ID, bytes.NewReader(b))
	req.SetPathValue("id", ab.ID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	api.Auth(api.UpdateApexBlock)(rr, req)

	json.NewDecoder(rr.Body).Decode(&ab)
	if ab.Title != "Updated Strategy" {
		t.Errorf("got title %q, want %q", ab.Title, "Updated Strategy")
	}
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func TestWorkBlockWithApexHandlers(t *testing.T) {
	d := testDB(t)
	sse := NewSSEHub()
	defer sse.Close()
	tmpl, _ := templates.Parse()
	api := NewAPI(d, sse, tmpl, nil, nil, nil)

	agent := &models.Agent{ID: "a1", Name: "Alice", Slug: "alice"}
	d.CreateAgent(agent)
	token := "test-token"
	d.CreateAPIKey(agent.ID, "run-apex", hashToken(token), "test", 60*time.Minute)

	ab := &models.ApexBlock{Title: "Apex", Goal: "Goal"}
	d.CreateApexBlock(ab)

	// Create Work Block
	body := map[string]any{
		"title":             "Work Block",
		"goal":              "Goal",
		"north_star_metric": "Revenue",
		"north_star_target": "$1M",
		"apex_block_id":     ab.ID,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/work-blocks", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	api.Auth(api.CreateWorkBlock)(rr, req)

	var wb models.WorkBlock
	json.NewDecoder(rr.Body).Decode(&wb)
	if wb.NorthStarMetric != "Revenue" {
		t.Errorf("got metric %q, want %q", wb.NorthStarMetric, "Revenue")
	}

	// Update Work Block
	updateBody := map[string]any{
		"north_star_metric": "Profit",
	}
	b, _ = json.Marshal(updateBody)
	req = httptest.NewRequest("PATCH", "/api/v1/work-blocks/"+wb.ID, bytes.NewReader(b))
	req.SetPathValue("id", wb.ID)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	api.Auth(api.UpdateWorkBlock)(rr, req)

	json.NewDecoder(rr.Body).Decode(&wb)
	if wb.NorthStarMetric != "Profit" {
		t.Errorf("got metric %q, want %q", wb.NorthStarMetric, "Profit")
	}
}
