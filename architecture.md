# SecondOrder v2 Architecture: Simplified Agent Orchestration

## Overview

SecondOrder is a lean orchestration layer for autonomous AI agents. A CEO agent handles triage, delegation, and review -- it never does implementation work itself. All other agents are fire-and-forget subprocesses that communicate exclusively through API callbacks. Token usage is minimized because context lives in files (archetypes + artifact-docs), not in prompts. All agents have Chrome browser automation available via `--chrome`.

https://github.com/msoedov/secondorder


## Core Principles

1. **Event-driven, not polling** -- agents are spawned immediately when work appears, not on timers. Heartbeat timers serve as a safety net for missed events
2. **Fully autonomous** -- agents never ask interactive questions; they post comments and mark blocked
3. **API-first communication** -- agents use REST callbacks to checkout, update, comment, and complete issues
4. **Minimal prompts** -- role identity comes from archetype files, project context from artifact-docs

## Core Concepts

### CEO Agent

The CEO is a regular agent with `archetype_slug = "ceo"`. It is auto-assigned all new issues (unless explicitly assigned to someone else). The CEO **never does implementation work** -- it only delegates, reviews, and orchestrates.

**Workflow:**
1. Receive issue -> break into sub-issues with clear scope and acceptance criteria
2. Assign each sub-issue to the right agent via `assignee_slug` and link via `parent_issue_key`
3. Mark parent as `in_progress`, comment with delegation plan
4. When sub-issues complete -> review, approve, or send back

**Separate API:** The CEO gets a trimmed API reference (no checkout, no usage) with delegation-specific endpoints and an inline team roster showing each agent's slug and archetype. This reduces confusion and token waste.

The CEO runs as a claude subprocess like any other agent.

### Event-Driven Wake Chain

Agents are not constantly running. They are spawned on-demand when state changes:

```
Board creates issue
  -> auto-assigned to CEO
  -> wakeAgent(CEO) spawns CEO immediately with the issue
  -> CEO creates sub-issues via API with assignee_slug + parent_issue_key
  -> Each sub-issue auto-wakes the assigned agent
  -> Agent checks out sub-issue, does work, posts comments, marks done
  -> status change to "done" -> wakeAgent(reviewer) for review
     Reviewer resolution: agent.review_agent_id -> agent.reports_to -> CEO fallback
  -> Reviewer approves or sends back
  -> When all sub-issues done, CEO marks parent done
```

Wake triggers fire on:
- **Issue created** (UI or API) -> wake assignee (CEO by default)
- **Issue status changed** (done/blocked/in_review) -> wake CEO/reviewer
- **Issue reassigned** -> wake new assignee
- **Board updates issue** (UI) -> wake current assignee

Heartbeat timers still exist as a safety net (agents with `heartbeat_enabled` run periodically to catch anything missed), but the primary mechanism is event-driven waking.

### Agent Archetypes

Short markdown files in `archetypes/` describing roles. Injected into agent system prompt via `--append-system-prompt-file`.

```
archetypes/
  ceo.md              # triage, delegation, review
  auditor.md          # knowledge base audit (on-demand, no source edits)
  backend.md          frontend.md          fullstack.md
  architect.md        devops.md            qa.md
  designer.md         product.md           marketing.md
  sales.md            support.md           legal.md
  finance.md          operations.md        hr.md
  admin.md            design-partner.md    annoyed-customer.md
  other.md
```

Each file is ~10-30 lines: role name, what they produce, what they must NOT do.

### Artifact Docs (Local Knowledge Base)

Stored in the agent's working directory under `artifact-docs/`. Shared memory between all agents working on the same project.

```
artifact-docs/
  CLAUDE.md              # always loaded -- project policies, conventions
  decisions/             # ADRs, architecture decisions
  prds/                  # product requirement docs
  tech-specs/            # technical specifications
  features/              # feature descriptions, acceptance criteria
```

Agents read from and write to this folder via `--add-dir`. The `CLAUDE.md` in this folder is the persistent project context every agent inherits.

### Auditor Skill

On-demand skill (triggered manually, not on heartbeat). Reviews:
- artifact-docs/ for consistency, staleness, contradictions
- Process compliance and knowledge base health

Produces audit reports in `artifact-docs/decisions/`. Does NOT modify project source code.

## Agent Lifecycle

### 1. Issue Creation + Auto-Assignment

```
Board creates issue via UI or agent creates via API
  -> if no assignee, auto-assign to CEO (archetype_slug = "ceo")
  -> wakeAgent() spawns CEO subprocess immediately
```

### 2. Agent Spawn (Fire-and-Forget)

The scheduler spawns the agent as a non-interactive subprocess:

```
claude --print \
  -p "{prompt}" \
  --output-format stream-json \
  --verbose \
  --dangerously-skip-permissions \
  --max-turns {max_turns} \
  --model {model} \
  --chrome \
  --append-system-prompt-file archetypes/{archetype_slug}.md \
  --add-dir {working_dir}/artifact-docs/
```

The agent receives:
- Tiny prompt with task details + role-specific API reference (CEO gets delegation API, workers get execution API)
- Archetype file for role identity
- Artifact-docs for project context
- Chrome browser automation via `--chrome`
- `SO_API_KEY` env var for API callbacks (auto-provisioned per agent)

### 3. Agent Checkout

```
POST /api/v1/issues/{key}/checkout
Authorization: Bearer $SO_API_KEY
{ "agentId": "$SECONDORDER_AGENT_ID", "expectedStatuses": ["todo", "backlog"] }
```

Atomic: fails if issue is not in expected status (prevents double-checkout). Sets status to `in_progress`.

### 4. Agent Work

Agent is fully autonomous:
- Does the work in the working directory
- Writes documentation to artifact-docs/
- Posts progress as comments via API
- **Never asks interactive questions** -- if blocked, posts a comment and marks blocked
- Can create sub-issues via API

### 5. Agent Completion

```
PATCH /api/v1/issues/{key}
{ "status": "done", "comment": "Summary of what was done." }
```

Or if blocked:
```
PATCH /api/v1/issues/{key}
{ "status": "blocked", "comment": "Question or blocker description." }
```

Token usage is parsed from stdout by the scheduler after process exit and recorded as cost events. Agents can also self-report via `POST /api/v1/runs/{id}/tokens`.

Status change triggers `wakeAgent(reviewer)` so the designated reviewer processes the result immediately.

### 6. Live Output Streaming

While an agent runs, stdout is flushed to the DB every 2 seconds. The run detail page polls `GET /runs/{id}/stdout` via HTMX every 2s, showing a live "running" indicator. Polling stops when the run finishes (HTTP 286 tells HTMX to stop).

The stream-json formatter parses stdout into a rich view with collapsible tool use/result cards, text blocks with styled borders, and a footer with cost/token/duration stats. The formatter skips re-rendering when content hasn't changed between polls to preserve UI state (expanded/collapsed details, scroll position). Double-click toggles between formatted and raw JSON views.

### 7. Stuck Detection

- Each run has a timeout (`agent.timeout_sec`)
- If exceeded, context is cancelled, run marked failed
- Heartbeat timer (safety net) re-triggers on next cycle
- Budget enforcement pauses agents exceeding daily token limits

## Prompt Design

Agents receive role-specific prompts. The CEO and worker agents get different API references and rules to match their responsibilities.

### Worker agent rules

```
RULES:
- You are fully autonomous. Do NOT ask questions interactively. Do NOT wait for human input.
- If you have a question or need clarification, post it as a comment on the ticket and mark the issue "blocked".
- Do NOT request approvals. Just do the work and mark done.
- Always checkout the issue first, then do the work, then update status.
- Write any documentation to the artifact-docs/ folder.
```

### Worker agent API

```
SO API (Authorization: Bearer $SO_API_KEY):
  GET    /api/v1/inbox                              - your assigned issues
  GET    /api/v1/issues/{key}                       - issue detail + comments
  POST   /api/v1/issues/{key}/checkout              - claim issue
  PATCH  /api/v1/issues/{key}                       - update status + comment
  POST   /api/v1/issues/{key}/comments              - add comment
  POST   /api/v1/issues                             - create sub-issue
  GET    /api/v1/usage                              - your token/cost usage
```

### CEO agent rules

```
RULES:
- You are fully autonomous. Do NOT ask questions interactively.
- Do NOT do implementation work yourself. Always delegate by creating sub-issues with assignee_slug and parent_issue_key.
- Break complex tasks into clear sub-issues with acceptance criteria.
- After delegating, mark the parent as "in_progress" and comment your plan.
- When reviewing completed work: approve, request changes via comment, or reassign.
- If blocked, post a comment and mark "blocked".
```

### CEO agent API

```
SO API (Authorization: Bearer $SO_API_KEY):
  GET    /api/v1/inbox                              - your assigned issues
  GET    /api/v1/issues/{key}                       - issue detail + comments
  PATCH  /api/v1/issues/{key}                       - update status + comment
  POST   /api/v1/issues/{key}/comments              - add comment
  POST   /api/v1/issues                             - create & assign: {"title":"...","assignee_slug":"...","parent_issue_key":"..."}
  GET    /api/v1/agents                             - list team (slug, name, archetype)
  POST   /api/v1/approvals/{id}/resolve             - review: {"status":"approved","comment":"..."}

Your team:
  Founding Engineer (slug: founding-engineer, role: fullstack)
  Product Lead (slug: product-lead, role: product)
  ...
```

### Heartbeat prompt (CEO)

Includes the agent's full inbox (all assigned non-done issues), pending reviews, and team roster so it can prioritize, delegate, and review in one run.

### Task prompt

Includes issue title, description, recent comments (last 5), plus the role-specific API reference and rules above.

## Data Model

### Entities

| Entity | Purpose |
|--------|---------|
| agents | Agent config: archetype, working_dir, model, heartbeat, budget |
| issues | Work items: title, description, status, priority, assignee |
| runs | Execution records: stdout, diff, tokens, cost, status |
| comments | Issue discussion: agent or board authored |
| approvals | Review requests (legacy, CEO handles review now) |
| api_keys | Per-agent auth keys (auto-provisioned by scheduler) |
| labels | Issue categorization |
| activity_log | Audit trail |
| cost_events | Per-run token/cost tracking |
| budget_policies | Spend limits per agent |
| secrets | Encrypted key-value storage |
| skills | Registered capabilities (CEO, auditor, etc.) |
| agent_config_revisions | Config change audit trail |
| run_events | Detailed run event log |

### Key Fields

- `agents.archetype_slug` -- links to `archetypes/{slug}.md`
- `issues.assignee_agent_id` -- auto-set to CEO on creation
- `issues.status` -- todo, in_progress, in_review, done, blocked, cancelled

### Migrations

- **004**: Dropped legacy tables (projects, goals, routines, workspaces, etc.). Added `agents.archetype_slug`.
- **005**: Added `runs.diff TEXT` for capturing git changes after each agent run.

## Environment Variables (Agent Subprocess)

```
SECONDORDER_AGENT_ID={agent-uuid}
SECONDORDER_AGENT_NAME={agent-name}
SECONDORDER_RUN_ID={run-uuid}
SECONDORDER_API_URL=http://localhost:{port}
SECONDORDER_ISSUE_KEY={issue-key}
SECONDORDER_ARTIFACT_DOCS={working_dir}/artifact-docs
SO_API_KEY={auto-provisioned-raw-key}
```

## File Structure

```
secondorder/
  cmd/secondorder/
    main.go                # entry point, template application, logrus init
    templates/             # org templates (startup.json, dev-team.json, etc.)
  internal/
    models/                # Agent, Issue, Run, Comment, Approval, etc.
    db/                    # SQLite migrations, queries, checkout logic
    handlers/              # HTTP routes, API endpoints, wake triggers
    scheduler/             # heartbeat loop, agent execution, liveWriter, API key provisioning
    telegram/              # notifications
    templates/             # templ UI components (HTMX)
  archetypes/              # 21 agent role definitions (markdown)
  docs/
    architecture.md        # this file
```

The `artifact-docs/` folder lives in the **target project directory** (agent's working_dir), not in this repo.

## Logging

Uses `github.com/sirupsen/logrus` with structured fields. Log lines include:
- **Start**: agent name, archetype, run ID, model, issue key + title (or "heartbeat" mode)
- **Completion**: agent name, run ID, status, elapsed, issue key, cost, token counts
- No prompt content in logs (was previously dumped in args, now removed)

## API Endpoints

### UI (HTMX)

| Method | Path | Description |
|--------|------|-------------|
| GET | /dashboard | Dashboard with stats |
| GET/POST | /issues | List/create issues |
| GET/PATCH | /issues/{key} | Issue detail/update |
| GET/POST | /agents | List/create agents |
| GET/PATCH | /agents/{slug} | Agent detail/update |
| POST | /agents/{slug}/heartbeat | Manual heartbeat trigger |
| POST | /agents/{slug}/assign | Assign agent to issue + wake |
| GET | /runs/{id} | Run detail with live stdout |
| GET | /runs/{id}/stdout | Stdout fragment (HTMX poll) |

### REST API (Agent Callbacks)

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/v1/inbox | Agent's assigned issues |
| GET | /api/v1/issues/{key} | Issue + comments |
| POST | /api/v1/issues/{key}/checkout | Atomic claim (worker agents) |
| PATCH | /api/v1/issues/{key} | Update status, comment, or reassignment (`assignee_slug`) |
| POST | /api/v1/issues/{key}/comments | Add comment (triggers SSE toast) |
| POST | /api/v1/issues | Create issue with `assignee_slug` + `parent_issue_key` |
| GET | /api/v1/agents | List all agents (id, slug, name, archetype) |
| GET | /api/v1/agents/me | Current agent info |
| GET | /api/v1/usage | Agent's token/cost usage (today + total) |
| POST | /api/v1/approvals/{id}/resolve | Approve or reject review |

### Reviewer Resolution

When an agent marks an issue done/blocked/in_review, the system wakes the appropriate reviewer using this chain:

1. `agent.review_agent_id` -- explicit reviewer override
2. `agent.reports_to` -- management chain
3. CEO agent (fallback) -- agent with `archetype_slug = "ceo"`

### Token Reporting

Token usage is parsed from claude's `stream-json` output (the `{"type":"result",...}` line) after process exit. Extracts `input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`, and `total_cost_usd`. Recorded as cost events for budget enforcement and dashboard display.

### Git Diff Capture

After each agent run, the scheduler runs `git diff HEAD` in the agent's working directory. The diff is stored on the run record (capped at 100KB) and displayed:
- On the **run detail page** with syntax-highlighted line coloring (green/red/indigo)
- On the **issue detail page** as collapsible `<details>` sections per run

### SSE + Toast Notifications

Comment and run_complete events are broadcast via Server-Sent Events (`GET /events`). The JS client maintains a persistent connection with automatic reconnection -- on error, the connection is closed and re-established after 3 seconds with all event listeners re-registered. Toast notifications appear in the bottom-right corner with author, issue key, and content preview. Toasts auto-dismiss after 5 seconds and link to the relevant issue or agent.

The SSE handler listens for a shutdown signal so connections close cleanly during server shutdown rather than blocking for the full timeout.

### Command Palette

`Cmd+K` opens a search palette over issues and agents. Results include a "+ Create" action that creates a new issue with the search query as the title.

### Issue Actions

Issues can be restarted (reset to `todo`, clears `started_at`, re-wakes assigned agent) or cancelled from the sidebar. The status dropdown also supports all transitions.

## Work Blocks

A work block is a shippable unit of work -- analogous to a sprint in Agile but scoped to what can be demoed or deployed as a complete iteration. Work blocks group related issues into a coherent deliverable.

### Concept

- A work block contains a set of issues that together form a complete, deployable increment
- The CEO delegates and orchestrates issues within a work block toward a single shippable goal
- Each block has a clear definition of done: all grouped issues completed, reviewed, and approved
- Deployment is handled **outside** the system -- SecondOrder produces the work, humans ship it

### Lifecycle

```
Board creates work block (title, goal, issues)
  -> CEO plans delegation across the block's issues
  -> Agents work on individual issues as normal (checkout, work, done)
  -> CEO reviews completed issues in context of the block goal
  -> When all issues in block are done -> block status = "ready"
  -> Human reviews the block, approves for deployment
  -> Deployment happens externally (CI/CD, manual, etc.)
  -> Human marks block as "shipped"
```

### Key Properties

- **Atomic scope**: a block should be small enough to demo or deploy in one shot
- **Human gate**: blocks require explicit human approval before deployment -- agents produce, humans ship
- **Iterative**: each block is a complete iteration, not a partial milestone. If something isn't done, it stays in the next block
- **No deployment automation**: SecondOrder orchestrates the work, not the release. Deployment tooling, CI/CD, and release processes live outside the system

### Relationship to Issues

Issues are the unit of work. Work blocks are the unit of delivery. A block groups issues that together produce something shippable. An issue can belong to at most one work block. Issues without a block are standalone work (bug fixes, housekeeping, etc.).

## Browser Automation

All agents are spawned with `--chrome` when `chrome_enabled` is true on the agent config. This gives agents access to Chrome browser automation tools for web interaction, testing, and research.

## Infrastructure

### Database

SQLite with WAL mode. Connection pool allows up to 4 concurrent connections for parallel reads (SQLite WAL supports concurrent readers with a single writer). Busy timeout of 5 seconds prevents lock contention errors.

### Server

Go `net/http` with no WriteTimeout -- SSE connections need to stay open indefinitely. ReadTimeout is 30 seconds for initial request parsing. The server supports graceful shutdown: SSE connections are signaled first, then the scheduler stops running agents, then the HTTP server drains with a 10-second timeout.

### Shutdown Sequence

```
SIGINT/SIGTERM received
  -> app.CloseSSE()           -- signal all SSE goroutines to exit
  -> sched.Stop()             -- cancel running agents, wait for goroutines
  -> srv.Shutdown(10s)        -- drain remaining HTTP handlers
  -> database.Close()         -- close SQLite
```

Future work:
- [ ] JWT auth instead of API keys (short-lived tokens with agent/run claims)
- [ ] Board-level governance (quorum voting, escalation policies, approval gates)
- [ ] Work block UI (create blocks, assign issues to blocks, block dashboard)
- [ ] Work block API for CEO to manage block composition
