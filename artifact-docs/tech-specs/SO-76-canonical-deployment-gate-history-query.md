# SO-76: Canonical deployment gate status history query support

## Scope

Implemented backend API behavior so deployment/release issues expose the canonical gate record and append-only gate status history directly from the existing canonical persistence model.

## What changed

- Extended `GET /api/v1/issues/{key}` in `internal/handlers/api.go`:
  - for `release` and `deployment` issue types, response now includes:
    - `deployment_gate` (current canonical gate projection)
    - `deployment_gate_history` (append-only status event history)
- Canonical compatibility behavior is enforced in the read path:
  - issue reads call `EnsureCanonicalDeploymentGate(...)` for deployment/release issue types before projecting gate data
  - this preserves legacy compatibility by backfilling missing initial history when required
- Added handler coverage in `internal/handlers/handlers_test.go`:
  - `TestGetIssue_IncludesCanonicalDeploymentGateAndHistory`
  - `TestGetIssue_DoesNotIncludeDeploymentGateForNonDeploymentIssueType`

## API shape example

`GET /api/v1/issues/SO-601` now includes canonical gate context:

```json
{
  "issue": {
    "key": "SO-601",
    "type": "release",
    "gate_status": "blocked",
    "unblock_state": "blocked",
    "unblock_condition": "wait for canary metrics"
  },
  "deployment_gate": {
    "issue_key": "SO-601",
    "status": "blocked",
    "unblock_state": "blocked",
    "unblock_condition": "wait for canary metrics"
  },
  "deployment_gate_history": [
    {
      "reason": "created"
    },
    {
      "reason": "recheck"
    }
  ]
}
```

## Validation

- `go test ./internal/handlers -run 'TestGetIssue_(ExposesDeploymentGateFields|IncludesCanonicalDeploymentGateAndHistory|DoesNotIncludeDeploymentGateForNonDeploymentIssueType)$' -count=1`

## Notes

- This change is additive and preserves existing fields (`gate_status`, `unblock_state`, `unblock_condition`) for current clients.
- No parallel decision files are required for clients to read current unblock status or inspect status history.
