package main

import (
	"errors"
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

func TestClaimQueueItemByIDAllowsRetryForPartiallyCompletedItems(t *testing.T) {
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
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Partial retry", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	startedAt := int64(1234)
	partialItem, err := store.MarkQueueItemPartiallyCompleted(
		t.Context(),
		item.ID,
		"Completed document type suggestion.",
		"correspondent suggestion: invalid llm output",
		`{"document_type":{"status":"completed"}}`,
		"llama3.2",
		"llava",
		&startedAt,
	)
	if err != nil {
		t.Fatalf("mark queue item partially completed: %v", err)
	}
	if partialItem.Status != queueItemStatusPartiallyCompleted {
		t.Fatalf("expected partially completed status, got %q", partialItem.Status)
	}

	retriedItem, err := store.ClaimQueueItemByID(t.Context(), item.ID)
	if err != nil {
		t.Fatalf("claim partially completed queue item by id: %v", err)
	}
	if retriedItem.Status != queueItemStatusProcessing {
		t.Fatalf("expected processing status after retry claim, got %q", retriedItem.Status)
	}
	if retriedItem.Attempts != partialItem.Attempts+1 {
		t.Fatalf("expected attempts to increment from %d to %d, got %d", partialItem.Attempts, partialItem.Attempts+1, retriedItem.Attempts)
	}
	if retriedItem.LastError != "" {
		t.Fatalf("expected last_error to be cleared on retry claim, got %q", retriedItem.LastError)
	}
	if retriedItem.ResultSummary != "" {
		t.Fatalf("expected result_summary to be cleared on retry claim, got %q", retriedItem.ResultSummary)
	}
}

func TestDeleteQueueItemRemovesNonProcessingItems(t *testing.T) {
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
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Delete me", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	if err := store.DeleteQueueItem(t.Context(), item.ID); err != nil {
		t.Fatalf("delete queue item: %v", err)
	}

	_, err = store.GetQueueItem(t.Context(), item.ID)
	if !errors.Is(err, errQueueItemNotFound) {
		t.Fatalf("expected queue item to be deleted, got %v", err)
	}
	items, err := store.ListQueueItems(t.Context(), "", 10)
	if err != nil {
		t.Fatalf("list queue items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected queue to be empty after deletion, got %d items", len(items))
	}
}

func TestCreateQueueItemWithRequestedStagesPersists(t *testing.T) {
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
	item, err := store.CreateQueueItemWithRequestedStages(
		t.Context(),
		&documentID,
		"Plan me",
		"paperless",
		"webhook",
		`{}`,
		[]string{"extract_text", "document_type", "tags"},
	)
	if err != nil {
		t.Fatalf("create queue item with requested stages: %v", err)
	}

	if strings.Join(item.RequestedStages, ",") != "extract_text,document_type,tags" {
		t.Fatalf("unexpected requested stages: %+v", item.RequestedStages)
	}

	reloaded, err := store.GetQueueItem(t.Context(), item.ID)
	if err != nil {
		t.Fatalf("get queue item: %v", err)
	}
	if strings.Join(reloaded.RequestedStages, ",") != "extract_text,document_type,tags" {
		t.Fatalf("unexpected persisted requested stages: %+v", reloaded.RequestedStages)
	}
}

func TestMarkQueueItemAppliedPersistsMetadata(t *testing.T) {
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
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Apply me", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	startedAt := int64(1234)
	completedItem, err := store.MarkQueueItemCompleted(
		t.Context(),
		item.ID,
		"Completed tag suggestion.",
		`{"tags":{"status":"completed"}}`,
		"llama3.2",
		"llava",
		&startedAt,
	)
	if err != nil {
		t.Fatalf("mark queue item completed: %v", err)
	}

	appliedItem, err := store.MarkQueueItemApplied(t.Context(), completedItem.ID, "Applied tags to Paperless document.")
	if err != nil {
		t.Fatalf("mark queue item applied: %v", err)
	}
	if appliedItem.ApplyStatus != "applied" {
		t.Fatalf("expected applied status, got %q", appliedItem.ApplyStatus)
	}
	if appliedItem.AppliedAtMS == nil {
		t.Fatal("expected applied_at_ms to be set")
	}
	if appliedItem.AppliedSummary != "Applied tags to Paperless document." {
		t.Fatalf("unexpected applied summary: %q", appliedItem.AppliedSummary)
	}
	if appliedItem.ApplyError != "" {
		t.Fatalf("expected empty apply error, got %q", appliedItem.ApplyError)
	}
}

func TestClaimQueueItemByIDClearsApplyState(t *testing.T) {
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
	completedItem, err := store.MarkQueueItemPartiallyCompleted(
		t.Context(),
		item.ID,
		"Completed tag suggestion.",
		"tag writeback pending",
		`{"tags":{"status":"completed"}}`,
		"llama3.2",
		"llava",
		&startedAt,
	)
	if err != nil {
		t.Fatalf("mark queue item partially completed: %v", err)
	}
	if _, err := store.MarkQueueItemApplyFailed(t.Context(), completedItem.ID, "paperless unavailable"); err != nil {
		t.Fatalf("mark queue item apply failed: %v", err)
	}

	retriedItem, err := store.ClaimQueueItemByID(t.Context(), item.ID)
	if err != nil {
		t.Fatalf("claim queue item by id: %v", err)
	}
	if retriedItem.ApplyStatus != "" {
		t.Fatalf("expected apply status cleared on retry claim, got %q", retriedItem.ApplyStatus)
	}
	if retriedItem.AppliedAtMS != nil {
		t.Fatalf("expected applied_at_ms cleared on retry claim, got %+v", retriedItem.AppliedAtMS)
	}
	if retriedItem.ApplyError != "" {
		t.Fatalf("expected apply error cleared on retry claim, got %q", retriedItem.ApplyError)
	}
	if retriedItem.AppliedSummary != "" {
		t.Fatalf("expected applied summary cleared on retry claim, got %q", retriedItem.AppliedSummary)
	}
}

func TestStoreDeleteQueueItemRejectsProcessingItems(t *testing.T) {
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
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Delete me", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}
	if _, err := store.ClaimQueueItemByID(t.Context(), item.ID); err != nil {
		t.Fatalf("claim queue item: %v", err)
	}

	err = store.DeleteQueueItem(t.Context(), item.ID)
	if !errors.Is(err, errQueueItemNotRemovable) {
		t.Fatalf("expected errQueueItemNotRemovable, got %v", err)
	}
}
