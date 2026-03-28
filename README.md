# thelastorg

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Go-based autonomous agent orchestration platform. Agents check out issues, do work, post comments, and mark completion -- all without human interaction. A CEO agent handles triage and delegation; all other agents are fire-and-forget subprocesses that communicate exclusively through a REST API.

## Features

- Event-driven agent wake chain: agents spawn immediately when work appears
- Atomic issue checkout prevents double-assignment
- Per-agent token budget enforcement and cost tracking
- Live stdout streaming with HTMX polling
- Git diff capture and display per run
- SQLite with WAL mode, no external dependencies
- Chrome browser automation available to all agents

## Prerequisites

- Go 1.26+

## Build

```sh
go build ./cmd/thelastorg
```

## Run

```sh
./thelastorg
```

The server starts on `http://localhost:9001` by default. Set environment variables as needed:

| Variable | Description |
|----------|-------------|
| `THELASTORG_API_URL` | Base URL agents use for callbacks |
| `TLO_API_KEY` | API key for agent authentication |

## Quickstart

1. Build the binary: `go build ./cmd/thelastorg`
2. Start the server: `./thelastorg`
3. Open `http://localhost:9001/dashboard` in your browser
4. Create an agent with an archetype (e.g. `product`, `backend`)
5. Create an issue -- it auto-assigns to the CEO agent, which delegates to the appropriate agent

## Architecture

TLO uses a lean orchestration model:

- **CEO agent** (`archetypes/ceo.md`) -- triage, delegation, review only; never does implementation
- **Worker agents** -- check out issues, do work, post comments, mark done or blocked
- **Archetypes** -- short markdown files in `archetypes/` injected into each agent's system prompt
- **Artifact docs** -- shared knowledge base in `artifact-docs/` in the agent's working directory
- **Wake chain** -- status changes trigger immediate agent spawns; heartbeat timers serve as a safety net

See [architecture.md](architecture.md) for the full design, data model, API reference, and agent lifecycle.

## License

[MIT](LICENSE)
