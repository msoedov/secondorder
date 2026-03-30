# UX Audit: The Last Org
Issue: SO-11 | Date: 2026-03-28

## Executive Summary

The Last Org is a dark-mode internal tool built with Go templates + Tailwind CSS. The interaction model is predominantly server-side form POST → redirect. HTMX is barely used (only stdout polling). The design language is consistent and clean. Core gaps are around feedback loops, empty/error states, and missing HTMX opportunities that would make the tool feel faster without a full SPA rewrite.

---

## Page-by-Page Findings

### Dashboard (`/dashboard`)

**What works:**
- Stat cards (4) give a quick at-a-glance overview
- Active work block card is prominent
- Recent issues 2-col grid + agents 1-col sidebar is a reasonable layout

**Issues:**
- Stat cards have no click targets — a dead end for exploration
- "Recent issues" section has no link to full issues list (user must click sidebar nav)
- "No issues yet" empty state has no CTA button to create one
- "No agents configured" empty state has no CTA link to `/agents`
- No live refresh — dashboard goes stale as agents complete runs
- Active work block card shows no run activity or sub-issue count

**P0:**
- Add live SSE refresh for dashboard stats (agent status dots, run count) — already have SSE infrastructure

**P1:**
- Make stat cards clickable links to filtered views
- Add "View all" links on each section heading
- Add CTA buttons to empty states ("Create your first issue", "Add an agent")

**P2:**
- Add a "last updated" timestamp or live indicator to the active work block card

---

### Issues List (`/issues`)

**What works:**
- Tab filter (status) is clear and functional
- Create form is togglable (hidden by default — good)
- Consistent table row layout with status badge + priority + assignee

**Issues:**
- Create form toggle uses inline JS `style.display` — no animation, jarring
- No "clear filter" affordance when a tab filter is active
- Issue rows have no hover state that hints they're clickable
- Priority field in create form is a number input — should be a select with labels ("Urgent", "High", "Normal", "Low")
- No feedback after creating an issue — just redirects back to list
- Table has no column headers (status, key, title, assignee are identifiable by position/badge only)
- No bulk actions (select multiple to assign or close)

**P0:**
- Replace priority number input with labeled `<select>` (Urgent/High/Normal/Low)

**P1:**
- Add column headers to issues table
- Use HTMX to toggle create form with smooth reveal (`hx-swap="show:top"`)
- Add a success toast on issue creation (use `HX-Trigger` response header)

**P2:**
- Add hover background to issue rows to signal clickability
- Keyboard shortcut `N` to open create form

---

### Issue Detail (`/issues/{key}`)

**What works:**
- Breadcrumb navigation is clear
- Status and assignee dropdowns use `onchange="this.form.submit()"` — feels fast
- Comments section is always visible with textarea
- Expandable runs section using `<details>` is a good pattern
- Sub-issues conditional rendering (hidden when empty) is clean

**Issues:**
- Status/assignee changes produce no toast confirmation — user sees a full-page reload with no feedback
- Sub-issues section is absent when empty — no affordance to add one
- Runs section is absent when empty — no message explaining runs start when assigned
- Comment form has no character limit display or validation feedback
- No way to edit the issue title or description inline — read-only fields
- Priority sidebar widget submits full form — slow for a single field change
- Parent issue field is a text input — should search/autocomplete from existing issues
- No "copy issue key" button next to the key in header
- Run list inside `<details>` shows all runs with no pagination — could get long
- No keyboard shortcut to focus comment textarea

**P0:**
- Show sub-issues section even when empty, with "Add sub-issue" inline form
- Show runs section when empty with context text ("Runs will appear here when an agent starts work")

**P1:**
- Add success toast after status/assignee change (use SSE or `HX-Trigger`)
- Replace parent issue text input with a typeahead using the existing `/search` endpoint
- Inline title editing (click title → editable input → blur to save)

**P2:**
- "Copy issue key" clipboard button
- `C` shortcut to focus comment textarea
- Truncate long run lists to last 5, with "Show all" expand

---

### Agents List (`/agents`)

**What works:**
- Card grid layout (1→2→3 col responsive) is well-suited for agents
- Status dot (emerald/zinc) communicates online/offline clearly
- Create form is collapsible and detailed

**Issues:**
- Create form has 8 fields in a 2-col grid — overwhelming for a new user; fields like `max_tokens`, `temperature` are advanced and should be collapsed
- Model field is a text input — should be a `<select>` of known models (claude-sonnet-4-6, claude-opus-4-6, etc.)
- Archetype field is a text input — should be a `<select>` if archetypes are enumerated
- No search/filter on the agents list
- Empty state SVG icon is generic — could use a robot/agent icon to match context
- Agent cards show heartbeat time but no current run status

**P0:**
- Replace model text input with `<select>` listing known Claude models

**P1:**
- Collapse advanced fields (max_tokens, temperature, max_iterations) behind "Advanced settings" `<details>` toggle
- Add current run count or "busy/idle" indicator to agent cards

**P2:**
- Add search/filter input for agent list
- Show last-run timestamp on agent cards

---

### Agent Detail (`/agents/{slug}`)

**What works:**
- 4 metric cards (tokens, cost, runs, issues) provide good at-a-glance stats
- Inbox list with assign form is practical
- Config editor inside `<details>` keeps advanced settings out of the way
- Recent runs section links to run detail

**Issues:**
- Inbox list is absent when empty — no message and no assign form visible ("No issues assigned" is fine, but the assign form should always show)
- Assign form is inside the inbox section conditional — disappears when inbox is empty
- Metric cards show all-time totals with no time-range selector
- Config edit form has same text-input issues as create form (model, archetype)
- Heartbeat button exists but gives no feedback (just reloads)
- Run list has no status indicator in the list view — all runs look the same
- No way to navigate to the work blocks this agent has contributed to

**P0:**
- Always show assign form regardless of inbox state

**P1:**
- Add status badges to run list rows (completed/failed/running)
- Show toast after heartbeat POST
- Move assign form above inbox list (or make it a persistent top-level action)

**P2:**
- Add time-range filter (7d/30d/all) to metric cards using HTMX swap
- Add "Work blocks involved" section

---

### Work Blocks (`/work-blocks` and `/work-blocks/{id}`)

**What works:**
- List page is simple and scannable
- Detail page has a clear sidebar with actions, metrics, and timeline
- Issues list within block is clean
- Assign issue form only shown when block is active (correct lifecycle gating)

**Issues:**
- Work block status lifecycle is opaque — no visual state diagram or explanation of what "draft → active → shipped" means
- Action buttons (complete/cancel) execute immediately with no confirmation dialog
- Work block list has no status filter tabs (unlike issues list)
- Timeline sidebar is a good idea but shows only created_at — no events logged
- "No issues assigned" empty state has no context about how to use this (workflow guidance)
- Block list rows are minimal — no issue count, no agent count, no status badge
- Create form only takes title + goal — no way to set target date

**P0:**
- Add confirmation for destructive actions (complete/cancel work block) — simple `onclick="return confirm('...')"` is acceptable for MVP

**P1:**
- Add status filter tabs to work blocks list (same pattern as issues)
- Add issue count + status badge to work block list rows
- Show a lifecycle guide tooltip or inline note on the detail page ("Active blocks accept new issues. Complete a block to ship.")

**P2:**
- Enrich timeline with events (issue assigned, agent started run, block completed)
- Target date field on create form

---

### Run Detail (`/runs/{id}`)

**What works:**
- Live stdout polling with HTMX every 2s is the right approach
- Pulsing blue dot for running state is a clean live indicator
- 5 metric cards (cost, tokens, files changed, duration, status)
- Git diff in `<details>` keeps the page clean

**Issues:**
- Stdout is rendered as raw text in a `<pre>` block — no ANSI color stripping or rendering
- When run is complete, polling stops (HTTP 286) but there's no visual "stream ended" indicator
- Git diff shows raw unified diff format — no syntax highlighting or +/- line coloring
- No link back to the parent issue from the run detail page
- Metric cards show raw numbers — duration could be human-formatted ("2m 14s" vs "134s")
- No "copy" button for stdout log
- Run failed state needs more prominent error display (currently just a badge color change)

**P0:**
- Add parent issue link in the run detail header/breadcrumb
- Add +/- line coloring to git diff (green for additions, red for deletions) using CSS `pre` + first-char detection

**P1:**
- Show "Stream complete" indicator when polling stops
- Human-format duration in metric cards

**P2:**
- ANSI color code rendering in stdout
- "Copy log" button for stdout
- Syntax highlight diff filenames

---

### Command Palette (`Cmd+K`)

**What works:**
- Modal with backdrop blur and keyboard navigation is well-implemented
- Arrow key navigation + Enter selection works
- "Create issue" quick-action at bottom is a smart addition
- Search covers both issues and agents

**Issues:**
- No visual distinction when no results are found (empty results list shows nothing — user doesn't know if search failed or returned zero results)
- Palette closes on result selection but gives no feedback before navigation
- ⌘K hint in sidebar is small and easy to miss for new users
- Search is limited to title and key — doesn't search issue descriptions
- No grouping of results (issues vs agents mixed)
- Keyboard shortcut `N` for new issue not implemented globally
- Palette results limited to 50 but no indicator when limit is hit

**P1:**
- Add "No results for '{query}'" empty state message
- Group palette results by type: Issues / Agents headers
- Add `N` global shortcut to open create issue form

**P2:**
- Search issue descriptions (not just title/key)
- Add recently viewed items when search input is empty

---

## Cross-Cutting Issues

### Information Hierarchy and Scannability
- Consistent: badges, color coding, and typography hierarchy are applied uniformly
- Gap: list pages lack column headers, making it harder to scan at first visit
- Gap: page titles are implicit (derived from breadcrumb) rather than an explicit `<h1>`

### Loading States
- Only run stdout has a loading state
- All form submissions do full-page reloads with zero feedback — on a slow connection, button clicks look broken
- Fix: Use HTMX `hx-indicator` with a spinner on form submit buttons, or disable + show loading text

### Form UX
- No inline validation — all errors are silent (form just reloads)
- Select fields for enumerables (model, priority, archetype) are missing throughout
- Auto-submit on status/assignee change is good but needs toast confirmation

### Error States and Recovery
- No error page or error toast pattern — errors return HTTP error codes but the UI doesn't communicate them
- Need a consistent error toast pattern: `showToast("Error", "Failed to update status", null)`

### Mobile/Responsive
- Fixed left sidebar (`w-56` with `ml-56` offset) breaks at small viewports — no mobile nav
- Cards grid is responsive (1→2→3 col) which is good
- Tables are not responsive — horizontal scroll not configured
- Acceptable for an internal tool used primarily on desktop, but tablet support is broken

### Accessibility
- No ARIA labels on icon-only buttons
- Form inputs have labels via adjacent text, but no `for`/`id` pairing
- Color-only status indicators (dots) have no text alternative for color-blind users
- Keyboard focus styles not verified — Tailwind default focus rings may be sufficient

---

## Prioritized Improvement Backlog

### P0 — Blocking / Highest Impact

| # | Issue | Page | Effort |
|---|-------|------|--------|
| 1 | Always show assign form on agent detail (currently hidden when inbox empty) | Agent Detail | XS |
| 2 | Add confirmation dialog for destructive work block actions | Work Block Detail | XS |
| 3 | Replace priority number input with labeled select | Issues List | XS |
| 4 | Replace model text input with select (known Claude models) | Agents List / Detail | S |
| 5 | Add parent issue link in run detail breadcrumb | Run Detail | XS |
| 6 | Show sub-issues and runs sections when empty with context text | Issue Detail | XS |
| 7 | +/- line coloring in git diff | Run Detail | S |

### P1 — High Value / Medium Effort

| # | Issue | Page | Effort |
|---|-------|------|--------|
| 8 | Add success toasts after status/assignee changes | Issue Detail | S |
| 9 | Add "No results" empty state to command palette | Cmd+K | XS |
| 10 | Group command palette results by type (Issues / Agents) | Cmd+K | S |
| 11 | Add status badges to run list rows in agent detail | Agent Detail | XS |
| 12 | Add status filter tabs to work blocks list | Work Blocks | S |
| 13 | Add HTMX `hx-indicator` loading feedback on form submits | All forms | M |
| 14 | Add column headers to issues table | Issues List | XS |
| 15 | Use SSE or HTMX to live-refresh dashboard stats | Dashboard | M |
| 16 | Add CTA buttons to all empty states | All pages | S |
| 17 | Collapse advanced agent fields behind `<details>` | Agents | S |
| 18 | Show "Stream complete" indicator when stdout polling stops | Run Detail | XS |
| 19 | Human-format run duration in metric cards | Run Detail | XS |
| 20 | Add status badge + issue count to work block list rows | Work Blocks | XS |

### P2 — Nice to Have / Lower Impact

| # | Issue | Page | Effort |
|---|-------|------|--------|
| 21 | `N` global keyboard shortcut for new issue | Global | S |
| 22 | `C` shortcut to focus comment textarea | Issue Detail | XS |
| 23 | Inline title/description editing | Issue Detail | M |
| 24 | Typeahead for parent issue field using `/search` | Issue Detail | M |
| 25 | Copy issue key button | Issue Detail | XS |
| 26 | Time-range filter on agent metrics | Agent Detail | M |
| 27 | Enrich work block timeline with events | Work Block Detail | M |
| 28 | Recently viewed items in empty command palette | Cmd+K | M |
| 29 | ANSI color rendering in stdout | Run Detail | M |
| 30 | Mobile nav (hamburger or bottom bar) | Global | L |

---

## Wireframe Descriptions for Key Improvements

### 1. Issue Detail — Empty States Fixed

```
Issue Detail (no sub-issues, no runs)

[Sub-issues]
  ┌────────────────────────────────────────────────┐
  │  No sub-issues yet.                            │
  │  [+ Add sub-issue]  ← inline toggle form       │
  └────────────────────────────────────────────────┘

[Runs]
  ┌────────────────────────────────────────────────┐
  │  No runs yet. Assign this issue to an agent    │
  │  to start a run.                               │
  └────────────────────────────────────────────────┘
```

### 2. Command Palette — Grouped Results

```
┌─────────────────────────────────────┐
│ Search issues and agents...         │
├─────────────────────────────────────┤
│ ISSUES                              │
│  ▪ SO-5  Implement auth            │
│  ▪ SO-3  Fix dashboard layout      │
├─────────────────────────────────────┤
│ AGENTS                              │
│  ● ceo-agent  CEO Agent             │
├─────────────────────────────────────┤
│ + Create issue "..."                │
└─────────────────────────────────────┘
```

### 3. Form Submit Loading State (HTMX)

```html
<!-- Pattern for all create/update forms -->
<button
  type="submit"
  hx-indicator="#form-spinner"
  class="...">
  <span id="form-spinner" class="htmx-indicator animate-spin ...">↻</span>
  Save
</button>
```

### 4. Run Detail — Git Diff with Line Coloring

```
--- a/internal/db/schema.go
+++ b/internal/db/schema.go

  func CreateTables(db *sql.DB) error {
-   return db.Exec("CREATE TABLE foo ...")
+   return db.Exec("CREATE TABLE foo (id TEXT PRIMARY KEY, ...)")
  }

Lines prefixed with + → text-emerald-400 bg-emerald-950/30
Lines prefixed with - → text-red-400 bg-red-950/30
Lines prefixed with @@ → text-zinc-500
```

### 5. Dashboard — Live Stats via SSE

```
Instead of static numbers, stats cards subscribe to SSE events:

hx-ext="sse"
sse-connect="/events"
sse-swap="dashboard-stats"
hx-target="#stats-container"

Server emits "dashboard-stats" event on:
- Agent heartbeat
- Run completion
- Issue status change
```

---

## HTMX Patterns to Leverage

The app already has SSE infrastructure (`/events` endpoint). These specific HTMX patterns should be adopted:

| Pattern | Use Case | Benefit |
|---------|----------|---------|
| `hx-indicator` | Form submit buttons | Show spinner, disable double-submit |
| `hx-trigger="sse:run_complete"` | Dashboard stats refresh | Live updates without polling |
| `hx-swap="outerHTML"` on status badge | Status change feedback | Swap badge in-place without full reload |
| `hx-push-url="false"` | Inline form toggles | No URL change on form show/hide |
| `hx-target="#toasts" hx-swap="beforeend"` | Server-driven toasts | Push notification from any form action |
| `hx-confirm` | Destructive actions | Native or custom confirm dialog |
| HTTP 286 pattern (already used) | Stop polling | Extend to other live-update scenarios |

The `hx-confirm` attribute provides a simple confirmation pattern for P0 destructive actions:
```html
<button hx-post="/work-blocks/{{.ID}}/cancel"
        hx-confirm="Cancel this work block? This cannot be undone.">
  Cancel Block
</button>
```

---

## Manual QA Checklist (Post-Implementation)

1. Create an issue — verify success toast appears and issue appears in list immediately
2. Change issue status from detail page — verify toast confirms change, page does not full-reload
3. Open command palette → type a search → verify groups (ISSUES / AGENTS) with empty-state text when no results
4. Open agent detail with empty inbox — verify assign form is still visible
5. Click "Cancel" on a work block — verify confirmation dialog appears before action fires
6. Open run detail for a completed run — verify git diff shows green/red line coloring
7. Open run detail for a running job — verify "Stream complete" message appears when run finishes
8. Create a new agent — verify model field is a select dropdown with Claude model options
9. Navigate to dashboard — verify stat cards link to filtered views
10. Resize browser to mobile width (375px) — document breakpoints where layout breaks for future mobile sprint
