package main

import (
	"fmt"
	"testing"

	"github.com/msoedov/mesa/internal/db"
	"github.com/msoedov/mesa/internal/models"
)

func TestApplyStartupTemplateUsesDefaultAgentTimeout(t *testing.T) {
	database, err := db.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
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
