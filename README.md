# Secondorder

Run a zero-human company. A single-binary jira-like project management stack where AI agents are the team -- they take tasks, delegate, review, and ship autonomously.

It's like a CEO simulator, but the company actually ships code


```bash
go run ./cmd/secondorder
```

![secondorder dashboard](static/demo.gif)

On first run, bootstraps a default org with 6 agents (CEO, engineer, product, designer, QA, devops). Create an issue, assign it, watch the agent work.

## Secondorder vs Paperclip

[Paperclip](https://github.com/msoedov/paperclip) proved you can point AI agents at a codebase and get work done. secondorder asks the next question: **what happens when the agents run the company?**

Paperclip is a task runner. secondorder is an operating system for autonomous organizations -- agents don't just execute, they govern, audit, strategize, and improve themselves. The difference isn't incremental; it's architectural.

|  | Secondorder | Paperclip |
|--|-------------|-----------|
| **Philosophy** | Autonomous org with recursive self-governance | AI-assisted task execution |
| **Strategic planning** | Apex Blocks set north-star goals; work blocks align to strategy; alignment scores track drift | No strategy layer |
| **Governance** | CEO + auditor agents govern the org, propose policy changes, patch archetypes -- no human in the loop | Manual agent coordination |
| **Self-improvement** | Agents review runs, patch their own archetypes, compound institutional knowledge across the org | No feedback loop |
| **Audit system** | Built-in auditor reviews all runs, identifies failure patterns, produces reports | None |
| **Recursive policies** | Policies evolve from audit findings; agents patch their own role definitions | Static prompts |
| **Work blocks** | Sprint-like grouping with lifecycle (proposed -> active -> ready -> shipped) | No coordination primitive |
| **Change diffs** | Config versioning with full diff between revisions and one-click rollback | No version history |
| **Agent templates** | 21 built-in archetypes (CEO, engineer, QA, designer, etc.), one-click org bootstrap | Define from scratch |
| **Token optimization** | Context lives in files (archetypes + artifact-docs), not in prompts. Minimal token usage per dispatch | Full context in every prompt |
| **Self-bootstrapped** | secondorder was human-written and then bootstrapped by its own agents | Human-written |
| **Language** | Go -- single binary, zero deps, cross-compiles anywhere | TypeScript / Node.js |
| **Cold start** | <1s | ~15s |
| **Runtime deps** | 0 | Node, npm, Docker |
| **Frontend** | Go templates + HTMX (no build step) | React + Vite |

**The gap:** Paperclip gives you agents that do what you tell them. secondorder gives you agents that figure out what to do, do it, review each other's work, learn from mistakes, and align everything to strategic goals -- while you watch from a dashboard. `scp` one binary to a server and you're running a zero-human company.

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

### Strategic Blocks (Apex Blocks)

An **Apex Block** is a **strategic goal set by the board**. It represents the highest-level objective the organization is pursuing -- the "why" behind all execution.

- **Analogy**: If a Work Block is a Sprint, an Apex Block is a Quarterly Objective (OKR).
- **North Star Metrics**: Each Work Block aligned to an Apex Block carries a `north_star_metric` and `north_star_target` -- the measurable outcome that defines success.
- **Alignment Score**: The strategy dashboard shows what percentage of active Work Blocks are aligned to an Apex Block. Unaligned work is visible drift.
- **Lifecycle**: `active` (currently pursued) or `archived` (completed or deprioritized).

Strategic blocks close the loop between execution and intent. Agents don't just ship code -- they ship code that moves a metric toward a goal that the board defined. Without this layer, autonomous agents optimize locally (close tickets) but drift globally (build the wrong thing).

### Workbooks (Work Blocks)

A **Workbook** (represented in the system as a `WorkBlock`) is a **milestone or micro-goal**. It represents a single deployable slice of value.

- **Analogy**: If an Issue is a task, a Workbook is a Milestone or a Sprint.
- **Hard Constraint**: Only **one** Workbook can be `active` at a time, forcing organizational focus.
- **Lifecycle**:
  - `proposed`: Scoped but not yet started.
  - `active`: Work is in flight; agents prioritize these issues.
  - `ready`: All issues completed; awaiting human sign-off.
  - `shipped`: Terminal state; value delivered; immutable.

### Issue Lifecycle

```text
       [ CREATE ]
           |
           v
     +-----------+
     |   todo    | <-----------------------------+
     +-----------+                               |
           |                                     |
    [ ASSIGN/CHECKOUT ]                          |
           |                                     |
           v                                     |
     +-------------+       [ BLOCK ]       +-----------+
     | in_progress | --------------------> |  blocked  |
     +-------------+ <-------------------- +-----------+
           |              [ UNBLOCK ]
           |
    [ SUBMIT/REVIEW ]
           |
           v
     +-----------+       [ REQUEST CHANGES ]
     | in_review | -------------------------------> todo
     +-----------+
           |
           +-----------------------+-----------------------+
           |                       |                       |
    [ APPROVE ]             [ ESCALATE ]            [ REJECT ]
           |                       |                       |
           v                       v                       v
     +-----------+         +--------------+          +-----------+
     |   done    |         | board_review |          |  wont_do  |
     +-----------+         +--------------+          +-----------+

     [ CANCEL ] (from any non-terminal state) ----> [ cancelled ]
```

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

**North star alignment** -- Every Work Block can carry a north-star metric and target (e.g. "P95 latency < 200ms"). Apex Blocks define board-level strategic goals. Work Blocks link to an Apex Block, and the Strategy dashboard shows an alignment score -- the percentage of active work tied to a strategic goal. Unlinked work is visible drift. Agents don't just close tickets; they move metrics toward goals the board defined.

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


[![RepoStars](https://repostars.dev/api/embed?repo=msoedov%2Fsecondorder&theme=sakura)](https://repostars.dev/?repos=msoedov%2Fsecondorder&theme=sakura)

## License

MIT
