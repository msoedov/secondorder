## SO-33 Wiki DB/Handler Signature Alignment

This change aligns wiki repository method signatures with handler and test expectations to resolve compile-time mismatches.

### Updated DB method contracts

- `GetWikiPageBySlug(slug string) (*models.WikiPage, error)` is the canonical slug lookup method.
- `UpdateWikiPage(page *models.WikiPage) error` now accepts a `*models.WikiPage` model and sets `UpdatedAt` internally.
- `DeleteWikiPage(id string) error` now deletes by page ID.

### Call-site updates

- API wiki handlers now call `GetWikiPageBySlug`, pass full page models to `UpdateWikiPage`, and delete by `page.ID`.
- UI wiki handlers now use the same repository contract (`GetWikiPageBySlug`, model-based update).
- Wiki DB and handler tests were updated to use the aligned method names and argument types.

### Validation

- `go build ./...` passes.
- `go test ./internal/db/ -run Wiki` passes.
