-- Performance indexes for tables that may have been created in earlier migrations.
-- CREATE TABLE IF NOT EXISTS ensures compatibility with partial-schema test databases.

CREATE TABLE IF NOT EXISTS activity_log (
    id TEXT PRIMARY KEY,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    agent_id TEXT,
    details TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS comments (
    id TEXT PRIMARY KEY,
    issue_key TEXT NOT NULL,
    agent_id TEXT,
    author TEXT NOT NULL DEFAULT 'Board',
    body TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS approvals (
    id TEXT PRIMARY KEY,
    issue_key TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    reviewer_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    comment TEXT NOT NULL DEFAULT '',
    resolved_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cost_events (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_cost_usd REAL NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_activity_log_created ON activity_log(created_at);
CREATE INDEX IF NOT EXISTS idx_activity_log_entity_type ON activity_log(entity_type);
CREATE INDEX IF NOT EXISTS idx_comments_issue_agent ON comments(issue_key, agent_id);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);
CREATE INDEX IF NOT EXISTS idx_approvals_issue ON approvals(issue_key);
CREATE INDEX IF NOT EXISTS idx_cost_events_created_date ON cost_events(created_at);
