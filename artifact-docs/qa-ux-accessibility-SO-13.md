# QA UX + Accessibility Report — SO-13

Date: 2026-03-28
Scope: All UI pages (dashboard, issues, issue detail, agents, agent detail, work blocks, work block detail, run detail, command palette, all forms)

---

## Critical (P0) — Security

### BUG-1: XSS in Toast Notifications via innerHTML
**File:** `internal/templates/partials.html:110`
**WCAG:** N/A — security issue

```javascript
el.innerHTML = '<div ...>' + title + '</div><div ...>' + body + '</div>';
```

`title` = `d.author + ' on ' + d.issue_key` and `body` = `d.body.substring(0, 80)` come from SSE JSON. `d.body` is a user-supplied comment body inserted raw into `innerHTML`. An attacker who can post a comment with `<img src=x onerror=alert(document.cookie)>` triggers stored XSS on every page for every user.

**Fix:** Replace `innerHTML` with `textContent` for title and body, or HTML-escape before insertion.

---

### BUG-2: XSS in Command Palette Search Results
**File:** `internal/templates/partials.html:139`

```javascript
html += '<a href="' + it.url + '" ...><span>' + it.title + '</span></a>';
```

`it.title`, `it.key`, and `it.url` are unescaped API responses. An issue titled `<script>alert(1)</script>` or an agent slug containing `"` + JS would inject into the DOM.

**Fix:** HTML-escape `it.title`, `it.key`, and `it.url` before concatenation (use a helper like `escapeHtml()`).

---

### BUG-3: XSS via onclick in "Create Issue" Link (Palette)
**File:** `internal/templates/partials.html:142`

```javascript
onclick="event.preventDefault();createFromPalette('" + q.replace(/'/g, "\\'") + "')"
```

The escape only handles single quotes. A query containing `"` (double quote) breaks out of the HTML `onclick="..."` attribute, allowing arbitrary JS injection.

**Fix:** JSON-encode the value: `onclick="createFromPalette(' + JSON.stringify(q) + ')"`, or build the link without inline-onclick.

---

## High (P1) — Functional / UX

### BUG-4: No Mobile Navigation
**Files:** `partials.html:20-69` (nav), every template (`ml-56`)

The sidebar nav is `position: fixed; width: 224px` with no responsive breakpoint. On screens ≤ 640px the main content offset (`ml-56`) shifts content under or past the nav. No hamburger menu, no mobile drawer. The entire app is unusable on mobile.

**Fix:** Add responsive handling — at minimum hide the sidebar below `md:` and add a mobile-accessible menu.

---

### BUG-5: Destructive Actions Fire Without Confirmation
**Files:** `issue_detail.html:150-156`, `work_block_detail.html:87-93`

"Cancel" on an issue and "Cancel" on a work block POST immediately on button click with no confirmation dialog. Clicking "Ship" on a work block also fires without confirmation. These actions change state that is not easily reversed through the UI.

**Fix:** Add `onclick="return confirm('Cancel this issue?')"` or a JS confirmation modal for Cancel and Ship actions.

---

### BUG-6: Status Dropdown Auto-Submits on Change Without Undo
**File:** `issue_detail.html:101`

```html
<select name="status" onchange="this.form.submit()">
```

An accidental `onchange` immediately posts the new status. There is no confirmation and no undo available.

**Fix:** Remove `onchange` auto-submit; add an explicit "Update" button.

---

### BUG-7: Headers Set After WriteHeader in RunStdout
**File:** `internal/handlers/ui.go:386-391`

```go
w.WriteHeader(286)           // sends headers immediately
w.Header().Set("Content-Type", "text/html")  // ignored — headers already sent
```

When a run is complete, `WriteHeader(286)` is called first. The subsequent `w.Header().Set("Content-Type", ...)` is a no-op because headers are already sent. Go emits a superfluous call warning in the stdlib. This is functionally tolerable (HTMX handles 286 specially) but is incorrect HTTP practice.

**Fix:** Move `w.Header().Set("Content-Type", "text/html")` before the `if` block.

---

### BUG-8: Agent Slug Collision — No Uniqueness Check
**File:** `internal/handlers/ui.go:226`

Slug is derived from name: `strings.ToLower(strings.ReplaceAll(name, " ", "-"))`. Two agents named "My Agent" produce the same slug. `CreateAgent` will fail silently (DB unique constraint), and the user is redirected to `/agents/my-agent` which now shows the first agent — no error shown.

**Fix:** Check for existing slug before insertion; return an error response if duplicate.

---

## Medium (P2) — Accessibility (WCAG)

### A11Y-1: SVG Icons Not Hidden from Screen Readers
**File:** `partials.html:30-58` (nav links)
**WCAG:** 1.1.1 Non-text Content — Level A

Nav link SVG icons lack `aria-hidden="true"`. Screen readers will attempt to read the path data. Combined with the visible text, this is redundant and confusing.

**Fix:** Add `aria-hidden="true"` to all decorative `<svg>` elements.

---

### A11Y-2: Active Agent Indicator Dot Has No Text Alternative
**Files:** `dashboard.html:88`, `agents.html:94`, `agent_detail.html:22`
**WCAG:** 1.1.1 Non-text Content — Level A

```html
<div class="w-2 h-2 rounded-full bg-emerald-400"></div>
```

Color-only status indicator. Screen readers and colorblind users cannot determine active/inactive state.

**Fix:** Add `aria-label="Active"` or `aria-label="Inactive"`, or add a visually hidden span.

---

### A11Y-3: `<nav>` Has No aria-label
**File:** `partials.html:20`
**WCAG:** 1.3.1 Info and Relationships — Level A

The page has one `<nav>` element without `aria-label`. When navigating by landmarks, screen readers announce it as "navigation" with no context.

**Fix:** Add `aria-label="Main navigation"` to `<nav>`.

---

### A11Y-4: Form Labels Not Associated with Inputs
**Files:** `work_blocks.html:17,21`, `agents.html:22+`
**WCAG:** 1.3.1 Info and Relationships — Level A

Labels use `<label class="...">Title</label>` without `for="..."` matching an input `id`. Screen readers cannot associate the label with the control.

**Fix:** Add `for` + `id` pairs: `<label for="wb-title">Title</label><input id="wb-title" name="title" ...>`.

---

### A11Y-5: Command Palette Input Lacks aria-label
**File:** `partials.html:83`
**WCAG:** 1.3.1 Info and Relationships — Level A

```html
<input id="palette-input" type="text" placeholder="Search issues and agents...">
```

`placeholder` alone is not a label (disappears on input, not announced reliably). No `aria-label` present.

**Fix:** Add `aria-label="Search issues and agents"`.

---

### A11Y-6: Active Nav Link Missing aria-current
**File:** `partials.html:28-58`
**WCAG:** 2.4.1 Bypass Blocks — Level AA (Best Practice)

Nav links always render with the same classes — there is no active state indicator and no `aria-current="page"` on the current page link.

**Fix:** Pass current path from handler and apply `aria-current="page"` to the matching nav link.

---

### A11Y-7: Low Contrast on Timestamp Text
**Files:** All templates (e.g., `dashboard.html:71`)
**WCAG:** 1.4.3 Contrast (Minimum) — Level AA

`text-zinc-600` (#52525b) on `bg-zinc-950` (#09090b) = ~4.1:1 contrast ratio. WCAG AA requires 4.5:1 for normal text. Timestamps, secondary metadata, and dividers fail this threshold.

**Affected elements:** `timeAgo` outputs, parent breadcrumbs separators, "Created/Updated" labels.

**Fix:** Replace `text-zinc-600` with `text-zinc-500` (~5.4:1) for any meaningful text.

---

### A11Y-8: Details/Summary Chevron Does Not Animate
**Files:** `issue_detail.html:52`, `agent_detail.html:122`
**WCAG:** N/A — UX issue, not pure accessibility

The chevron SVG has class `chev` and `transition-transform` but never receives a rotation class when the `<details>` is opened. No CSS targets `details[open] .chev`.

**Fix:** Add CSS: `details[open] .chev { transform: rotate(90deg); }`

---

## Low (P3) — UX / Polish

### UX-1: Page `<title>` Never Changes
**File:** `partials.html:6`

Every page shows `<title>The Last Org</title>`. Browser tabs, screen reader page announcements, and browser history show no context. The issue key, agent name, or section name should be in the title.

---

### UX-2: No Success/Error Feedback After Form Submissions
**File:** `internal/handlers/ui.go` (all handlers)

Create issue, create agent, create work block, comment, assign — all silently redirect on both success and failure. DB errors are ignored. Users have no confirmation that their action succeeded, or why it failed.

---

### UX-3: Issue Titles Not Shown in Full on Hover
**Files:** `issues.html:66`, `dashboard.html:68`

Issue titles use `truncate` (overflow: hidden) with no `title="{{.Title}}"` tooltip. Long titles are cut off with no way to view them without clicking through.

---

### UX-4: Work Block Assign-Issue Accepts Free Text Without Validation Feedback
**File:** `work_block_detail.html:47`

The assign-issue form accepts a free-text issue key input. If the key doesn't exist, `AssignIssueToWorkBlock` fails silently and the user is redirected back with no error.

---

### UX-5: Keyboard: Escape Doesn't Restore Focus After Palette Close
**File:** `partials.html:127`

Pressing Escape closes the command palette, but focus is not returned to the element that opened it (the Search button in nav). This breaks keyboard navigation flow.

---

## Responsive Breakpoints Summary

| Breakpoint | Issues |
|---|---|
| Mobile < 640px | Nav overlaps content (P1 — BUG-4) |
| Tablet 640–1024px | Stats grid degrades gracefully (OK) |
| Desktop > 1024px | All layouts render as designed (OK) |

---

## WCAG Violation Summary

| ID | Criterion | Level | Status |
|---|---|---|---|
| A11Y-1 | 1.1.1 Non-text Content | A | FAIL |
| A11Y-2 | 1.1.1 Non-text Content | A | FAIL |
| A11Y-3 | 1.3.1 Info & Relationships | A | FAIL |
| A11Y-4 | 1.3.1 Info & Relationships | A | FAIL |
| A11Y-5 | 1.3.1 Info & Relationships | A | FAIL |
| A11Y-6 | 2.4.1 Bypass Blocks | AA | FAIL |
| A11Y-7 | 1.4.3 Contrast (Minimum) | AA | FAIL |

---

## Prioritized Fix List

1. **BUG-1** — XSS in toast via innerHTML (security, trivial fix)
2. **BUG-2** — XSS in command palette (security, trivial fix)
3. **BUG-3** — XSS in onclick from palette query (security)
4. **BUG-4** — No mobile nav (UX, significant work)
5. **BUG-5** — Destructive actions without confirmation (UX)
6. **A11Y-4** — Labels not associated with inputs (a11y Level A)
7. **A11Y-2** — Active dot no text alternative (a11y Level A)
8. **A11Y-7** — Contrast failures on zinc-600 text (a11y Level AA)
9. **BUG-7** — Headers set after WriteHeader (correctness)
10. **BUG-8** — Agent slug collision no error (UX)

---

## Manual Verification Checklist

1. [ ] Open a new incognito tab, navigate each of the 8 pages — confirm no JS errors in console
2. [ ] Post a comment containing `<b>bold</b>` — verify it renders as plain text in the toast, not as HTML
3. [ ] Search the command palette for a term with `"` and `<` — verify no HTML injection in results
4. [ ] Resize browser to 375px width — verify nav and content are accessible
5. [ ] Use Tab key from the top of each page — verify focus order is logical and all interactive elements receive visible focus ring
6. [ ] Use a screen reader (VoiceOver on Mac) and navigate the Issues page — verify status badges and timestamps are readable
7. [ ] Click "Cancel" on an issue — verify the action requires confirmation before firing
8. [ ] Create two agents with the same name — verify an error message is shown
9. [ ] View a run with status "running" — verify the 2s HTMX poll stops after run completes (verify in Network tab)
10. [ ] Check all page `<title>` tags in browser tabs reflect the current page/entity name
