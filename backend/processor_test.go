package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func writeImplicitSuggestionResponse(w http.ResponseWriter, requestBody []byte) bool {
	payload := string(requestBody)
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.Contains(payload, "extracting the document creation date"):
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"created\":\"2026-03-15\",\"confidence\":\"high\",\"reasoning\":\"Das Dokument ist auf den 15.03.2026 datiert.\"}"}}`))
		return true
	case strings.Contains(payload, "generating a document title"):
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"title\":\"Allgemeiner Dokumenttitel\",\"confidence\":\"medium\",\"reasoning\":\"Ein kompakter Titel wurde aus dem Dokumentinhalt abgeleitet.\"}"}}`))
		return true
	default:
		return false
	}
}

func TestProcessorProcessesDocumentTypeSuggestion(t *testing.T) {
	ollamaRequests := 0
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		if writeImplicitSuggestionResponse(w, body) {
			return
		}
		ollamaRequests++
		if !strings.Contains(string(body), "Invoice text from processor test") {
			t.Fatalf("expected document text in ollama request, got %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"document_type_id\":7,\"document_type_name\":\"Invoice\",\"suggested_new_document_type\":null,\"confidence\":\"high\",\"reasoning\":\"Matches invoice wording\"}"}}`))
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"document-type"}],"next":null}`))
		case r.URL.Path == "/api/document_types/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":5,"name":"Receipt"},{"id":7,"name":"Invoice"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"April Invoice","original_file_name":"invoice.txt","document_type":5,"tags":[1,2]}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="invoice.txt"`)
			_, _ = w.Write([]byte("Invoice text from processor test"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTypeTag: "document-type",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "April Invoice", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != "completed" {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	if ollamaRequests != 1 {
		t.Fatalf("expected 1 ollama request, got %d", ollamaRequests)
	}

	var result ProcessingResult
	if err := json.Unmarshal([]byte(item.ResultPayload), &result); err != nil {
		t.Fatalf("decode result payload: %v", err)
	}
	if result.Extraction.Status != stageStatusCompleted || result.Extraction.Source != "document-text" {
		t.Fatalf("unexpected extraction result: %+v", result.Extraction)
	}
	if result.DocumentType.Status != stageStatusCompleted {
		t.Fatalf("expected document type stage completed, got %+v", result.DocumentType)
	}
	if result.Document.DocumentTypeID == nil || *result.Document.DocumentTypeID != 5 {
		t.Fatalf("expected current document type id in result document summary, got %+v", result.Document)
	}
	if result.Document.DocumentTypeName != "Receipt" {
		t.Fatalf("expected current document type name in result document summary, got %+v", result.Document)
	}
	if result.Correspondent.Status != stageStatusSkipped {
		t.Fatalf("expected correspondent stage skipped, got %+v", result.Correspondent)
	}
	if !strings.Contains(item.ResultSummary, "document type suggestion") {
		t.Fatalf("expected document type summary, got %q", item.ResultSummary)
	}
	if !strings.Contains(item.ResultPayload, `"document_type_id":7`) {
		t.Fatalf("expected document type suggestion in result payload, got %s", item.ResultPayload)
	}
}

func TestProcessorPersistsRunningStageProgress(t *testing.T) {
	suggestionStarted := make(chan struct{}, 1)
	releaseSuggestion := make(chan struct{})

	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		if writeImplicitSuggestionResponse(w, body) {
			return
		}

		select {
		case suggestionStarted <- struct{}{}:
		default:
		}

		<-releaseSuggestion

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"document_type_id\":7,\"document_type_name\":\"Invoice\",\"suggested_new_document_type\":null,\"confidence\":\"high\",\"reasoning\":\"Matches invoice wording\"}"}}`))
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"document-type"}],"next":null}`))
		case r.URL.Path == "/api/document_types/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":7,"name":"Invoice"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"April Invoice","original_file_name":"invoice.txt","tags":[1,2]}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="invoice.txt"`)
			_, _ = w.Write([]byte("Invoice text from processor test"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTypeTag: "document-type",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	createdItem, err := store.CreateQueueItem(t.Context(), &documentID, "April Invoice", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	processedItemCh := make(chan *QueueItem, 1)
	errCh := make(chan error, 1)
	go func() {
		item, processErr := processor.ProcessNext(t.Context())
		if processErr != nil {
			errCh <- processErr
			return
		}
		processedItemCh <- item
	}()

	select {
	case <-suggestionStarted:
	case err := <-errCh:
		t.Fatalf("process next queue item: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for suggestion stage to start")
	}

	progressItem, err := store.GetQueueItem(t.Context(), createdItem.ID)
	if err != nil {
		t.Fatalf("get queue item progress: %v", err)
	}
	if progressItem.Status != "processing" {
		t.Fatalf("expected processing status during progress snapshot, got %q", progressItem.Status)
	}
	if progressItem.ResultSummary != "Running document type suggestion." {
		t.Fatalf("expected running stage summary, got %q", progressItem.ResultSummary)
	}

	var progressResult ProcessingResult
	if err := json.Unmarshal([]byte(progressItem.ResultPayload), &progressResult); err != nil {
		t.Fatalf("decode progress result payload: %v", err)
	}
	if progressResult.Extraction.Status != stageStatusCompleted {
		t.Fatalf("expected extraction completed in progress snapshot, got %+v", progressResult.Extraction)
	}
	if progressResult.DocumentType.Status != stageStatusRunning {
		t.Fatalf("expected document type stage running, got %+v", progressResult.DocumentType)
	}

	close(releaseSuggestion)

	select {
	case err := <-errCh:
		t.Fatalf("process next queue item: %v", err)
	case item := <-processedItemCh:
		if item.Status != "completed" {
			t.Fatalf("expected completed status, got %q", item.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for processor completion")
	}
}

func TestProcessorSkipsWhenTriggerTagMissing(t *testing.T) {
	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"document-type"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"April Invoice","original_file_name":"invoice.txt","tags":[2]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTypeTag: "document-type",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: "http://localhost:11434", DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "April Invoice", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != "completed" {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	if !strings.Contains(item.ResultSummary, "trigger tag") {
		t.Fatalf("expected trigger-tag skip summary, got %q", item.ResultSummary)
	}

	var result ProcessingResult
	if err := json.Unmarshal([]byte(item.ResultPayload), &result); err != nil {
		t.Fatalf("decode result payload: %v", err)
	}
	if result.Plan.TriggerTagPresent {
		t.Fatalf("expected trigger tag to be absent in plan: %+v", result.Plan)
	}
	if len(result.Notes) == 0 {
		t.Fatalf("expected skip notes in result payload")
	}
}

func TestProcessorAutoModeAppliesSuggestionsAfterProcessing(t *testing.T) {
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		if writeImplicitSuggestionResponse(w, body) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"tag_ids\":[2],\"tag_names\":[\"invoice\"],\"suggested_new_tags\":[],\"confidence\":\"high\",\"reasoning\":\"Matches invoice wording\"}"}}`))
	}))
	defer ollamaServer.Close()

	patched := false
	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"invoice"},{"id":3,"name":"completed"},{"id":4,"name":"document-tags"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"April Invoice","original_file_name":"invoice.txt","tags":[1,4]}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="invoice.txt"`)
			_, _ = w.Write([]byte("Invoice text from processor test"))
		case r.URL.Path == "/api/documents/42/" && r.Method == http.MethodPatch:
			patched = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode patch payload: %v", err)
			}
			tags := payload["tags"].([]any)
			if len(tags) != 2 || tags[0].(float64) != 2 || tags[1].(float64) != 3 {
				t.Fatalf("expected final tag ids [2,3], got %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"April Invoice","tags":[2,3]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeAuto, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTagsTag: "document-tags",
			ProcessCompletedTag:    "completed",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "April Invoice", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != queueItemStatusCompleted {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	if item.ApplyStatus != "applied" {
		t.Fatalf("expected apply status applied, got %q", item.ApplyStatus)
	}
	if item.AppliedSummary == "" {
		t.Fatal("expected applied summary on auto-applied item")
	}
	if !patched {
		t.Fatal("expected paperless patch request in auto mode")
	}
}

func TestProcessorProcessesCreatedDateAndTitleSuggestions(t *testing.T) {
	createdDateRequests := 0
	titleRequests := 0
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		payload := string(body)

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(payload, "extracting the document creation date"):
			createdDateRequests++
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"created\":\"2026-03-15\",\"confidence\":\"high\",\"reasoning\":\"Das Dokument ist auf den 15.03.2026 datiert.\"}"}}`))
		case strings.Contains(payload, "generating a document title"):
			titleRequests++
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"title\":\"Telekom Rechnung Maerz 2026\",\"confidence\":\"high\",\"reasoning\":\"Aussteller, Dokumenttyp und Monat sind klar erkennbar.\"}"}}`))
		default:
			t.Fatalf("unexpected ollama prompt: %s", payload)
		}
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"created-date"},{"id":3,"name":"title"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"scan-42","original_file_name":"invoice.txt","created":"2026-04-12","added":"2026-04-13T10:00:00Z","tags":[1,2,3]}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="invoice.txt"`)
			_, _ = w.Write([]byte("Telekom Rechnung vom 15.03.2026 fuer Maerz 2026"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:     "process",
			ProcessCreatedDateTag: "created-date",
			ProcessTitleTag:       "title",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "scan-42", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != queueItemStatusCompleted {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	if createdDateRequests != 1 {
		t.Fatalf("expected 1 creation-date request, got %d", createdDateRequests)
	}
	if titleRequests != 1 {
		t.Fatalf("expected 1 title request, got %d", titleRequests)
	}

	var result ProcessingResult
	if err := json.Unmarshal([]byte(item.ResultPayload), &result); err != nil {
		t.Fatalf("decode result payload: %v", err)
	}
	if !result.Plan.CreatedDate || !result.Plan.Title {
		t.Fatalf("expected configured created-date and title stages in plan, got %+v", result.Plan)
	}
	if result.CreatedDate.Status != stageStatusCompleted {
		t.Fatalf("expected created-date stage completed, got %+v", result.CreatedDate)
	}
	if result.Title.Status != stageStatusCompleted {
		t.Fatalf("expected title stage completed, got %+v", result.Title)
	}
	if !strings.Contains(item.ResultSummary, "creation date suggestion") {
		t.Fatalf("expected creation date summary, got %q", item.ResultSummary)
	}
	if !strings.Contains(item.ResultSummary, "title suggestion") {
		t.Fatalf("expected title summary, got %q", item.ResultSummary)
	}
	if !strings.Contains(item.ResultPayload, `"created":"2026-03-15"`) {
		t.Fatalf("expected created-date suggestion in result payload, got %s", item.ResultPayload)
	}
	if !strings.Contains(item.ResultPayload, `"title":"Telekom Rechnung Maerz 2026"`) {
		t.Fatalf("expected title suggestion in result payload, got %s", item.ResultPayload)
	}
}

func TestProcessorAutoModeAppliesCreatedDateAndTitleBeforeCompletedTag(t *testing.T) {
	patched := false
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		payload := string(body)

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(payload, "extracting the document creation date"):
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"created\":\"2026-03-15\",\"confidence\":\"high\",\"reasoning\":\"Das Dokument ist auf den 15.03.2026 datiert.\"}"}}`))
		case strings.Contains(payload, "generating a document title"):
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"title\":\"Telekom Rechnung Maerz 2026\",\"confidence\":\"high\",\"reasoning\":\"Aussteller, Dokumenttyp und Monat sind klar erkennbar.\"}"}}`))
		default:
			t.Fatalf("unexpected ollama prompt: %s", payload)
		}
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"created-date"},{"id":3,"name":"title"},{"id":4,"name":"completed"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"scan-42","original_file_name":"invoice.txt","created":"2026-04-12","tags":[1,2,3]}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="invoice.txt"`)
			_, _ = w.Write([]byte("Telekom Rechnung vom 15.03.2026 fuer Maerz 2026"))
		case r.URL.Path == "/api/documents/42/" && r.Method == http.MethodPatch:
			patched = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode patch payload: %v", err)
			}
			if payload["created"].(string) != "2026-03-15" {
				t.Fatalf("expected created 2026-03-15, got %+v", payload)
			}
			if payload["title"].(string) != "Telekom Rechnung Maerz 2026" {
				t.Fatalf("expected title suggestion, got %+v", payload)
			}
			tags := payload["tags"].([]any)
			if len(tags) != 1 || tags[0].(float64) != 4 {
				t.Fatalf("expected final tag ids [4], got %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Telekom Rechnung Maerz 2026","created":"2026-03-15","tags":[4]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeAuto, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:     "process",
			ProcessCreatedDateTag: "created-date",
			ProcessTitleTag:       "title",
			ProcessCompletedTag:   "completed",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "scan-42", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != queueItemStatusCompleted {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	if item.ApplyStatus != "applied" {
		t.Fatalf("expected apply status applied, got %q", item.ApplyStatus)
	}
	if !patched {
		t.Fatal("expected paperless patch request in auto mode")
	}
	if !strings.Contains(item.AppliedSummary, "completed tag") {
		t.Fatalf("expected applied summary mentioning completed tag, got %q", item.AppliedSummary)
	}
}

func TestProcessorUsesHistoricalDocumentsForCorrespondentSuggestion(t *testing.T) {
	ollamaRequests := 0
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		if writeImplicitSuggestionResponse(w, body) {
			return
		}
		ollamaRequests++
		payload := string(body)
		if !strings.Contains(payload, "Historical library evidence") {
			t.Fatalf("expected historical evidence in ollama request, got %s", payload)
		}
		if !strings.Contains(payload, "Telekom Rechnung April") {
			t.Fatalf("expected historical Telekom example in ollama request, got %s", payload)
		}
		if !strings.Contains(payload, "Aktuelle Telekom Rechnung") {
			t.Fatalf("expected current document text in ollama request, got %s", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"correspondent_id\":12,\"correspondent_name\":\"Telekom\",\"suggested_new_correspondent\":null,\"confidence\":\"high\",\"reasoning\":\"Historical Telekom examples and the current text both match.\"}"}}`))
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"correspondent"}],"next":null}`))
		case r.URL.Path == "/api/correspondents/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":12,"name":"Telekom"},{"id":21,"name":"Vodafone"},{"id":22,"name":"Allianz"},{"id":23,"name":"Stadtwerke"}],"next":null}`))
		case r.URL.Path == "/api/documents/" && r.URL.Query().Get("ordering") == "-created":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":99,"title":"Telekom Rechnung April","original_file_name":"telekom-april.txt","correspondent":12,"content":"Telekom Rechnung Kundennummer 123 Rechnungsnummer 456"},{"id":98,"title":"Vodafone Rechnung","original_file_name":"vodafone.txt","correspondent":21,"content":"Vodafone Tarif Vertragsnummer 555"},{"id":97,"title":"Allianz Beitrag","original_file_name":"allianz.txt","correspondent":22,"content":"Allianz Policennummer Jahresbeitrag"},{"id":96,"title":"Stadtwerke Abschlag","original_file_name":"stadtwerke.txt","correspondent":23,"content":"Stadtwerke Abschlag Zaehlerstand"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Mai Rechnung","original_file_name":"current.txt","tags":[1,2]}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="current.txt"`)
			_, _ = w.Write([]byte("Aktuelle Telekom Rechnung mit Kundennummer 123 und Rechnungsnummer 456"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:       "process",
			ProcessCorrespondentTag: "correspondent",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "Mai Rechnung", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != "completed" {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	if ollamaRequests != 1 {
		t.Fatalf("expected 1 ollama request, got %d", ollamaRequests)
	}
	if !strings.Contains(item.ResultPayload, `"correspondent_id":12`) {
		t.Fatalf("expected correspondent suggestion in result payload, got %s", item.ResultPayload)
	}
}

func TestProcessorInjectsSimilarDocumentEvidenceForDocumentTypeAndTags(t *testing.T) {
	chatRequests := 0
	embedRequests := 0
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			embedRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"embedding":[0.9,0.1,0.3]}`))
			return
		case "/api/chat":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read ollama request: %v", err)
			}
			if writeImplicitSuggestionResponse(w, body) {
				return
			}
			chatRequests++

			payload := string(body)
			if !strings.Contains(payload, "Similar library documents") {
				t.Fatalf("expected similar document evidence in prompt, got %s", payload)
			}
			if !strings.Contains(payload, "document_type=Invoice") {
				t.Fatalf("expected historical document type evidence in prompt, got %s", payload)
			}
			if !strings.Contains(payload, "tags=invoice, telecom") {
				t.Fatalf("expected historical tag evidence in prompt, got %s", payload)
			}

			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(payload, "Choose the best matching existing document type") {
				_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"document_type_id\":7,\"document_type_name\":\"Invoice\",\"suggested_new_document_type\":null,\"confidence\":\"high\",\"reasoning\":\"Passt zu den aehnlichen Rechnungen in der Bibliothek.\"}"}}`))
				return
			}
			if strings.Contains(payload, "Choose zero or more existing tags") {
				_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"tag_ids\":[10,11],\"tag_names\":[\"invoice\",\"telecom\"],\"suggested_new_tags\":[],\"confidence\":\"high\",\"reasoning\":\"Die aehnlichen Dokumente tragen dieselben Tags.\"}"}}`))
				return
			}

			t.Fatalf("unexpected chat request payload: %s", payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"document-type"},{"id":3,"name":"document-tags"},{"id":10,"name":"invoice"},{"id":11,"name":"telecom"}],"next":null}`))
		case r.URL.Path == "/api/correspondents/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":12,"name":"Telekom"}],"next":null}`))
		case r.URL.Path == "/api/document_types/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":5,"name":"Receipt"},{"id":7,"name":"Invoice"}],"next":null}`))
		case r.URL.Path == "/api/documents/" && r.URL.Query().Get("ordering") == "-created":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":99,"title":"Telekom Rechnung April","original_file_name":"telekom-april.pdf","document_type":7,"correspondent":12,"tags":[10,11],"content":"Rechnung Telekom April 2026","created":"2026-04-05","modified":"2026-04-05T08:00:00Z"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Mai Rechnung","original_file_name":"mai-rechnung.pdf","document_type":5,"tags":[1,2,3],"modified":"2026-05-01T08:00:00Z"}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="mai-rechnung.pdf"`)
			_, _ = w.Write([]byte("Aktuelle Telekom Rechnung fuer Mai 2026"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTypeTag: "document-type",
			ProcessDocumentTagsTag: "document-tags",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs: LLMConfig{
			OllamaURL:  ollamaServer.URL,
			DefaultLLM: "llama3.2",
			VisionLLM:  "llava",
			Embeddings: EmbeddingsConfig{
				Enabled:                 true,
				Model:                   "nomic-embed-text",
				SyncIntervalSeconds:     120,
				HistoricalDocumentLimit: 50,
				TopK:                    5,
				SimilarityThreshold:     0.1,
				MaxDocumentsPerRun:      10,
			},
		},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "Mai Rechnung", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != queueItemStatusCompleted {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	if chatRequests != 2 {
		t.Fatalf("expected 2 chat requests for document type and tags, got %d", chatRequests)
	}
	if embedRequests < 2 {
		t.Fatalf("expected at least 2 embedding requests (query + historical), got %d", embedRequests)
	}

	if !strings.Contains(item.ResultPayload, `"document_type_id":7`) {
		t.Fatalf("expected document type suggestion in payload, got %s", item.ResultPayload)
	}
	if !strings.Contains(item.ResultPayload, `"tag_ids":[10,11]`) {
		t.Fatalf("expected tag suggestion in payload, got %s", item.ResultPayload)
	}
}

func TestProcessorContinuesAfterSuggestionStageFailure(t *testing.T) {
	ollamaRequests := 0
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		if writeImplicitSuggestionResponse(w, body) {
			return
		}

		ollamaRequests++
		if ollamaRequests == 1 {
			http.Error(w, "correspondent model failure", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"document_type_id\":7,\"document_type_name\":\"Invoice\",\"suggested_new_document_type\":null,\"confidence\":\"high\",\"reasoning\":\"Matches invoice wording\"}"}}`))
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"correspondent"},{"id":3,"name":"document-type"}],"next":null}`))
		case r.URL.Path == "/api/correspondents/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":12,"name":"Telekom"}],"next":null}`))
		case r.URL.Path == "/api/document_types/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":7,"name":"Invoice"}],"next":null}`))
		case r.URL.Path == "/api/documents/" && r.URL.Query().Get("ordering") == "-created":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[],"next":null}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"April Invoice","original_file_name":"invoice.txt","tags":[1,2,3]}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="invoice.txt"`)
			_, _ = w.Write([]byte("Invoice text from processor test"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:       "process",
			ProcessCorrespondentTag: "correspondent",
			ProcessDocumentTypeTag:  "document-type",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "April Invoice", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != queueItemStatusPartiallyCompleted {
		t.Fatalf("expected partially completed status for partial processing failure, got %q", item.Status)
	}
	if ollamaRequests != 2 {
		t.Fatalf("expected 2 ollama requests, got %d", ollamaRequests)
	}
	if !strings.Contains(item.LastError, "correspondent suggestion") {
		t.Fatalf("expected stage failure in last error, got %q", item.LastError)
	}
	if !strings.Contains(item.ResultSummary, "document type suggestion") {
		t.Fatalf("expected successful stage summary to be preserved, got %q", item.ResultSummary)
	}

	var result ProcessingResult
	if err := json.Unmarshal([]byte(item.ResultPayload), &result); err != nil {
		t.Fatalf("decode result payload: %v", err)
	}
	if result.Correspondent.Status != stageStatusFailed {
		t.Fatalf("expected correspondent stage failed, got %+v", result.Correspondent)
	}
	if result.DocumentType.Status != stageStatusCompleted {
		t.Fatalf("expected document type stage completed, got %+v", result.DocumentType)
	}
	if !strings.Contains(item.ResultPayload, `"document_type_id":7`) {
		t.Fatalf("expected document type suggestion in result payload, got %s", item.ResultPayload)
	}
	if result.Extraction.Status != stageStatusCompleted {
		t.Fatalf("expected extraction stage completed, got %+v", result.Extraction)
	}
}

func TestProcessorFallsBackToVisionWhenDownloadLacksFilename(t *testing.T) {
	ollamaRequests := 0
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		if writeImplicitSuggestionResponse(w, body) {
			return
		}
		ollamaRequests++
		if ollamaRequests == 1 {
			http.Error(w, "ocr model failure", http.StatusInternalServerError)
			return
		}
		if ollamaRequests == 2 {
			if !bytes.Contains(body, []byte(`"model":"llava"`)) {
				t.Fatalf("expected fallback to vision model on second request, got %s", string(body))
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Vision extracted text from fallback path"}}`))
			return
		}
		if !bytes.Contains(body, []byte(`"model":"llama3.2"`)) {
			t.Fatalf("expected document type request to use default model, got %s", string(body))
		}
		if !bytes.Contains(body, []byte(`Vision extracted text from fallback path`)) {
			t.Fatalf("expected document type stage to receive fallback text, got %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"document_type_id\":7,\"document_type_name\":\"Invoice\",\"suggested_new_document_type\":null,\"confidence\":\"high\",\"reasoning\":\"Passt zum Rechnungsinhalt.\"}"}}`))
	}))
	defer ollamaServer.Close()

	pngBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c,
		0x02, 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
		0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00,
		0x03, 0x03, 0x01, 0xff, 0xa5, 0xfe, 0xff, 0x9f,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
		0xae, 0x42, 0x60, 0x82,
	}

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"document-type"}],"next":null}`))
		case r.URL.Path == "/api/document_types/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":7,"name":"Invoice"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Scanned Invoice","original_file_name":"scan.pdf","tags":[1,2]}`))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTypeTag: "document-type",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	documentID := int64(42)
	if _, err := store.CreateQueueItem(t.Context(), &documentID, "Scanned Invoice", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	item, err := processor.ProcessNext(t.Context())
	if err != nil {
		t.Fatalf("process next queue item: %v", err)
	}
	if item.Status != "completed" {
		t.Fatalf("expected completed status, got %q", item.Status)
	}
	if ollamaRequests != 3 {
		t.Fatalf("expected 3 ollama requests (failed OCR, vision fallback, document type), got %d", ollamaRequests)
	}

	var result ProcessingResult
	if err := json.Unmarshal([]byte(item.ResultPayload), &result); err != nil {
		t.Fatalf("decode result payload: %v", err)
	}
	if result.Extraction.Status != stageStatusCompleted || result.Extraction.Source != "vision-llm" {
		t.Fatalf("expected vision fallback extraction, got %+v", result.Extraction)
	}
	if result.Extraction.UsedModel != "llava" {
		t.Fatalf("expected vision model llava, got %+v", result.Extraction)
	}
	if result.DocumentType.Status != stageStatusCompleted {
		t.Fatalf("expected document type stage completed, got %+v", result.DocumentType)
	}
}

func TestProcessorStartAutoModeDrainsQueuedItemsWithoutExtraIdleWait(t *testing.T) {
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		if writeImplicitSuggestionResponse(w, body) {
			return
		}

		http.Error(w, "unexpected ollama prompt", http.StatusBadRequest)
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"}],"next":null}`))
		case r.URL.Path == "/api/documents/41/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":41,"title":"First Item","original_file_name":"first.txt","tags":[1]}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Second Item","original_file_name":"second.txt","tags":[1]}`))
		case r.URL.Path == "/api/documents/41/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="first.txt"`)
			_, _ = w.Write([]byte("First processor item dated 15.03.2026"))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="second.txt"`)
			_, _ = w.Write([]byte("Second processor item dated 15.03.2026"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	waitCalls := make(chan time.Duration, 4)
	releaseWait := make(chan struct{}, 1)
	processor.waitForNextCycle = func(ctx context.Context, waitInterval time.Duration) bool {
		waitCalls <- waitInterval
		select {
		case <-ctx.Done():
			return false
		case <-releaseWait:
			return true
		}
	}

	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeAuto, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag: "process",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	firstDocumentID := int64(41)
	firstItem, err := store.CreateQueueItem(t.Context(), &firstDocumentID, "First Item", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create first queue item: %v", err)
	}
	secondDocumentID := int64(42)
	secondItem, err := store.CreateQueueItem(t.Context(), &secondDocumentID, "Second Item", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create second queue item: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	processor.Start(ctx)

	select {
	case waitInterval := <-waitCalls:
		if waitInterval != 30*time.Second {
			t.Fatalf("expected 30s idle wait interval, got %v", waitInterval)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial auto-mode wait")
	}

	releaseWait <- struct{}{}

	waitForQueueItemStatus(t, store, firstItem.ID, queueItemStatusCompleted, 2*time.Second)
	waitForQueueItemStatus(t, store, secondItem.ID, queueItemStatusCompleted, 2*time.Second)

	select {
	case waitInterval := <-waitCalls:
		if waitInterval != 30*time.Second {
			t.Fatalf("expected resumed idle wait interval of 30s, got %v", waitInterval)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for idle polling to resume after queue drain")
	}
}

func TestProcessorStartAutoModeStopsClaimingWhenModeSwitchesToManual(t *testing.T) {
	suggestionStarted := make(chan struct{}, 1)
	releaseSuggestion := make(chan struct{})

	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read ollama request: %v", err)
		}
		if writeImplicitSuggestionResponse(w, body) {
			return
		}

		select {
		case suggestionStarted <- struct{}{}:
		default:
		}

		<-releaseSuggestion

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"document_type_id\":7,\"document_type_name\":\"Invoice\",\"suggested_new_document_type\":null,\"confidence\":\"high\",\"reasoning\":\"Matches invoice wording\"}"}}`))
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"document-type"}],"next":null}`))
		case r.URL.Path == "/api/document_types/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":7,"name":"Invoice"}],"next":null}`))
		case r.URL.Path == "/api/documents/41/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":41,"title":"First Item","original_file_name":"first.txt","tags":[1,2]}`))
		case r.URL.Path == "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Second Item","original_file_name":"second.txt","tags":[1,2]}`))
		case r.URL.Path == "/api/documents/41/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="first.txt"`)
			_, _ = w.Write([]byte("Invoice text from first processor item"))
		case r.URL.Path == "/api/documents/42/download/":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Disposition", `attachment; filename="second.txt"`)
			_, _ = w.Write([]byte("Invoice text from second processor item"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	processor, store := newProcessorTestHarness(t)
	waitCalls := make(chan time.Duration, 2)
	releaseWait := make(chan struct{}, 1)
	processor.waitForNextCycle = func(ctx context.Context, waitInterval time.Duration) bool {
		waitCalls <- waitInterval
		select {
		case <-ctx.Done():
			return false
		case <-releaseWait:
			return true
		}
	}

	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeAuto, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTypeTag: "document-type",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	firstDocumentID := int64(41)
	firstItem, err := store.CreateQueueItem(t.Context(), &firstDocumentID, "First Item", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create first queue item: %v", err)
	}
	secondDocumentID := int64(42)
	secondItem, err := store.CreateQueueItem(t.Context(), &secondDocumentID, "Second Item", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create second queue item: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	processor.Start(ctx)

	select {
	case <-waitCalls:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial auto-mode wait")
	}

	releaseWait <- struct{}{}

	select {
	case <-suggestionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first queued item to start processing")
	}

	saveProcessorTestConfig(t, store, BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTypeTag: "document-type",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL, DefaultLLM: "llama3.2", VisionLLM: "llava"},
	})

	close(releaseSuggestion)

	waitForQueueItemStatus(t, store, firstItem.ID, queueItemStatusCompleted, 2*time.Second)
	assertQueueItemStatus(t, store, secondItem.ID, queueItemStatusPending)
}

func newProcessorTestHarness(t *testing.T) (*Processor, *Store) {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "paperless-aiext-processor-test.db")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(databasePath)
	})

	logger := zerolog.New(io.Discard)
	return NewProcessor(store, logger), store
}

func waitForQueueItemStatus(t *testing.T, store *Store, id int64, wantStatus string, timeout time.Duration) *QueueItem {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		item, err := store.GetQueueItem(t.Context(), id)
		if err != nil {
			t.Fatalf("get queue item %d: %v", id, err)
		}
		if item.Status == wantStatus {
			return item
		}
		time.Sleep(10 * time.Millisecond)
	}

	item, err := store.GetQueueItem(t.Context(), id)
	if err != nil {
		t.Fatalf("get queue item %d after timeout: %v", id, err)
	}
	t.Fatalf("expected queue item %d status %q within %v, got %q", id, wantStatus, timeout, item.Status)
	return nil
}

func assertQueueItemStatus(t *testing.T, store *Store, id int64, wantStatus string) {
	t.Helper()

	item, err := store.GetQueueItem(t.Context(), id)
	if err != nil {
		t.Fatalf("get queue item %d: %v", id, err)
	}
	if item.Status != wantStatus {
		t.Fatalf("expected queue item %d status %q, got %q", id, wantStatus, item.Status)
	}
}

func saveProcessorTestConfig(t *testing.T, store *Store, cfg BackendConfig) {
	t.Helper()
	if err := store.SaveConfig(t.Context(), cfg); err != nil {
		t.Fatalf("save backend config: %v", err)
	}
}
