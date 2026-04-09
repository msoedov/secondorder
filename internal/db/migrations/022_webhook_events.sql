-- Webhook events table for idempotency and audit trail
CREATE TABLE IF NOT EXISTS webhook_events (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,          -- e.g. "github", "generic"
    event_type TEXT NOT NULL,      -- e.g. "issues", "comments"
    delivery_id TEXT,              -- external delivery ID for dedup
    payload TEXT NOT NULL,         -- raw JSON payload
    status TEXT NOT NULL DEFAULT 'received',  -- received, processed, failed
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_source ON webhook_events(source);
CREATE INDEX IF NOT EXISTS idx_webhook_events_delivery_id ON webhook_events(delivery_id);
CREATE INDEX IF NOT EXISTS idx_webhook_events_created_at ON webhook_events(created_at);
