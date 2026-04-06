-- Add type field to issues
ALTER TABLE issues ADD COLUMN type TEXT NOT NULL DEFAULT 'task';
