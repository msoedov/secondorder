# SO-107: Strategic Alignment UI/UX Design
_Issue: SO-107 | Date: 2026-04-09_

## Overview

Three surfaces require design changes to surface strategic alignment between Apex Blocks (the "Why") and Work Blocks (the "How"):

1. **Strategy page** — richer Apex Block management, per-block stats, unaligned visibility
2. **Work Block forms** — alignment-aware proposal and edit forms with alignment validation cue
3. **Dashboard** — richer strategic alignment panel showing per-apex progress

Static HTML mockups are in:
- `mockup-strategy.html`
- `mockup-workblock-forms.html`
- `mockup-dashboard.html`

---

## 1. Strategy Page

### Current state
- Lists Apex Blocks with aligned Work Blocks
- Single alignment score (%)
- Toggle status only (no edit, no delete)
- No visibility into unaligned Work Blocks
- No per-Apex aggregate metrics

### Design changes

**Apex Block card — add:**
- Edit button (pencil icon) → opens edit modal (same fields as create: title + goal)
- Aggregate metrics row: `N issues · $cost · X work blocks`
- Status badge pill remains; toggle stays as icon button

**New sidebar panel: Unaligned Work Blocks**
- Appears below Alignment Score when alignment < 100%
- Lists Work Blocks with `apex_block_id = NULL`, status not cancelled
- Each item: status badge + title + link to work block
- CTA: "Assign to a goal →" links to the block's edit modal

**Alignment Score widget — add:**
- Absolute counts below the bar: "X / Y Work Blocks aligned"
- Color coding: green ≥ 80%, amber 50–79%, red < 50%

**Edit Apex Block modal** (new):
- Same fields as create: Title, Strategic Intent
- Submit → PATCH /strategy/apex/{id}
- Separate "Archive" link at bottom (replaces toggle icon — makes destructive action explicit)

### Layout sketch

```
┌─ Header ──────────────────────────────────────── [Create Apex Block] ─┐
│ Strategy · Define constitutional goals and monitor alignment.          │
└────────────────────────────────────────────────────────────────────────┘

┌─ 2/3 column ──────────────────────────┐ ┌─ 1/3 sidebar ───────────────┐
│                                       │ │ Alignment Score             │
│  ┌─ Apex Block card ───────────────┐  │ │ ████████░░ 75%             │
│  │ [ACTIVE]  Market Expansion Q2   │  │ │ 6 / 8 Work Blocks aligned  │
│  │ [edit] [archive]                │  │ │                             │
│  │ Grow into APAC markets...       │  │ │ ─────────────────────────── │
│  │ ─────────────────────────────   │  │ │ Unaligned (2)               │
│  │ 3 issues · $12.40 · 2 blocks   │  │ │ · [PROPOSED] Auth Rework    │
│  │                                 │  │ │ · [ACTIVE] Cleanup Sprint   │
│  │  Aligned Work Blocks            │  │ │                             │
│  │  · [ACTIVE] User Onboarding →   │  │ │ ─────────────────────────── │
│  │  · [SHIPPED] Pricing Page →     │  │ │ Guidelines                  │
│  └─────────────────────────────────┘  │ │ • Apex Blocks = "Why"      │
│                                       │ │ • Work Blocks = "How"      │
│  ┌─ Apex Block card ───────────────┐  │ │ • Agents use these goals   │
│  │ [ACTIVE]  Reduce Time-to-Value  │  │ │   to self-prioritize.      │
│  │ ...                             │  │ └─────────────────────────────┘
│  └─────────────────────────────────┘  │
└───────────────────────────────────────┘
```

---

## 2. Work Block Forms

### Current state
- Propose form (inline in work_blocks.html): fields stack vertically, Apex Block dropdown is labeled "Parent Apex Block (Strategic Goal)" with option "None / Maintenance / Overhead"
- Edit modal (work_block_detail.html): same fields in modal overlay
- No visual cue when no Apex Block is selected (alignment gap not surfaced)

### Design changes

**Propose form — alignment indicator:**
- When Apex Block dropdown = "None", show a yellow warning row:
  `⚠ This block will be unaligned. Unaligned blocks don't contribute to strategic goals.`
- When Apex Block is selected, show a green confirmation row:
  `✓ Aligned to: [Goal Title]`
- Use JS (`change` event on select) to toggle the row — no server round-trip

**Propose form — field order resequence:**
- Move "Parent Apex Block" to position 2 (after Title, before Goal)
- This signals that alignment is a first-class concern, not an afterthought

**Edit modal — same alignment indicator:**
- Same JS behavior: show warning or confirmation based on selected value
- On load: if `apex_block_id` is set, pre-render the confirmation row

**Label copy:**
- Change "Parent Apex Block (Strategic Goal)" → "Strategic Goal (Apex Block)"
- Change "None / Maintenance / Overhead" → "None — unaligned"

### Propose form field order

```
Title                     [required]
Strategic Goal (Apex)     [dropdown — with alignment indicator below]
  └─ ⚠ Unaligned / ✓ Aligned to: X
Goal                      [textarea]
Acceptance Criteria       [textarea]
North Star Metric / Target [2-col grid]
[Create]
```

### Edit modal field order

Same as propose form, values pre-filled.

---

## 3. Dashboard

### Current state
- `Strategic Alignment` card: score %, progress bar, link to /strategy
- `Board Pulse` card: static text referencing alignment score
- Active Work Block card: shows title, goal, issues/cost
- No per-apex visibility

### Design changes

**Strategic Alignment panel — expand to show apex blocks:**
- Replace generic "X%" with a per-apex-block row list (up to 3, then "View all")
- Each row: Apex Block title + work block count + mini progress bar (completed/planned issues ratio across all aligned blocks)
- Keep overall score % as the header number

**Board Pulse — make dynamic:**
- If alignment < 100%: "X work blocks are unaligned. The CEO will prioritize linking them."
- If alignment = 100%: "All work blocks are aligned. Full strategic coverage."
- If no apex blocks: "No strategic goals defined. Create an Apex Block to start."

**Active Work Block card — add apex link:**
- Below the block title, add: `Strategic Goal: [Apex Title]` as a small badge/link
- If not aligned: show `⚠ Unaligned` in amber

### Dashboard layout change

```
┌─ Strategic Alignment (md:col-span-2) ───────────────────────────┐
│ Strategic Alignment                               75% →          │
│ ███████████████░░░░░                                             │
│                                                                  │
│  Market Expansion Q2        2 blocks  ██████░░ 4/6 issues       │
│  Reduce Time-to-Value       1 block   ████████ 8/8 issues       │
│  Infrastructure Hardening   1 block   ██░░░░░░ 1/5 issues       │
└──────────────────────────────────────────────────────────────────┘

┌─ Board Pulse ──────────────────────────────────────────────────┐
│  ● LIVE                                                         │
│  2 work blocks are unaligned. CEO will prioritize linking them. │
└─────────────────────────────────────────────────────────────────┘
```

---

## Token / Style Notes

All mockups use the existing design token system (`bg-ac`, `text-ink`, etc.) without introducing new tokens. Specific classes:

| Element | Classes |
|---|---|
| Alignment warning row | `text-amber-400 bg-amber-500/10 border border-amber-500/20 rounded px-3 py-2 text-xs` |
| Alignment confirmed row | `text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 rounded px-3 py-2 text-xs` |
| Unaligned badge in detail | `text-amber-400 text-[11px] font-medium` |
| Apex mini-bar | `h-1 bg-sf rounded overflow-hidden` + inner `bg-ac` |
| Unaligned count pill | `text-[11px] text-amber-400 bg-amber-500/10 px-2 py-0.5 rounded` |

---

## Interaction Notes

- All alignment indicator toggles are pure JS (`addEventListener('change', ...)`) — no HTMX required
- Edit Apex Block form: POST to `/strategy/apex/{id}` with `action=update` hidden field
- Unaligned panel in sidebar: rendered server-side, filtered from Work Blocks where `apex_block_id IS NULL AND status NOT IN ('shipped','cancelled')`
- Dashboard per-apex rows: new data passed to dashboard template as `ApexProgress []ApexProgressRow` where each row has `ApexBlock`, `WorkBlockCount`, `IssuesPlanned`, `IssuesCompleted`

---

## Manual Checklist

1. [ ] Open `mockup-strategy.html` and verify Apex Block cards render correctly with aggregate metrics
2. [ ] Check the Unaligned panel appears/hides based on alignment state in strategy mockup
3. [ ] Open `mockup-workblock-forms.html` — toggle the Apex Block dropdown and verify the alignment indicator changes
4. [ ] Confirm warning row (amber) and confirmed row (green) are visually distinct and accessible
5. [ ] Open `mockup-dashboard.html` — verify per-apex rows render and mini progress bars are legible
6. [ ] Check Board Pulse copy matches the three states (unaligned, full coverage, no apex blocks)
7. [ ] Check the "⚠ Unaligned" badge on the active work block card (dashboard) against the green accent color of the card border
8. [ ] Verify all anchor links in mockups point to correct routes (`/strategy`, `/work-blocks/{id}`)
9. [ ] Check color contrast: amber-400 on card background (light and dark mode)
10. [ ] Confirm no new CSS tokens are introduced — all classes map to existing design token set
