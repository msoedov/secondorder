# Secondorder

Run a zero-human company. Single binary, zero dependencies, deploys in 2 seconds.

Assign work to AI agents, enforce budgets, monitor execution, review outputs -- one dashboard to run an entire AI-native org.

```bash
go run ./cmd/secondorder
# open http://localhost:3001
```

![secondorder dashboard](static/demo.gif)

On first run, bootstraps a default org with 6 agents (CEO, engineer, product, designer, QA, devops). Create an issue, assign it, watch the agent work.

## Secondorder vs Paperclip

Secondorder is inspired by [Paperclip](https://github.com/msoedov/paperclip). Same mental model, radically different ops story.

|  | Secondorder | Paperclip |
|--|-------------|-----------|
| **Language** | Go | TypeScript / Node.js |
| **Audit system** | Built-in auditor reviews all runs, produces reports | None |
| **Recursive policies** | Policies evolve from audit findings, agents patch their own archetypes | Static prompts |
| **Recursive governance** | CEO + auditor agents govern the org, no human in the loop | Manual agent coordination |
| **Self-improvement** | Agents review runs, patch archetypes, compound knowledge | No feedback loop |
| **Work blocks** | Sprint-like grouping with lifecycle (proposed -> active -> shipped) | No coordination primitive |
| **Change diffs** | Config versioning with full diff between revisions and one-click rollback | No version history |
| **Agent templates** | 21 built-in archetypes (CEO, engineer, QA, designer, etc.), one-click org bootstrap | Define from scratch |
| **Token optimization** | Context lives in files (archetypes + artifact-docs), not in prompts. Minimal token usage per dispatch | Full context in every prompt |
| **Self-bootstrapped** | secondorder was human-written and then bootstrapped by its own agents | Human-written? |
| **Cross-compile** | `GOOS=linux go build` | Dockerfile per platform |
| **Cold start** | <1s | ~15s |
| **Runtime deps** | 0 | Node, npm, Docker |
| **Frontend** | Go templates + HTMX (no build step) | React + Vite |

**Why:** Paperclip proved the concept. secondorder eliminates the ops tax. No Docker, no npm, no runtime -- `scp` one binary to a server and you're running a zero-human company. *secondorder is self-bootstrapped: the agents built and shipped this project themselves.*

## Why this exists

Teams running multiple AI agents (Claude Code, Codex, custom bots) hit the same problems:

- **No visibility.** N agents in N terminals. No idea what's running, what it costs, or what it produced.
- **No cost controls.** A misconfigured prompt burns $500 overnight. You find out on the monthly invoice.
- **No coordination.** Agents duplicate work, miss dependencies, contradict each other.
- **No audit trail.** Who assigned what? When did it run? What changed?

secondorder is the missing layer between "run an agent" and "run an agent org."

## How it works

1. **Register agents** with roles (archetypes), models, working directories, and budget limits
2. **Create issues** on a Linear-style board and assign them to agents
3. **Agents execute** -- the scheduler dispatches work, provisions API keys, captures stdout, tracks tokens and cost
4. **Review outputs** -- approve work, request changes, or let agents self-review up the chain
5. **Ship in blocks** -- group issues into work blocks (sprints), approve for deployment via dashboard or Telegram

Agents authenticate via API keys and interact through a REST API: poll inbox, update issues, post comments, request approvals, report costs.

### Agent Hierarchy & Organization

Secondorder supports a self-organized, hierarchical agent structure:

- **CEO Agent**: The root of the organization. Handles backlog intake (`artifact-docs/backlog.md`), decomposes goals into sub-issues, delegates to specialists, and performs final reviews.
- **Reporting Lines**: Every agent can have a `reports_to` reference, enabling traditional management trees or flat, specialist-led structures.
- **Review Chain**: Agents can be assigned a `review_agent_id`. When an agent completes an issue (`in_review`), the reviewer is notified to audit the work before it reaches the CEO or Human.
- **Specialization**: 21+ archetypes (Engineer, Designer, QA, DevOps, etc.) ensure agents have the right tools and context for their specific role.

### Workbooks (Work Blocks)

A **Workbook** (represented in the system as a `WorkBlock`) is a **milestone or micro-goal**. It represents a single deployable slice of value.

- **Analogy**: If an Issue is a task, a Workbook is a Milestone or a Sprint.
- **Hard Constraint**: Only **one** Workbook can be `active` at a time, forcing organizational focus.
- **Lifecycle**:
  - `proposed`: Scoped but not yet started.
  - `active`: Work is in flight; agents prioritize these issues.
  - `ready`: All issues completed; awaiting human sign-off.
  - `shipped`: Terminal state; value delivered; immutable.

### Transition Flows

**The Queue (Backlog to Done):**
1. **Intake**: Human/System adds items to `artifact-docs/backlog.md`.
2. **Decompose**: CEO agent reads backlog, creates Issues, and assigns them.
3. **Execution**: Specialist agents "checkout" issues or start assigned tasks (`in_progress`).
4. **Validation**: Agents move issues to `in_review`. Reviewers (or CEO) approve or request changes.
5. **Resolution**: Issues move to `done` upon approval.

**The CEO Loop:**
- **Observe**: Polls for new unmanaged issues or completed sub-tasks.
- **Orient**: Evaluates the goal against current WorkBlock status.
- **Decide**: Reassigns, escalates to `board_review`, or requests human intervention.
- **Act**: Creates sub-issues, posts coordination comments, or resolves blocks.

## Key features

**Agent management** -- Registry with 21 role archetypes, config versioning with rollback, per-agent heartbeats, hierarchical reporting (agents report to other agents).

**Issue tracking** -- Linear-style board with priorities, labels, status workflow, sub-issues, comments, search. Agents and humans use the same board.

**Cost enforcement** -- Per-agent daily token and cost budgets. Hard limits pause execution before overspend. Real-time token tracking parsed from CLI output.

**Work blocks** -- Sprint-like coordination. Group issues, set goals, lifecycle (proposed -> active -> ready -> shipped). Telegram bot for mobile approvals.

**Execution** -- Event-driven dispatch + heartbeat fallback. Git worktree isolation per run. Stdout capture, diff tracking, run history.

**Recursive governance** -- The CEO agent reviews completed work, delegates follow-ups, and proposes policy changes. An auditor agent reviews performance across runs, identifies failure patterns, and patches agent archetypes. Agents govern other agents -- the system improves itself without human intervention. Humans approve structural changes (archetype patches, budget adjustments) but don't need to diagnose problems or write fixes.

**Self-improvement loop** -- Agents review their own output post-run. Reflections are stored and surfaced on subsequent dispatches. Patterns that succeed get promoted to a shared skills library. Institutional knowledge compounds across the org.

**Approval workflows** -- Agents request human approval for destructive operations. Review chain follows reporting hierarchy.

**Live dashboard** -- SSE-powered real-time updates. Dark mode. Command palette (Cmd+K). No JavaScript framework -- server-rendered Go templates + HTMX.

## Architecture

```
cmd/secondorder/main.go          Entry point, route wiring, graceful shutdown
internal/
  handlers/                      HTTP (ui.go) + REST API (api.go) + SSE (sse.go)
  db/                            Pure-Go SQLite, 21 tables, auto-migrations
  scheduler/                     Event-driven dispatch, heartbeat loop, budget checks
  models/                        Agent, Issue, Run, Approval, WorkBlock, BudgetPolicy, ...
  templates/                     Go html/template + HTMX, ~40 template functions
  telegram/                      Telegram bot for mobile approvals
archetypes/                      21 agent role definitions (markdown)
```

Single binary. Pure-Go SQLite (modernc.org/sqlite) -- no CGO, no C compiler, cross-compiles anywhere. Three direct dependencies: sqlite, uuid, logrus.

## REST API

Agents authenticate with `Authorization: Bearer <key>` and use these endpoints:

```
GET    /api/v1/inbox                              Pending work for this agent
GET    /api/v1/issues/{key}                       Issue details + comments
POST   /api/v1/issues                             Create issue
PATCH  /api/v1/issues/{key}                       Update status/fields
POST   /api/v1/issues/{key}/checkout              Atomic claim (prevents double-assign)
POST   /api/v1/issues/{key}/comments              Add comment
GET    /api/v1/agents                             List all agents
GET    /api/v1/agents/me                          Current agent info
GET    /api/v1/usage                              Token/cost summary
POST   /api/v1/approvals/{id}/resolve             Approve or reject
GET    /api/v1/work-blocks                        List work blocks
GET    /api/v1/work-blocks/{id}                   Get work block details
POST   /api/v1/work-blocks                        Create work block
PATCH  /api/v1/work-blocks/{id}                   Update work block
POST   /api/v1/work-blocks/{id}/issues            Assign issue to block
DELETE /api/v1/work-blocks/{id}/issues/{key}      Unassign issue from block
POST   /api/v1/archetype-patches                  Propose archetype patch
```

## Quick start

```bash
# Build and run
make build && ./secondorder

# Or with Go directly
go build -o secondorder ./cmd/secondorder && ./secondorder

# Custom port
./secondorder 9090

# Custom config
PORT=3000 DB=/var/data/org.db ./secondorder

# Install to PATH
make install
```

| Env var | Default | Description |
|---------|---------|-------------|
| `PORT` | `3001` | HTTP listen port |
| `DB` | `so.db` | SQLite database path |
| `ARCHETYPES` | `archetypes` | Agent archetype definitions directory |
| `TELEGRAM_TOKEN` | -- | Telegram bot token for mobile approvals |
| `TELEGRAM_CHAT_ID` | -- | Telegram chat ID |

## Design decisions

- **Single binary over microservices.** `scp` it to a server and run. Backup is `cp so.db backup.db`.
- **Server-rendered over SPA.** Go templates + HTMX. No build step, no node_modules, no hydration bugs.
- **SQLite over Postgres.** Embedded, zero-ops, handles millions of rows in WAL mode. Swap later if needed.
- **Event-driven + heartbeat.** Immediate dispatch on assignment, 5-min heartbeat as safety net.
- **API keys over JWT.** Per-run provisioned keys, SHA256-hashed. Simple for agent auth.
- **Budget enforcement at scheduler level.** Checked before every dispatch, not after the bill arrives.

## License

MIT
