CREATE TABLE IF NOT EXISTS deployment_gates (
    id TEXT PRIMARY KEY,
    issue_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'open',
    unblock_condition TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (issue_key) REFERENCES issues(key) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS deployment_gate_events (
    id TEXT PRIMARY KEY,
    gate_id TEXT NOT NULL,
    status TEXT NOT NULL,
    unblock_condition TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (gate_id) REFERENCES deployment_gates(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_deployment_gate_events_gate_created_at
    ON deployment_gate_events(gate_id, created_at);

INSERT INTO deployment_gates (id, issue_key, status, unblock_condition, created_at, updated_at)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' ||
    lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))) AS id,
    i.key,
    CASE WHEN i.status = 'blocked' THEN 'blocked' ELSE 'open' END,
    CASE WHEN i.status = 'blocked' THEN 'Resolve blockers and run deployment gate recheck.' ELSE '' END,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM issues i
LEFT JOIN deployment_gates g ON g.issue_key = i.key
WHERE g.id IS NULL
  AND i.type IN ('release', 'deployment');

INSERT INTO deployment_gate_events (id, gate_id, status, unblock_condition, reason, created_at)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' ||
    lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))) AS id,
    g.id,
    g.status,
    g.unblock_condition,
    'legacy_backfill',
    g.created_at
FROM deployment_gates g
LEFT JOIN deployment_gate_events e ON e.gate_id = g.id
WHERE e.id IS NULL;
