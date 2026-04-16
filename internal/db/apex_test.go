package db

import (
	"testing"
	"github.com/msoedov/mesa/internal/models"
)

func TestApexBlockCRUD(t *testing.T) {
	d := testDB(t)
	ab := &models.ApexBlock{
		Title: "Test Apex Block",
		Goal:  "Test Goal",
	}

	if err := d.CreateApexBlock(ab); err != nil {
		t.Fatalf("create apex block: %v", err)
	}

	if ab.ID == "" {
		t.Fatal("expected ID to be set")
	}

	got, err := d.GetApexBlock(ab.ID)
	if err != nil {
		t.Fatalf("get apex block: %v", err)
	}
	if got.Title != ab.Title {
		t.Errorf("got title %q, want %q", got.Title, ab.Title)
	}

	blocks, err := d.ListApexBlocks()
	if err != nil {
		t.Fatalf("list apex blocks: %v", err)
	}
	if len(blocks) != 1 {
		t.Errorf("got %d blocks, want 1", len(blocks))
	}

	ab.Title = "Updated Title"
	if err := d.UpdateApexBlock(ab); err != nil {
		t.Fatalf("update apex block: %v", err)
	}

	got, _ = d.GetApexBlock(ab.ID)
	if got.Title != "Updated Title" {
		t.Errorf("got title %q, want %q", got.Title, "Updated Title")
	}

	if err := d.DeleteApexBlock(ab.ID); err != nil {
		t.Fatalf("delete apex block: %v", err)
	}

	_, err = d.GetApexBlock(ab.ID)
	if err == nil {
		t.Fatal("expected error getting deleted block")
	}
}

func TestWorkBlockWithApex(t *testing.T) {
	d := testDB(t)
	ab := &models.ApexBlock{
		Title: "Apex",
		Goal:  "Goal",
	}
	d.CreateApexBlock(ab)

	wb := &models.WorkBlock{
		Title:           "Work Block",
		Goal:            "Goal",
		NorthStarMetric: "Metric",
		NorthStarTarget: "Target",
		ApexBlockID:     &ab.ID,
	}

	if err := d.CreateWorkBlock(wb); err != nil {
		t.Fatalf("create work block: %v", err)
	}

	got, err := d.GetWorkBlock(wb.ID)
	if err != nil {
		t.Fatalf("get work block: %v", err)
	}

	if got.NorthStarMetric != "Metric" {
		t.Errorf("got metric %q, want %q", got.NorthStarMetric, "Metric")
	}
	if got.ApexBlockID == nil || *got.ApexBlockID != ab.ID {
		t.Errorf("got apex block id %v, want %q", got.ApexBlockID, ab.ID)
	}

	// Test update
	got.NorthStarMetric = "New Metric"
	if err := d.UpdateWorkBlock(got); err != nil {
		t.Fatalf("update work block: %v", err)
	}

	updated, _ := d.GetWorkBlock(wb.ID)
	if updated.NorthStarMetric != "New Metric" {
		t.Errorf("got metric %q, want %q", updated.NorthStarMetric, "New Metric")
	}
}
