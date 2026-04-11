# SO-63: Canonical deployment gate persistence and API exposure

## Scope

Backend implementation for release/deployment issue gate persistence was updated to use a single canonical gate record per issue with append-only status history, and to expose the current gate state directly from issue API responses.

## Data model changes

Migration: `internal/db/migrations/025_deployment_gates.sql`

- Added `deployment_gates` table
  - `issue_key` is unique, enforcing one canonical gate per release/deployment issue
  - stores current gate status and unblock condition
- Added `deployment_gate_events` table
  - append-only event/history records per gate
  - every recheck or status update appends a new event
- Added migration backfill:
  - creates canonical gates for existing `release` and `deployment` issues
  - seeds one initial history event (`legacy_backfill`) where history is missing

## Backend behavior

Implemented in `internal/db/queries.go` and wired through `internal/handlers/api.go`.

- Canonical gate creation
  - `CreateIssue` and `UpdateIssue` call `EnsureCanonicalDeploymentGate(...)`
  - only applies to issue types `release` and `deployment`
- Recheck append semantics
  - `AppendDeploymentGateStatus(...)` updates current canonical gate status and appends an immutable event
  - no new parallel gate records are created
- Compatibility handling
  - if a legacy gate exists without events, `EnsureCanonicalDeploymentGate(...)` backfills a synthetic `legacy_backfill` history event

## API exposure

Issue API now exposes current gate state via `GET /api/v1/issues/{key}` in the issue object:

- `gate_status`
- `unblock_condition`

These are projected from the canonical gate row and are directly queryable by consumers without parsing comments.

`PATCH /api/v1/issues/{key}` now accepts:

- `unblock_condition` (optional)

When status or unblock condition is updated on release/deployment issues, a recheck event is appended to gate history.

## Automated coverage

Added tests for:

- canonical gate creation for deployment/release issues
- recheck append behavior on a single canonical gate record
- current status/unblock derivation via issue API projections
- compatibility path that backfills gate history for preexisting canonical rows missing events

Primary tests:

- `internal/db/db_test.go`
  - `TestDeploymentGateCanonicalCreationForDeploymentIssue`
  - `TestDeploymentGateRecheckAppendsEventsOnSingleGate`
  - `TestDeploymentGateLegacyCompatibilityBackfillsHistory`
  - `TestGetIssueIncludesCurrentGateFields`
- `internal/handlers/handlers_test.go`
  - `TestGetIssue_ExposesDeploymentGateFields`
