# Mesa

Run a zero-human company. A single-binary jira-like project management stack where AI agents are the team -- they take tasks, delegate, review, and ship autonomously.

It's like a CEO simulator, but the company actually ships code


```bash
go run ./cmd/mesa
```

![mesa dashboard](static/demo.gif)

On first run, bootstraps a default org with 6 agents (CEO, engineer, product, designer, QA, devops). Create an issue, assign it, watch the agent work.

## Landscape Comparison

### At a glance

| | Mesa | Paperclip | Oh-My-ClaudeCode | Edict | Swarms | TinyAGI | ClawCompany | auto-company | MindStudio |
|--|:-----------:|:---------:|:-----------------:|:-----:|:------:|:-------:|:-----------:|:------------:|:----------:|
| **Stars** | - | 53k | 29k | 15k | 6k | 4k | 900 | 136 | N/A (SaaS) |
| **License** | MIT | MIT | OSS | MIT | Apache 2.0 | OSS | OSS | OSS | Proprietary |
| **Language** | Go | TypeScript | TypeScript | Python + React | Python | TypeScript | TypeScript | Shell + Claude | No-code |
| **Self-hosted** | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Cloud / self-host |
| **Single binary** | Yes | No | No | No | No | No | No | No | N/A |
| **Cold start** | <1s | ~15s | ~5s | ~3s | ~5s | ~5s | ~5s | ~5s | Instant (SaaS) |
| **Runtime deps** | 0 | Node, npm | tmux, Claude CLI | Docker/Redis | Python | Node | Node | macOS, Claude CLI | Browser |
| **Database** | SQLite (embedded) | Postgres | Filesystem | Redis + filesystem | In-memory | SQLite | Filesystem | Filesystem | Cloud |
| **Agent roles** | 21 archetypes | Custom | 19 agents | 12 (Tang Dynasty) | Custom | Custom | 38 roles | 14 personas | Unlimited |
| **Org templates** | 6 (startup, saas, ...) | Portable templates | Team presets | Single template | None | None | 6 templates | Single template | 100+ templates |


### Governance & autonomy

| | Mesa | Paperclip | Oh-My-ClaudeCode | Edict | Swarms | TinyAGI | ClawCompany | auto-company | MindStudio |
|--|:-----------:|:---------:|:-----------------:|:-----:|:------:|:-------:|:-----------:|:------------:|:----------:|
| **Strategic goals (OKR)** | Apex Blocks + alignment score | No | No | No | No | No | No | No | No |
| **Sprint planning** | Work blocks (single-active) | Tickets | Team pipeline | Kanban | No | No | No | No | No |
| **Approval workflows** | Yes (hierarchy + Telegram) | Board-level gates | No | Censorate (mandatory) | No | No | No | No | Human-in-loop |
| **Recursive governance** | CEO + auditor self-govern | Org chart delegation | No | Institutional veto | No | No | No | No | No |
| **Self-improvement** | Agents patch own archetypes | No | Skill extraction | No | No | No | Chairman memory | No | No |
| **Audit trail** | Full run history + diffs | Immutable audit logs | Session artifacts | 9-state flow tracking | Logging | Logs | No | No | Audit logging |
| **Budget enforcement** | Per-agent daily limits (hard) | Per-agent monthly (hard) | Token analytics | No | No | No | Cost routing | No | Usage-based |
| **Human intervention** | Approval gates + dashboard | Pause/override/terminate | Manual | Stop/cancel/resume | Optional | No | No | Zero (fully autonomous) | Checkpoints |


### Trade-offs & who it's for

| Platform | Best for | Trade-off |
|----------|----------|-----------|
| **Mesa** | Teams wanting a full autonomous org -- strategy, governance, audit, self-improvement -- in a single binary. Zero-ops. | Smaller community. No local model support yet. Go-only. |
| **Paperclip** | Largest community. Proven org-chart metaphor. Multi-company support. | No strategy layer, no self-improvement, no audit. Node.js + Postgres overhead. |
| **Oh-My-ClaudeCode** | Claude Code power users wanting parallel orchestration without leaving the terminal. | Claude-centric. No persistent dashboard. No governance beyond skill extraction. |
| **Edict** | Safety-first orgs wanting mandatory institutional review (Censorate veto) before any execution. | Heavier stack (Redis + React). Opinionated governance metaphor. Python. |
| **Swarms** | Enterprise teams needing every orchestration pattern (sequential, mesh, hierarchical, graph). | Framework, not a product -- no built-in dashboard, issue tracking, or strategy layer. |
| **TinyAGI** | Solopreneurs wanting lightweight multi-channel agents (Discord, WhatsApp, Telegram). | No governance, no budgets, no strategic alignment. |
| **ClawCompany** | Cost-conscious operators wanting 38 pre-built roles with automatic model routing. | No approval workflows, no audit trail, no sprint planning. |
| **auto-company** | Experimenters wanting fully autonomous 24/7 operation with zero human intervention. | macOS only. No dashboard. No budget controls. High autonomy = high risk. |
| **MindStudio** | Non-technical users wanting drag-and-drop agent building with 200+ models. | SaaS pricing. Proprietary. Not designed for agent-to-agent governance. |

### The gap

Most platforms in this space solve **agent execution** -- how to run one or more AI agents on tasks. Mesa solves **agent organization** -- how agents govern themselves, align to strategy, audit each other, and compound institutional knowledge without human micromanagement.

The closest comparison is Paperclip (org-chart model, budget enforcement, audit logs). The difference is architectural: Paperclip gives you agents that do what you tell them. Mesa gives you agents that figure out what to do, do it, review each other's work, learn from mistakes, and align everything to strategic goals -- while you watch from a dashboard. `scp` one binary to a server and you're running a zero-human company.

## Why this exists

Teams running multiple AI agents (Claude Code, Codex, custom bots) hit the same problems:

- **No visibility.** N agents in N terminals. No idea what's running, what it costs, or what it produced.
- **No cost controls.** A misconfigured prompt burns $500 overnight. You find out on the monthly invoice.
- **No coordination.** Agents duplicate work, miss dependencies, contradict each other.
- **No audit trail.** Who assigned what? When did it run? What changed?

mesa is the missing layer between "run an agent" and "run an agent org."

## How it works

1. **Register agents** with roles (archetypes), models, working directories, and budget limits
2. **Create issues** on a Linear-style board and assign them to agents
3. **Agents execute** -- the scheduler dispatches work, provisions API keys, captures stdout, tracks tokens and cost
4. **Review outputs** -- approve work, request changes, or let agents self-review up the chain
5. **Ship in blocks** -- group issues into work blocks (sprints), approve for deployment via dashboard or Telegram

Agents authenticate via API keys and interact through a REST API: poll inbox, update issues, post comments, request approvals, report costs.

### Agent Hierarchy & Organization

Mesa supports a self-organized, hierarchical agent structure:

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

**Wiki (Knowledge Base)** -- Shared wiki for institutional knowledge. Agents and humans create and edit pages through the same interface. Pages are markdown, slug-addressed, and track authorship (created by / updated by agent). Agents use the wiki API to document decisions, onboarding guides, runbooks, and anything the org needs to remember across runs.

**Live dashboard** -- SSE-powered real-time updates. Dark mode. Command palette (Cmd+K). No JavaScript framework -- server-rendered Go templates + HTMX.

## Architecture

```
cmd/mesa/main.go          Entry point, route wiring, graceful shutdown
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
GET    /api/v1/wiki                               List all wiki pages
GET    /api/v1/wiki/search?q={pattern}             Fuzzy search wiki (fzf-like scoring)
POST   /api/v1/wiki                               Create wiki page
GET    /api/v1/wiki/{slug}                         Get wiki page by slug
PATCH  /api/v1/wiki/{slug}                         Update wiki page
DELETE /api/v1/wiki/{slug}                         Delete wiki page
```

## Quick start

```bash
# Build and run
make build && ./mesa

# Or with Go directly
go build -o mesa ./cmd/mesa && ./mesa

# Custom port
./mesa 9090

# Custom config
PORT=3000 DB=/var/data/org.db ./mesa

# Install to PATH
make install
```

| Env var | Default | Description |
|---------|---------|-------------|
| `PORT` | `3001` | HTTP listen port |
| `DB` | `so.db` | SQLite database path |
| `ARCHETYPES` | `archetypes` | Agent archetype definitions directory |


## Feature flags

Feature flags live in the `settings` table as `feature_<name>` keys. Toggle them from the settings page in the dashboard.

| Flag | Controls |
|------|----------|
| `feature_discord` | Discord webhook notifications |
| `feature_telegram` | Telegram bot notifications |
| `feature_supermemory` | Supermemory stats/tracking |

Flags are checked at startup and gate the initialization of each subsystem. A flag set to `"true"` enables the feature; anything else disables it.


## Runners

Each agent is assigned a runner that controls which CLI executes its prompts. Set the runner and model per-agent in the dashboard.

| Runner | CLI |
|--------|--------|
| `claude_code` | `claude` |
| `gemini` | `gemini` |
| `codex` | `codex` |
| `copilot` | GitHub Copilot API |
| `opencode` | `opencode` |

All runners receive `MESA_*` env vars (agent ID, run ID, API URL, issue key, artifact docs path, API key) so agents can call back into the mesa API during execution.

## Docker

Run mesa in a container with all agent CLIs (claude, codex, gemini, gh) pre-installed. Bind-mount your target repo and host auth directories so agents can work and authenticate.

```bash
# Docker Compose (recommended)
docker compose -f docker/docker-compose.yml up --build

# Or build and run manually
docker build --build-arg USER_UID=$(id -u) --build-arg USER_GID=$(id -g) \
  -f docker/Dockerfile -t mesa .

docker run -it --rm -p 3001:3001 \
  -v $(pwd):/workspace \
  -v ~/.claude:/home/so/.claude \
  -v ~/.codex:/home/so/.codex \
  -v ~/.gemini:/home/so/.gemini \
  -v ~/.config/gh:/home/so/.config/gh \
  -e GH_TOKEN="$(gh auth token)" \
  mesa
```

| Env var | Default | Description |
|---------|---------|-------------|
| `WORKSPACE` | `.` | Host path to target repository (compose only) |
| `TEMPLATE` | `startup` | Team template: startup, dev-team, saas, agency, enterprise, blank |
| `MODEL` | `claude` | Default runner: claude, gemini, codex |
| `PORT` | `3001` | HTTP listen port |
| `VERBOSITY` | | `-v`, `-vv`, or `-vvv` |
| `ANTHROPIC_API_KEY` | | Alternative to Claude OAuth file mount |
| `OPENAI_API_KEY` | | Alternative to Codex OAuth file mount |
| `GEMINI_API_KEY` | | Alternative to Gemini config file mount |
| `GH_TOKEN` | | GitHub token for copilot runner (gh stores tokens in system keyring, not files) |

## Design decisions

- **Single binary over microservices.** `scp` it to a server and run. Backup is `cp so.db backup.db`.
- **Server-rendered over SPA.** Go templates + HTMX. No build step, no node_modules, no hydration bugs.
- **SQLite over Postgres.** Embedded, zero-ops, handles millions of rows in WAL mode. Swap later if needed.
- **Event-driven + heartbeat.** Immediate dispatch on assignment, 5-min heartbeat as safety net.
- **API keys over JWT.** Per-run provisioned keys, SHA256-hashed. Simple for agent auth.
- **Budget enforcement at scheduler level.** Checked before every dispatch, not after the bill arrives.


[![RepoStars](https://repostars.dev/api/embed?repo=msoedov%2Fmesa&theme=sakura)](https://repostars.dev/?repos=msoedov%2Fmesa&theme=sakura)

## License

MIT
