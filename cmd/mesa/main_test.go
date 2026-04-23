package main

import (
	"fmt"
	"testing"

	"github.com/msoedov/mesa/internal/archetypes"
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

func TestSMMAndMediaTemplatesSeed(t *testing.T) {
	cases := []struct {
		template      string
		requiredSlugs map[string]string // agent slug -> archetype slug
	}{
		{
			template: "smm",
			requiredSlugs: map[string]string{
				"social-lead":       "social-media",
				"community-manager": "social-media",
				"copywriter":        "marketing",
			},
		},
		{
			template: "media",
			requiredSlugs: map[string]string{
				"pr-lead":               "pr",
				"communications-writer": "pr",
				"media-relations":       "pr",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.template, func(t *testing.T) {
			database, err := db.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			t.Cleanup(func() { database.Close() })

			applyStartupTemplate(database, tc.template, "claude", "")

			agents, err := database.ListAgents()
			if err != nil {
				t.Fatalf("list agents: %v", err)
			}
			bySlug := map[string]*models.Agent{}
			for i := range agents {
				bySlug[agents[i].Slug] = &agents[i]
			}

			for slug, archetype := range tc.requiredSlugs {
				a, ok := bySlug[slug]
				if !ok {
					t.Fatalf("template %q missing agent %q", tc.template, slug)
				}
				if a.ArchetypeSlug != archetype {
					t.Fatalf("agent %q archetype = %q, want %q", slug, a.ArchetypeSlug, archetype)
				}
				if !archetypes.Exists(archetype) {
					t.Fatalf("archetype %q not embedded", archetype)
				}
			}
		})
	}
}
