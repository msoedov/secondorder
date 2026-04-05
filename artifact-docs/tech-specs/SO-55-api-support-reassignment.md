# SO-55: API Support Reassignment in UpdateIssue

## Problem
Currently, the `PATCH /api/v1/issues/{key}` endpoint does not support updating the assignee of an issue. This is required for the CEO to rebalance workload among agents.

## Solution
Update the `UpdateIssue` API handler to support an optional `assignee_slug` field in the request body.

### API Changes

#### `PATCH /api/v1/issues/{key}`

**Request Body:**
- `status` (optional, string): The new status of the issue.
- `comment` (optional, string): A comment to add to the issue.
- `title` (optional, string): The new title of the issue.
- `description` (optional, string): The new description of the issue.
- `priority` (optional, integer): The new priority of the issue.
- `assignee_slug` (optional, string): The slug of the agent to reassign the issue to. Use an empty string (`""`) to unassign the issue.

**Response:**
- `200 OK`: Issue updated successfully.
- `400 Bad Request`: Invalid request body.
- `403 Forbidden`: The authenticated agent is not the current assignee and is not the CEO.
- `404 Not Found`: Issue or target agent not found.

## Implementation Details
1.  Modified `internal/handlers/api.go`'s `UpdateIssue` handler to include `AssigneeSlug *string` in the request body struct.
2.  If `AssigneeSlug` is provided:
    - If it is an empty string, set `issue.AssigneeAgentID` to `nil`.
    - Otherwise, lookup the agent by slug using `a.db.GetAgentBySlug`. If not found, return `404 Not Found`.
3.  The issue is then updated in the database via `a.db.UpdateIssue(issue)`.
4.  Standard ownership checks remain in place, allowing only the current assignee or the CEO to perform the update.
5.  Added `assignee_slug` to the SSE `issue_updated` broadcast. Fixed a bug where it was incorrectly reporting empty slug when not changed.
6.  Added logic to wake the new assignee if an `in_progress` issue is re-assigned.
7.  Added bypass for retry limit (6 runs) when an issue is being reassigned or updated by the CEO, allowing stuck issues to be revived.
8.  Modified `models.Issue` and database queries to include `assignee_slug` in all issue-related API responses.
9.  Fixed compilation and logic issues in `internal/handlers/handlers_test.go`.
