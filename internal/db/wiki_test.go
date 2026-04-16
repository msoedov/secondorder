package db

import (
	"database/sql"
	"testing"
	"time"

	"github.com/msoedov/mesa/internal/models"
)

// --- Wiki Pages CRUD Tests ---

func makeWikiPage(slug string, title string) *models.WikiPage {
	return &models.WikiPage{
		Slug:    slug,
		Title:   title,
		Content: "Content for " + title,
	}
}

func TestCreateWikiPage(t *testing.T) {
	d := testDB(t)
	p := makeWikiPage("home", "Home")

	if err := d.CreateWikiPage(p); err != nil {
		t.Fatalf("create wiki page: %v", err)
	}

	if p.ID == "" {
		t.Fatal("expected ID to be set after create")
	}
	if p.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if p.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestCreateWikiPageWithAgent(t *testing.T) {
	d := testDB(t)
	a := makeAgent("wiki-agent")
	d.CreateAgent(a)

	p := makeWikiPage("faq", "FAQ")
	p.CreatedByAgentID = &a.ID
	p.UpdatedByAgentID = &a.ID

	if err := d.CreateWikiPage(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := d.GetWikiPageBySlug("faq")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CreatedByAgentID == nil || *got.CreatedByAgentID != a.ID {
		t.Error("expected CreatedByAgentID to be set")
	}
}

func TestGetWikiPage(t *testing.T) {
	d := testDB(t)
	p := makeWikiPage("docs", "Documentation")
	d.CreateWikiPage(p)

	got, err := d.GetWikiPageBySlug("docs")
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	if got.Title != "Documentation" {
		t.Errorf("title = %q, want Documentation", got.Title)
	}
	if got.Slug != "docs" {
		t.Errorf("slug = %q, want docs", got.Slug)
	}
	if got.Content != "Content for Documentation" {
		t.Errorf("content = %q", got.Content)
	}
}

func TestGetWikiPageNotFound(t *testing.T) {
	d := testDB(t)
	_, err := d.GetWikiPageBySlug("nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestListWikiPages(t *testing.T) {
	d := testDB(t)
	p1 := makeWikiPage("guide", "Guide")
	p2 := makeWikiPage("tutorial", "Tutorial")
	p3 := makeWikiPage("api-ref", "API Reference")

	d.CreateWikiPage(p1)
	d.CreateWikiPage(p2)
	d.CreateWikiPage(p3)

	pages, err := d.ListWikiPages()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pages) != 3 {
		t.Errorf("got %d pages, want 3", len(pages))
	}

	// Verify ordered by updated_at DESC (most recent first)
	if pages[0].Slug != "api-ref" {
		t.Errorf("first page = %q, want api-ref (most recent)", pages[0].Slug)
	}
}

func TestListWikiPagesEmpty(t *testing.T) {
	d := testDB(t)
	pages, err := d.ListWikiPages()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
}

func TestUpdateWikiPage(t *testing.T) {
	d := testDB(t)
	p := makeWikiPage("install", "Installation")
	d.CreateWikiPage(p)

	// Get original updated time
	originalPage, _ := d.GetWikiPageBySlug("install")
	originalUpdatedAt := originalPage.UpdatedAt

	time.Sleep(10 * time.Millisecond) // Ensure time difference

	// Update the page
	p.Title = "Installation Guide"
	p.Content = "New content for installation"
	if err := d.UpdateWikiPage(p); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := d.GetWikiPageBySlug("install")
	if got.Title != "Installation Guide" {
		t.Errorf("title = %q, want Installation Guide", got.Title)
	}
	if got.Content != "New content for installation" {
		t.Errorf("content = %q", got.Content)
	}
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Error("expected UpdatedAt to be updated")
	}
}

func TestUpdateWikiPageWithAgentID(t *testing.T) {
	d := testDB(t)
	a1 := makeAgent("agent1")
	a2 := makeAgent("agent2")
	d.CreateAgent(a1)
	d.CreateAgent(a2)

	p := makeWikiPage("shared", "Shared Doc")
	p.CreatedByAgentID = &a1.ID
	p.UpdatedByAgentID = &a1.ID
	d.CreateWikiPage(p)

	// Verify initial state
	before, _ := d.GetWikiPageBySlug("shared")
	if before.CreatedByAgentID == nil || *before.CreatedByAgentID != a1.ID {
		t.Error("expected CreatedByAgentID to be set")
	}

	// Update with different agent
	p.UpdatedByAgentID = &a2.ID
	if err := d.UpdateWikiPage(p); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := d.GetWikiPageBySlug("shared")
	if got.CreatedByAgentID == nil || *got.CreatedByAgentID != a1.ID {
		t.Error("expected CreatedByAgentID to be preserved")
	}
	if got.UpdatedByAgentID == nil || *got.UpdatedByAgentID != a2.ID {
		t.Error("expected UpdatedByAgentID to be updated")
	}
}

func TestDeleteWikiPage(t *testing.T) {
	d := testDB(t)
	p := makeWikiPage("temp", "Temporary")
	d.CreateWikiPage(p)

	// Verify it exists
	_, err := d.GetWikiPageBySlug("temp")
	if err != nil {
		t.Fatalf("page should exist before delete: %v", err)
	}

	// Delete it
	if err := d.DeleteWikiPage(p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify it's gone
	_, err = d.GetWikiPageBySlug("temp")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows after delete, got %v", err)
	}
}

func TestSlugUniquenessConstraint(t *testing.T) {
	d := testDB(t)
	p1 := makeWikiPage("unique", "First")
	p2 := makeWikiPage("unique", "Second")

	if err := d.CreateWikiPage(p1); err != nil {
		t.Fatalf("create first: %v", err)
	}

	// Attempting to create second with same slug should fail
	if err := d.CreateWikiPage(p2); err == nil {
		t.Fatal("expected error for duplicate slug")
	}
}

func TestWikiPageTimestamps(t *testing.T) {
	d := testDB(t)
	before := time.Now().UTC()
	p := makeWikiPage("timestamps", "Timestamps Test")
	d.CreateWikiPage(p)
	after := time.Now().UTC()

	got, _ := d.GetWikiPageBySlug("timestamps")
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Error("CreatedAt is outside expected range")
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Error("UpdatedAt is outside expected range")
	}
	if !got.CreatedAt.Equal(got.UpdatedAt) {
		t.Error("expected CreatedAt and UpdatedAt to be equal on creation")
	}
}

func TestWikiPageEmptyContent(t *testing.T) {
	d := testDB(t)
	p := makeWikiPage("empty", "Empty Page")
	p.Content = ""

	if err := d.CreateWikiPage(p); err != nil {
		t.Fatalf("create with empty content: %v", err)
	}

	got, _ := d.GetWikiPageBySlug("empty")
	if got.Content != "" {
		t.Errorf("expected empty content, got %q", got.Content)
	}
}

func TestCreateAndUpdateWikiPage(t *testing.T) {
	d := testDB(t)
	p := makeWikiPage("workflow", "Workflow")
	d.CreateWikiPage(p)

	// Update multiple times
	for i := 1; i <= 3; i++ {
		p.Title = "Workflow version " + string(rune(48+i))
		p.Content = "Content version " + string(rune(48+i))
		d.UpdateWikiPage(p)
	}

	got, _ := d.GetWikiPageBySlug("workflow")
	if got.Title != "Workflow version 3" {
		t.Errorf("title = %q, want Workflow version 3", got.Title)
	}
	if got.Content != "Content version 3" {
		t.Errorf("content = %q, want Content version 3", got.Content)
	}
}

func TestListWikiPageSummaries(t *testing.T) {
	d := testDB(t)
	p1 := makeWikiPage("summary-a", "Summary A")
	p2 := makeWikiPage("summary-b", "Summary B")

	d.CreateWikiPage(p1)
	d.CreateWikiPage(p2)

	summaries, err := d.ListWikiPageSummaries()
	if err != nil {
		t.Fatalf("list summaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("got %d summaries, want 2", len(summaries))
	}

	for _, s := range summaries {
		if s.ID == "" {
			t.Error("expected ID to be set")
		}
		if s.Slug == "" {
			t.Error("expected Slug to be set")
		}
		if s.Title == "" {
			t.Error("expected Title to be set")
		}
		if s.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be set")
		}
	}
}

// --- FTS5 Full-Text Search Tests ---

func TestSearchWikiPagesFTS(t *testing.T) {
	d := testDB(t)

	pages := []*models.WikiPage{
		{Slug: "deploy-runbook", Title: "Deployment Runbook", Content: "Step-by-step guide to deploying the application"},
		{Slug: "api-auth", Title: "API Authentication", Content: "How to authenticate with API keys and tokens"},
		{Slug: "onboarding", Title: "New Agent Onboarding", Content: "Getting started guide for new agents"},
	}
	for _, p := range pages {
		if err := d.CreateWikiPage(p); err != nil {
			t.Fatalf("create page %q: %v", p.Slug, err)
		}
	}

	tests := []struct {
		query   string
		wantMin int
		wantMax int
	}{
		{`"deploy"*`, 1, 1},
		{`"api"*`, 1, 1},
		{`"guide"*`, 2, 3},
		{`"nonexistent"*`, 0, 0},
	}

	for _, tt := range tests {
		results, err := d.SearchWikiPagesFTS(tt.query, 10)
		if err != nil {
			t.Fatalf("query=%q: %v", tt.query, err)
		}
		if len(results) < tt.wantMin || len(results) > tt.wantMax {
			t.Errorf("query=%q: got %d results, want %d-%d", tt.query, len(results), tt.wantMin, tt.wantMax)
		}
	}
}

func TestSearchWikiPagesFTSUpdatedContent(t *testing.T) {
	d := testDB(t)

	p := &models.WikiPage{Slug: "evolving", Title: "Evolving Page", Content: "Original content about databases"}
	if err := d.CreateWikiPage(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	results, err := d.SearchWikiPagesFTS(`"databases"*`, 10)
	if err != nil {
		t.Fatalf("search before update: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result before update, got %d", len(results))
	}

	p.Content = "Updated content about kubernetes"
	if err := d.UpdateWikiPage(p); err != nil {
		t.Fatalf("update: %v", err)
	}

	results, err = d.SearchWikiPagesFTS(`"databases"*`, 10)
	if err != nil {
		t.Fatalf("search after update: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for old content after update, got %d", len(results))
	}

	results, err = d.SearchWikiPagesFTS(`"kubernetes"*`, 10)
	if err != nil {
		t.Fatalf("search new content: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for new content, got %d", len(results))
	}
}

func TestSearchWikiPagesFTSDeletedContent(t *testing.T) {
	d := testDB(t)

	p := &models.WikiPage{Slug: "ephemeral", Title: "Ephemeral Page", Content: "Temporary knowledge base entry"}
	if err := d.CreateWikiPage(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	results, err := d.SearchWikiPagesFTS(`"ephemeral"*`, 10)
	if err != nil {
		t.Fatalf("search before delete: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result before delete, got %d", len(results))
	}

	if err := d.DeleteWikiPage(p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	results, err = d.SearchWikiPagesFTS(`"ephemeral"*`, 10)
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}
