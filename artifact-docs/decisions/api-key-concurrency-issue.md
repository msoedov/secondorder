# Decision: API Key Concurrency Issue
**Date:** 2026-04-05
**Status:** Documented

## Context
When an agent (especially the CEO) has multiple concurrent runs (e.g., a heartbeat run starting while a task run is active), the `spawnAgent` function rotates the API key and revokes all previous keys for that agent. This causes the earlier run to lose API access immediately, leading to `invalid api key` errors.

## Problem
This is particularly problematic for the CEO agent, who frequently updates issue statuses. Marking an issue as `done` or `in_review` triggers a "wake reviewer" event. If the reviewer is the same agent (defaulting to the CEO), it spawns a new run for the same agent, thereby revoking the active run's key.

## Proposed Fix
1. Modify `provisionAPIKey` to only revoke keys if they are older than a certain threshold or if explicitly requested.
2. In `wakeReviewerForIssue`, check if the reviewer is already active for the current issue or if the reviewer is the same as the current agent, and handle it without spawning a full new run.
