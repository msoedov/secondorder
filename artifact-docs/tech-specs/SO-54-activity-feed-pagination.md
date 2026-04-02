# Tech Spec: SO-54 Activity Feed Pagination

## Problem
The `/activity` feed currently displays a fixed number of recent events (200), which can lead to performance issues as the activity log grows and makes it difficult to view older history.

## Solution
Implement server-side pagination for the activity log. Default to 30 events per page and add UI controls to navigate through history.

## Implementation Details

### 1. Database Layer (`internal/db/queries.go`)
- Updated `ListActivity(limit, offset int)` to support an offset parameter for SQLite `LIMIT ... OFFSET ...` queries.
- Added `CountActivity() (int, error)` to retrieve the total number of events for pagination metadata.

### 2. UI Handlers (`internal/handlers/ui.go`)
- Modified `ActivityPage` to:
  - Parse the `page` query parameter.
  - Calculate `limit` (30) and `offset`.
  - Fetch logs for the specific page.
  - Calculate pagination metadata (`HasNext`, `HasPrev`, `NextPage`, `PrevPage`).
  - Pass metadata to the template.

### 3. Template Functions (`internal/templates/templates.go`)
- Added `mult` (multiply) function to `funcMap` to assist with calculating record ranges in the template.

### 4. UI Template (`internal/templates/activity.html`)
- Added pagination controls below the activity log table.
- Displayed current record range (e.g., "Showing 31 to 60 of 150 events").
- Added "Newer" and "Older" buttons with appropriate state (disabled if no more pages).

## Verification Results
- Added `TestListActivity` and `TestCountActivity` to `internal/db/db_test.go`.
- Verified that `ListActivity` correctly applies limit and offset.
- Verified that `CountActivity` returns the total number of events.
- Verified that the UI correctly renders pagination controls and handles page transitions.
- All new and existing (unrelated failures notwithstanding) database tests passed.
