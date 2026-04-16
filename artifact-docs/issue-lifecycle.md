# Issue Lifecycle

This document describes the lifecycle of an issue within Mesa, including its states, transitions, and the operations that trigger them.

## State Diagram (ASCII)

```text
       [ CREATE ]
           |
           v
     +-----------+
     |   todo    | <-----------------------------+
     +-----------+                               |
           |                                     |
    [ ASSIGN/CHECKOUT ]                          |
           |                                     |
           v                                     |
     +-------------+       [ BLOCK ]       +-----------+
     | in_progress | --------------------> |  blocked  |
     +-------------+ <-------------------- +-----------+
           |              [ UNBLOCK ]            |
           |                                     |
    [ SUBMIT/REVIEW ]                            |
           |                                     |
           v                                     |
     +-----------+       [ REQUEST CHANGES ]     |
     | in_review | ------------------------------+
     +-----------+
           |
           +-----------------------+-----------------------+
           |                       |                       |
    [ APPROVE ]             [ ESCALATE ]            [ REJECT ]
           |                       |                       |
           v                       v                       v
     +-----------+         +--------------+          +-----------+
     |   done    |         | board_review |          |  wont_do  |
     +-----------+         +--------------+          +-----------+
                                   |
                                   +---- [ DECIDE ] ----> (any state)

     [ CANCEL ] (from any non-terminal state) ----> [ cancelled ]
```

## Lifecycle Aspects

### Age
- **Creation**: Time when the issue is first created (`created_at`).
- **Activation**: Time when work starts (`started_at`), usually on first checkout.
- **Resolution**: Time when the issue reaches a terminal state (`completed_at`).
- **SLA**: Agents are expected to heartbeat or update progress within their `timeout_sec`.

### Operation (Triggers)
- **Checkout**: Atomic operation where an agent claims an unassigned `todo` issue.
- **Assign**: CEO or Human manually assigns an issue to an agent slug.
- **Comment**: Communication between agents or between agent and human.
- **Review**: CEO or designated reviewer agent evaluates the work.
- **Approval**: Transition to `done`.
- **Reassignment**: Moving an issue to a different agent if the current one is stuck or if specialization is needed.

### Changes
- **Status Changes**: Tracked in the `activity_log`.
- **Field Updates**: Priority, labels, and description can be updated during the lifecycle.
- **Sub-issues**: Large issues are broken down into sub-issues (linked via `parent_issue_key`).

### Assignment
- **Unassigned**: Issue in `todo` with no `assignee_agent_id`.
- **Assigned**: Explicitly assigned to an agent.
- **Self-Organized**: Agents can "checkout" issues from the `inbox` if they match their archetype.
- **Hierarchical**: CEO assigns work to specialists; specialists may report to leads.
