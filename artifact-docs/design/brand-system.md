# Mesa Brand System
## Design Tokens & UI Architecture Reference
_Updated: 2026-04-09 | Replaces SO-10 version (2026-03-28)_

---

## 1. Architecture Overview

The UI is built on **Tailwind CSS (CDN)** with a **CSS variable semantic token layer**. Templates use semantic token classes (`bg-pg`, `text-ink`, `border-bd`, etc.) rather than raw Tailwind palette classes. This enables dual-theme (light/dark) without template changes.

### How the token system works

CSS variables are defined in `:root` (light) and `:root.dark` (dark). Tailwind's `theme.extend.colors` maps each token name to its CSS variable. Templates use only the semantic names.

```html
<!-- In partials.html <script> -->
tailwind.config = {
  theme: {
    extend: {
      colors: {
        pg:     'rgb(var(--c-pg) / <alpha-value>)',
        card:   'rgb(var(--c-card) / <alpha-value>)',
        sf:     'rgb(var(--c-sf) / <alpha-value>)',
        bd:     'rgb(var(--c-bd) / <alpha-value>)',
        ink:    'rgb(var(--c-ink) / <alpha-value>)',
        ink2:   'rgb(var(--c-ink2) / <alpha-value>)',
        ink3:   'rgb(var(--c-ink3) / <alpha-value>)',
        ac:     'rgb(var(--c-ac) / <alpha-value>)',
        'ac-h': 'rgb(var(--c-ac-h) / <alpha-value>)',
        'ac-t': 'rgb(var(--c-ac-t) / <alpha-value>)',
        ln:     'rgb(var(--c-ln) / <alpha-value>)',
        code:   'rgb(var(--c-code) / <alpha-value>)',
        ov:     'rgb(var(--c-ov) / <alpha-value>)',
      }
    }
  }
}
```

Theme is toggled by adding/removing the `.dark` class on `<html>`, persisted in `localStorage('so-theme')`.

---

## 2. Color Tokens

### Light mode (`:root`)

| Token | CSS var | Hex approx | Role |
|---|---|---|---|
| `pg` | `--c-pg` | `#f9f8f3` | Page background — warm cream |
| `card` | `--c-card` | `#ffffff` | Card / panel surface |
| `sf` | `--c-sf` | `#f1efee` | Elevated surface, hover state |
| `bd` | `--c-bd` | `#dfdedd` | Borders, dividers |
| `ink` | `--c-ink` | `#141211` | Primary text |
| `ink2` | `--c-ink2` | `#33312f` | Secondary text |
| `ink3` | `--c-ink3` | `#302f2d` | Tertiary text, nav items |
| `ac` | `--c-ac` | `#7adf9c` | Accent fill (heatmap, indicators) |
| `ac-h` | `--c-ac-h` | `#6bcf8c` | Accent hover |
| `ac-t` | `--c-ac-t` | `#009f60` | Accent text (superscript, links) |
| `ln` | `--c-ln` | `#009f60` | Accent line (top bar, link hover) |
| `code` | `--c-code` | `#141211` | Code background |
| `ov` | `--c-ov` | `#141211` | Overlay backdrop |

### Dark mode (`:root.dark`)

| Token | CSS var | Hex approx | Role |
|---|---|---|---|
| `pg` | `--c-pg` | `#09090b` | Page background — zinc-950 |
| `card` | `--c-card` | `#18181b` | Card / panel surface — zinc-900 |
| `sf` | `--c-sf` | `#27272a` | Elevated surface — zinc-800 |
| `bd` | `--c-bd` | `#27272a` | Borders — zinc-800 |
| `ink` | `--c-ink` | `#e4e4e7` | Primary text — zinc-200 |
| `ink2` | `--c-ink2` | `#d4d4d8` | Secondary text — zinc-300 |
| `ink3` | `--c-ink3` | `#a1a1aa` | Tertiary text — zinc-400 |
| `ac` | `--c-ac` | `#4f46e5` | Accent fill — indigo-600 |
| `ac-h` | `--c-ac-h` | `#6366f1` | Accent hover — indigo-500 |
| `ac-t` | `--c-ac-t` | `#818cf8` | Accent text — indigo-400 |
| `ln` | `--c-ln` | `#6366f1` | Accent line — indigo-500 |
| `code` | `--c-code` | `#09090b` | Code background |
| `ov` | `--c-ov` | `#000000` | Overlay backdrop |

### Design rationale

- **Light mode accent (green)**: `#7adf9c` / `#009f60` — suggests growth, execution, forward motion. Differentiates from the typical indigo SaaS default.
- **Dark mode accent (indigo)**: Retained for legibility against the dark zinc base. Indigo at full saturation reads cleanly on dark backgrounds where green would require higher brightness.
- **Warm cream base (light)**: `#f9f8f3` is slightly warm rather than cool white — avoids the harsh brightness of pure white and gives the tool a less generic feel.

---

## 3. Status Colors

Status badge colors are **not** part of the semantic token system. They use raw Tailwind palette classes and are generated in `templates.go` via `statusColor()`.

| Status | Badge classes | Text-only class |
|---|---|---|
| `todo` | (no badge style, default) | — |
| `in_progress` | `bg-blue-500/15 text-blue-400 ring-1 ring-inset ring-blue-500/25` | `text-blue-400` |
| `in_review` | `bg-amber-500/15 text-amber-400 ring-1 ring-inset ring-amber-500/25` | `text-amber-400` |
| `done` | `bg-emerald-500/15 text-emerald-400 ring-1 ring-inset ring-emerald-500/25` | `text-emerald-400` |
| `blocked` | `bg-red-500/15 text-red-400 ring-1 ring-inset ring-red-500/25` | `text-red-400` |
| `cancelled` | `bg-zinc-500/15 text-zinc-400 ring-1 ring-inset ring-zinc-500/25` | `text-zinc-400` |

These classes intentionally use raw Tailwind palette names because they must read well in both light and dark mode without being theme-sensitive. The `*/15` opacity fill pattern keeps badges subtle while the ring adds a visible border at low contrast.

### Diff syntax highlighting

Added lines: `text-emerald-400 bg-emerald-950/30`
Removed lines: `text-red-400 bg-red-950/30`
Hunk headers: resolved to `text-amber-400 bg-amber-950/20` (check `diffLines()` in `templates.go`)

---

## 4. Typography

**Primary font**: Inter, loaded from Google Fonts.

```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
```

```css
body { font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif; }
```

### Type scale

| Role | Classes | Notes |
|---|---|---|
| Page heading | `text-lg font-semibold tracking-tight` | — |
| Section heading | `text-[13px] font-medium` | — |
| Label / metadata | `text-[11px] font-medium uppercase tracking-wider` | — |
| Body | `text-[13px]` / `text-sm` | — |
| Monospace (keys, IDs) | `font-mono text-xs` | System mono fallback |
| Timestamp | `text-[11px] text-ink3 tabular-nums` | — |
| Nav items | `text-sm text-ink3 hover:text-ink` | — |

The compressed scale (10–14px) is intentional for a dense task management tool. Do not increase base sizes without validating against the sidebar and issue list layouts.

---

## 5. Brand Identity

### Name and wordmark

Brand: **Mesa**
Tagline: **Zero human company**

The wordmark is rendered inline in the nav header using HTML/CSS — no image asset:

```html
<h1 class="text-[17px] font-bold tracking-tight text-ink leading-none">Mesa</h1>
<p class="text-[10px] text-ink3/[0.50] mt-1 tracking-widest uppercase">Zero human company</p>
```

### Favicon

Current `static/favicon.svg`: double-chevron/arrow mark on near-black background.

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="6" fill="#0c0a09"/>
  <path d="M8 10l8 6-8 6" stroke="#e7e5e4" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
  <path d="M16 10l8 6-8 6" stroke="#e7e5e4" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" fill="none" opacity=".5"/>
</svg>
```

Two overlapping forward chevrons: suggests sequential execution, order, and forward motion. The trailing chevron fades to 50% opacity to imply depth/cascading.

### Top accent line

A 3px horizontal line pinned to the top of the viewport uses `bg-ln` (green in light, indigo in dark). This is the primary accent "signature" of the UI — consistent across all pages.

```html
<div class="fixed top-0 left-0 right-0 h-[3px] bg-ln z-50"></div>
```

---

## 6. Layout

| Variable | Value | Tailwind equiv |
|---|---|---|
| `--layout-sidebar` | `14rem` | `w-56` |
| `--layout-max-w` | `72rem` | `max-w-6xl` |
| `--layout-pad` | `2rem` | `p-8` |
| `--layout-pad-top-mob` | `4rem` | `pt-16` (mobile only) |

Sidebar is fixed, full-height. On mobile it is offscreen (`-translate-x-full`) and slides in via JS toggle.

---

## 7. Component Patterns

### Cards / surfaces

```html
<div class="bg-card border border-bd rounded-lg p-4">...</div>
```

Elevated surfaces (modals, dropdowns): `bg-sf`
Hover state for nav/list items: `hover:bg-sf`

### Buttons — primary (CTA)

```html
<button class="bg-ac text-code px-3 py-1.5 rounded text-sm font-medium hover:bg-ac-h transition-colors">
  Action
</button>
```

Note: `text-code` is used on accent buttons to ensure the text color matches the current background in both themes (dark text on light green, dark text on dark base when inverted).

### Focus ring

```css
.focus-ring:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px rgb(var(--c-pg)), 0 0 0 4px rgb(var(--c-ac) / 0.6);
}
```

Apply `.focus-ring` to interactive elements. The double-ring (page color offset, then accent) works in both themes.

### Heatmap

```css
.heatmap-0 { background: rgb(var(--c-sf)); }
.heatmap-1 { background: rgb(var(--c-ac) / 0.25); }
.heatmap-2 { background: rgb(var(--c-ac) / 0.50); }
.heatmap-3 { background: rgb(var(--c-ac) / 0.75); }
.heatmap-4 { background: rgb(var(--c-ac)); }
```

---

## 8. Usage Rules

1. **Use semantic tokens in templates.** Never write `bg-zinc-950` or `bg-stone-900` in a template. Use `bg-pg`, `bg-card`, `bg-sf`.
2. **Use raw Tailwind palette only for status/semantic colors.** Status badges (`blue`, `amber`, `emerald`, `red`) are intentionally theme-agnostic and stay as raw Tailwind classes.
3. **Accent color is theme-aware.** `bg-ac` and `text-ac-t` will shift from green (light) to indigo (dark) automatically — do not hard-code either color.
4. **Warning/alert callouts** use `amber-500` raw classes (`bg-amber-500/10 border-amber-500/30 text-amber-400`). This is intentional and separate from the accent system.
5. **Do not add a new CSS variable** unless the new role is genuinely theme-sensitive and reused in 3+ locations.

---

## 9. Manual QA Checklist

1. [ ] Verify Inter font loads at all target sizes (10px, 11px, 13px) on macOS and Windows
2. [ ] Confirm top accent line color: green in light mode, indigo in dark mode
3. [ ] Check `text-ac-t` (superscript "ND") is readable in both themes
4. [ ] Verify status badges read clearly in light mode (colored text on near-white card)
5. [ ] Confirm amber warning banners (`issue_detail.html`) remain visible in both themes
6. [ ] Check sidebar nav hover state (`hover:bg-sf`) is visible in light mode
7. [ ] Test favicon at 16px (browser tab) and 32px (bookmark) — double-chevron must be legible
8. [ ] Verify focus ring appears correctly in both themes on form inputs and selects
9. [ ] Check diff view hunk headers — amber on both light and dark
10. [ ] Test theme toggle persistence: reload after switching themes, confirm correct theme restores
