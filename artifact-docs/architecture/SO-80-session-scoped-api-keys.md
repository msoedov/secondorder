# SO-80: Session-scoped API Keys

**Status:** Implemented (PR #3)  
**Priority:** HIGH (P2)  
**Replaces:** SO-72 (cancelled)

## Problem

Prior to this change, the `api_keys` table had no run binding — `RevokeAPIKeys(agentID)` would invalidate **all** keys for an agent. This meant starting a second run for agent A would revoke the key used by any currently-running instance of agent A, causing:

- Mid-task auth failures (`401 Unauthorized`)
- Agents falling back to SQLite direct writes (fragile workaround)
- Incorrect status updates and orphaned run state

## Solution

Bind each API key to a specific `run_id` instead of just `agent_id`. Key lifecycle:

```
spawnAgent() → provisionAPIKey(agentID, runID) → CreateAPIKey(..., runID, ttl=2h)
                                                         ↓
                                               api_keys row: run_id = runID
                                                             expires_at = now+2h
                                                             revoked_at = NULL
```

When a run completes (normally or via timeout): `RevokeRunAPIKey(runID)` only revokes that specific run's key — **other runs are unaffected**.

## Data Model

```sql
-- Migration 018 adds to existing api_keys table:
ALTER TABLE api_keys ADD COLUMN run_id TEXT REFERENCES runs(id);
ALTER TABLE api_keys ADD COLUMN expires_at DATETIME;
CREATE INDEX idx_api_keys_run ON api_keys(run_id);
```

Full schema:
```sql
CREATE TABLE api_keys (
  id         TEXT PRIMARY KEY,
  agent_id   TEXT NOT NULL REFERENCES agents(id),
  run_id     TEXT REFERENCES runs(id),    -- NULL for legacy keys
  key_hash   TEXT NOT NULL UNIQUE,
  prefix     TEXT NOT NULL,
  revoked_at DATETIME,                     -- NULL = active
  expires_at DATETIME,                     -- NULL = no expiry (legacy)
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

## DB Functions

| Function | Signature | Notes |
|---|---|---|
| `CreateAPIKey` | `(agentID, runID, keyHash, prefix string, ttl time.Duration) error` | Stores hash, sets `expires_at = now + ttl` |
| `GetAgentByAPIKey` | `(keyHash string) (*Agent, error)` | Checks `revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now)` |
| `RevokeRunAPIKey` | `(runID string) error` | Sets `revoked_at = now` WHERE `run_id = runID` only |
| `ExpireStaleAPIKeys` | `() (int64, error)` | Batch-revoke keys past `expires_at` |

## Auth Middleware

`internal/handlers/api.go — Auth()` calls `GetAgentByAPIKey(keyHash)` which already handles expiry:

- Key found, not revoked, not expired → authenticated as the key's agent ✅
- Key not found / revoked / expired → `ErrNoRows` → `401 Unauthorized` ✅
- Legacy key (no `expires_at`) → `expires_at IS NULL` treated as no expiry → still works ✅

## Scheduler Integration

`internal/scheduler/scheduler.go`:
- `provisionAPIKey(agentID, runID)` called at run start, injects `rawKey` into agent environment
- Key TTL: **2 hours** (satisfies AC#2: ≥2h)
- `StartAPIKeyExpiryLoop(interval)` runs a periodic sweep (recommended: 1 min) to expire stale keys

## Acceptance Criteria

| AC | Description | How Verified |
|---|---|---|
| AC#1 | Starting run B for agent A does NOT revoke run A key | `TestSessionKeyNotRevokedOnNewRun`, `TestRevokeRunAPIKeyOnlyAffectsTargetRun` |
| AC#2 | Each run gets its own key, valid ≥2h | `provisionAPIKey` uses `2*time.Hour`; `TestSessionKeyNotRevokedOnNewRun` |
| AC#3 | Expired key returns 401 | `TestSessionKeyExpiresAfterIdleTimeout` |
| AC#4 | `ValidateAPIKey()` path unchanged | `TestLegacyKeyBackwardCompat` |
| AC#5 | `go build ./...` and `go test ./...` pass | CI / all 8 packages green |
| AC#6 | PR on castrojo/secondorder | PR #3 |

## Files Changed

- `internal/db/migrations/018_session_scoped_api_keys.sql` — schema migration
- `internal/db/queries.go` — `CreateAPIKey`, `GetAgentByAPIKey`, `RevokeRunAPIKey`, `ExpireStaleAPIKeys`
- `internal/scheduler/scheduler.go` — `provisionAPIKey` (2h TTL), `StartAPIKeyExpiryLoop`
- `internal/db/db_test.go` — 5 new tests covering all ACs
