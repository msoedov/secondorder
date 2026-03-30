# Contributing to secondorder

## Quick start

```bash
go build -o secondorder ./cmd/secondorder
go test ./...
./secondorder
```

Open `http://localhost:3001`. On first run, a default org (CEO + 5 agents) is bootstrapped automatically.

## Project structure

```
cmd/secondorder/
  main.go                 Entry point, env config, route wiring
  templates/              Bootstrap org templates (dev-team.json, startup.json)
internal/
  handlers/               HTTP handlers (ui.go), REST API (api.go), SSE (sse.go), request context
  db/                     SQLite connection, migrations, query functions
  scheduler/              Agent dispatch, heartbeat loop, budget enforcement
  models/                 Data types (Agent, Issue, Run, WorkBlock, etc.)
  templates/              Go html/template + HTMX partials
  telegram/               Telegram bot for mobile approvals
archetypes/               21 agent role definitions (markdown files)
static/                   Static assets (favicon, images)
docs/                     Internal documentation
artifact-docs/            Generated documentation artifacts
```

## How it works

secondorder is a single-binary Go application. Server-rendered HTML (Go templates + HTMX), pure-Go SQLite (modernc.org/sqlite), SSE for real-time updates. No JavaScript framework, no external database, no build pipeline.

**Recursive governance:** The system governs itself. A CEO agent triages and delegates. An auditor agent reviews performance across runs, identifies failure patterns, and proposes archetype patches. Agents review other agents' work up the reporting chain. Humans approve structural changes but don't need to diagnose problems.

## Making changes

### Code

- Run `go test ./...` before submitting
- Keep dependencies minimal -- three runtime deps (sqlite, uuid, logrus)
- Templates live on the filesystem, not embedded -- edit and reload
- Migrations in `internal/db/migrations/` are applied automatically on startup
- Add new migrations as sequentially numbered SQL files

### Agent archetypes

Archetypes in `archetypes/` define agent behavior as markdown. Each file is the system prompt for that role. Changes to archetypes affect how agents approach work on next dispatch.

### UI

Templates use Go `html/template` with HTMX attributes for interactivity. Tailwind CSS via CDN, inline JS in templates. No build step -- edit HTML, restart server, refresh browser.

## Running tests

```bash
# All tests
go test ./...
make test

# Specific package
go test ./internal/db/...
go test ./internal/handlers/...
go test ./internal/scheduler/...
```

Tests use in-memory SQLite databases -- no setup required.

## Makefile targets

```bash
make build     # Build the binary
make test      # Run all tests
make run       # Build and run
make lint      # Run golangci-lint
make gl        # Run gitleaks scan
make install   # Install to /usr/local/bin
make clean     # Remove binary
```

## Secret scanning

We use [gitleaks](https://github.com/gitleaks/gitleaks) locally to prevent accidental credential commits.

```bash
# Install
brew install gitleaks

# Scan repo
make gl

# Scan staged changes only
gitleaks protect --staged -v
```

Configuration is in `.gitleaks.toml`. It allowlists test files and known placeholder values. If gitleaks flags a false positive, add a `// gitleaks:allow` inline comment or update the allowlist in `.gitleaks.toml`.

## Guidelines

- Keep it simple. Single binary, zero external dependencies at runtime.
- No new runtime dependencies without strong justification (currently: sqlite, uuid, logrus).
- Server-rendered HTML. No JavaScript frameworks.
- Budget enforcement is infrastructure, not optional. Cost checks happen at the scheduler level.
- Agent archetypes are the primary lever for behavior change -- prefer archetype edits over code changes when possible.
- CI runs via GitHub Actions (`.github/workflows/`). Releases are managed with GoReleaser.
