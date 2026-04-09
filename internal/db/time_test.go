package db

import (
	"testing"
	"time"
)

func TestDBTimeScan(t *testing.T) {
	cases := []string{
		"2026-04-09T07:18:56.232503 +0000 UTC",
		"2026-04-09 03:35:07",
		"2026-04-09T03:35:07Z",
		"2026-04-09T03:35:07.123456789Z",
	}
	for _, c := range cases {
		var dt DBTime
		if err := dt.Scan(c); err != nil {
			t.Errorf("failed to parse %q: %v", c, err)
			continue
		}
		if dt.Time.IsZero() {
			t.Errorf("parsed zero time for %q", c)
		}
	}
}

func TestNullDBTimeScanNull(t *testing.T) {
	var nt NullDBTime
	if err := nt.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if nt.Valid {
		t.Fatal("expected Valid=false for nil")
	}
}

func TestNullDBTimeScanString(t *testing.T) {
	var nt NullDBTime
	if err := nt.Scan("2026-04-09T07:18:56.232503 +0000 UTC"); err != nil {
		t.Fatal(err)
	}
	if !nt.Valid {
		t.Fatal("expected Valid=true")
	}
	if nt.Time.Year() != 2026 {
		t.Fatalf("wrong year: %d", nt.Time.Year())
	}
}

func TestDBTimeValue(t *testing.T) {
	dt := DBTime{Time: time.Date(2026, 4, 9, 7, 18, 56, 0, time.UTC)}
	v, err := dt.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != "2026-04-09 07:18:56" {
		t.Fatalf("unexpected value: %v", v)
	}
}
