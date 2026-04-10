# SO-26 Wiki API Handlers and Routes

## Scope Delivered

- Added authenticated wiki REST handlers in `internal/handlers/api.go`:
  - `GET /api/v1/wiki`
  - `POST /api/v1/wiki`
  - `GET /api/v1/wiki/{slug}`
  - `PATCH /api/v1/wiki/{slug}`
  - `DELETE /api/v1/wiki/{slug}`
- Registered all wiki routes in `cmd/secondorder/main.go` under existing API key auth middleware.
- Implemented request validation and API-convention JSON responses.
- Wired `created_by_agent_id` and `updated_by_agent_id` using authenticated agent identity for create and update operations.

## Handler Behavior

- `GET /api/v1/wiki`
  - Returns all wiki pages ordered by `updated_at DESC`.
  - `200 OK` with JSON array.

- `POST /api/v1/wiki`
  - Requires JSON body with `slug` and `title`.
  - Optional `content`.
  - Populates `created_by_agent_id` and `updated_by_agent_id` from auth context.
  - Returns:
    - `201 Created` on success
    - `400 Bad Request` for invalid/missing fields
    - `409 Conflict` for duplicate slug

- `GET /api/v1/wiki/{slug}`
  - Returns page by slug.
  - Returns:
    - `200 OK` on success
    - `404 Not Found` if slug does not exist

- `PATCH /api/v1/wiki/{slug}`
  - Supports partial updates for `slug`, `title`, and `content`.
  - Rejects empty `slug`/`title` when present.
  - Sets `updated_by_agent_id` to authenticated agent.
  - Returns:
    - `200 OK` on success
    - `400 Bad Request` for invalid body or invalid field values
    - `404 Not Found` if page does not exist
    - `409 Conflict` for duplicate slug updates

- `DELETE /api/v1/wiki/{slug}`
  - Deletes page by resolved page ID.
  - Returns:
    - `200 OK` with `{ "deleted": "<slug>" }`
    - `404 Not Found` if page does not exist

## Tests Added

- New handler tests in `internal/handlers/wiki_handlers_test.go` covering:
  - End-to-end CRUD flow with auth.
  - Auth enforcement (`401` without API key).
  - Duplicate slug conflict behavior (`409`).
