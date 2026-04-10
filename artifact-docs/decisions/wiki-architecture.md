# SO-24: Wiki Feature Architecture

## Overview

The wiki provides a shared knowledge base within secondorder. Both human operators (via UI) and agents (via REST API) can create, read, update, and delete pages. Content is stored as plain text/Markdown in SQLite alongside all other secondorder data.

## Database Schema

Migration: `internal/db/migrations/023_wiki_pages.sql`

```sql
CREATE TABLE IF NOT EXISTS wiki_pages (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    created_by_agent_id TEXT,
    updated_by_agent_id TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by_agent_id) REFERENCES agents(id),
    FOREIGN KEY (updated_by_agent_id) REFERENCES agents(id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_slug ON wiki_pages(slug);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_updated_at ON wiki_pages(updated_at);
```

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| `id` as UUID (TEXT) | Consistent with all other secondorder tables |
| `slug` as unique lookup key | Human-readable URLs (`/wiki/onboarding-guide`), avoids exposing UUIDs in the UI |
| `created_by_agent_id` / `updated_by_agent_id` nullable | Allows pages created by human operators (no agent context) |
| No `deleted_at` soft-delete | Matches existing pattern — `DELETE` is a hard delete. Wiki pages are ephemeral knowledge, not audit records |
| Content as plain TEXT | Markdown rendered client-side; no server-side parsing needed. Keeps schema simple |

### Model

`internal/models/models.go` — `WikiPage` struct:

```go
type WikiPage struct {
    ID               string    `json:"id"`
    Slug             string    `json:"slug"`
    Title            string    `json:"title"`
    Content          string    `json:"content"`
    CreatedByAgentID *string   `json:"created_by_agent_id"`
    UpdatedByAgentID *string   `json:"updated_by_agent_id"`
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
}
```

## REST API Contract

Base path: `/api/v1/wiki`

All endpoints require Bearer token authentication (`Authorization: Bearer <token>`), validated through the existing `api.Auth()` middleware.

### Endpoints

#### List Pages

```
GET /api/v1/wiki
```

**Response** `200 OK`:
```json
[
  {
    "id": "uuid",
    "slug": "onboarding-guide",
    "title": "Onboarding Guide",
    "content": "# Welcome\n\nThis is the onboarding guide...",
    "created_by_agent_id": "agent-uuid-or-null",
    "updated_by_agent_id": "agent-uuid-or-null",
    "created_at": "2026-04-09T12:00:00Z",
    "updated_at": "2026-04-09T14:30:00Z"
  }
]
```

Returns all pages ordered by `updated_at DESC`. Empty array when no pages exist.

#### Create Page

```
POST /api/v1/wiki
Content-Type: application/json
```

**Request body**:
```json
{
  "slug": "onboarding-guide",
  "title": "Onboarding Guide",
  "content": "# Welcome\n\nThis is the onboarding guide..."
}
```

| Field | Required | Validation |
|-------|----------|------------|
| `slug` | Yes | Non-empty, unique |
| `title` | Yes | Non-empty |
| `content` | No | Defaults to empty string |

**Response** `200 OK`: Created `WikiPage` object.

**Errors**:
- `400` — Missing `slug` or `title`
- `409` — Slug already exists

`created_by_agent_id` and `updated_by_agent_id` are set automatically from the authenticated agent context.

#### Read Page

```
GET /api/v1/wiki/{slug}
```

**Response** `200 OK`: Single `WikiPage` object.

**Errors**:
- `404` — Page not found

#### Update Page

```
PATCH /api/v1/wiki/{slug}
Content-Type: application/json
```

**Request body** (all fields optional):
```json
{
  "slug": "new-slug",
  "title": "Updated Title",
  "content": "Updated content..."
}
```

Only provided fields are updated. `updated_by_agent_id` is set from the authenticated agent. `updated_at` is refreshed.

**Response** `200 OK`: Updated `WikiPage` object.

**Errors**:
- `404` — Page not found
- `409` — New slug conflicts with existing page

#### Delete Page

```
DELETE /api/v1/wiki/{slug}
```

**Response** `200 OK`:
```json
{"ok": true}
```

**Errors**:
- `404` — Page not found

### Access Control

All wiki endpoints use the same `api.Auth()` middleware as other endpoints. Any authenticated agent can read, create, update, or delete any wiki page — there is no per-page ownership restriction. This is intentional: the wiki is a shared knowledge base, not a per-agent resource.

## Agent Interaction

Agents interact with the wiki using the same REST API and their provisioned Bearer token (`SECONDORDER_API_KEY`).

### Use Cases

| Use Case | Flow |
|----------|------|
| **Agent documents a decision** | Agent calls `POST /api/v1/wiki` with slug, title, and Markdown content |
| **Agent reads context** | Agent calls `GET /api/v1/wiki/{slug}` to fetch a specific page, or `GET /api/v1/wiki` to discover available pages |
| **Agent updates knowledge** | Agent calls `PATCH /api/v1/wiki/{slug}` with updated content |
| **Agent cleans up** | Agent calls `DELETE /api/v1/wiki/{slug}` to remove obsolete pages |

### Archetype Integration

Archetypes that produce documentation (CEO, architect, product) can reference the wiki API in their prompts. Example instruction in an archetype:

```
When you make an architectural decision, document it in the wiki:
  POST /api/v1/wiki with slug "decision-{topic}", title, and Markdown body.
```

The wiki is passive infrastructure — agents are not required to use it, but archetypes can instruct them to.

### Prompt Injection

The wiki API includes the base URL and endpoints in the archetype/issue prompt, following the same pattern as the existing issue and work-block APIs:

```
Wiki API (Authorization: Bearer $SECONDORDER_API_KEY):
  GET    $SECONDORDER_API_URL/api/v1/wiki                - list all wiki pages
  POST   $SECONDORDER_API_URL/api/v1/wiki                - create wiki page
  GET    $SECONDORDER_API_URL/api/v1/wiki/{slug}          - read wiki page
  PATCH  $SECONDORDER_API_URL/api/v1/wiki/{slug}          - update wiki page
  DELETE $SECONDORDER_API_URL/api/v1/wiki/{slug}          - delete wiki page
```

## UI Integration

### Navigation

A "Wiki" link is added to the sidebar navigation in `internal/templates/partials.html`, positioned between "Issues" and "Work Blocks". Uses a book icon (SVG path for open book).

### Views

#### List View (`GET /wiki`)

Template: `internal/templates/wiki.html`

Displays all wiki pages in a card/table layout:

```
┌──────────────────────────────────────────────────┐
│  Wiki                              [+ New Page]  │
├──────────────────────────────────────────────────┤
│  Onboarding Guide                   2 hours ago  │
│  Architecture Overview              1 day ago    │
│  Deployment Runbook                 3 days ago   │
└──────────────────────────────────────────────────┘
```

- Each row links to the detail view (`/wiki/{slug}`)
- Sorted by `updated_at DESC` (most recently updated first)
- "New Page" button opens an inline create form (HTMX)
- Empty state: "No wiki pages yet. Create the first one."

#### Detail / Edit View (`GET /wiki/{slug}`)

Template: reuses `wiki.html` with conditional rendering, or a separate `wiki_detail.html`.

**Read mode**:
```
┌──────────────────────────────────────────────────┐
│  ← Back to Wiki          [Edit]  [Delete]        │
│                                                  │
│  Onboarding Guide                                │
│  ─────────────────                               │
│  Rendered Markdown content...                    │
│                                                  │
│  Last updated 2 hours ago by founding-engineer   │
└──────────────────────────────────────────────────┘
```

**Edit mode** (toggled by Edit button):
```
┌──────────────────────────────────────────────────┐
│  ← Back to Wiki                     [Cancel]     │
│                                                  │
│  Title: [Onboarding Guide_____________]          │
│  Slug:  [onboarding-guide_____________]          │
│                                                  │
│  Content:                                        │
│  ┌──────────────────────────────────────────┐    │
│  │ # Welcome                                │    │
│  │ This is the onboarding guide...          │    │
│  └──────────────────────────────────────────┘    │
│                                   [Save]         │
└──────────────────────────────────────────────────┘
```

### UI Handler

Handler: `internal/handlers/ui.go` — new methods:

| Method | Route | Purpose |
|--------|-------|---------|
| `WikiList` | `GET /wiki` | Render wiki list page |
| `WikiDetail` | `GET /wiki/{slug}` | Render wiki detail/edit page |

These call the existing DB methods (`ListWikiPages`, `GetWikiPageBySlug`) and render via Go templates. Create/update/delete operations are handled by the existing API endpoints, invoked from the UI via HTMX or standard form POST.

### Route Registration

In `cmd/secondorder/main.go`, UI routes are registered alongside existing pages:

```go
mux.HandleFunc("GET /wiki", ui.WikiList)
mux.HandleFunc("GET /wiki/{slug}", ui.WikiDetail)
```

## Component Diagram

```
Browser ──GET /wiki──────────► WikiList (ui.go)
                                    │
                        db.ListWikiPages()
                                    │
                        wiki.html template
                                    ▼
                            Rendered page list

Browser ──GET /wiki/{slug}──► WikiDetail (ui.go)
                                    │
                        db.GetWikiPageBySlug()
                                    │
                        wiki.html template (detail)
                                    ▼
                            Rendered page content

Agent ──POST /api/v1/wiki──► CreateWikiPage (api.go)
                                    │
                         api.Auth() middleware
                                    │
                        db.CreateWikiPage()
                                    │
                        JSON response
```

## File Inventory

| File | Status | Role |
|------|--------|------|
| `internal/db/migrations/023_wiki_pages.sql` | Implemented | Schema migration |
| `internal/models/models.go` | Implemented | `WikiPage` struct |
| `internal/db/queries.go` | Implemented | CRUD: `CreateWikiPage`, `GetWikiPage`, `ListWikiPages`, `UpdateWikiPage`, `DeleteWikiPage` |
| `internal/db/wiki_test.go` | Implemented | DB layer tests |
| `internal/handlers/api.go` | Implemented | API handlers: `ListWikiPages`, `CreateWikiPage`, `GetWikiPage`, `UpdateWikiPage`, `DeleteWikiPage` |
| `cmd/secondorder/main.go` | Implemented | API route registration |
| `internal/templates/partials.html` | Implemented | Sidebar "Wiki" link |
| `internal/templates/wiki.html` | **Not yet** | Wiki list + detail template |
| `internal/handlers/ui.go` | **Not yet** | `WikiList`, `WikiDetail` UI handlers |
| `cmd/secondorder/main.go` | **Not yet** | UI route registration (`GET /wiki`, `GET /wiki/{slug}`) |

## Acceptance Criteria Mapping

| Criterion | Location |
|-----------|----------|
| DB schema DDL specified | `023_wiki_pages.sql` + this document (Schema section) |
| API contract specified | This document (REST API Contract section) |
| Agent interaction patterns documented | This document (Agent Interaction section) |
| Human interaction patterns documented | This document (UI Integration section) |
