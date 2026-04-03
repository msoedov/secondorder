ALTER TABLE audit_runs ADD COLUMN runner TEXT NOT NULL DEFAULT 'claude_code';
ALTER TABLE audit_runs ADD COLUMN model TEXT NOT NULL DEFAULT 'sonnet';
