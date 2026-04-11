# SO-74: Canonical deployment gate remediation (QA uncached gaps)

## Context

QA identified uncached execution gaps after SO-63 where canonical deployment gate coverage still referenced missing backend pieces in a clean environment. This remediation completes the missing model constants/aliases, issue exposure fields, and migration-safe test scaffolding so canonical gate state and history work end to end.

## Remediation implemented

- Added deployment gate model constants in `internal/models/models.go`:
  - `GateStatusOpen`, `GateStatusBlocked`, `GateStatusPassed`, `GateStatusClosed`
  - `UnblockStateBlocked`, `UnblockStateUnblocked`, `UnblockStateUnknown`
- Added issue/API field support for current unblock state:
  - `Issue.UnblockState` (`json:"unblock_state,omitempty"`)
  - `DeploymentGate.UnblockState` derived in DB reads
- Added compatibility issue-type alias:
  - `TypeDeployment = TypeDeploy`

## Persistence and API wiring updates

- Updated `internal/db/queries.go` to derive and expose `unblock_state` in issue projections.
- Added gate-status to unblock-state derivation helper and wired it into canonical gate creation/read/update flows.
- Existing canonical behavior remains intact:
  - one gate per deployment/release issue (`deployment_gates.issue_key` unique)
  - rechecks append immutable history events (`deployment_gate_events`)
  - current gate summary remains sourced from canonical gate row + latest updates

## Clean-environment test fixes

- Updated migration compatibility test fixture in `internal/db/db_test.go` to include minimal `issues` table required by migration `025_deployment_gates.sql`.
- Extended tests to assert current unblock state alongside gate status:
  - `internal/db/db_test.go`
  - `internal/handlers/handlers_test.go`

## Validation

- `go test ./internal/db -count=1`
- `go test ./internal/handlers -count=1`

## Compatibility notes

- `TypeDeployment` is an additive alias; existing `deployment` behavior is unchanged.
- `unblock_state` is additive in API payloads and backward compatible for consumers that ignore unknown fields.
