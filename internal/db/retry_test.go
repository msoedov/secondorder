package db

import (
	"errors"
	"testing"
	"time"
)

func TestIsSQLiteBusy(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"other error", errors.New("something else"), false},
		{"database is locked", errors.New("database is locked (SQLITE_BUSY)"), true},
		{"SQLITE_BUSY", errors.New("SQLITE_BUSY"), true},
		{"wrapped locked", errors.New("exec: database is locked"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSQLiteBusy(tt.err); got != tt.want {
				t.Errorf("isSQLiteBusy(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryOnBusy_Success(t *testing.T) {
	calls := 0
	err := retryOnBusy("test", func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetryOnBusy_NonBusyError(t *testing.T) {
	calls := 0
	want := errors.New("not busy")
	err := retryOnBusy("test", func() error {
		calls++
		return want
	})
	if err != want {
		t.Errorf("err = %v, want %v", err, want)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (should not retry non-busy)", calls)
	}
}

func TestRetryOnBusy_RetriesThenSucceeds(t *testing.T) {
	calls := 0
	err := retryOnBusy("test", func() error {
		calls++
		if calls < 3 {
			return errors.New("database is locked (SQLITE_BUSY)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetryOnBusy_ExponentialBackoff(t *testing.T) {
	calls := 0
	start := time.Now()
	err := retryOnBusy("test", func() error {
		calls++
		if calls < 3 {
			return errors.New("database is locked")
		}
		return nil
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 100ms + 200ms = 300ms minimum
	if elapsed < 250*time.Millisecond {
		t.Errorf("elapsed = %v, expected >= 250ms for exponential backoff", elapsed)
	}
}

func TestRetryOnBusy_GivesUpAfterMaxDuration(t *testing.T) {
	// Temporarily shrink limits for fast test
	origMax := retryMaxDuration
	origBase := retryBaseDelay
	origMaxDelay := retryMaxDelay
	retryMaxDuration = 500 * time.Millisecond
	retryBaseDelay = 50 * time.Millisecond
	retryMaxDelay = 100 * time.Millisecond
	defer func() {
		retryMaxDuration = origMax
		retryBaseDelay = origBase
		retryMaxDelay = origMaxDelay
	}()

	calls := 0
	busyErr := errors.New("database is locked")
	err := retryOnBusy("test", func() error {
		calls++
		return busyErr
	})
	if err != busyErr {
		t.Errorf("expected busy error returned after exhaustion, got %v", err)
	}
	if calls < 2 {
		t.Errorf("calls = %d, expected multiple retries before giving up", calls)
	}
}

func TestDBExecRetry(t *testing.T) {
	d := testDB(t)
	// Normal exec should work through retry wrapper
	_, err := d.Exec("SELECT 1")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
}

func TestDBQueryRowRetry(t *testing.T) {
	d := testDB(t)
	var n int
	err := d.QueryRow("SELECT 1").Scan(&n)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}
}

func TestDBQueryRetry(t *testing.T) {
	d := testDB(t)
	rows, err := d.Query("SELECT 1")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected row")
	}
}

func TestTxCommitReleasesWriteLock(t *testing.T) {
	d := testDB(t)

	tx, err := d.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := d.Exec("SELECT 1")
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("Exec completed before Commit released the lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Exec after Commit: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Exec stayed blocked after Commit")
	}
}

func TestTxRollbackReleasesWriteLock(t *testing.T) {
	d := testDB(t)

	tx, err := d.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := d.Exec("SELECT 1")
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("Exec completed before Rollback released the lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Exec after Rollback: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Exec stayed blocked after Rollback")
	}
}
