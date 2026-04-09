CREATE TABLE IF NOT EXISTS supermemory_events (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    run_id       TEXT NOT NULL,
    event_type   TEXT NOT NULL DEFAULT 'recall',  -- 'store' or 'recall'
    query        TEXT NOT NULL DEFAULT '',         -- recall query (empty for store)
    result_count INTEGER NOT NULL DEFAULT 0,       -- result count for recall; 1 = success for store
    success      INTEGER NOT NULL DEFAULT 1,       -- 1 = API call succeeded, 0 = error
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agents(id),
    FOREIGN KEY (run_id)   REFERENCES runs(id)
);

CREATE INDEX IF NOT EXISTS idx_supermemory_agent ON supermemory_events(agent_id);
CREATE INDEX IF NOT EXISTS idx_supermemory_created ON supermemory_events(created_at);
