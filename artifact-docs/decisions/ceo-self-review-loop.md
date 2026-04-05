# Decision: CEO Self-Review Loop
**Date:** 2026-04-05
**Status:** Documented

## Context
The `GetReviewer` logic defaults to the CEO if no explicit reviewer is assigned. When the CEO marks an issue as `done`, the system attempts to wake the CEO as the reviewer.

## Problem
This triggers a new CEO run, which rotates the API key and kills the current CEO run. It also creates redundant runs in the system.

## Proposed Fix
Update `GetReviewer` or `wakeReviewerForIssue` to avoid self-review loops. If the agent is the CEO and no reviewer is assigned, it should likely transition to a final state or await human board approval instead of spawning a new CEO run.
