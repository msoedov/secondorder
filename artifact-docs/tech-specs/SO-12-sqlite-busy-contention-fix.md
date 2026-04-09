# SO-12: SQLITE_BUSY Warning Reduction in `secondorder`

## Summary

The frequent `SQLITE_BUSY` warnings were caused by SQLite lock contention across multiple pooled connections. The fix reduces writer contention by forcing a single pooled SQLite connection and explicitly applying runtime pragmas for WAL and busy timeout at startup.

## Root Cause

Code inspection in `internal/db` showed:

- `Open()` configured SQLite with WAL and `_busy_timeout` in DSN, but still allowed `SetMaxOpenConns(2)`.
- `retryOnBusy` in `internal/db/retry.go` logs each lock retry (`SQLITE_BUSY` / `database is locked`).
- Under concurrent scheduler + API activity, multiple connections increased lock collision windows even though writes are mutex-serialized.

Result: many operations eventually succeeded, but warning logs were noisy due to repeated retries.

## Fix Implemented

### 1) Single pooled SQLite connection

In `internal/db/db.go`:

- Added `sqliteMaxOpenConns = 1`
- Set both:
  - `sqlDB.SetMaxOpenConns(sqliteMaxOpenConns)`
  - `sqlDB.SetMaxIdleConns(sqliteMaxOpenConns)`

This aligns pool behavior with SQLite's single-writer model and reduces internal connection lock churn.

### 2) Explicit startup pragmas

In `internal/db/db.go`:

- Added `sqliteBusyTimeout = 5000`
- Added `applySQLitePragmas()` called during `Open()`:
  - `PRAGMA busy_timeout=5000`
  - `PRAGMA journal_mode=WAL`
  - validates returned journal mode is `wal`

This makes pragma state deterministic instead of relying only on DSN interpretation.

## Verification

### New tests (`internal/db/db_test.go`)

- `TestOpenSQLiteConnectionPoolIsSingleConnection`
- `TestOpenConfiguresBusyTimeoutPragma`
- `TestOpenConfiguresJournalModeWAL`

### Command verification

- `go test ./internal/db` passed
- `go build ./...` passed

## Expected Outcome

- `SQLITE_BUSY` warnings should drop to near-zero under normal load.
- Retry logic remains in place for transient contention spikes.
- No schema or data model changes were introduced.
