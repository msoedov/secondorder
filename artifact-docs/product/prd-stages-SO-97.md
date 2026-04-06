# PRD: Incremental Delivery Checkpoints (Stages)
Issue: SO-97 | Date: 2026-04-05

---

## Problem

Complex issues (touching >3 files or integrating 2+ systems) often result in long "black box" `in_progress` states. Reviewers only see the final output, which may contain fundamental directional errors that require total backtracking. 

Existing sub-issues are too heavy for simple linear checkpoints, and free-text comments are hard for humans to track at a glance.

---

## What a "Stage" Is

A stage is an **incremental, verifiable checkpoint within a single issue**. It represents a stable partial state that has passed all local tests (`gates.sh`) but is not yet a complete feature.

### Characteristics

- **Linear**: Stages are completed in a fixed sequence (Stage 1 → Stage 2 → Stage 3).
- **Atomic**: A stage represents a logically coherent slice of the issue's total work.
- **Verifiable**: Completion of a stage must be accompanied by a successful run passing all gates.

---

## Proposed Workflow: Hybrid Trigger

To support both human and agent efficiency, the system will use a hybrid model: **Comment-driven for agents, UI-driven for humans.**

### 1. Initialization
An agent or human defines the stages in an issue comment using a specific syntax:
> `Stages: [Setup], [Core Logic], [Integration], [UI]`

The system parses this list and initializes the `stages` field for the issue.

### 2. Progress Updates (Agent)
When an agent completes a stage's work, they include a completion marker in their comment:
> `Stage 1: [Setup] - Complete`

The system parses this and marks the stage as `done` in the structured `stages` field.

### 3. Progress Updates (Human/UI)
Humans can check/uncheck stages in the Issue Detail view via a checklist UI. This updates the structured `stages` data immediately.

### 4. Stage Acknowledgment
When a stage is marked `done` (via comment or UI), the system:
1. Updates the `issues.current_stage_id` and `issues.stages` (JSON).
2. Broadcasts an SSE `issue_updated` event to the UI.
3. (If comment-triggered) Responds with a system comment: `Stage 1 acknowledged. Progress: 25%`.

---

## Stage-Based Review (The "Checkpoint" Gate)

To fulfill the policy requirement ("Passing Stage 1 is always shipped before Stage 2 begins"), we introduce a **Stage Review** mechanism:

1. After completing Stage 1, an agent *may* move the issue to `in_review` while explicitly noting the stage.
2. The `StatusInReview` handler will recognize if an issue has pending stages and treat it as a **checkpoint review**.
3. Approving a checkpoint review merges/commits the partial work (if supported by the runner) and moves the issue back to `in_progress` for the next stage.
4. If a human reviewer finds a directional error at Stage 1, they "reject" it, and the agent fixes Stage 1 before ever touching Stage 2 logic.

---

## Data Model Changes

### `issues` table
| Field | Type | Rationale |
|---|---|---|
| `stages` | JSON (TEXT) | A serialized array of stage objects: `[{"id": 1, "title": "Setup", "status": "done"}, ...]` |
| `current_stage_id` | INTEGER | The ID (1-based index) of the stage currently being worked on. |

### `IssueStage` Model (Go)
```go
type IssueStage struct {
    ID      int    `json:"id"`
    Title   string `json:"title"`
    Status  string `json:"status"` // "todo", "done"
}
```

---

## What Changes

### 1. DB
- Migration to add `stages TEXT NOT NULL DEFAULT '[]'` and `current_stage_id INTEGER NOT NULL DEFAULT 0` to `issues`.

### 2. Handlers (`UpdateIssue` & `CreateComment`)
- **Parser logic**: Add a regex parser to `CreateComment` that looks for:
  - `Stages: [Title1], [Title2], ...` (to initialize)
  - `Stage \d+: .* - Complete` (to mark done)
- **Status update**: When a stage is marked done, update `current_stage_id` and the `stages` JSON.
- **SSE**: Include `stages` and `current_stage_id` in the `issue_updated` broadcast.

### 3. UI (Issue Detail View)
- Show a **Progress Bar** at the top of the issue (Current Stage / Total Stages).
- Add a **Stages Checklist** sidebar or tab.
- Allow human operators to manually toggle stage status (UI-triggered).

### 4. API
- `GET /api/v1/issues/{key}` returns `stages` and `current_stage_id`.
- `PATCH /api/v1/issues/{key}` accepts `stages` and `current_stage_id`.

---

## Out of Scope

- Branching stage logic (stages must be linear).
- Mandatory stage reviews for every issue (only recommended for complex ones).
- Automated test coverage per-stage (gates.sh is run on the whole diff).
- "Stages" for Work Blocks (blocks already have issues as their granularity).
