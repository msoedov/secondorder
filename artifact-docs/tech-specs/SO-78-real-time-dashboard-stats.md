# Tech Spec: SO-78 Real-time Dashboard Stats

## Overview
Add real-time refresh to the dashboard for agent status dots and run counts. This ensures the dashboard reflects current activity without manual refreshes.

## Changes

### Backend: SSE Broadcasts
- Added a `run_started` SSE event in `cmd/secondorder/main.go` that broadcasts when an agent run begins.
- Ensured existing `run_complete`, `agent_created`, `agent_updated`, and issue-related events are being broadcasted correctly.

### Database: Running Status
- Added `GetRunningAgentIDs` to `internal/db/queries.go` to efficiently identify which agents have active runs.

### UI Handlers: Dashboard Data
- Updated the `Dashboard` handler in `internal/handlers/ui.go` to populate a `RunningAgents` map (agent_id -> bool).

### Frontend: SSE Listeners & HTMX
- Updated `internal/templates/partials.html` to dispatch DOM events (`run-started`, `run-complete`, `agent-changed`, `issue-changed`) when SSE messages are received.
- Updated `internal/templates/dashboard.html` to listen for these DOM events using HTMX (`hx-trigger`).
- The stats and agents sections now automatically refresh via `hx-get="/dashboard"` when relevant events occur.
- Added a pulsing blue status dot for agents that are currently running a task.

## Verification
- Added a test case in `internal/scheduler/scheduler_test.go` to verify the `onRunStart` callback.
- Verified that SSE events trigger the expected DOM events in the browser.
- Manually confirmed that agent status dots change to blue/pulsing when a run starts and revert when it completes.
