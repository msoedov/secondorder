# SecondOrder Brand System
## Brand Audit + Design Tokens Reference
_Issue: SO-10 | Date: 2026-03-28_

---

## 1. Audit: Current Visual Identity

### Color System

| Role | Current value | Problem |
|---|---|---|
| Background | `bg-zinc-950` (#09090b) | Identical to Linear, Plane, Raycast, 90% of dark SaaS |
| Surface | `bg-zinc-900`, `bg-zinc-800/50` | Cool-gray default; no warmth or character |
| Border | `border-zinc-800`, `border-zinc-700` | Invisible at scale, no depth |
| Text primary | `text-zinc-100` / `text-zinc-200` | Fine |
| Text muted | `text-zinc-500` / `text-zinc-600` | Fine |
| CTA/Accent | `bg-indigo-600`, `text-indigo-400` | Indigo is the most overused SaaS accent of 2023-2025 |
| Status: in_progress | `text-blue-400` | Generic |
| Status: in_review | `text-amber-400` | Good — distinctive |
| Status: done | `text-emerald-400` | Common but readable |
| Status: blocked | `text-red-400` | Appropriate |

**Key finding**: The entire UI is zinc-950 + indigo-600. This is indistinguishable from a Tailwind template. There is zero brand signal in the color choices.

### Typography

- Font: system stack only — `-apple-system, BlinkMacSystemFont, 'Inter', sans-serif`
- No web font loaded. No brand typographic voice.
- Type scale is very compressed: 10px labels, 11px metadata, 13px body, 14px (sm) forms.
- The compressed scale is a good fit for a dense tool, but without a distinctive typeface, it reads as a Figma wireframe.
- Mono: falls back to system mono (Menlo/Consolas). No explicit mono choice.

### Logo / Wordmark

- Favicon: `zinc-950` background, indigo horizontal lines, emerald circle. Describes the UI but is not a mark.
- Wordmark: Plain text "The Last Org" `text-sm font-semibold text-zinc-100`. No logotype, no mark, no character.
- "L5 Agent" subtitle under the wordmark is unexplained and visually orphaned.

### Brand Voice mismatch

The name "The Last Org" is a bold claim — final, definitive, authoritative. The visual language is the opposite: tentative, generic, grey. A product with this name should feel like a command center, not a Tailwind starter.

---

## 2. Color Palette Proposal

### Design decision

Move from **cool zinc** (blue-tinted neutral) to **warm stone** (brown-tinted neutral). This single shift immediately differentiates from the 90% of tools using zinc/slate.

Replace **indigo** accent with **amber/gold**. Amber reads as decisive, premium, and authoritative — matches the product's claim. It's almost absent from the current dark SaaS landscape.

### Token definitions

```
// Base palette
--color-bg:          stone-950   (#0c0a09)  — page background
--color-surface:     stone-900   (#1c1917)  — nav, sidebar, cards
--color-surface-2:   stone-800   (#292524)  — elevated surfaces, inputs
--color-border:      stone-700   (#44403c)  — primary borders
--color-border-sub:  stone-800   (#292524)  — subtle dividers

// Text
--color-text-1:      stone-50    (#fafaf9)  — headings, primary
--color-text-2:      stone-200   (#e7e5e4)  — body
--color-text-3:      stone-400   (#a8a29e)  — secondary/metadata
--color-text-4:      stone-500   (#78716c)  — muted/timestamps
--color-text-5:      stone-600   (#57534e)  — placeholder/disabled

// Accent (CTA, links, highlights)
--color-accent:      amber-400   (#fbbf24)  — primary accent
--color-accent-bg:   amber-500/10            — subtle accent fill
--color-accent-ring: amber-500/25            — focus ring

// Status colors (unchanged — semantics are sound)
--status-todo:       stone-400/zinc-400
--status-progress:   blue-400
--status-review:     amber-400    (reuses accent — intentional)
--status-done:       emerald-400
--status-blocked:    red-400
--status-cancelled:  stone-500
```

### Tailwind class mappings (swap table)

| Current | Proposed | Location |
|---|---|---|
| `bg-zinc-950` | `bg-stone-950` | `<body>` in partials.html |
| `bg-zinc-900` | `bg-stone-900` | nav, sidebar |
| `bg-zinc-800/50` | `bg-stone-800/50` | cards, surfaces |
| `bg-zinc-800` | `bg-stone-800` | hover states, stat chips |
| `border-zinc-800` | `border-stone-800` | all card borders |
| `border-zinc-700` | `border-stone-700` | input borders |
| `text-zinc-100` | `text-stone-50` | primary text |
| `text-zinc-200` | `text-stone-200` | body text |
| `text-zinc-400` | `text-stone-400` | secondary text |
| `text-zinc-500` | `text-stone-500` | muted text |
| `text-zinc-600` | `text-stone-600` | timestamps, dividers |
| `bg-indigo-600` | `bg-amber-500` | CTA buttons (New Issue, New Agent, Create) |
| `hover:bg-indigo-500` | `hover:bg-amber-400` | CTA hover |
| `text-white` (on indigo btn) | `text-stone-950` | button label — dark on amber |
| `text-indigo-400` | `text-amber-400` | links, key hover, assignee |
| `hover:text-indigo-300` | `hover:text-amber-300` | link hover |
| `focus:ring-indigo-500` | `focus:ring-amber-500` | all form focus rings |
| `focus:border-indigo-500` | `focus:border-amber-500` | all form focus borders |
| `text-indigo-400` (palette +) | `text-amber-400` | command palette create |
| `bg-indigo-950/20` (diff @@) | `bg-amber-950/20` | diff hunk header |
| `text-indigo-400` (diff @@) | `text-amber-400` | diff hunk text |
| `text-indigo-600` (checkbox) | `text-amber-500` | checkbox accent (agents form) |
| `border-indigo-500/30` (active wb) | `border-amber-500/30` | active work block border |

### Status colors (keep, minor adjustment)

`in_review` already uses `amber-400` — this now harmonizes with the accent instead of clashing. No change needed to status color functions in templates.go.

---

## 3. Typography Recommendation

### Primary: Geist

Geist is designed specifically for developer tools. It reads cleanly at 10-13px, has a strong geometric character without being cold, and pairs naturally with its companion mono variant.

**CDN (Google Fonts):**
```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Geist:wght@300;400;500;600&family=Geist+Mono:wght@400;500&display=swap" rel="stylesheet">
```

**CSS update in partials.html:**
```css
body {
  font-family: 'Geist', -apple-system, BlinkMacSystemFont, sans-serif;
  font-feature-settings: "cv01", "cv02", "cv03";
}
code, kbd, .font-mono, pre {
  font-family: 'Geist Mono', 'Menlo', 'Monaco', monospace;
}
```

**Tailwind config extension (in CDN script tag):**
```html
<script>
  tailwind.config = {
    theme: {
      extend: {
        fontFamily: {
          sans: ['Geist', '-apple-system', 'BlinkMacSystemFont', 'sans-serif'],
          mono: ['Geist Mono', 'Menlo', 'Monaco', 'monospace'],
        }
      }
    }
  }
</script>
```

### Type hierarchy (unchanged scale, new voice)

| Role | Classes | Notes |
|---|---|---|
| Page heading | `text-lg font-semibold tracking-tight` | Keep — right size |
| Section heading | `text-[13px] font-medium` | Keep |
| Label / metadata | `text-[11px] font-medium uppercase tracking-wider` | Keep |
| Body | `text-[13px]` / `text-sm` | Keep |
| Monospace (keys, IDs) | `font-mono text-xs` | Now uses Geist Mono |
| Timestamp | `text-[11px] text-stone-600 tabular-nums` | Keep |

No size changes needed — the compact scale is appropriate for a dense task tool.

---

## 4. Logo / Wordmark Concept

### Mark: "SO" monogram

Replace the favicon SVG with a geometric mark built on the letter T:

**Concept**: A square icon where a bold capital T has its crossbar doubled — two parallel horizontal lines with a vertical stem. This reads as a "stack of tasks" which is the core metaphor, and forms the SO monogram subtly.

**Updated favicon.svg:**
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <!-- Warm dark background -->
  <rect width="32" height="32" rx="6" fill="#0c0a09"/>
  <!-- T-mark: crossbar doubled (two stacked lines) -->
  <rect x="7" y="9" width="18" height="2" rx="1" fill="#fbbf24"/>
  <rect x="7" y="13" width="12" height="2" rx="1" fill="#fbbf24" opacity=".5"/>
  <!-- Vertical stem -->
  <rect x="14" y="11" width="4" height="12" rx="1" fill="#fbbf24"/>
</svg>
```

**Visual description**: Amber-gold mark on near-black stone. The doubled crossbar suggests hierarchy/stacking of tasks. The proportions make it read as a strong T at small sizes (16px favicon) and a deliberate logo mark at larger sizes.

### Wordmark

In-app wordmark (nav header):
```html
<div class="px-4 py-5 border-b border-stone-800">
  <div class="flex items-center gap-2.5">
    <!-- Mark (inline SVG or img) -->
    <svg width="20" height="20" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg">
      <rect width="32" height="32" rx="5" fill="#0c0a09"/>
      <rect x="7" y="9" width="18" height="2" rx="1" fill="#fbbf24"/>
      <rect x="7" y="13" width="12" height="2" rx="1" fill="#fbbf24" opacity=".5"/>
      <rect x="14" y="11" width="4" height="12" rx="1" fill="#fbbf24"/>
    </svg>
    <!-- Wordmark -->
    <div>
      <h1 class="text-sm font-semibold tracking-tight text-stone-50 leading-none">The Last Org</h1>
      <p class="text-[10px] text-stone-500 mt-0.5 tracking-wide uppercase">L5 Agent</p>
    </div>
  </div>
</div>
```

---

## 5. Actionable Code Changes Summary

### partials.html

1. Replace `bg-zinc-950` → `bg-stone-950` on `<body>`
2. Replace `bg-zinc-900` → `bg-stone-900` on `<nav>`
3. Replace all `border-zinc-800` → `border-stone-800`
4. Replace all `border-zinc-700` → `border-stone-700`
5. Replace all `text-zinc-*` → `text-stone-*` (body, nav items, inputs)
6. Replace `hover:bg-zinc-800` → `hover:bg-stone-800`
7. Add Geist font link tags in `<head>`
8. Add Tailwind fontFamily config in `<script>`
9. Update CSS body font to Geist
10. Replace nav wordmark section with mark + wordmark per section 4

### issues.html, agents.html (and all page templates)

11. Replace `bg-indigo-600` / `hover:bg-indigo-500` → `bg-amber-500` / `hover:bg-amber-400` on CTA buttons
12. Replace `text-white` on amber buttons → `text-stone-950`
13. Replace `focus:ring-indigo-500` / `focus:border-indigo-500` → `focus:ring-amber-500` / `focus:border-amber-500` on all form inputs
14. Replace `hover:text-indigo-400` on issue key column → `hover:text-amber-400`
15. Replace all `bg-zinc-*` surface classes → `bg-stone-*`
16. Replace all `text-zinc-*` text classes → `text-stone-*`

### issue_detail.html

17. Replace `text-indigo-400` (assignee link, parent link, run ID link) → `text-amber-400`
18. Replace `hover:text-indigo-300` → `hover:text-amber-300`
19. Replace `focus:ring-indigo-500` on all selects/textareas → `focus:ring-amber-500`
20. Replace diff hunk: `text-indigo-400 bg-indigo-950/20` → `text-amber-400 bg-amber-950/20`

### templates.go

21. In `diffLines()`: replace `"text-indigo-400 bg-indigo-950/20"` → `"text-amber-400 bg-amber-950/20"`
22. In `statusColor("todo")`: replace `"bg-zinc-600/80 text-zinc-200"` → `"bg-stone-600/80 text-stone-200"`
23. In `statusColor("cancelled")`: replace zinc classes → stone equivalents

### static/favicon.svg

24. Replace with the updated mark SVG from section 4

---

## Manual Checklist

1. [ ] Verify Geist font loads correctly at all target sizes (10px, 11px, 13px) — check for rendering artifacts on macOS/Windows
2. [ ] Confirm amber CTA button has sufficient contrast: `text-stone-950` on `bg-amber-500` (should pass WCAG AA)
3. [ ] Check amber accent against `bg-stone-800` background — amber-400 on stone-800 contrast ratio
4. [ ] Verify `in_review` amber badges no longer clash with amber accent (they should now harmonize)
5. [ ] Test favicon at 16px (browser tab) and 32px (bookmark) sizes
6. [ ] Check diff view — amber hunk headers should not conflict with green/red add/remove lines
7. [ ] Validate form focus states: amber ring on dark stone inputs
8. [ ] Check status badge readability: stone-50 headings against stone-950 background in low-light
9. [ ] Verify agent card avatar initials are legible: `text-stone-400` on `bg-stone-900`
10. [ ] Test command palette overlay — `bg-black/60` backdrop on stone base should read correctly
