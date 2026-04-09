package db

import (
	"context"
	"database/sql"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"
)

var (
	retryMaxDuration = 5 * time.Minute
	retryBaseDelay   = 100 * time.Millisecond
	retryMaxDelay    = 30 * time.Second
)

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "SQLITE_BUSY")
}

func retryOnBusy(op string, fn func() error) error {
	var elapsed time.Duration
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil || !isSQLiteBusy(err) {
			return err
		}

		delay := time.Duration(float64(retryBaseDelay) * math.Pow(2, float64(attempt)))
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
		if elapsed+delay > retryMaxDuration {
			slog.Error("db: SQLITE_BUSY retry exhausted, giving up", "op", op, "elapsed", elapsed.Round(time.Millisecond), "attempt", attempt+1)
			return err
		}

		slog.Warn("db: SQLITE_BUSY, retrying with backoff", "op", op, "delay", delay.Round(time.Millisecond), "attempt", attempt+1)
		time.Sleep(delay)
		elapsed += delay
	}
}

func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	var result sql.Result
	err := retryOnBusy("Exec", func() error {
		var execErr error
		result, execErr = d.DB.Exec(query, args...)
		return execErr
	})
	return result, err
}

func (d *DB) QueryRow(query string, args ...any) *Row {
	return &Row{db: d, query: query, args: args}
}

type Row struct {
	db    *DB
	query string
	args  []any
}

func (d *DB) reader() *sql.DB {
	if d.rdb != nil {
		return d.rdb
	}
	return d.DB
}

func (r *Row) Scan(dest ...any) error {
	return retryOnBusy("QueryRow", func() error {
		return r.db.reader().QueryRow(r.query, r.args...).Scan(dest...)
	})
}

func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	var rows *sql.Rows
	err := retryOnBusy("Query", func() error {
		var queryErr error
		rows, queryErr = d.reader().Query(query, args...)
		return queryErr
	})
	return rows, err
}

func (d *DB) Begin() (*Tx, error) {
	d.wmu.Lock()
	var tx *sql.Tx
	err := retryOnBusy("Begin", func() error {
		var beginErr error
		tx, beginErr = d.DB.Begin()
		return beginErr
	})
	if err != nil {
		d.wmu.Unlock()
		return nil, err
	}
	return &Tx{Tx: tx, mu: &d.wmu}, nil
}

// Tx wraps sql.Tx and holds the write mutex until commit/rollback.
type Tx struct {
	*sql.Tx
	mu *sync.Mutex
}

func (t *Tx) Commit() error {
	defer t.mu.Unlock()
	return retryOnBusy("Commit", t.Tx.Commit)
}

func (t *Tx) Rollback() error {
	defer t.mu.Unlock()
	return retryOnBusy("Rollback", t.Tx.Rollback)
}

func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	var rows *sql.Rows
	err := retryOnBusy("QueryContext", func() error {
		var queryErr error
		rows, queryErr = d.reader().QueryContext(ctx, query, args...)
		return queryErr
	})
	return rows, err
}

func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *CtxRow {
	return &CtxRow{db: d, ctx: ctx, query: query, args: args}
}

type CtxRow struct {
	db    *DB
	ctx   context.Context
	query string
	args  []any
}

func (r *CtxRow) Scan(dest ...any) error {
	return retryOnBusy("QueryRowContext", func() error {
		return r.db.reader().QueryRowContext(r.ctx, r.query, r.args...).Scan(dest...)
	})
}
