CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT OR IGNORE INTO settings (key, value) VALUES ('issue_prefix', 'SO');
INSERT OR IGNORE INTO settings (key, value) VALUES ('telegram_token', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('telegram_chat_id', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('github_url', '');
INSERT OR IGNORE INTO settings (key, value) VALUES ('instance_name', '');
