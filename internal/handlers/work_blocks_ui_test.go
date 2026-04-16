package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/msoedov/mesa/internal/models"
	"github.com/msoedov/mesa/internal/templates"
)

func newTestUI(t *testing.T) (*UI, *SSEHub) {
	t.Helper()

	d := testDB(t)
	sse := NewSSEHub()
	t.Cleanup(sse.Close)

	tmpl, err := templates.Parse()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	return NewUI(d, sse, tmpl, nil, nil), sse
}

func TestListWorkBlocksRendersNorthStarInputs(t *testing.T) {
	ui, _ := newTestUI(t)

	req := httptest.NewRequest(http.MethodGet, "/work-blocks", nil)
	rr := httptest.NewRecorder()

	ui.ListWorkBlocks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	for _, snippet := range []string{
		`name="north_star_metric"`,
		`name="north_star_target"`,
		`North Star Metric`,
		`North Star Target`,
	} {
		if !strings.Contains(body, snippet) {
			t.Errorf("response missing %q", snippet)
		}
	}
}

func TestWorkBlockDetailRendersNorthStarDisplayAndEditInputs(t *testing.T) {
	ui, _ := newTestUI(t)

	wb := &models.WorkBlock{
		Title:           "Stabilize queue workers",
		Goal:            "Reduce retries during peak load",
		Status:          models.WBStatusProposed,
		NorthStarMetric: "Avg runs per issue",
		NorthStarTarget: "< 8",
	}
	if err := ui.db.CreateWorkBlock(wb); err != nil {
		t.Fatalf("create work block: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/work-blocks/"+wb.ID, nil)
	req.SetPathValue("id", wb.ID)
	rr := httptest.NewRecorder()

	ui.WorkBlockDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	for _, snippet := range []string{
		`North Star:`,
		`Avg runs per issue`,
		`&lt; 8`,
		`name="north_star_metric" value="Avg runs per issue"`,
		`name="north_star_target" value="&lt; 8"`,
	} {
		if !strings.Contains(body, snippet) {
			t.Errorf("response missing %q", snippet)
		}
	}
}

func TestWorkBlockDetailHidesNorthStarDisplayWhenMetricEmpty(t *testing.T) {
	ui, _ := newTestUI(t)

	wb := &models.WorkBlock{
		Title:  "Polish release checklist",
		Goal:   "Tighten the final QA pass",
		Status: models.WBStatusProposed,
	}
	if err := ui.db.CreateWorkBlock(wb); err != nil {
		t.Fatalf("create work block: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/work-blocks/"+wb.ID, nil)
	req.SetPathValue("id", wb.ID)
	rr := httptest.NewRecorder()

	ui.WorkBlockDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	if strings.Contains(rr.Body.String(), `North Star:`) {
		t.Fatal("response unexpectedly rendered north star summary")
	}
}
