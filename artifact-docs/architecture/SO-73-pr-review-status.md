# SO-73: PR Review Status Indicator — Architecture

## Overview

This document describes the design and implementation of the PR review status indicator added to the mesa issue detail view.

## Problem

When an agent files a PR and marks an issue `in_review`, the CEO had to navigate to GitHub manually to check review state (CodeRabbit/Gemini comments, approval status) before approving the issue.

## Solution Summary

A PR Status section is injected into the issue detail sidebar whenever PR URLs are detected in issue comments. The section shows:

- A clickable link to the PR (`owner/repo#number`)
- A review status badge (`✅ Approved`, `⚠️ Changes Requested`, `🔄 Review Pending`)
- A bot blocker warning if CodeRabbit or Gemini has `CHANGES_REQUESTED`
- A graceful fallback ("PR status unavailable") when the GitHub API is inaccessible

## Architecture

### Component Diagram

```
Browser ──GET /issues/{key}──► IssueDetail (ui.go)
                                     │
                        ExtractPRURLs(commentBodies)
                                     │ returns []PRInfo
                        FetchAllPRReviews(prs)          ──► GitHub REST API
                                     │ (concurrent, max 5)    (with optional token)
                                     │ returns []PRInfo (with ReviewData or FetchError)
                                     ▼
                        issue_detail.html template
                        (PRInfos → sidebar "PR Status" card)
```

### File Inventory

| File | Role |
|------|------|
| `internal/handlers/github_pr.go` | Core logic: URL extraction, GitHub API calls, review state aggregation |
| `internal/handlers/github_pr_test.go` | Unit tests (13 tests) covering URL extraction, bot detection, review state, error fallback |
| `internal/handlers/ui.go` | `IssueDetail` handler — extracts PR URLs and passes `PRInfos` to template |
| `internal/templates/issue_detail.html` | Issue detail view — new PR Status sidebar card |

### Data Flow

1. **URL Extraction** (`ExtractPRURLs`): Scans issue description + all comments using the regex `https?://github\.com/([^/\s]+)/([^/\s]+)/pull/(\d+)`. Returns deduplicated `[]PRInfo`.

2. **Review Fetching** (`FetchAllPRReviews`): For each PR, calls `GET https://api.github.com/repos/{owner}/{repo}/pulls/{number}/reviews` concurrently (max 5 goroutines). Uses `GITHUB_PAT`, `GITHUB_TOKEN`, or `GH_TOKEN` env var if set; falls back to unauthenticated (60 req/hr).

3. **Review Aggregation**: Per-user latest review state wins. `COMMENTED` and `DISMISSED` do not override an earlier `APPROVED`/`CHANGES_REQUESTED`. Overall state is:
   - `"approved"` — at least one approval, no changes_requested
   - `"changes_requested"` — any reviewer requested changes
   - `"open"` — no qualifying reviews

4. **Bot Detection** (`isBotReviewer`): Logins containing `coderabbitai`, `gemini-code-assist`, `github-actions`, or `copilot` (case-insensitive) are flagged as bot reviewers.

5. **Template Rendering** (`issue_detail.html`): The `PRInfos` slice is iterated in the sidebar. The section is hidden (via `{{if .PRInfos}}`) when no PR URLs were found, satisfying AC#4/AC#5.

### Error Handling

| Condition | Behaviour |
|-----------|-----------|
| HTTP 429 (rate-limited) | `FetchError` set; template shows "PR status unavailable" |
| HTTP 403 (forbidden) | `FetchError` set; template shows "PR status unavailable" |
| HTTP 404 (private/missing PR) | `FetchError` set; template shows "PR status unavailable" |
| Network timeout (10s) | `FetchError` set; template shows "PR status unavailable" |
| No PR URLs in comments | `PRInfos` is empty; PR Status card hidden entirely |

### UI Rendered Output

**Issue with approved PR (no bot issues):**
```
┌─────────────────────────────┐
│  PR STATUS                  │
│  ↗ msoedov/mesa#17   │
│  ✅ Approved (2)            │
└─────────────────────────────┘
```

**Issue with changes requested by CodeRabbit:**
```
┌────────────────────────────────────────┐
│  PR STATUS                             │
│  ↗ msoedov/mesa#17             │
│  ⚠️ Changes Requested                 │
│  ⚠ coderabbitai has blocking comments │
└────────────────────────────────────────┘
```

**Issue with inaccessible PR:**
```
┌────────────────────────────────┐
│  PR STATUS                     │
│  ↗ msoedov/mesa#17     │
│  ⚠ PR status unavailable      │
└────────────────────────────────┘
```

**Issue with no PR URL:**  
*(PR Status card not rendered)*

## Acceptance Criteria Mapping

| AC | Implementation |
|----|----------------|
| AC#1: PR URLs extracted and shown as links | `ExtractPRURLs` + template `<a href="{{.URL}}">` |
| AC#2: Review state shown as badge | `FetchPRReviews` + template state conditional |
| AC#3: Warning if CodeRabbit/Gemini has `CHANGES_REQUESTED` | `isBotReviewer` + `BotBlockers` rendered in template |
| AC#4: Graceful when no PR URL | `{{if .PRInfos}}` gate — section hidden |
| AC#5: Graceful when GitHub API unavailable | `FetchError` path — shows fallback text, no error thrown |

## Token Configuration

Set one of the following environment variables to authenticate with GitHub and increase the rate limit from 60 to 5000 req/hr:

```bash
export GITHUB_TOKEN=ghp_...
# or
export GITHUB_PAT=ghp_...
# or
export GH_TOKEN=ghp_...
```

Unauthenticated mode is acceptable for low-traffic dashboard usage.

## Test Coverage

```
TestExtractPRURLs_Empty                    PASS
TestExtractPRURLs_NoPRs                   PASS
TestExtractPRURLs_SinglePR                PASS
TestExtractPRURLs_DedupAcrossComments     PASS
TestExtractPRURLs_MultiplePRsInOneBody    PASS
TestExtractPRURLs_HTTPVariant             PASS
TestIsBotReviewer                         PASS
TestFetchPRReviews_RateLimit              PASS
TestFetchPRReviews_NotFound               PASS
TestFetchPRReviews_Approved               PASS
TestFetchPRReviews_ChangesRequestedByBot  PASS
TestFetchPRReviews_NoReviews              PASS
TestFetchPRReviews_ChangesRequestedByHuman PASS
```

13/13 tests passing. Full suite `go test ./...` green with no regressions.
