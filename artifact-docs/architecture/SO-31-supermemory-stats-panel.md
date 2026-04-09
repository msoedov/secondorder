# SO-31 — Supermemory Efficiency & Stats Panel
## Architecture Design Document
_Date: 2026-04-09 | Author: Principal Engineer (Architect) | Status: **FINAL — Ready for Implementation**_

---

## 1. Problem Statement

Agents call `supermemory_store` and `supermemory_recall` during task execution via the Copilot runner, but there is no observability layer. The dashboard has no metrics on:
- How many memories each agent has stored vs recalled
- Whether recalls are finding useful context (hit rate)
- Whether supermemory usage is growing or improving over time

**Acceptance criteria require:** stats that reflect **actual API calls** (no synthetic/estimated numbers), per-agent breakdown, recall hit-rate, and trend over time.

---

## 2. Where Supermemory Calls Happen Today

| Runner | How supermemory tools are invoked | Interceptable by secondorder? |
|--------|-----------------------------------|-------------------------------|
| `copilot.go` | `executeTool()` intercepts tool calls and makes HTTP requests to `api.supermemory.ai` | **YES — full control** |
| `claude_code` | MCP supermemory plugin (external subprocess) | NO — opaque subprocess |
| `gemini`, `codex`, others | MCP supermemory plugin (external subprocess) | NO — opaque subprocess |

**Conclusion:** We can only instrument the Copilot runner without external log scraping. For all other runners, we have no reliable signal. Stats for non-Copilot agents will show zeros — this is architecturally honest and preferable to synthetic estimates.

---

## 3. Key Design Decisions

### 3.1 Primary instrumentation: DB event table in copilot.go

Instrument `executeTool()` in `copilot.go` to write a row to `supermemory_events` after every `supermemory_store` or `supermemory_recall` call completes.

**Why not a log file?** The DB is the single source of truth for all agent metrics. Consistent with `cost_events`, `activity_log`, and `run_events` patterns.

**Why not query the Supermemory API from the dashboard?** The acceptance criteria say "stats must reflect actual API calls". Querying the Supermemory API from the dashboard would add an external dependency, latency, and failure risk. Instrumenting at the call site is more reliable and offline-capable.

### 3.2 Recall hit-rate definition

A recall is a **hit** when:
1. `supermemory_recall` was called, AND
2. The response returned `result_count > 0` (at least one memory found), AND
3. `success = 1` (the API call itself succeeded)

**Hit-rate = `recall_hits / total_recalls * 100`**

> **Attribution caveat (documented for transparency):** Full attribution — "did the LLM actually use the recall content in its reasoning?" — is not deterministically measurable without LLM introspection. The agreed proxy is: *a recall that returned ≥1 result is counted as a hit.* This is conservative and objective.

### 3.3 Trend: daily aggregates via SQL

No separate time-series store. The `supermemory_events` table has a `created_at` timestamp. Trend data is derived by grouping `DATE(created_at)` using SQLite CTEs — exactly the same pattern as `GetDailyActivityStats`.

### 3.4 Dashboard panel placement

The panel renders at the bottom of `dashboard.html` as a full-width section with per-agent cards and a 7-day bar chart. It uses HTMX `run-complete from:body` trigger (same as `#dashboard-stats` and `#dashboard-agents`).

---

## 4. Data Model

### Existing table: `supermemory_events` (migration `017_supermemory_events.sql`)

```sql
CREATE TABLE IF NOT EXISTS supermemory_events (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    run_id       TEXT NOT NULL,
    event_type   TEXT NOT NULL DEFAULT 'recall',  -- 'store' or 'recall'
    query        TEXT NOT NULL DEFAULT '',         -- recall query (empty for store)
    result_count INTEGER NOT NULL DEFAULT 0,       -- result count for recall; 1 = success for store
    success      INTEGER NOT NULL DEFAULT 1,       -- 1 = API call succeeded, 0 = error
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agents(id),
    FOREIGN KEY (run_id)   REFERENCES runs(id)
);

CREATE INDEX IF NOT EXISTS idx_supermemory_agent ON supermemory_events(agent_id);
CREATE INDEX IF NOT EXISTS idx_supermemory_created ON supermemory_events(created_at);
```

> ✅ Migration file already present. No new migration needed.

### Per-agent stats query

```sql
SELECT
    se.agent_id,
    COALESCE(a.name, '') AS agent_name,
    COALESCE(a.slug, '') AS agent_slug,
    COUNT(*) FILTER (WHERE se.event_type = 'store') AS stores,
    COUNT(*) FILTER (WHERE se.event_type = 'recall') AS recalls,
    COUNT(*) FILTER (WHERE se.event_type = 'recall' AND se.result_count > 0 AND se.success = 1) AS recall_hits,
    ROUND(
        100.0 * COUNT(*) FILTER (WHERE se.event_type = 'recall' AND se.result_count > 0 AND se.success = 1)
              / NULLIF(COUNT(*) FILTER (WHERE se.event_type = 'recall'), 0),
        1
    ) AS hit_rate_pct
FROM supermemory_events se
LEFT JOIN agents a ON a.id = se.agent_id
GROUP BY se.agent_id
ORDER BY recalls DESC
```

### 7-day trend query

Uses a SQLite CTE to generate all 7 days (even if no events), joining against actual data:

```sql
WITH RECURSIVE days(d) AS (
    SELECT DATE('now', '-6 days')
    UNION ALL
    SELECT DATE(d, '+1 day') FROM days WHERE d < DATE('now')
)
SELECT
    d AS date,
    COALESCE(SUM(CASE WHEN event_type='store'  THEN 1 ELSE 0 END), 0) AS stores,
    COALESCE(SUM(CASE WHEN event_type='recall' THEN 1 ELSE 0 END), 0) AS recalls
FROM days
LEFT JOIN supermemory_events ON DATE(created_at) = d
GROUP BY d
ORDER BY d
```

---

## 5. Component Map

```
[Agent (Copilot runner)]
    ↓ calls supermemory_store / supermemory_recall
[scheduler/copilot.go  executeTool(name, argsJSON, workingDir, agentID, runID string, db *db.DB)]
    ↓ after each supermemory call completes
[db.LogSupermemoryEvent(agentID, runID, eventType, query string, resultCount int, success bool) error]
    ↓ inserts row into
[supermemory_events table (SQLite)]
    ↓ queried at dashboard load
[db.GetSupermemoryStats() ([]models.SupermemoryAgentStat, error)]
[db.GetSupermemoryTrend(days int) ([]models.SupermemoryDailyStat, error)]
    ↓ passed to template
[handlers/ui.go  Dashboard()]  →  data["SupermemoryStats"], data["SupermemoryTrend"]
    ↓ rendered in
[templates/dashboard.html  #supermemory-panel]
```

---

## 6. New Model Types

Add to `internal/models/models.go`:

```go
// SupermemoryAgentStat holds per-agent supermemory usage stats.
// Stats are derived from the supermemory_events table and reflect
// actual API calls made by the Copilot runner only.
type SupermemoryAgentStat struct {
    AgentID    string  `json:"agent_id"`
    AgentName  string  `json:"agent_name"`
    AgentSlug  string  `json:"agent_slug"`
    Stores     int     `json:"stores"`
    Recalls    int     `json:"recalls"`
    RecallHits int     `json:"recall_hits"` // recalls with result_count > 0 AND success=1
    HitRatePct float64 `json:"hit_rate_pct"` // 0.0–100.0; -1 if no recalls yet
}

// SupermemoryDailyStat holds daily aggregate counts for the trend chart.
type SupermemoryDailyStat struct {
    Date    string `json:"date"`  // "2026-04-09"
    Label   string `json:"label"` // "Apr 9"
    Stores  int    `json:"stores"`
    Recalls int    `json:"recalls"`
}
```

---

## 7. New DB Methods

Add to `internal/db/queries.go`:

### `LogSupermemoryEvent`
```go
func (d *DB) LogSupermemoryEvent(agentID, runID, eventType, query string, resultCount int, success bool) error
```
- Inserts one row into `supermemory_events`
- Called from `executeTool()` after each supermemory API call returns
- `success` = true when HTTP status < 300; false on error or non-2xx

### `GetSupermemoryStats`
```go
func (d *DB) GetSupermemoryStats() ([]models.SupermemoryAgentStat, error)
```
- Returns per-agent aggregation across all time
- Returns empty slice (not nil) when no rows exist
- `HitRatePct = -1.0` when `Recalls == 0` (distinguish "no data" from 0% hit-rate)

### `GetSupermemoryTrend`
```go
func (d *DB) GetSupermemoryTrend(days int) ([]models.SupermemoryDailyStat, error)
```
- Returns exactly `days` rows (one per calendar day, newest last)
- Missing days have `Stores=0, Recalls=0`
- `Label` is formatted as `"Jan 2"` (time.Format `"Jan 2"`)

---

## 8. Scheduler Changes: `executeTool()` Refactor

**Current signature:**
```go
func executeTool(name, argsJSON, workingDir string) string
```

**New signature:**
```go
func executeTool(name, argsJSON, workingDir, agentID, runID string, database *db.DB) string
```

**Call site in `execCopilot()`** — update the existing loop:
```go
result := executeTool(tc.Function.Name, tc.Function.Arguments, agent.WorkingDir, agent.ID, runID, s.db)
```

**Instrumentation logic** (inside `executeTool`, after each supermemory case returns a result):

For `supermemory_store`:
```go
success := !strings.HasPrefix(result, "error") && !strings.HasPrefix(result, "supermemory error")
if database != nil {
    database.LogSupermemoryEvent(agentID, runID, "store", "", 1, success)
}
```

For `supermemory_recall`:
```go
// Parse result count from the "Found N memories" prefix
resultCount := parseRecallCount(result) // returns 0 if "no memories found"
success := !strings.HasPrefix(result, "error") && !strings.HasPrefix(result, "supermemory error")
if database != nil {
    database.LogSupermemoryEvent(agentID, runID, "recall", query, resultCount, success)
}
```

Helper:
```go
// parseRecallCount extracts N from "Found N memories:\n\n..."
// Returns 0 if result starts with "no memories" or on parse failure.
func parseRecallCount(result string) int {
    if strings.HasPrefix(result, "no memories found") {
        return 0
    }
    // "Found N memories:\n"
    var n int
    fmt.Sscanf(result, "Found %d memories", &n)
    return n
}
```

> **Backward compat:** Pass `nil` for `database` in unit tests that don't need DB. The nil-guard prevents panics.

---

## 9. Dashboard Handler Changes

In `handlers/ui.go`, `Dashboard()`:

```go
supermemoryStats, _ := u.db.GetSupermemoryStats()
supermemoryTrend, _ := u.db.GetSupermemoryTrend(7)

data := map[string]any{
    // ... existing keys ...
    "SupermemoryStats": supermemoryStats,
    "SupermemoryTrend": supermemoryTrend,
}
```

---

## 10. Dashboard Template Changes

Add `#supermemory-panel` below the existing agents section in `templates/dashboard.html`:

```html
<!-- Supermemory Stats Panel -->
<div id="supermemory-panel" class="mt-8"
     hx-get="/dashboard" hx-select="#supermemory-panel"
     hx-target="#supermemory-panel" hx-swap="outerHTML"
     hx-trigger="run-complete from:body">

  <div class="flex items-center justify-between mb-3">
    <h2 class="text-[13px] font-medium text-ink2">Supermemory</h2>
    <span class="text-xs text-ink3/50">Copilot runner only · recall hit = ≥1 result returned</span>
  </div>

  {{if .SupermemoryStats}}
  <!-- Per-agent cards -->
  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 mb-4 stagger">
    {{range .SupermemoryStats}}
    <a href="/agents/{{.AgentSlug}}" class="bg-card border border-bd rounded-lg p-4 hover:border-bd transition-colors">
      <div class="text-[11px] font-medium text-ink3/50 truncate mb-2">{{.AgentName}}</div>
      <div class="grid grid-cols-3 gap-2 text-center">
        <div>
          <div class="text-xl font-semibold text-ink tabular-nums">{{.Stores}}</div>
          <div class="text-[10px] text-ink3/50 uppercase tracking-wider">Stored</div>
        </div>
        <div>
          <div class="text-xl font-semibold text-ink tabular-nums">{{.Recalls}}</div>
          <div class="text-[10px] text-ink3/50 uppercase tracking-wider">Recalled</div>
        </div>
        <div>
          {{if lt .HitRatePct 0.0}}
          <div class="text-xl font-semibold text-ink3/40 tabular-nums">—</div>
          {{else if ge .HitRatePct 70.0}}
          <div class="text-xl font-semibold text-emerald-400 tabular-nums">{{printf "%.0f" .HitRatePct}}%</div>
          {{else if ge .HitRatePct 40.0}}
          <div class="text-xl font-semibold text-amber-400 tabular-nums">{{printf "%.0f" .HitRatePct}}%</div>
          {{else}}
          <div class="text-xl font-semibold text-red-400 tabular-nums">{{printf "%.0f" .HitRatePct}}%</div>
          {{end}}
          <div class="text-[10px] text-ink3/50 uppercase tracking-wider">Hit Rate</div>
        </div>
      </div>
    </a>
    {{end}}
  </div>

  <!-- 7-day trend bar chart -->
  {{if .SupermemoryTrend}}
  <div class="bg-card border border-bd rounded-lg p-4">
    <div class="text-[11px] font-medium text-ink3/50 uppercase tracking-wider mb-3">7-Day Trend</div>
    <div class="flex items-end gap-1.5 h-16">
      {{$maxVal := 1}}
      {{range .SupermemoryTrend}}{{if gt (add .Stores .Recalls) $maxVal}}{{$maxVal = add .Stores .Recalls}}{{end}}{{end}}
      {{range .SupermemoryTrend}}
      <div class="flex-1 flex flex-col items-center gap-0.5" title="{{.Label}}: {{.Stores}} stored, {{.Recalls}} recalled">
        <div class="w-full flex flex-col justify-end gap-px" style="height:48px">
          {{if gt .Recalls 0}}
          <div class="w-full rounded-sm bg-ac/70" style="height:{{percent .Recalls $maxVal 48}}px"></div>
          {{end}}
          {{if gt .Stores 0}}
          <div class="w-full rounded-sm bg-ac/30" style="height:{{percent .Stores $maxVal 48}}px"></div>
          {{end}}
        </div>
        <div class="text-[9px] text-ink3/40 truncate w-full text-center">{{shortDate .Label}}</div>
      </div>
      {{end}}
    </div>
    <div class="flex gap-3 mt-2">
      <span class="flex items-center gap-1 text-[10px] text-ink3/50"><span class="w-2 h-2 rounded-sm bg-ac/70 inline-block"></span>Recalls</span>
      <span class="flex items-center gap-1 text-[10px] text-ink3/50"><span class="w-2 h-2 rounded-sm bg-ac/30 inline-block"></span>Stores</span>
    </div>
  </div>
  {{end}}

  {{else}}
  <div class="bg-card border border-bd rounded-lg p-6 text-center text-sm text-ink3/50">
    No supermemory activity recorded yet. Stats appear when Copilot-runner agents use supermemory_store or supermemory_recall.
  </div>
  {{end}}
</div>
```

### Required template helper functions

The dashboard template uses `add`, `percent`, and `shortDate` helpers. Add to `internal/templates/templates.go`:

```go
"add": func(a, b int) int { return a + b },
"percent": func(val, max, scale int) int {
    if max == 0 { return 0 }
    v := val * scale / max
    if v < 1 && val > 0 { return 1 } // always show nonzero as at least 1px
    return v
},
"shortDate": func(label string) string {
    // "Apr 9" → "9", "Apr 10" → "10"
    parts := strings.Fields(label)
    if len(parts) == 2 { return parts[1] }
    return label
},
```

---

## 11. Test Coverage Required

The test file `internal/db/supermemory_test.go` is already written and covers:
- `LogSupermemoryEvent` for store + recall hit + recall miss
- `GetSupermemoryStats` — verifies stores=1, recalls=2, recall_hits=1, hit_rate_pct=50.0
- `GetSupermemoryTrend(7)` — verifies 7 rows returned, today has correct counts

Implementation must make `TestSupermemoryEventsRoundtrip` pass with `go test ./internal/db/...`.

---

## 12. What This Does NOT Cover

| Gap | Rationale |
|-----|-----------|
| Claude Code / Gemini / Codex runners | Supermemory calls go through MCP — not interceptable. Excluded for accuracy. |
| MCP-native recall scoring | Requires LLM introspection. Out of scope. |
| Supermemory API document count | Outbound API call at load time — adds latency+failure risk. Excluded. |
| Memory content viewer | Requires Supermemory API search. Future enhancement. |
| Per-run supermemory breakdown | Available in data model; not exposed in v1 UI. |

---

## 13. Implementation Sub-Issues

| # | Title | Owner | Depends on |
|---|-------|-------|------------|
| SO-31-A | `models.go`: Add SupermemoryAgentStat, SupermemoryDailyStat | DevOps/Backend | — |
| SO-31-B | `db/queries.go`: Implement LogSupermemoryEvent, GetSupermemoryStats, GetSupermemoryTrend | DevOps/Backend | SO-31-A |
| SO-31-C | `scheduler/copilot.go`: Refactor executeTool(), add instrumentation | DevOps/Backend | SO-31-B |
| SO-31-D | `handlers/ui.go` + `dashboard.html`: Add supermemory panel | DevOps/Backend | SO-31-B |

> All sub-issues should be assigned to the DevOps agent or Backend engineer. Each sub-issue must link `parent_issue_key: SO-31`.

---

## 14. Acceptance Criteria Mapping

| AC | Design element | Met? |
|----|----------------|------|
| Panel visible in dashboard alongside existing metrics | `#supermemory-panel` below Agents section | ✅ |
| Per-agent: stores, recalls, hit-rate, trend | Per-agent cards + 7-day bar chart | ✅ |
| Accuracy: actual API calls only | Instrumented at `executeTool()` call site; other runners show zero (not fabricated) | ✅ |
| Refresh on page load (minimum) | HTMX refresh on `run-complete from:body` + every dashboard page load | ✅ |
| Consistent visual design | Uses `bg-card border-bd rounded-lg`, `text-2xl font-semibold tabular-nums` (same as existing stats) | ✅ |

---

_Architecture approved. Proceed to sub-issue implementation._
