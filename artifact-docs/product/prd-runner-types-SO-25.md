# PRD: Multi-Runner Support (SO-25)

**Status:** Draft
**Date:** 2026-03-30
**Author:** Product (SO-25)

---

## Problem

All agents today run on a single hardcoded runner: Claude Code (`claude` CLI). As the ecosystem grows, teams need to run agents using other coding agent frameworks — specifically OpenAI Codex and Antigravity Agent. Without runner abstraction, every new runner requires forking core scheduler logic.

---

## Goals

- Allow each agent to declare which runner it uses
- Support Codex (OpenAI) and Antigravity Agent alongside Claude Code
- Minimal config surface — runners share the same issue/task protocol
- No change to how issues, comments, or the SO API work

---

## Non-Goals

- Centralized credential management (API keys stay in environment or agent config)
- Runtime runner switching (runner is set at agent creation/edit time)
- Per-org runner restrictions (out of scope for now)

---

## Runner Type Abstraction

### Scope: per-agent

Runner type is a property of the **agent**, not the org. Different agents on the same team can use different runners. This matches how `model` already works — it varies per agent.

### New field: `runner`

Add `runner` to the `agents` table alongside `model`. Default: `claude_code`.

| Runner slug | CLI binary | Notes |
|---|---|---|
| `claude_code` | `claude` | Existing behavior, no change |
| `codex` | `codex` | OpenAI Codex CLI |
| `antigravity` | `antigravity` | Antigravity Agent CLI |

---

## Runner Configurations

### claude_code (existing)

```
claude --print -p <prompt> --output-format stream-json --verbose
       --dangerously-skip-permissions --max-turns <N> --model <model>
       [--append-system-prompt-file <archetype>]
       [--add-dir <artifact-docs>]
       [--chrome]
```

Environment injected: `ANTHROPIC_API_KEY` (from `ANTHROPIC_API_KEY` env), plus SO env vars.

### codex

```
codex exec --full-auto --json <prompt>
      [--model <model>]
```

- **API key**: `OPENAI_API_KEY` — set per-agent via the `api_key_env` field (see below) or inherited from process env
- **Model field**: maps to OpenAI model names (e.g. `gpt-4o`, `o4-mini`). Dropdown options differ from Claude models.
- **Working dir**: same `cmd.Dir = agent.WorkingDir` behavior
- **Archetype**: injected as `CODEX_SYSTEM_PROMPT` env var (Codex supports system prompt via env)
- **Max turns**: not directly supported by Codex CLI; omit or map to `--max-steps` if available
- **Chrome**: not applicable, ignore
- **Output**: NDJSON (via `--json`), parsed for token usage via `turn.completed` events.

### antigravity

```
antigravity run --non-interactive --prompt <prompt>
```

- **API key**: `ANTIGRAVITY_API_KEY` — set per-agent via `api_key_env` field or process env
- **Model field**: passed as `--model <model>` (Antigravity uses its own model identifiers)
- **Working dir**: same behavior
- **Archetype**: passed via `--system-prompt-file <archetype-path>`
- **Max turns**: passed as `--max-turns <N>`
- **Output format**: plain text stdout (not JSON stream); scheduler captures raw stdout

---

## New Agent Fields

| Field | Type | Default | Notes |
|---|---|---|---|
| `runner` | string | `claude_code` | Runner slug |
| `api_key_env` | string | `""` | Name of env var holding the API key for this runner. Empty = use runner default. |

`api_key_env` lets operators point an agent at a specific env var (e.g. `CODEX_KEY_BACKEND_AGENT`) without storing secrets in the DB.

---

## Output / Streaming Protocol

The SO scheduler currently streams Claude Code's JSON output to the DB via `liveWriter`. Protocol varies per runner:

| Runner | Output format | Streaming |
|---|---|---|
| `claude_code` | NDJSON (stream-json) | Yes, 2s interval |
| `codex` | Plain text | Yes, 2s interval (same liveWriter) |
| `antigravity` | Plain text | Yes, 2s interval (same liveWriter) |

The scheduler's run record, token counting, and diff capture remain runner-agnostic. Token counts for non-Claude runners will be 0 (not exposed by their CLIs) — display as "N/A" in the UI.

---

## UI Changes

### Agent Create / Edit form

- Add **Runner** dropdown: `Claude Code`, `Codex`, `Antigravity`
- Model dropdown options change dynamically based on runner selection:
  - `claude_code`: Sonnet / Opus / Haiku (existing)
  - `codex`: gpt-4o / o4-mini
  - `antigravity`: default / (runner-specific options TBD)
- Add **API Key Env Var** text input (optional). Helper text: "Name of the environment variable containing the API key for this runner."

### Agent Detail view

- Show **Runner** alongside Model in the info panel
- Model label stays "Model" (the meaning is consistent across runners)

### Agent list / dashboard

- No change needed — runner is an implementation detail, not surfaced in list view

---

## Template Files (dev-team.json, startup.json)

Add optional `runner` field to agent objects. Omitting defaults to `claude_code`. Example:

```json
{
  "name": "Backend Engineer",
  "slug": "backend-engineer",
  "archetype_slug": "backend",
  "model": "gpt-4o",
  "runner": "codex"
}
```

---

## Acceptance Criteria

- [ ] `runner` field exists on Agent model, DB, and API responses
- [ ] `api_key_env` field exists on Agent model and DB
- [ ] Scheduler dispatches to correct runner binary based on `agent.Runner`
- [ ] Codex runner invokes `codex` CLI with correct args and env
- [ ] Antigravity runner invokes `antigravity` CLI with correct args and env
- [ ] Unsupported runner slug causes run to fail with clear error in stdout
- [ ] Agent create/edit form shows Runner dropdown and dynamic Model options
- [ ] Agent detail shows runner in info panel
- [ ] Token counts display as "N/A" for non-Claude runners
- [ ] Template JSON supports optional `runner` field
- [ ] Existing agents without `runner` field default to `claude_code` (backward compat)

---

## Open Questions

1. **Antigravity output format** — confirm whether Antigravity Agent supports structured output or only plain text. If it emits JSON, update streaming to parse accordingly.
2. **Codex max-turns equivalent** — confirm whether `codex` CLI has a step/turn limit flag and what it's called.
3. **Antigravity model identifiers** — get list of valid model slugs from the Antigravity team to populate the dropdown.
