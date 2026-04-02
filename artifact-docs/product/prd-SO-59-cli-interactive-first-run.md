# PRD: CLI Interactive First-Run Configuration (SO-59)

## Problem

When a user runs `secondorder` for the first time in a new directory, agents are silently created from the `startup` template with `claude` as the default runner. The user has no opportunity to make choices without knowing CLI flags exist. New users especially have no visibility into the template options or runner alternatives.

## Goal

On first run (empty database), prompt the user interactively in the terminal to select a team template and default agent runtime before the server starts.

## Success Criteria

- First-time users see a prompt and can select template + runtime without reading docs
- Returning users (agents already exist) see no prompt — zero friction
- Non-interactive environments (CI, piped stdin, flags already provided) skip the prompt silently
- Selection persists to the database exactly as flags would today

---

## User Stories

**US-1 — First-time setup**
As a developer running `secondorder` for the first time, I want to be guided through choosing my team template and AI runner so I start with a configuration that fits my project.

Acceptance criteria:
- On launch with empty DB and no `-t`/`-m` flags, a prompt appears before the server starts
- Prompt lists all available templates (startup, dev-team, saas, agency, enterprise) with short descriptions
- Prompt lists available runtimes (claude, gemini, codex)
- After selection, agents are seeded and the server starts normally
- Selected choices are echoed back: `Using template: startup | runner: claude`

**US-2 — Blank start**
As a developer who wants to configure agents manually via the UI, I want to skip template seeding.

Acceptance criteria:
- "blank (no agents)" is a valid template choice in the prompt
- Selecting blank skips `applyStartupTemplate` and starts with an empty database

**US-3 — Non-interactive / flag bypass**
As a developer running secondorder in CI or a script, I want the prompt skipped when I pass `-t` and `-m` flags.

Acceptance criteria:
- If `-t` flag is provided, skip the template prompt entirely
- If `-m` flag is provided, skip the runtime prompt entirely
- If stdin is not a TTY (`!isatty(stdin)`), skip all prompts and use defaults (startup / claude)

**US-4 — Returning user**
As a developer with an existing database, I want no prompt on restart.

Acceptance criteria:
- `applyStartupTemplate` already guards on `len(agents) > 0` — this behavior is preserved

---

## Functional Requirements

### FR-1: First-run detection
Trigger interactive mode when ALL of the following are true:
1. `len(agents) == 0` (empty DB)
2. No `-t` flag provided
3. stdin is a TTY

### FR-2: Template selection prompt

```
Select a team template:
  1. startup     - Small founding team: CEO, Engineer, Product, Designer, QA, DevOps
  2. dev-team    - Engineering-focused team
  3. saas        - SaaS product team
  4. agency      - Agency delivery team
  5. enterprise  - Larger org structure
  6. blank       - No agents, configure manually

Enter choice [1]:
```

- Default is `1` (startup) if user presses Enter
- Invalid input re-prompts once, then uses default

### FR-3: Runtime selection prompt

```
Select default agent runner:
  1. claude  - Claude Code (default)
  2. gemini  - Google Gemini
  3. codex   - OpenAI Codex

Enter choice [1]:
```

- Default is `1` (claude) if user presses Enter
- Invalid input re-prompts once, then uses default
- Runtime prompt shown after template prompt (skip if blank selected)

### FR-4: Confirmation echo

After selection, before server starts:
```
Starting with template=startup runner=claude
```

### FR-5: Non-TTY / flag bypass
- Detect TTY via `os.Stdin`'s `fd` + `term.IsTerminal` (golang.org/x/term)
- When non-interactive: use CLI flag values or env vars as today, no output

---

## Out of Scope

- Saving config to a file (`.secondorder.yml`) — the DB is the source of truth
- Changing template/runtime after initial setup via this flow
- Wizard-style per-agent configuration

---

## Template Descriptions (for prompt copy)

| Key        | Description                                              |
|------------|----------------------------------------------------------|
| startup    | Founding team: CEO, Engineer, Product, Designer, QA, DevOps |
| dev-team   | Engineering-focused: leads, backend, frontend, QA        |
| saas       | SaaS product: growth, product, engineering, support      |
| agency     | Agency delivery: account, PM, developers, QA             |
| enterprise | Larger org with multiple team leads and specialists      |

---

## Implementation Notes (for engineering)

- All prompt logic lives in `cmd/secondorder/main.go` in a `promptFirstRun(args)` function called before `applyStartupTemplate`
- Use `golang.org/x/term` for TTY detection (`term.IsTerminal(int(os.Stdin.Fd()))`)
- `bufio.NewReader(os.Stdin)` for reading input
- `promptFirstRun` returns `(templateName, defaultModel string)` — same types already used by `applyStartupTemplate`
- No new dependencies beyond `golang.org/x/term` (already likely in module graph via existing deps)

Tech spec: `artifact-docs/tech-specs/SO-59-cli-interactive-first-run.md`
