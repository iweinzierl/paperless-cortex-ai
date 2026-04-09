package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

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

func saveProcessorTestConfig(t *testing.T, store *Store, cfg BackendConfig) {
	t.Helper()
	if err := store.SaveConfig(t.Context(), cfg); err != nil {
		t.Fatalf("save backend config: %v", err)
	}
}
