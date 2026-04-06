# Design Spec: Issue Stages Progress & Checklist (SO-104)

## Overview

This document specifies the UI/UX changes for implementing incremental delivery checkpoints (Stages) in the Issue Detail view. The goal is to provide high visibility into the progress of complex issues and allow for manual human intervention at each checkpoint.

## New Components

### 1. Progress Bar (Global Issue Scope)

A horizontal, segmented progress bar located at the top of the Issue Detail main content area.

- **Placement**: Below breadcrumbs, above the Issue Title.
- **Visuals**:
    - **Completed Stages**: Solid accent color (`bg-ac`).
    - **Current Stage**: Pulsing light accent background (`bg-ac/30 animate-pulse`) with a border.
    - **Future Stages**: Empty state with border (`bg-sf border-bd`).
- **Interactions**:
    - **Hover**: Displays a tooltip with the stage ID, title, and status.
- **Responsive**: Segments flex to fill the available width equally.

### 2. Stages Checklist (Sidebar)

A vertical list of stages with interactive checkboxes for manual status toggling.

- **Placement**: Top of the sidebar, above the "Status" widget.
- **Visuals**:
    - **Header**: "Stages" with a progress fraction (e.g., "2/4").
    - **Checkbox**: Standard styled checkbox.
    - **Label**:
        - `line-through` and dimmed text for completed stages.
        - Bold text and an "In Progress" pulsing dot for the current active stage.
- **Interactions**:
    - **Checking/Unchecking**: Triggers an immediate `PATCH` request to the API.
    - **SSE Updates**: The checklist refreshes automatically when an `issue_updated` event is received.

## Data Mapping

The UI relies on the following fields added to the `Issue` model:

- `stages`: JSON array of `IssueStage` objects.
- `current_stage_id`: Integer representing the 1-based index of the active stage.

```go
type IssueStage struct {
    ID      int    `json:"id"`
    Title   string `json:"title"`
    Status  string `json:"status"` // "todo", "done"
}
```

## Interaction Patterns

### Manual Human Toggle
1. Human operator clicks a checkbox in the sidebar.
2. Frontend sends `PATCH /api/v1/issues/{key}` with the modified `stages` array.
3. Backend updates the database and broadcasts `issue_updated` via SSE.
4. All connected clients (including the one that initiated the change) receive the SSE event.
5. HTMX triggers a surgical refresh of the stages components.

### Agent Progress Update
1. Agent completes work and posts a comment: `Stage 1: [Setup] - Complete`.
2. Backend parses the comment, updates the stage to `done`, and increments `current_stage_id`.
3. Backend broadcasts `issue_updated`.
4. UI refreshes in real-time, showing Stage 1 as complete and Stage 2 as active.

## Accessibility

- Use `<label>` elements for all stage titles in the checklist to ensure a large hit area and screen reader compatibility.
- Tooltips on the progress bar segments must have appropriate ARIA attributes if they contain critical information not present elsewhere.
- Checkboxes should follow standard keyboard navigation patterns.

## Mockups

Detailed HTML/CSS mockup available at: `artifact-docs/design/SO-104-issue-stages-mockup.html`
