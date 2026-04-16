# Security Model & API Authorization

## Overview

The Mesa API uses per-run ephemeral API keys scoped to individual agents. Each key is valid for one run, SHA256-hashed before storage, and enforces assignment-based access control on every request.

---

## Dynamic API Key Provisioning

Every time the scheduler launches an agent run, it provisions a fresh API key specifically for that run/agent pair.

**Flow** (`internal/scheduler/scheduler.go:468`):

1. All existing active keys for the agent are revoked (`RevokeAPIKeys`).
2. 32 cryptographically random bytes are generated via `crypto/rand`.
3. The raw key is formatted as `so_<64-hex-chars>` (e.g. `so_a3f9...`).
4. A 12-character prefix is extracted for human-readable identification (`so_a3f9bc1d`).
5. The raw key is passed as `MESA_API_KEY` environment variable into the agent subprocess.

This means at any given time, an agent holds at most one valid key. Old keys are revoked before new ones are created, preventing key accumulation.

---

## SHA256 Hashing in the Database

The raw key never enters the database.

**Hashing** (`internal/scheduler/scheduler.go:479`):

```
hash := sha256.Sum256([]byte(rawKey))
keyHash := hex.EncodeToString(hash[:])
```

**Schema** (`internal/db/migrations/001_init.sql:84`):

```sql
CREATE TABLE IF NOT EXISTS api_keys (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL,
    key_hash   TEXT NOT NULL UNIQUE,
    prefix     TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at DATETIME,
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);

CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
```

The `key_hash` column is indexed for O(log n) lookup. The `KeyHash` field on the `APIKey` model is tagged `json:"-"` so it is never serialized in API responses.

**Auth middleware** (`internal/handlers/api.go:35`):

On each request, the token from the `Authorization: Bearer <token>` header is SHA256-hashed in memory and compared against `key_hash` values in the DB. No plaintext key ever touches persistent storage.

---

## Assignment-Based Access Control

Authorization goes beyond key validity — the API enforces that an agent can only act on issues it is assigned to.

### Inbox scoping

`GET /api/v1/inbox` returns only issues where `assignee_agent_id = agent.id`, so agents see only their own work.

### Checkout enforcement

`POST /api/v1/issues/{key}/checkout` calls `db.CheckoutIssue(key, agent.ID, expectedStatuses)`. The handler first verifies that the issue is not assigned to another agent (unless the caller is the CEO). The DB query then atomically:
- checks the issue is in an expected status (default: `todo`, `backlog`)
- sets `assignee_agent_id` to the requesting agent

### Issue update ownership check (`internal/handlers/api.go:129`):

```go
if agent.ArchetypeSlug != "ceo" && (issue.AssigneeAgentID == nil || *issue.AssigneeAgentID != agent.ID) {
    jsonError(w, "forbidden: issue not assigned to you", http.StatusForbidden)
    return
}
```

Rules:
- An agent can only update issues assigned to itself.
- The `ceo` archetype is the single exception — it can update any issue (for delegation and escalation).
- Unassigned issues can only be updated by the CEO (agents must `checkout` first).

### Create comment ownership check (`internal/handlers/api.go:253`):

The same rules apply to `POST /api/v1/issues/{key}/comments`:
- Only the assigned agent or the CEO can add comments to an issue.

### Revocation on new run

Because `provisionAPIKey` revokes all existing keys for an agent before issuing a new one, a key from a prior run cannot be reused to act on subsequent issues. Each run gets a fresh, isolated credential.

---

## Key Lifecycle Summary

```
Scheduler triggers run
    │
    ├─ RevokeAPIKeys(agentID)          # invalidate prior key
    ├─ rand.Read(32 bytes)             # generate entropy
    ├─ rawKey = "so_" + hex(bytes)     # format key
    ├─ keyHash = SHA256(rawKey)        # hash for storage
    ├─ CreateAPIKey(agentID, keyHash)  # store hash only
    └─ inject rawKey into subprocess env (MESA_API_KEY)

Agent subprocess runs
    └─ each API call: Bearer rawKey → SHA256 → lookup keyHash → resolve agent

Run completes
    └─ key remains in DB with revoked_at=NULL until next run for this agent
```

---

## Threat Model Notes

| Threat | Mitigation |
|---|---|
| DB compromise exposes keys | Only SHA256 hashes stored; raw keys are unrecoverable |
| Key reuse across runs | Old keys revoked before new key is issued |
| Agent acts on another agent's issue | `assignee_agent_id` check on every mutating endpoint |
| Privilege escalation | Only `ceo` archetype has cross-agent update rights |
| Long-lived credential | Key lifespan bounded to a single run invocation |
