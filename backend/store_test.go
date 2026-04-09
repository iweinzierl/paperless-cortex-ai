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

func TestClaimQueueItemByIDAllowsRetryForFailedItems(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "paperless-aiext-store-test.db")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(databasePath)
	})

	documentID := int64(42)
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Retry me", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	startedAt := int64(1234)
	failedItem, err := store.MarkQueueItemFailed(
		t.Context(),
		item.ID,
		"invalid llm output",
		`{"failure":true}`,
		"llama3.2",
		"llava",
		&startedAt,
	)
	if err != nil {
		t.Fatalf("mark queue item failed: %v", err)
	}
	if failedItem.Status != "failed" {
		t.Fatalf("expected failed status, got %q", failedItem.Status)
	}

	retriedItem, err := store.ClaimQueueItemByID(t.Context(), item.ID)
	if err != nil {
		t.Fatalf("claim failed queue item by id: %v", err)
	}
	if retriedItem.Status != "processing" {
		t.Fatalf("expected processing status after retry claim, got %q", retriedItem.Status)
	}
	if retriedItem.Attempts != failedItem.Attempts+1 {
		t.Fatalf("expected attempts to increment from %d to %d, got %d", failedItem.Attempts, failedItem.Attempts+1, retriedItem.Attempts)
	}
	if retriedItem.StartedAtMS == nil {
		t.Fatal("expected started_at_ms to be reset on retry claim")
	}
	if retriedItem.CompletedAtMS != nil {
		t.Fatalf("expected completed_at_ms cleared on retry claim, got %+v", retriedItem.CompletedAtMS)
	}
	if retriedItem.LastError != "" {
		t.Fatalf("expected last_error to be cleared on retry claim, got %q", retriedItem.LastError)
	}
	if retriedItem.ResultSummary != "" {
		t.Fatalf("expected result_summary to be cleared on retry claim, got %q", retriedItem.ResultSummary)
	}
	if retriedItem.ResultPayload != "" {
		t.Fatalf("expected result_payload to be cleared on retry claim, got %q", retriedItem.ResultPayload)
	}
	if retriedItem.UsedLLM != "" || retriedItem.UsedVisionLLM != "" {
		t.Fatalf("expected used model fields to be cleared on retry claim, got llm=%q vision=%q", retriedItem.UsedLLM, retriedItem.UsedVisionLLM)
	}
	if retriedItem.ProcessingDurationMS != nil {
		t.Fatalf("expected processing duration to be cleared on retry claim, got %+v", retriedItem.ProcessingDurationMS)
	}
}
