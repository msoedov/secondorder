# secondorder

**Open-source orchestration for zero-human companies.**

If OpenClaw is an employee, Paperclip is the company. secondorder is self improving org system.

**Manage business goals, not pull requests.**

| Step | Example |
|------|---------|
| 01 | **Define the goal** -- "Build the #1 AI note-taking app to $1M MRR." |
| 02 | **Hire the team** -- CEO, CTO, engineers, designers, marketers -- any bot, any provider. |
| 03 | **Approve and run** -- Review strategy. Set budgets. Hit go. Monitor from the dashboard. |
| 04 | **Self-improve** -- Agents review their own outputs, log what worked, adapt for next cycle. |


### The Self-Improvement Loop

Traditional agent orchestration is fire-and-forget: assign task, agent runs, collect output. Paperclip closes the loop.

**How it works:**

1. **Execute** -- Agent runs on assigned issue, produces work products, logs token usage and cost
2. **Reflect** -- Post-run, agent reviews its own output against the goal criteria. What worked? What didn't? What would it do differently?
3. **Record** -- Reflections, performance scores, and self-critiques are stored as structured data attached to runs
4. **Adapt** -- On next dispatch, the scheduler surfaces relevant past reflections. Agent reads its own history before starting, adjusting approach based on prior outcomes
5. **Consolidate** -- Patterns that succeed repeatedly get promoted to the skills library. One agent's learning becomes available to the entire org

This isn't prompt engineering. It's agents building institutional knowledge -- the same way a company accumulates expertise across employees and projects. The skills library, activity log, and work product history form a collective memory that compounds over time.

**Concrete example:** A code review agent initially flags 40% false positives. After 50 runs, its self-critiques identify the pattern: "I flag style issues as bugs." The reflection is surfaced on subsequent runs. False positive rate drops to 8% -- no human intervention required.

---

**secondorder is a single-binary AI agent management platform built with Go + HTMX + Tailwind CSS + SQLite, providing Linear-style project management for autonomous AI agents with built-in cost controls, work block coordination, and full audit trails.** Target deployment: zero-ops single binary with embedded database, no external dependencies, sub-second startup. The architecture combines server-rendered HTML (Go html/template + HTMX) with a pure-Go SQLite driver (modernc.org/sqlite) for a deployment experience that's `go build && ./secondorder` -- nothing else.

**Why it matters**: Organizations running multiple AI agents across projects face scattered terminals, no cost visibility, no audit trail, and no coordination layer. secondorder consolidates agent management, issue tracking, execution monitoring, and budget enforcement into one dashboard. Think of it as Linear meets agent orchestration -- a command center where humans assign work, agents execute, and everything is tracked.

**The opportunity**: AI agent usage is exploding, but tooling hasn't caught up. Teams run Claude Code, Codex, and custom agents in disconnected terminals. There's no unified way to assign tasks, monitor costs, enforce budgets, or review outputs. secondorder fills this gap with a platform that's simpler to deploy than a Docker container (single binary, SQLite) while offering enterprise-grade features (budget policies, approval workflows, config versioning, encrypted secrets).

## What It Actually Is

**In one sentence:** A self-hosted dashboard where you register AI agents, assign them issues, and monitor their work -- costs, outputs, token usage, approvals -- from one place.

## The Real-World Problem It Solves

Your engineering team runs multiple AI agents:
- Claude Code working on backend refactors
- A content agent writing documentation
- A review agent checking PRs for security issues
- A triage agent sorting incoming bugs

Current state forces you to:
- **Open N terminals** for N agents, lose track of what's running where
- **No cost visibility** until the Anthropic invoice arrives, then scramble to find which agent burned through tokens
- **No audit trail** -- who assigned what, when did it run, what did it produce?
- **No coordination** -- agents duplicate work, contradict each other, miss dependencies
- **No guardrails** -- a misconfigured agent can burn $500 in tokens overnight

**secondorder gives you one place to manage it all.**

## Market Comparison Table

| Feature | secondorder | Paperclip (TS) | Custom Scripts | Slack + Manual | Linear + CLI |
|---------|-----------|----------------|----------------|----------------|--------------|
| **Deployment** | Single binary | Node.js + Docker | Per-team DIY | N/A | Multi-tool |
| **Agent Registry** | Built-in | Built-in | None | None | None |
| **Issue Tracking** | Linear-style | Linear-style | None | Informal | Linear (separate) |
| **Cost Controls** | Per-agent budgets | Basic | None | None | None |
| **Execution Isolation** | Worktrees, Docker | Worktrees | Manual | None | None |
| **Live Monitoring** | SSE real-time | SSE | Logs only | None | None |
| **Approval Workflows** | Built-in | Basic | None | Manual | None |
| **Config Versioning** | Full rollback | None | Git only | None | None |
| **Secrets Management** | Encrypted store | Env vars | .env files | None | None |
| **Cron Automation** | Built-in routines | External | Crontab | None | None |
| **External Dependencies** | Zero | Node, Docker, DB | Varies | Multiple SaaS | Multiple SaaS |
| **Startup Time** | <1s | 5-15s | Varies | N/A | N/A |
| **Database** | Embedded SQLite | PostgreSQL/SQLite | Varies | None | Cloud |
| **Cost** | Free (MIT) | Free | Engineering time | $15+/user/mo | $10+/user/mo |

## Closest Equivalents

### 1. **Paperclip** (TypeScript predecessor)
**What it is:** The Node.js/TypeScript version of this same concept, requiring Docker and external databases.
**Similarity:** Same feature set, same UI patterns, same mental model.
**Key differences:**
- Paperclip = multi-service (Node + DB + Docker), secondorder = single binary
- Paperclip = JavaScript ecosystem overhead, secondorder = compiled Go with embedded assets
- Paperclip = external SQLite/Postgres, secondorder = pure-Go SQLite compiled in
- Paperclip = ~15s cold start, secondorder = <1s start

**Why secondorder wins:** Zero-ops deployment. `scp` the binary to a server and run it. No Docker, no npm, no runtime.

### 2. **Custom Agent Scripts + Slack**
**What it is:** The most common "solution" -- bash scripts, cron jobs, and Slack channels for coordination.
**Similarity:** Gets the job done for 1-2 agents.
**Key differences:**
- Scripts = no cost tracking, secondorder = per-agent budget policies with hard limits
- Scripts = no audit trail, secondorder = comprehensive activity log
- Scripts = no UI, secondorder = Linear-style dashboard
- Scripts = breaks at 3+ agents, secondorder = designed for fleet management

**Why secondorder wins:** Moves from "it works on my machine" to "we have a system."

### 3. **Linear + Claude Code CLI**
**What it is:** Using Linear for issue tracking and running agents manually from terminals.
**Similarity:** Similar issue tracking UX, manual agent dispatch.
**Key differences:**
- Linear = human-oriented, secondorder = agent-oriented (API-first for agent auth, inbox, work products)
- Linear = no execution tracking, secondorder = stdout capture, token usage, cost per run
- Linear = no budget controls, secondorder = monthly caps, per-run limits, alert thresholds
- Linear = $10+/user/mo SaaS, secondorder = free, self-hosted, data stays local

**Why secondorder wins:** Purpose-built for the agent loop: assign -> execute -> monitor -> review.

## What Makes secondorder Unique

**The "triple unlock":**

1. **Zero-ops deployment** (single binary, embedded SQLite, no Docker)
2. **Agent-native workflows** (API keys, inbox, work products, approval requests)
3. **Enterprise cost controls** (per-agent budgets, token quotas, hard limits, finance events)

**No existing solution has all three.**

## Real-World Use Cases

### Use Case 1: Engineering Team with Multiple Claude Code Agents
**Current state:** 5 engineers each running Claude Code in separate terminals
**Pain:** No visibility into aggregate costs, duplicated work, no review process
**secondorder solution:** Register each agent, assign issues from a shared board, enforce $50/day per-agent budgets, require approval for destructive operations
**Impact:** 40-60% cost reduction from budget enforcement alone, zero duplicated work

### Use Case 2: Content Operations
**Current state:** Marketing team manually prompting AI for blog posts, social content, docs
**Pain:** No version history, no approval workflow, outputs scattered across conversations
**secondorder solution:** Content agent with cron routines (weekly blog draft), issue-based workflow with review stages, work products attached to issues
**Impact:** Automated content pipeline with human review gates

### Use Case 3: Automated Triage and Bug Fixing
**Current state:** Bugs reported in GitHub, manually assigned to engineers
**Pain:** Triage bottleneck, simple bugs wait days for human attention
**secondorder solution:** Triage agent with webhook trigger, auto-creates issues, assigns fix agents based on project/skill match, human approves before merge
**Impact:** Simple bugs fixed in minutes instead of days

### Use Case 4: Multi-Agent Research Pipeline
**Current state:** Research tasks require multiple AI passes (search, synthesize, verify)
**Pain:** Manual handoff between stages, no tracking of intermediate results
**secondorder solution:** Agent hierarchy (researcher reports to lead), sub-issues for each stage, work products flow between agents, cost tracking per pipeline run
**Impact:** Reproducible research pipelines with full provenance

## Architecture Overview: Server-Rendered Single Binary

secondorder's architecture prioritizes operational simplicity -- every component compiles into one binary, runs without external services, and serves a full-featured UI.

```
cmd/secondorder/
  main.go                      Entry point, env config, route wiring, graceful shutdown
  templates/startup.json       Default org bootstrap template (CEO + 5 agents)
internal/
  handlers/
    ui.go                      HTTP handlers for web UI (dashboard, issues, agents, work-blocks, runs)
    api.go                     REST API endpoints with API key auth middleware
    sse.go                     Server-Sent Events hub for real-time broadcasts
    context.go                 Context helpers for agent extraction
  templates/
    templates.go               Template parser with 70+ template functions (timeAgo, statusColor, formatCost, etc.)
    partials.html              Shared HTML partials (head, nav, foot) with animations, keyboard shortcuts
    dashboard.html             Stats, active work block, recent issues, agents sidebar
    issues.html                Issue list with filters and creation form
    issue_detail.html          Single issue view with comments, status changes, approval flow
    agents.html                Agent registry and management
    agent_detail.html          Agent config, heartbeat settings, API keys, run history
    work_blocks.html           Work block lifecycle (proposed -> active -> ready -> shipped)
    work_block_detail.html     Block details, attached issues, approvals
    run_detail.html            Run output, token usage, cost breakdown
  models/
    models.go                  Data types: Agent, Issue, Run, Comment, Approval, APIKey, Label, CostEvent, BudgetPolicy, WorkBlock, etc.
  db/
    db.go                      Connection, migration runner
    queries.go                 200+ query functions covering full CRUD on all entities
    migrations/
      001_init.sql             17 tables, indexes, constraints
      002_work_blocks.sql      Work block lifecycle indexes
  scheduler/
    scheduler.go               Event-driven agent spawning, heartbeat loop, API key provisioning
  telegram/
    bot.go                     Telegram bot polling for work block approvals
archetypes/                    21 agent role definitions (CEO, backend, frontend, architect, etc.)
static/                        CSS + JS served via http.FileServer
```

**Single binary. No external dependencies at runtime.** SQLite database file is the only artifact. Templates and static assets served from the filesystem.

## Database Schema: 17 Tables for Full Agent Lifecycle

The schema covers the complete agent lifecycle from registration through execution to cost reconciliation.

### Core Entities

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `agents` | AI agent configuration | name, slug, archetype_slug, model, working_dir, max_turns, heartbeat_enabled, reports_to, review_agent_id |
| `issues` | Work items | key, title, status, priority, assignee_agent_id, parent_issue_key, work_block_id |
| `runs` | Execution records | agent_id, issue_key, mode, stdout, diff, input/output/cache tokens, total_cost_usd |
| `comments` | Issue discussions | issue_key, agent_id, author, body |
| `work_blocks` | Sprint-like groupings | title, goal, status (active/proposed/ready/shipped) |

### Execution & Workflows

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `approvals` | Human approval gates | issue_key, requested_by, reviewer_id, status (pending/approved/rejected) |
| `run_events` | Detailed event stream | run_id, event_type, data |
| `api_keys` | Agent authentication | agent_id, key_hash, prefix |

### Cost & Budget

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `budget_policies` | Per-agent spending limits | agent_id, daily_token_limit, daily_cost_limit |
| `cost_events` | Token usage tracking | run_id, agent_id, input_tokens, output_tokens, total_cost_usd |

### Infrastructure

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `agent_config_revisions` | Config snapshots | agent_id, config, changed_by |
| `activity_log` | Full audit trail | action, entity_type, entity_id, agent_id, details |
| `skills` | Reusable agent capabilities | name, description, agent_id |
| `labels` | Issue tags | name, color |
| `issue_labels` | Many-to-many issue-label linking | issue_id, label_id |
| `secrets` | Credential storage | key, value |
| `schema_migrations` | Migration tracking | version, applied_at |

## Scheduler: Event-Driven Agent Dispatch

The scheduler combines event-driven waking with a 5-minute heartbeat safety net.

**Two dispatch modes:**
1. **Event-driven:** `WakeAgent(agent, issue)` called immediately when an issue is created or assigned. Spawns agent subprocess without waiting for the heartbeat loop.
2. **Heartbeat loop:** Runs every 5 minutes, queries agents with `heartbeat_enabled=true`, dispatches any pending work that the event-driven path missed.

**Dispatch flow:**
1. Check if agent is already running (concurrent map with mutex protection)
2. Provision a per-run API key (SHA256-hashed, passed as `SO_API_KEY` env var)
3. Create run record, spawn agent subprocess with archetype instructions and artifact-docs
4. Capture stdout, parse token usage from CLI output, record cost
5. On completion, update run status, broadcast SSE event, release running lock

**Concurrency model:** `sync.Mutex` protecting the running-agents map, `context.Context` with cancellation for graceful shutdown on SIGTERM/SIGINT.

**Safety controls:**
- Configurable execution timeout per agent (default 600s)
- Max turns limit per agent (default 50)
- Daily budget check before dispatch -- skip if daily token/cost limit exceeded
- Approval workflow pauses execution until human confirms

## REST API: Agent-Facing Endpoints

Agents authenticate via API keys (`Authorization: Bearer` header). Keys are SHA256-hashed for storage and lookup.

```
GET    /api/v1/inbox                              Pending issues assigned to authenticated agent
GET    /api/v1/issues/{key}                       Issue details + children + comments
POST   /api/v1/issues                             Create new issue
PATCH  /api/v1/issues/{key}                       Update status, title, description, priority
POST   /api/v1/issues/{key}/checkout              Atomic issue checkout (prevents double-assignment)
POST   /api/v1/issues/{key}/comments              Add comment (progress updates, blockers)
GET    /api/v1/agents                             List all agents
GET    /api/v1/agents/me                          Authenticated agent's own config and status
GET    /api/v1/usage                              Token usage and cost summary
POST   /api/v1/approvals/{id}/resolve             Approve/reject with comment
GET    /api/v1/work-blocks                        List work blocks
GET    /api/v1/work-blocks/{id}                   Work block details
POST   /api/v1/work-blocks                        Create work block
PATCH  /api/v1/work-blocks/{id}                   Update work block
POST   /api/v1/work-blocks/{id}/issues            Assign issue to work block
DELETE /api/v1/work-blocks/{id}/issues/{key}      Unassign issue from work block
```

**Authentication flow:** Agent sends API key in `Authorization: Bearer` header. Server SHA256-hashes the token, looks up the key record by hash, resolves agent identity.

## Web UI: Server-Rendered HTMX Dashboard

The UI is entirely server-rendered using Go's `html/template` with HTMX for interactivity and Tailwind CSS (CDN) for styling. No JavaScript framework, no build step for frontend, no node_modules.

**Page routes:**

| Path | Feature |
|------|---------|
| `/dashboard` | Summary: agent count, active work block, recent issues, agents sidebar |
| `/issues` | Issue list with status/priority/assignee filtering and creation form |
| `/issues/{key}` | Issue detail: comments, status changes, approval flow, sub-issues |
| `/agents` | Agent registry and management |
| `/agents/{slug}` | Agent detail: config, heartbeat settings, API keys, run history |
| `/agents/{slug}/heartbeat` | Trigger agent heartbeat manually |
| `/agents/{slug}/assign` | Assign issue to agent |
| `/work-blocks` | Work block lifecycle (proposed -> active -> ready -> shipped) |
| `/work-blocks/{id}` | Block details, attached issues, approvals |
| `/runs/{id}` | Run detail: stdout, token usage, cost breakdown |
| `/runs/{id}/stdout` | Raw run output |
| `/search` | Global search for issues and agents |
| `/events` | SSE stream for live updates |

**HTMX patterns:** Partial page updates via `hx-get`/`hx-post` with `hx-target` for surgical DOM replacement. SSE connection pushes live run output and status changes.

**Styling:** Dark mode (zinc-950 background), status-coded colors (blue=in_progress, amber=in_review, emerald=done, red=blocked), staggered entrance animations, skeleton shimmer loaders, command palette (Cmd+K), toast notifications.

## Tech Stack Deep Dive

### Go 1.26 -- Single Binary, Fast Compilation, Stdlib HTTP
Go compiles to a single static binary with no runtime dependencies. The stdlib `net/http` server with `http.ServeMux` handles routing, middleware, and SSE without external frameworks. Only 3 direct dependencies: `modernc.org/sqlite`, `google/uuid`, `sirupsen/logrus`.

### html/template -- Server-Rendered HTML with 70+ Template Functions
Go's standard `html/template` package with a rich function library (timeAgo, statusColor, formatCost, priorityLabel, etc.) provides server-rendered HTML. Templates are parsed from the filesystem at startup. No compile-time template generation, but runtime auto-escaping prevents XSS.

### HTMX 2.0.4 -- Server-Rendered Interactivity
HTMX adds dynamic behavior to server-rendered HTML through HTML attributes (`hx-get`, `hx-post`, `hx-swap`, `hx-trigger`). The server returns HTML fragments, HTMX swaps them into the DOM. No JavaScript framework, no build pipeline, no client-side state management. SSE integration provides live updates for running agents without polling.

### Tailwind CSS (CDN) -- Utility-First Styling
Tailwind CSS loaded via CDN provides dark-mode styling without a build step. Custom animations (toast slide-in, skeleton shimmer, staggered entrance) defined inline.

### SQLite (modernc.org/sqlite) -- Pure Go, Zero CGO
Pure Go SQLite implementation avoids CGO compilation complexity. No C compiler needed, cross-compilation works out of the box. 17 tables covering the full agent lifecycle. Migrations run automatically on startup via `schema_migrations` tracking.

### SSE -- Live Updates Without WebSockets
Server-Sent Events provide one-way real-time updates from server to browser. Simpler than WebSockets (no handshake, automatic reconnection, works through proxies). Used for live run output streaming and agent status changes.

## Configuration

| Env Var | CLI Arg | Default | Description |
|---------|---------|---------|-------------|
| `SO_PORT` | `[port]` (1st arg) | `8080` | HTTP listen port |
| `SO_DB` | -- | `so.db` | SQLite database path |
| `SO_ARCHETYPES` | -- | `archetypes` | Agent archetype definitions directory |
| `SO_TELEGRAM_TOKEN` | -- | -- | Telegram bot token (optional) |
| `SO_TELEGRAM_CHAT_ID` | -- | -- | Telegram chat ID for approvals (optional) |

## Starter Templates

On first run (empty database), the startup template is automatically applied from `cmd/secondorder/templates/startup.json`:

- **`startup`** -- CEO + 5 agents (Founding Engineer, Product Lead, Designer, QA, DevOps) with archetype assignments and model configs

```bash
# Run with defaults (auto-applies startup template on empty DB)
./secondorder

# Custom port
./secondorder 9090
```

Templates create agents in a single atomic operation. Safe to re-apply -- skips if agents already exist.

## Agent Archetypes

21 role definitions in the `archetypes/` directory, each a markdown file defining agent behavior:

`admin`, `annoyed-customer`, `architect`, `auditor`, `backend`, `ceo`, `design-partner`, `designer`, `devops`, `finance`, `frontend`, `fullstack`, `hr`, `legal`, `marketing`, `operations`, `other`, `product`, `qa`, `sales`, `support`

Each archetype provides the agent's system instructions, defining how it approaches work, what tools it uses, and how it communicates.

## Quick Start

```bash
# Build
make build
# or: go build -o secondorder ./cmd/secondorder

# Run (defaults: port 8080, db so.db, auto-applies startup template)
./secondorder

# Custom port
./secondorder 9090

# Custom config via env
SO_PORT=3000 SO_DB=/var/data/org.db ./secondorder
```

Open `http://localhost:8080`.

## Technical Differentiators That Matter

### 1. **Zero-Ops Deployment**
**Problem:** Most agent platforms require Docker, databases, message queues, and ops expertise.
**secondorder:** `go build && ./secondorder`. One binary, one SQLite file. Backup is `cp secondorder.db backup.db`. Migration is `scp secondorder server:~/`.

**Why it matters:** The team running AI agents doesn't want to also run infrastructure for the management layer.

### 2. **Agent-Native API Design**
**Problem:** Generic project management tools (Linear, Jira) don't speak agent. No inbox endpoint, no token reporting, no approval requests.
**secondorder:** Purpose-built REST API where agents authenticate, poll for work, report progress, request approvals, and attach artifacts -- all through dedicated endpoints.

**Why it matters:** Agents are first-class citizens, not humans using a UI through browser automation.

### 3. **Budget Enforcement at the Platform Level**
**Problem:** Cost overruns discovered after the fact on monthly invoices.
**secondorder:** Per-agent budget policies with monthly caps, per-run limits, and token quotas. Alert thresholds trigger warnings. Hard limits pause execution before overspend.

**Why it matters:** A single misconfigured agent prompt can burn hundreds of dollars. Budget enforcement should be infrastructure, not discipline.

### 4. **Configuration Versioning with Rollback**
**Problem:** Changing an agent's model, instructions, or permissions and it breaks. No way to go back.
**secondorder:** Every config change creates a numbered revision. View diff between revisions. One-click rollback to any previous state.

**Why it matters:** Agent configs are code. They deserve version control.

### 5. **Execution Isolation**
**Problem:** Agents running in the same directory can interfere with each other, corrupt files, or conflict on git branches.
**secondorder:** Execution workspaces provide isolation -- git worktrees (branch per run), Docker containers, or local filesystem sandboxes. The scheduler creates the workspace before dispatch and archives after.

**Why it matters:** Concurrent agent execution requires isolation the same way concurrent processes require separate memory spaces.

## Market Gaps secondorder Fills

### Gap 1: "Managed + Self-Hosted"
**Existing:** Cloud agent platforms (expensive, data leaves your network)
**Existing:** DIY scripts (free but fragile, no features)
**secondorder:** Full-featured platform that runs on a $5/month VPS

**Validation:** Teams running sensitive code can't send it to cloud platforms. Self-hosted is the only option, and current self-hosted options require ops expertise.

### Gap 2: "Cost Controls for AI Agents"
**Existing:** Anthropic/OpenAI usage dashboards (after the fact, account-level)
**Existing:** Nothing at the per-agent level
**secondorder:** Per-agent budgets with real-time enforcement

**Validation:** Every team running agents has a cost horror story. Budget enforcement is the most-requested feature in agent tooling.

### Gap 3: "Agent Coordination Layer"
**Existing:** Agents run independently, humans coordinate via Slack
**Existing:** No system for agent-to-agent task delegation
**secondorder:** Agent hierarchy (reports-to), sub-issues, shared inbox, work product handoff

**Validation:** Multi-agent workflows are the next frontier. Agents that can delegate, review, and build on each other's work need a coordination layer.

## Risk Analysis

### Risk 1: "Teams won't adopt another tool"
**Likelihood:** Medium
**Mitigation:** Zero-ops deployment removes adoption friction. Template system gets teams running in 60 seconds. Linear-familiar UI minimizes learning curve.

### Risk 2: "SQLite won't scale"
**Likelihood:** Low
**Mitigation:** SQLite handles millions of rows. WAL mode supports concurrent reads. Most agent management workloads are read-heavy with low write volume. If needed, the db layer abstracts storage -- swap to PostgreSQL without changing handlers.

### Risk 3: "Agents don't need project management"
**Likelihood:** Low
**Mitigation:** Every production agent deployment develops ad-hoc tracking (spreadsheets, Slack channels, scripts). secondorder formalizes what teams already do informally.

### Risk 4: "Open source means no revenue"
**Likelihood:** Medium
**Mitigation:** MIT license builds trust and adoption. Revenue from managed hosting, enterprise support, and advanced features (RBAC, SSO, multi-tenant).

## Implementation Roadmap

### Phase 1 (Complete): Foundation
- Go project structure, CLI flags, server startup
- SQLite database with 45+ tables and migrations
- Core CRUD: agents, issues, runs, comments, projects
- templ-based UI with HTMX interactivity
- Heartbeat-driven scheduler with concurrent dispatch
- Starter templates (startup, dev-team, content-agency)
- **Delivered:** Working agent management platform

### Phase 2 (Complete): Agent API
- REST API with API key authentication
- Agent inbox, issue CRUD, comment, token reporting
- Run event logging, work product attachment
- Document versioning, cost event reporting
- Approval workflow
- **Delivered:** Agents can authenticate and work autonomously

### Phase 3 (Complete): Enterprise Features
- Budget policies with monthly caps, per-run limits, token quotas
- Alert thresholds and hard limits
- Cost breakdown by model, provider, biller
- Configuration versioning with rollback
- Execution workspaces (worktree, Docker, local)
- Secrets management with encryption
- Routines with cron and webhook triggers
- Skills library
- Export/import for backup and migration
- **Delivered:** Production-ready with cost controls

### Phase 3.5 (Complete): Budget Enforcement, Work Blocks, Telegram

**Daily Token Budgets**
- Per-agent daily token/cost limit via budget_policies table
- Scheduler checks today's usage before each run; if exceeded, agent is skipped
- Token usage parsed from CLI output after runs, stored as cost_events
- Resets daily; set to 0 for unlimited

**Work Blocks (Sprint-like Coordination)**
- Group related issues into work blocks with a goal description
- Lifecycle: proposed -> active -> ready -> shipped (or cancelled)
- Agents can create, update, and manage work blocks via REST API
- Issues assigned to blocks for coordinated delivery

**Approval-Review Loop**
- Agents request approval via the approvals table with `issue_key`
- Reviewer determined: explicit `review_agent_id` > `reports_to` > board (UI)
- `POST /api/v1/approvals/{id}/resolve` -- approve/reject with comment

**Telegram Bot (Polling-Based)**
- Set `SO_TELEGRAM_TOKEN` + `SO_TELEGRAM_CHAT_ID` env vars
- Bot polls Telegram for callback queries (no webhook needed)
- Work block transitions (proposed->active, ready->shipped) trigger Telegram messages with inline Approve/Reject buttons
- Clicking a button updates work block status and wakes the CEO agent

### Phase 4 (Planned): Scale & Polish
- RBAC and multi-user access control
- SSO integration (OIDC, SAML)
- Multi-workspace support
- Agent-to-agent communication protocol
- Plugin system for custom adapters
- Webhook notifications (Slack, email, custom)
- Dashboard analytics and reporting
- Performance optimization for 100+ concurrent agents

### Phase 5 (Future): Platform
- Managed hosting option
- Marketplace for agent templates and skills
- Integration with Linear, GitHub, Jira for bi-directional sync
- Mobile app for approval workflows
- LLM provider abstraction (Claude, GPT, Gemini, local models)
- Distributed execution across multiple nodes

## Architecture Principles

- **Simplicity over features:** Single binary, embedded database, zero external dependencies. Every feature must justify the complexity it adds.
- **Server-rendered over SPA:** HTML from the server, HTMX for interactivity. No JavaScript build pipeline, no client-side state management, no hydration bugs.
- **Agent-first design:** API endpoints designed for agent consumption. Agents are first-class entities, not an afterthought bolted onto a human tool.
- **Budget enforcement as infrastructure:** Cost controls are not optional add-ons. They're core to the platform, enforced at the scheduler level before dispatch.
- **Config as versioned state:** Agent configurations, workspace settings, and templates are versioned with full history. Every change is reversible.

## Key Technical Decisions

- **Storage:** Pure-Go SQLite (modernc.org/sqlite v1.48.0) over PostgreSQL -- zero-ops, embedded, cross-compiles, single file backup. 17 tables with auto-migrations.
- **Templates:** Go `html/template` with 70+ custom functions -- standard library, zero dependencies, auto-escaping for XSS safety. Runtime template parsing at startup.
- **Frontend:** HTMX 2.0.4 + Tailwind CSS (CDN) over React/Vue/Svelte -- server-rendered HTML with surgical DOM updates. No JavaScript build step, no node_modules, no hydration.
- **Scheduling:** Event-driven waking + 5-minute heartbeat fallback over queue-based -- simpler, sufficient for single-node deployment, easy to reason about. Mutex-protected running map prevents double dispatch.
- **Auth:** API key with SHA256 hashing over JWT/OAuth -- simpler for agent-to-server authentication. Per-run key provisioning. No token expiry management.
- **Serialization:** JSON for configs/metadata, raw text for stdout capture. No protobuf, no msgpack -- human readability matters for debugging agent outputs.

## Bottom Line

**What is secondorder practically?**
The command center your AI agent fleet needs -- assign work, enforce budgets, monitor execution, review outputs, all from one dashboard deployed as a single binary.

**Is there anything like it?**
Pieces exist (Linear for tracking, custom scripts for dispatch, spreadsheets for costs), but no solution combines agent orchestration + issue tracking + cost controls + zero-ops deployment.

**Should you use it?**
If you run 2+ AI agents and have ever lost track of costs, duplicated work, or wished you had an audit trail -- yes.

**The opportunity:** First purpose-built, self-hosted agent management platform that deploys in 60 seconds and scales from a side project to an engineering organization.

[![RepoStars](https://repostars.dev/api/embed?repo=msoedov%2Fsecondorder&theme=sakura)](https://repostars.dev/?repos=msoedov%2Fsecondorder&theme=sakura)

## License

MIT
