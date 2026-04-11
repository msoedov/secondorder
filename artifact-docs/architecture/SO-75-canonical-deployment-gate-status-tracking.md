# SO-75 Architecture: Canonical deployment gate status tracking

Date: 2026-04-11
Issue: SO-75
Owner: Architect
Status: Proposed for implementation and QA validation

## 1. Purpose

This document defines the canonical architecture for deployment gate status tracking in Secondorder. The objective is to make deployment gate state a first-class machine-readable concept with:

- exactly one canonical gate record per `release` or `deployment` issue,
- append-only status history for each recheck,
- a queryable current unblock condition in the API,
- additive migration from legacy behavior that relied on comments/files/manual interpretation.

This architecture aligns with and sharpens the earlier SO-62 model, while also documenting the backend/API shape that has already been implemented for SO-63 so QA and future contributors have one authoritative design reference.

---

## 2. Problem statement

Historically, deployment decisions could be expressed through issue status, comments, or separate decision artifacts. That created three operational problems:

1. **Parallel records**: a recheck could create another artifact instead of updating one canonical record.
2. **Ambiguous current state**: consumers had to infer whether deployment was blocked by reading comments or artifact files.
3. **Weak API semantics**: there was no stable API field that answered “what is the current deployment gate state?” for a release/deployment issue.

The design goal is therefore:

> For any release/deployment issue, there must be at most one canonical deployment gate state record, while every recheck remains visible as immutable historical events.

---

## 3. Canonical model overview

### 3.1 Aggregate relationship

For issue types `release` and `deployment`:

- an issue has **zero or one** canonical gate record,
- a gate record has **one or many** historical gate events,
- every state change or recheck appends a new event,
- the canonical gate record stores the **current projection** for efficient reads.

### 3.2 Source of truth

The source of truth is split into two layers:

- **history layer**: append-only gate events
- **current projection layer**: one canonical gate row containing the latest state

The current projection is authoritative for API reads, but it must always be derived by the write path from the latest event.

---

## 4. Data model

The implemented backend uses concise product-oriented naming. This architecture keeps those names as the canonical contract.

### 4.1 `deployment_gates`

One row per gated issue.

Recommended/implemented shape:

| Column | Type | Meaning |
|---|---|---|
| `id` | text/uuid | Gate identifier |
| `issue_key` | text unique | Parent issue key; enforces one canonical gate per issue |
| `status` | text | Current gate status projection |
| `unblock_condition` | text | Current human-readable condition to become or remain unblocked |
| `created_at` | timestamp | Gate creation time |
| `updated_at` | timestamp | Last projection update |

#### Invariant

`deployment_gates.issue_key` must be unique.

That uniqueness is the mechanism that prevents parallel canonical gate records for the same deployment/release issue.

### 4.2 `deployment_gate_events`

Append-only history table.

| Column | Type | Meaning |
|---|---|---|
| `id` | text/uuid | Event identifier |
| `gate_id` | text/uuid FK | Parent canonical gate |
| `status` | text | Gate status at the time of the event |
| `unblock_condition` | text | Human-readable condition at the time of the event |
| `reason` | text | Why this event was recorded (`initial`, `status_update`, `legacy_backfill`, etc.) |
| `created_at` | timestamp | Event timestamp |

#### Invariant

Events are immutable after insertion. Rechecks append a new row; they do not replace or delete prior rows.

---

## 5. Canonical semantics

### 5.1 Current gate record

The `deployment_gates` row is the canonical answer to:

- does this release/deployment issue currently have a gate?
- what is the gate’s current status?
- what condition must be satisfied to unblock or confirm readiness?

### 5.2 Event history

`deployment_gate_events` preserves every meaningful gate update, including:

- initial gate creation,
- blocked-to-blocked rechecks,
- blocked-to-unblocked transitions,
- unblocked-to-blocked regressions,
- compatibility backfills during migration.

### 5.3 Recheck rule

A recheck must:

1. locate the existing canonical gate by `issue_key`,
2. update the gate row with the newest `status` and `unblock_condition`,
3. append a new `deployment_gate_events` row describing that recheck.

It must **not** create another gate row for the same issue.

---

## 6. State model

The product currently exposes a compact status model rather than the richer SO-62 `unblock_state` enum. The canonical product contract is therefore:

### 6.1 Gate status

`deployment_gates.status` holds the current gate state projection.

Practical supported values are those already meaningful in issue workflows, especially:

- `blocked`
- `in_progress`
- `todo`
- `done`
- other issue-compatible statuses where needed

For deployment gating behavior, the important semantic distinction is whether the issue is currently blocked or not. The gate row carries the stronger domain meaning; issue status alone does not.

### 6.2 Unblock condition

`deployment_gates.unblock_condition` is the canonical machine-queryable text field representing the current unblock requirement or readiness note.

Examples:

- `awaiting QA signoff`
- `wait for canary metrics`
- `ready to deploy`
- empty string when there is no explicit condition recorded

### 6.3 Why use `unblock_condition` instead of a separate enum?

This matches the existing implemented backend and is sufficient for the current problem:

- API consumers can directly read one field for the current unblock requirement.
- humans still get readable context without parsing comments.
- the model remains additive and low-risk.

If a future product need requires machine-classified unblock states, an enum can be added later without breaking this canonical record structure.

---

## 7. Write-path behavior

### 7.1 Gate creation

A canonical gate must exist for issue types `release` and `deployment`.

Creation points:

- when a release/deployment issue is created,
- when an existing issue is updated into a gated type,
- during migration/backfill for preexisting issues.

Backend implementation guidance:

- use an `EnsureCanonicalDeploymentGate(issueKey, status, unblockCondition)` helper,
- if a gate already exists, do not create another,
- if a gate exists without events, backfill one synthetic event.

### 7.2 Gate update / recheck append

When a release/deployment issue receives a status update or unblock-condition update:

- update the canonical `deployment_gates` row,
- append a `deployment_gate_events` row with the new values,
- stamp a reason such as `status_update`.

#### Important case: blocked remains blocked

Even if the gate remains blocked, a recheck still appends a new event if the update is intended to record a new evaluation or refreshed reason. This preserves audit history.

### 7.3 Atomicity requirement

Updating the canonical gate and inserting the event should occur in one transaction so the current projection cannot drift from history.

---

## 8. Read-path / API behavior

### 8.1 Issue detail API

`GET /api/v1/issues/{key}` must expose the current gate projection directly on the issue payload.

Current implemented API fields:

- `gate_status`
- `unblock_condition`

Example:

```json
{
  "issue": {
    "key": "SO-600",
    "type": "deployment",
    "status": "blocked",
    "gate_status": "blocked",
    "unblock_condition": "wait for canary metrics"
  }
}
```

### 8.2 Semantics of `gate_status`

`gate_status` is derived from the canonical `deployment_gates.status` row, not from comments and not solely from issue status.

Consumers should prefer:

- `gate_status` for deployment gate state,
- `unblock_condition` for the current unblock requirement.

### 8.3 Queryability requirement

The unblock condition becomes queryable because it is persisted on the canonical gate row and surfaced through the issue API. Consumers no longer need to scrape markdown artifacts or comments.

### 8.4 Future-compatible API extension

If richer workflows are later needed, the API can add a nested object such as:

```json
{
  "deployment_gate": {
    "status": "blocked",
    "unblock_condition": "wait for canary metrics"
  }
}
```

However, the current flat issue fields are acceptable and backward-compatible with the implemented backend.

---

## 9. Migration and backward compatibility

### 9.1 Migration goals

Migration must be additive and safe:

- preserve old comments/files,
- introduce canonical machine-readable records,
- avoid changing historical issue semantics for consumers that have not yet adopted gate fields.

### 9.2 Implemented migration behavior

Migration `025_deployment_gates.sql` performs three important actions:

1. creates `deployment_gates`,
2. creates `deployment_gate_events`,
3. backfills canonical gate rows for existing `release` and `deployment` issues and seeds one event when history is missing.

### 9.3 Legacy compatibility path

A legacy gate row may exist without any event history. In that case:

- the backend must detect the missing history,
- append a synthetic `legacy_backfill` event,
- preserve the existing current `status` and `unblock_condition`.

This guarantees that old records can participate in the new append-only model without losing their present state.

### 9.4 Backward-compatibility rules

- Existing issue status remains valid for issue workflow purposes.
- Deployment gate consumers should prefer `gate_status` and `unblock_condition` when present.
- Legacy comments/artifacts remain historical narrative, not machine source of truth.
- No existing consumer should break if it ignores the new fields.

---

## 10. Backend implementation guidance

### 10.1 Required backend invariants

Backend must enforce:

1. one `deployment_gates` row per `issue_key`,
2. append-only `deployment_gate_events`,
3. no parallel gate rows on recheck,
4. current issue API projection sourced from canonical gate row.

### 10.2 Recommended helper responsibilities

`EnsureCanonicalDeploymentGate(...)`

- only apply to `release` and `deployment` issues,
- create missing canonical row,
- seed initial event,
- backfill missing legacy history when needed.

`AppendDeploymentGateStatus(...)`

- load canonical gate,
- update `status` and `unblock_condition`,
- insert a new event with the same new values,
- return current gate projection.

### 10.3 Failure handling

If event insert fails, the gate row update should not commit independently. Use one transaction.

### 10.4 Performance

Current projection fields belong on `deployment_gates` so issue reads remain cheap. Clients should not have to scan event history to know current gate state.

---

## 11. QA validation guidance

QA should validate both data-shape and behavioral invariants.

### 11.1 Core scenarios

1. **Canonical creation**
   - Create a `deployment` issue.
   - Verify one gate row exists.
   - Verify one initial event exists.

2. **Recheck appends history**
   - Update a deployment issue’s `status` or `unblock_condition` multiple times.
   - Verify only one gate row exists.
   - Verify event count increments per update.

3. **Blocked-to-blocked recheck**
   - Apply a recheck that keeps `gate_status=blocked` but changes reason/condition.
   - Verify a new event is appended rather than silently ignored.

4. **Issue API projection**
   - Call `GET /api/v1/issues/{key}`.
   - Verify `gate_status` and `unblock_condition` match the latest gate row.

5. **Legacy compatibility**
   - Prepare a gate row with no events.
   - Trigger ensure/read path.
   - Verify a `legacy_backfill` event is created.

6. **Non-gated issues**
   - Create a non-`release`/`deployment` issue.
   - Verify no unnecessary gate row is created and API remains stable.

### 11.2 Regression risks to watch

- duplicate gate rows for the same issue,
- updates that change issue status but not gate projection,
- gate projection returned from stale issue fields instead of canonical gate row,
- migration that drops current unblock condition,
- rechecks that overwrite or delete old history.

---

## 12. Architectural decision

Secondorder should treat deployment gate tracking as an **issue-scoped canonical record with append-only event history**.

Concretely:

- `deployment_gates` is the one canonical current-state record per release/deployment issue,
- `deployment_gate_events` is the immutable audit/history stream,
- rechecks append events under the same canonical gate,
- `GET /api/v1/issues/{key}` exposes the current state via `gate_status` and `unblock_condition`,
- migration is additive and preserves legacy artifacts while moving machine-readable truth into the canonical tables.

This design satisfies the core requirement: the current deployment unblock state becomes queryable via API without creating parallel decision files or sacrificing audit history.
