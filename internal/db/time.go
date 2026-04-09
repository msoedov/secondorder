package db

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// timeFmts are the formats SQLite may store DATETIME values as.
var timeFmts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	// Go's time.Time.String() format (stored by earlier code that wrote time.Time directly)
	"2006-01-02T15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// DBTime is a time.Time that can scan SQLite string timestamps without
// requiring _loc=UTC in the DSN (which is a mattn/go-sqlite3-only param).
type DBTime struct{ Time time.Time }

func (t *DBTime) Scan(v any) error {
	switch val := v.(type) {
	case time.Time:
		t.Time = val.UTC()
	case string:
		for _, f := range timeFmts {
			if p, err := time.Parse(f, val); err == nil {
				t.Time = p.UTC()
				return nil
			}
		}
		return fmt.Errorf("db: cannot parse time %q", val)
	case []byte:
		return t.Scan(string(val))
	case nil:
		t.Time = time.Time{}
	default:
		return fmt.Errorf("db: unsupported time type %T", v)
	}
	return nil
}

// Value implements driver.Valuer so DBTime can be used as a write param.
func (t DBTime) Value() (driver.Value, error) {
	return t.Time.UTC().Format("2006-01-02 15:04:05"), nil
}

// NullDBTime is a nullable DBTime that can scan NULL or string timestamps.
type NullDBTime struct {
	Time  time.Time
	Valid bool
}

func (t *NullDBTime) Scan(v any) error {
	if v == nil {
		t.Valid = false
		return nil
	}
	var dt DBTime
	if err := dt.Scan(v); err != nil {
		return err
	}
	t.Time = dt.Time
	t.Valid = true
	return nil
}
