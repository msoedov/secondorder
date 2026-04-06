# PRD: Acceptance Criteria Completeness Validator (SO-98)

**Status:** Proposed
**Date:** 2026-04-05
**Author:** Product Lead (SO-98)

---

## 1. Problem Statement

The CEO agent is responsible for delegating work by creating sub-issues with clear scope and acceptance criteria (AC). Currently, there is no validation to ensure these AC are present or sufficient for specific categories of work. This results in:
- Worker agents being blocked because they lack critical implementation details (e.g., endpoint paths for API tasks).
- Increased token consumption due to back-and-forth communication between agents for clarification.
- Inconsistent issue quality across the board.

## 2. Goals

- **Structured Categorization:** Introduce an `issue_type` field to categorize work.
- **Automated Validation:** Implement rules to verify AC completeness based on `issue_type`.
- **Proactive Feedback:** Provide warnings to the CEO agent and human users during issue creation if AC are missing.

## 3. Proposed Solution

### 3.1. Issue "Type" Field
We will add a structured `type` field to the `Issue` model to drive validation logic.

- **Field Name:** `type`
- **Location:** New column in `issues` table.
- **Allowed Values:** `api`, `backend`, `frontend`, `task`, `bug`, `feature`.
- **Default Value:** `task`.

### 3.2. Validation Rules

Validation will be triggered when an issue is created or updated. The validator will scan the `description` field for an "Acceptance Criteria" or "AC" section and verify the presence of key information.

| Type | Required Acceptance Criteria (Key Information) |
|---|---|
| **api** | 1. **Endpoint Path & Method** (e.g., `GET /v1/users`) <br> 2. **Request Schema** (Body params or Query params) <br> 3. **Response Schema** (Success body) <br> 4. **Status Codes** (e.g., 200, 400, 404) |
| **backend** | 1. **Core Logic** (Algorithm or business rule description) <br> 2. **Persistence** (Database changes or state management) <br> 3. **Dependencies** (External APIs or internal services) |
| **generic** | At least one bullet point or numbered list under an AC header. |

### 3.3. Validator Logic (Regex-based)

The validator should look for a section starting with `## Acceptance Criteria`, `### AC`, or similar.
Within that section, it searches for keywords:
- For `api`: `path`, `method`, `endpoint`, `request`, `response`, `status code`, `200`, `400`.
- For `backend`: `logic`, `database`, `table`, `column`, `service`, `dependency`.

## 4. User Experience & UI

### 4.1. CEO Agent (API Feedback)
When the CEO agent calls `POST /api/v1/issues`, the response will include a `warnings` array if validation fails.

**Example Response:**
```json
{
  "id": "51a77b7c-...",
  "key": "SO-101",
  "status": "todo",
  "warnings": [
    "Type 'api' usually requires an endpoint path and method.",
    "Type 'api' should specify expected status codes."
  ]
}
```
*Note: The CEO agent's system prompt should be updated to instruct it to "Read API warnings and fix issues immediately if possible."*

### 4.2. Human Dashboard (UI Feedback)
- **Field:** Add a "Type" dropdown to the Create/Edit Issue forms.
- **Warning:** If a user saves an issue with missing AC for its type, show a non-blocking toast warning: *"Heads up: This [Type] issue seems to be missing some standard acceptance criteria. This might block your agents."*

## 5. Implementation Plan

1. **Database:** Add `type` column to `issues` table (Migration 011).
2. **Models:** Add `Type` field to `models.Issue` struct.
3. **Internal:** Create a `validator` package to encapsulate the regex logic.
4. **API:** Update `CreateIssue` and `UpdateIssue` handlers to run validation and return warnings.
5. **UI:** Update templates to include the `Type` field and display warnings.

## 6. Acceptance Criteria for this Feature

- [ ] `type` field is persisted in the database for every issue.
- [ ] API returns relevant warnings for `api` and `backend` types when AC are missing.
- [ ] UI shows a warning toast when a human creates an issue with incomplete AC.
- [ ] CEO agent continues to function without interruption (warnings are non-blocking).
