# SO-27 Wiki UI Templates and Page Handlers

## Outcome

The wiki UI is implemented with server-rendered Go templates and HTTP handlers, including list, detail, create, and edit flows.

## Implemented UI Routes

- `GET /wiki` - list all wiki pages
- `GET /wiki/{slug}` - view a wiki page
- `GET /wiki/new` - create form
- `GET /wiki/{slug}/edit` - edit form
- `POST /wiki` - create wiki page
- `POST /wiki/{slug}` - save wiki page edits

## Templates

Wiki templates are implemented in `internal/templates/wiki.html` and registered in template parsing.

- `wiki_list`
- `wiki_view`
- `wiki_new`
- `wiki_edit`
- shared `wiki_form`

The wiki detail page renders markdown-style content through template helpers and keeps styling consistent with existing pages.

## Handlers

UI handlers are implemented in `internal/handlers/ui.go`:

- `WikiList`
- `WikiView`
- `WikiNew`
- `WikiEdit`
- `WikiCreate`
- `WikiUpdate`

Create/edit handlers validate title, derive a slug from title, and redirect with flash query params.

## Navigation

Sidebar navigation includes a `Wiki` entry in `internal/templates/partials.html`.

## Router Wiring

UI routes are wired in `cmd/secondorder/main.go` with the expected path patterns.

## Notes

- Template parsing and wiki UI rendering are integrated and available through the server routes above.
