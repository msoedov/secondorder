# SO-71: Always show assign form on agent detail

## Changes

- Modified `internal/handlers/ui.go` in `AgentDetail` to fetch all open issues (`AvailableIssues`) and pass them to the template.
- Modified `internal/templates/agent_detail.html` to:
    - Move the "Assign" form above the inbox list (per audit recommendation).
    - Use the `.AvailableIssues` for the assignment dropdown, ensuring it is useful even when the agent's inbox is empty.
    - Ensure the "Inbox" section header is always visible regardless of whether the agent has assigned issues.
    - Show a "No issues assigned to this agent's inbox." message when the inbox is empty.

## Verification

- The "Assign" form is now outside the `{{if .Issues}}` block, making it persistently visible.
- The dropdown now contains all issues in `todo`, `in_progress`, or `in_review` status, instead of being limited to the agent's current inbox.
- The inbox section now provides clear feedback when empty instead of disappearing entirely.
