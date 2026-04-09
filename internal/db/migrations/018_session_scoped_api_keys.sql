-- Migration 018: Session-scoped API keys
-- 
-- Problem: api_keys had no run_id binding, so RevokeAPIKeys(agentID) would
-- invalidate ALL keys for an agent — including keys from concurrent runs.
-- This caused mid-task auth failures when a second run spawned for the same agent.
--
-- Solution:
--   1. Add run_id column: key is bound to a specific run, not just an agent
--   2. Add expires_at column: idle timeout enforcement (AC#3 — ≥30 min)
--   3. RevokeAPIKeys now scoped by run_id (not agent-wide)
--   4. GetAgentByAPIKey now checks expires_at in addition to revoked_at

ALTER TABLE api_keys ADD COLUMN run_id TEXT REFERENCES runs(id);
ALTER TABLE api_keys ADD COLUMN expires_at DATETIME;

CREATE INDEX IF NOT EXISTS idx_api_keys_run ON api_keys(run_id);
