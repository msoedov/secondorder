package main

import (
	"path/filepath"
	"testing"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

func TestApplyStartupTemplateUsesDefaultAgentTimeout(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	applyStartupTemplate(database, "startup", "claude", "")

	agents, err := database.ListAgents()
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) == 0 {
		t.Fatal("expected seeded agents")
	}
	for _, agent := range agents {
		if agent.TimeoutSec != models.DefaultAgentTimeoutSec {
			t.Fatalf("agent %s timeout = %d, want %d", agent.Slug, agent.TimeoutSec, models.DefaultAgentTimeoutSec)
		}
	}
}
