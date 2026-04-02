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
