-- Add stages and current_stage_id to issues
ALTER TABLE issues ADD COLUMN stages TEXT NOT NULL DEFAULT '[]';
ALTER TABLE issues ADD COLUMN current_stage_id INTEGER NOT NULL DEFAULT 0;
