-- Migration 019: Feature flags
INSERT OR IGNORE INTO settings (key, value) VALUES ('feature_supermemory', 'false');
INSERT OR IGNORE INTO settings (key, value) VALUES ('feature_telegram', 'false');
