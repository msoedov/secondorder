package db

import (
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	*sql.DB
	rdb *sql.DB    // read-only pool — higher concurrency, no write mutex
	wmu sync.Mutex // serializes writes to avoid SQLITE_BUSY
}

func Open(path string) (*DB, error) {
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_loc=UTC"

	// Write connection pool: low concurrency, serialized by wmu
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)

	// Read connection pool: high concurrency, WAL allows parallel reads
	rDB, err := sql.Open("sqlite", dsn+"&_query_only=true")
	if err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("open read db: %w", err)
	}
	rDB.SetMaxOpenConns(16)
	rDB.SetMaxIdleConns(8)

	d := &DB{DB: sqlDB, rdb: rDB}
	if err := d.RunMigrations(); err != nil {
		sqlDB.Close()
		rDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error {
	d.rdb.Close()
	return d.DB.Close()
}

func (d *DB) RunMigrations() error {
	// Ensure schema_migrations table exists
	_, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := parseVersion(entry.Name())
		if err != nil {
			return fmt.Errorf("parse migration version %s: %w", entry.Name(), err)
		}

		var count int
		if err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile(filepath.Join("migrations", entry.Name()))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		tx, err := d.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec migration %d: %w", version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}

	return nil
}

func parseVersion(filename string) (int, error) {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid migration filename: %s", filename)
	}
	return strconv.Atoi(parts[0])
}

// --- Webhook Events ---

// WebhookEventExists checks if a webhook event with the given delivery_id has already been processed
func (d *DB) WebhookEventExists(deliveryID string) (bool, error) {
	if deliveryID == "" {
		return false, nil // No delivery ID means we can't check for duplicates
	}
	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM webhook_events WHERE delivery_id = ?", deliveryID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateWebhookEvent inserts a new webhook event record
func (d *DB) CreateWebhookEvent(id, source, eventType, deliveryID, payload string) error {
	// Note: Exec() already acquires the write mutex
	_, err := d.Exec(
		"INSERT INTO webhook_events (id, source, event_type, delivery_id, payload, status) VALUES (?, ?, ?, ?, ?, 'received')",
		id, source, eventType, deliveryID, payload,
	)
	return err
}

// UpdateWebhookEventStatus updates the status and error message of a webhook event
func (d *DB) UpdateWebhookEventStatus(id, status, errorMsg string) error {
	// Note: Exec() already acquires the write mutex
	_, err := d.Exec(
		"UPDATE webhook_events SET status = ?, error_message = ?, processed_at = datetime('now') WHERE id = ?",
		status, errorMsg, id,
	)
	return err
}
