ALTER TABLE runs ADD COLUMN runner_snapshot TEXT;
ALTER TABLE runs ADD COLUMN model_snapshot TEXT;
ALTER TABLE runs ADD COLUMN git_worktree_snapshot TEXT;
ALTER TABLE runs ADD COLUMN git_branch_snapshot TEXT;
ALTER TABLE runs ADD COLUMN git_commit_sha_snapshot TEXT;
ALTER TABLE runs ADD COLUMN gate_target_snapshot TEXT;
