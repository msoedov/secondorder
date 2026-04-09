# SO-72: Session-Scoped API Keys for Agent Runs

**Status:** Implemented  
**Priority:** HIGH (P2)  
**Parent:** SO-69

---

## Problem

The API key embedded in each agent's system prompt was revoked when **any** new run started for
the same agent. A single `RevokeAPIKeys(agentID)` call wiped all keys regardless of which run
they belonged to. This caused:

- Mid-run 401 failures when a heartbeat or reviewer run spawned concurrently
- Agents falling back to SQLite direct writes (fragile, undocumented)
- Incorrect status updates and orphaned state
- CEO-reported: "The API key embedded in the system prompt gets revoked each session"

---

## Solution: Run-Scoped Key Lifecycle

Each agent **run** receives its own unique API key at spawn time. Keys are:

1. **Bound to a run** (`api_keys.run_id`) — not to the agent globally
2. **Agent-scoped** — the key resolves only to the agent that owns it
3. **Time-limited** — idle timeout enforced via `api_keys.expires_at`
4. **Non-interfering** — revoking run-A's key does not affect run-B (same agent)

### Key Format

```
so_<32-bytes-hex>     (e.g. so_a3f2...d9e1)
```

Keys are stored as SHA-256 hashes in the database; only the raw key is transmitted to the agent.

---

## Database Schema (Migration 018)

```sql
ALTER TABLE api_keys ADD COLUMN run_id TEXT REFERENCES runs(id);
ALTER TABLE api_keys ADD COLUMN expires_at DATETIME;
CREATE INDEX IF NOT EXISTS idx_api_keys_run ON api_keys(run_id);
```

### Full `api_keys` table structure

| Column       | Type     | Description                                          |
|-------------|----------|------------------------------------------------------|
| id           | TEXT PK  | UUID                                                 |
| agent_id     | TEXT FK  | Agent that owns this key                             |
| run_id       | TEXT FK  | Run that this key was issued to (nullable: legacy)   |
| key_hash     | TEXT     | SHA-256 of the raw key                               |
| prefix       | TEXT     | First 12 chars of raw key (display only)             |
| created_at   | DATETIME |                                                      |
| revoked_at   | DATETIME | Set when key is explicitly revoked                   |
| expires_at   | DATETIME | Set to `now + idle_timeout` at creation              |

---

## Key Lifecycle

```
Run starts → provisionAPIKey(agentID, runID)
              ├── RevokeRunAPIKey(runID)     ← revoke any prior key for *this run* only
              ├── generate 32-byte random key
              └── CreateAPIKey(agentID, runID, keyHash, prefix, idleTimeout=60min)
                        ↓
              Key injected into SECONDORDER_API_KEY env var for the agent process

Run completes → RevokeRunAPIKey(runID)      ← explicit cleanup

Background cron (every 60s) → ExpireStaleAPIKeys()
              └── UPDATE api_keys SET revoked_at=now WHERE expires_at <= now AND revoked_at IS NULL
```

### Validity check (Auth middleware)

```sql
SELECT ...
FROM api_keys k JOIN agents a ON k.agent_id = a.id
WHERE k.key_hash = ?
  AND k.revoked_at IS NULL
  AND (k.expires_at IS NULL OR k.expires_at > datetime('now'))
```

The `expires_at IS NULL` arm preserves backward compatibility with legacy keys that have no expiry.

---

## Acceptance Criteria — Verification

| AC  | Criterion | Status | Test |
|-----|-----------|--------|------|
| AC#1 | New run start for agent A does NOT revoke the API key of a prior still-running run by agent A | ✅ | `TestSessionKeyNotRevokedOnNewRun`, `TestRevokeRunAPIKeyOnlyAffectsTargetRun` |
| AC#2 | Keys are agent-scoped — a key for agent A cannot authenticate as agent B | ✅ | `TestSessionKeyNotRevokedOnNewRun` |
| AC#3 | Keys expire after run ends (idle timeout ≥ 30min) | ✅ | `TestSessionKeyExpiresAfterIdleTimeout`, `TestExpireStaleAPIKeys` (60min default) |
| AC#4 | Existing PATCH/POST/GET endpoints work with session-scoped keys (no API contract changes) | ✅ | All existing handler tests pass |
| AC#5 | Existing long-lived (legacy) keys still work | ✅ | `TestLegacyKeyBackwardCompat` |
| AC#6 | Unit tests for key lifecycle | ✅ | 5 new tests in `internal/db/db_test.go` |

---

## Files Changed

### New
- `internal/db/migrations/018_session_scoped_api_keys.sql` — schema migration

### Modified
- `internal/db/queries.go`
  - `CreateAPIKey(agentID, runID, keyHash, prefix string, idleTimeout time.Duration)` — new signature with run binding and expiry
  - `GetAgentByAPIKey(keyHash)` — adds `expires_at` check
  - `RevokeRunAPIKey(runID)` — replaces `RevokeAPIKeys(agentID)` (run-scoped, not agent-wide)
  - `ExpireStaleAPIKeys()` — periodic cleanup, returns rows affected
- `internal/scheduler/scheduler.go`
  - `provisionAPIKey(agentID, runID)` — calls `RevokeRunAPIKey` (not agent-wide) + `CreateAPIKey`
  - Background ticker: `ExpireStaleAPIKeys()` every 60s
- `internal/db/db_test.go` — 5 new session-scoped key lifecycle tests

### Documentation
- `artifact-docs/tech-specs/SO-72-session-scoped-api-keys.md` (this file)

---

## Idle Timeout Configuration

The default idle timeout is **60 minutes**. This is configurable in `provisionAPIKey` — change the
`idleTimeout` argument to `CreateAPIKey`. The AC requires ≥ 30 min; 60 min is the current default.

Future: expose via `settings` table so operators can adjust without recompile.

---

## Backward Compatibility

Legacy keys (inserted before migration 018, or without `run_id`/`expires_at`) continue to work.
The auth query includes `expires_at IS NULL OR expires_at > datetime('now')` — keys with no
expiry set are treated as valid indefinitely, preserving existing agent configurations.

---

## Testing

```bash
# Run all session-key lifecycle tests
go test ./internal/db/... -run "TestSession|TestLegacy|TestRevokeRun|TestExpireStale|TestAPIKey" -v

# Full suite
go test ./...
```

Expected output (all PASS):
```
=== RUN   TestAPIKeyLifecycle
--- PASS: TestAPIKeyLifecycle
=== RUN   TestSessionKeyNotRevokedOnNewRun
--- PASS: TestSessionKeyNotRevokedOnNewRun
=== RUN   TestSessionKeyExpiresAfterIdleTimeout
--- PASS: TestSessionKeyExpiresAfterIdleTimeout
=== RUN   TestLegacyKeyBackwardCompat
--- PASS: TestLegacyKeyBackwardCompat
=== RUN   TestRevokeRunAPIKeyOnlyAffectsTargetRun
--- PASS: TestRevokeRunAPIKeyOnlyAffectsTargetRun
=== RUN   TestExpireStaleAPIKeys
--- PASS: TestExpireStaleAPIKeys
```
