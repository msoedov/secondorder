# SO-31 Wiki UI Templates and Handlers

## Scope

Implemented wiki UI templates, UI handlers, route registration, and sidebar navigation updates for:

- `/wiki` (list)
- `/wiki/new` (create form)
- `/wiki/{slug}` (view)
- `/wiki/{slug}/edit` (edit form)

## Templates

Added `internal/templates/wiki.html` with template definitions:

- `wiki_list`
- `wiki_view`
- `wiki_new`
- `wiki_edit`
- `wiki_form` (shared form partial)

Design follows existing app patterns:

- shared `head`, `nav`, and `foot` partials
- same layout width, card styles, and form control styles used by existing pages
- flash messaging (`error`, `success`) pattern aligned with other templates

## UI Handlers

Added methods in `internal/handlers/ui.go`:

- `WikiList` (GET `/wiki`)
- `WikiView` (GET `/wiki/{slug}`)
- `WikiNew` (GET `/wiki/new`)
- `WikiEdit` (GET `/wiki/{slug}/edit`)
- `WikiCreate` (POST `/wiki`)
- `WikiUpdate` (POST `/wiki/{slug}`)

Behavior:

- list view shows title, slug, last updated, updated by
- title links to page view
- create/edit forms validate title
- slug auto-generated from title (lowercase kebab-case)
- create/update redirect to page view on success
- view page shows metadata and content in pre-formatted block

## Route Registration

Added UI routes in `cmd/secondorder/main.go` alongside existing UI routes:

- `GET /wiki`
- `GET /wiki/new`
- `POST /wiki`
- `GET /wiki/{slug}`
- `GET /wiki/{slug}/edit`
- `POST /wiki/{slug}`

## Navigation

Added `Wiki` link in `internal/templates/partials.html` sidebar navigation.

## Template Parsing

Updated `internal/templates/templates.go` to include `wiki.html` in parsed page files.

## Verification

- `go build ./...` passes.
