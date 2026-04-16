package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"github.com/msoedov/mesa/internal/models"
	"github.com/msoedov/mesa/internal/templates"
)

func TestStrategyPageAlignment(t *testing.T) {
	d := testDB(t)
	sse := NewSSEHub()
	defer sse.Close()
	tmpl, _ := templates.Parse()
	ui := NewUI(d, sse, tmpl, nil, nil)

	// Create an Apex Block
	ab := &models.ApexBlock{
		ID:     "apex-1",
		Title:  "Strategic Goal",
		Goal:   "Test Goal",
		Status: "active",
	}
	d.CreateApexBlock(ab)

	// Create 2 Work Blocks: 1 aligned, 1 not
	wb1 := &models.WorkBlock{
		ID:          "wb-1",
		Title:       "Aligned Block",
		Status:      models.WBStatusProposed,
		ApexBlockID: &ab.ID,
	}
	d.CreateWorkBlock(wb1)

	wb2 := &models.WorkBlock{
		ID:          "wb-2",
		Title:       "Not Aligned Block",
		Status:      models.WBStatusProposed,
		ApexBlockID: nil,
	}
	d.CreateWorkBlock(wb2)

	// Test Strategy Page
	req := httptest.NewRequest("GET", "/strategy", nil)
	rr := httptest.NewRecorder()
	ui.StrategyPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	// Should show 50% alignment
	if !strings.Contains(body, "50%") {
		t.Errorf("expected 50%% alignment score in response body")
	}

	if !strings.Contains(body, "Aligned Block") {
		t.Errorf("expected Aligned Block title in response body")
	}

	if !strings.Contains(body, "Strategic Goal") {
		t.Errorf("expected Strategic Goal title in response body")
	}
}

func TestStrategyCreateApexBlock(t *testing.T) {
	d := testDB(t)
	sse := NewSSEHub()
	defer sse.Close()
	tmpl, _ := templates.Parse()
	ui := NewUI(d, sse, tmpl, nil, nil)

	form := "title=New+Apex&goal=New+Goal"
	req := httptest.NewRequest("POST", "/strategy", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	ui.StrategyPage(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusSeeOther)
	}

	blocks, _ := d.ListApexBlocks()
	if len(blocks) != 1 {
		t.Errorf("expected 1 apex block, got %d", len(blocks))
	}
	if blocks[0].Title != "New Apex" {
		t.Errorf("expected title New Apex, got %q", blocks[0].Title)
	}
}
