package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenStoreConfiguresSQLiteForBusyWorkloads(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "paperless-aiext-store-test.db")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(databasePath)
	})

	stats := store.db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Fatalf("expected max open connections to be 1, got %d", stats.MaxOpenConnections)
	}

	var journalMode string
	if err := store.db.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode pragma: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("expected journal_mode WAL, got %q", journalMode)
	}

	var busyTimeout int
	if err := store.db.QueryRowContext(t.Context(), `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy timeout pragma: %v", err)
	}
	if busyTimeout < sqliteBusyTimeoutMS {
		t.Fatalf("expected busy_timeout >= %dms, got %dms", sqliteBusyTimeoutMS, busyTimeout)
	}
}
