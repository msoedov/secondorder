package db_test

import (
	"fmt"
	"testing"

	"github.com/msoedov/mesa/internal/db"
)

func TestSupermemoryEventsRoundtrip(t *testing.T) {
	d, err := db.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Seed agent
	_, err = d.Exec(`INSERT INTO agents (id, name, slug, archetype_slug, model, runner, api_key_env, working_dir, max_turns, timeout_sec, heartbeat_enabled, heartbeat_cron, chrome_enabled, active, created_at, updated_at)
		VALUES ('a1','Test Agent','test-agent','other','sonnet','claude_code','','.',50,1200,0,'',0,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}
	// Seed run
	_, err = d.Exec(`INSERT INTO runs (id, agent_id, mode, status, started_at, created_at)
		VALUES ('r1','a1','task','completed',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}

	// Log store event
	if err := d.LogSupermemoryEvent("a1", "r1", "store", "", 1, true); err != nil {
		t.Fatalf("LogSupermemoryEvent store: %v", err)
	}
	// Log recall hit
	if err := d.LogSupermemoryEvent("a1", "r1", "recall", "test query", 3, true); err != nil {
		t.Fatalf("LogSupermemoryEvent recall hit: %v", err)
	}
	// Log recall miss
	if err := d.LogSupermemoryEvent("a1", "r1", "recall", "empty query", 0, true); err != nil {
		t.Fatalf("LogSupermemoryEvent recall miss: %v", err)
	}

	stats, err := d.GetSupermemoryStats()
	if err != nil {
		t.Fatalf("GetSupermemoryStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	s := stats[0]
	if s.Stores != 1 {
		t.Errorf("stores: want 1, got %d", s.Stores)
	}
	if s.Recalls != 2 {
		t.Errorf("recalls: want 2, got %d", s.Recalls)
	}
	if s.RecallHits != 1 {
		t.Errorf("recall_hits: want 1, got %d", s.RecallHits)
	}
	if s.HitRatePct != 50.0 {
		t.Errorf("hit_rate_pct: want 50.0, got %.1f", s.HitRatePct)
	}

	trend, err := d.GetSupermemoryTrend(7)
	if err != nil {
		t.Fatalf("GetSupermemoryTrend: %v", err)
	}
	if len(trend) != 7 {
		t.Fatalf("expected 7 trend days, got %d", len(trend))
	}
	// Today should have 3 events (1 store + 2 recalls)
	today := trend[len(trend)-1]
	if today.Stores != 1 {
		t.Errorf("today stores: want 1, got %d", today.Stores)
	}
	if today.Recalls != 2 {
		t.Errorf("today recalls: want 2, got %d", today.Recalls)
	}
}

