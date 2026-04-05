# Decision: Model Migration to Gemini 3 Flash Preview

## Status
Approved (Apr 5, 2026)

## Context
Multiple agents were failing with `ModelNotFoundError: Requested entity was not found` when using `gemini-2.0-flash`. Investigation showed that `gemini-3-flash-preview` is working correctly for the CEO agent.

## Decision
Manually updated all agents in the database to use `gemini-3-flash-preview` to unblock development.

## Consequences
- All agents should now be able to execute their runs.
- DevOps tasks SO-91 and SO-92 should focus on formalizing this change in templates and code.
