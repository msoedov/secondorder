# Retro: Audit & Self-Improvement

The retro system is Mesa's self-improvement loop. It reviews completed work, identifies performance patterns, produces short actionable policies, and proposes archetype updates -- all with human approval gates.

## Concept

Agents do work, ship it, and the system learns nothing. Retro closes that gap:

1. **Audit** -- Review completed issues and shipped work blocks. Analyze agent performance: retries, rejections, cost, cycle time.
2. **Board Policy** -- Produce short, actionable rules (1-2 sentences max) that guide future agent behavior.
3. **Policy** -- Produce short, actionable rules (1-2 sentences max) that guide future agent behavior.
4. **Archetype evolution** -- Propose updates to agent archetype files based on audit findings. Human approves or rejects each change.
5. **Knowledge base cleanup** -- Compress, reconcile, and remove stale docs in artifact-docs/.

## How It Works

### Triggering an Audit

Navigate to `/policies` and click **Run Audit**. Configure:

- **Blocks** -- Number of recent shipped work blocks to review (default: 3)
- **Issues** -- Number of recent completed issues to review (default: 50)
- **Runner** -- Select the AI runner for this audit (Claude Code, Gemini, Codex). If blank, the default runner for the auditor agent is used.
- **Model** -- Select the model for the runner.
- **Focus** (optional) -- Free-text prompt guiding what the auditor should focus on (e.g. "Focus on backend agent retry rates and missing test coverage")

### Configuration File
You can also configure the default audit runner and model via a `.mesa.yml` (or `.mesa.json`) file in the project root:

```json
{
  "audit": {
    "runner": "gemini",
    "model": "gemini-1.5-pro"
  }
}
```
UI selections take precedence over this configuration file.

The system spawns the **auditor agent** (`archetype_slug = "auditor"`) with a prompt containing:

- Recent shipped work blocks with their issues, run counts, and costs
- Recent completed issues with agent assignments and retry counts
- Current archetype content for every agent in the team
- Any existing policies from `artifact-docs/policies/`
- The optional focus prompt

### What the Auditor Produces

**1. Policies** -- Written directly to `artifact-docs/policies/` as markdown files.

Each policy is 1-2 sentences -- a clear, actionable rule. No explanations in the policy file. Background and rationale go to `artifact-docs/decisions/` as separate documents.

Example `artifact-docs/policies/testing.md`:
```
Run go test before marking any issue done.
Always handle errors for new API endpoints -- never ignore returned errors.
```
Write only approved policies to this directory.

**1.5. Board Policy** -- Written directly to `artifact-docs/board-policy/` as markdown files.

Each policy is 1-2 sentences -- a clear, actionable rule. No explanations in the policy file. Background and rationale go to `artifact-docs/decisions/` as separate documents.

Example `artifact-docs/board-policy/testing.md`:
```
Run go test before marking any issue done.
Always handle errors for new API endpoints -- never ignore returned errors.
```


Policies are automatically loaded by all agents via `--add-dir artifact-docs/`.

**2. Archetype patches** -- Proposed via `POST /api/v1/archetype-patches`.

Each patch contains the full proposed archetype content. Patches are stored in the `archetype_patches` table with status `pending`. They appear on the retro page for human review.

- **Approve** -- Overwrites the archetype file in `archetypes/{slug}.md`. All future agent runs use the updated archetype.
- **Reject** -- Discards the patch. No file change.

**3. Knowledge base cleanup** -- The auditor can directly modify files in `artifact-docs/` (merge duplicates, delete stale docs, update outdated information).

### Audit Lifecycle

```
Human clicks "Run Audit" on /policies
  -> auditor agent spawns with performance data + current archetypes
  -> auditor reviews patterns (retries, rejections, cost)
  -> auditor writes policies to artifact-docs/policies/
  -> auditor proposes archetype patches via API
  -> auditor cleans up artifact-docs/
  -> audit completes, findings recorded

Human reviews on /policies:
  -> reads policies (auto-applied, visible in policies section)
  -> reviews archetype patches (diff view)
  -> approves or rejects each patch
  -> next work block: agents run with updated archetypes + policies
```

## Data Model

### audit_runs

| Column | Type | Description |
|--------|------|-------------|
| id | TEXT PK | UUID |
| run_id | TEXT | Links to the agent `runs` record |
| status | TEXT | `running`, `completed`, `failed` |
| issues_reviewed | INT | Count of issues analyzed |
| blocks_reviewed | INT | Count of blocks analyzed |
| findings | TEXT | Summary of audit findings |
| created_at | DATETIME | When the audit was triggered |
| completed_at | DATETIME | When the audit finished |

### archetype_patches

| Column | Type | Description |
|--------|------|-------------|
| id | TEXT PK | UUID |
| audit_run_id | TEXT | Which audit produced this patch |
| agent_slug | TEXT | Target archetype (e.g. `backend`, `qa`) |
| current_content | TEXT | Archetype content at time of proposal |
| proposed_content | TEXT | Full proposed replacement content |
| status | TEXT | `pending`, `approved`, `rejected` |
| reviewed_at | DATETIME | When human reviewed |
| created_at | DATETIME | When proposed |

## API

### Agent API (used by auditor)

```
POST /api/v1/archetype-patches
  Headers: Authorization: Bearer $SO_API_KEY, X-Audit-Run-ID: {audit_run_id}
  Body: {"agent_slug": "backend", "proposed_content": "# Backend Engineer\n..."}
  Response: 201 Created
```

### UI Routes

```
GET  /policies       -- Audit page: policies, pending patches, run history
POST /policies       -- Actions: run_audit, approve_patch, reject_patch, save_policy
```


## Auditor Archetype

The auditor agent (`archetypes/auditor.md`) is configured to:

- Review completed work patterns (retries, rejections, cost)
- Write short policies (1-2 sentences per rule) to `artifact-docs/policies/`
- Write rationale separately to `artifact-docs/decisions/`
- Propose archetype patches via the API (never edit archetypes directly)
- Clean up stale or contradictory docs in artifact-docs/

The auditor does NOT do implementation work, fix code, or make product decisions.

## Configuration

The auditor agent must exist in the agents table:

```json
{"name": "Auditor", "slug": "auditor", "archetype_slug": "auditor", "model": "sonnet"}
```

It is included in the default startup template (`cmd/mesa/templates/startup.json`). If agents were created before the auditor was added, create it manually via `/agents` or direct DB insert.

## Design Principles

- **Optional** -- The retro page exists but never runs automatically. Use it or don't.
- **Human-gated** -- Archetype changes always require human approval. Policies are auto-applied but visible and editable.
- **Short policies** -- Max 1-2 sentences. Background goes elsewhere. Agents read policies via `--add-dir`, so brevity matters for token efficiency.
- **Incremental** -- Each audit builds on previous policies and archetype state. The system improves over time without requiring wholesale rewrites.
