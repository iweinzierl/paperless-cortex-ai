package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestPaperlessWebhookRequiresSharedSecret(t *testing.T) {
	router, _ := newWebhookTestRouter(t, "")

	response := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(`{"document_title":"Invoice April","document_url":"https://paperless.example/documents/101/details"}`), map[string]string{
		"Content-Type":    "application/json",
		"x-shared-secret": "secret",
	})

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
}

func TestPaperlessWebhookRejectsInvalidSecret(t *testing.T) {
	router, _ := newWebhookTestRouter(t, "secret")

	response := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(`{"document_title":"Invoice April","document_url":"https://paperless.example/documents/101/details"}`), map[string]string{
		"Content-Type":    "application/json",
		"x-shared-secret": "wrong",
	})

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}
}

func TestPaperlessWebhookAcceptsJSONPayload(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")

	response := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(`{"document_title":"Invoice April","document_url":"https://paperless.example/documents/42/details"}`), map[string]string{
		"Content-Type":    "application/json",
		"x-shared-secret": "secret",
	})

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, response.Code)
	}

	items, err := store.ListQueueItems(t.Context(), "", 10)
	if err != nil {
		t.Fatalf("list queue items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 queue item, got %d", len(items))
	}
	if items[0].DocumentID == nil || *items[0].DocumentID != 42 {
		t.Fatalf("expected document ID 42, got %+v", items[0].DocumentID)
	}
	if items[0].DocumentTitle != "Invoice April" {
		t.Fatalf("expected title Invoice April, got %q", items[0].DocumentTitle)
	}
	if items[0].Trigger != "webhook" {
		t.Fatalf("expected trigger webhook, got %q", items[0].Trigger)
	}
	if !strings.Contains(items[0].Payload, `"document_url":"https://paperless.example/documents/42/details"`) {
		t.Fatalf("expected stored normalized payload, got %q", items[0].Payload)
	}
}

func TestPaperlessWebhookAcceptsJSONStringWrappedPayload(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")
	body := `"{\"document_title\":\"Invoice April\",\"document_url\":\"https://paperless.example/documents/42/details\"}"`

	response := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(body), map[string]string{
		"Content-Type":    "application/json",
		"x-shared-secret": "secret",
	})

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, response.Code, response.Body.String())
	}

	items, err := store.ListQueueItems(t.Context(), "", 10)
	if err != nil {
		t.Fatalf("list queue items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 queue item, got %d", len(items))
	}
	if items[0].DocumentID == nil || *items[0].DocumentID != 42 {
		t.Fatalf("expected document ID 42, got %+v", items[0].DocumentID)
	}
}

func TestPaperlessWebhookReusesActiveQueueItem(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")
	body := `{"document_title":"Invoice April","document_url":"https://paperless.example/documents/42/details"}`
	headers := map[string]string{
		"Content-Type":    "application/json",
		"x-shared-secret": "secret",
	}

	first := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(body), headers)
	if first.Code != http.StatusAccepted {
		t.Fatalf("expected first status %d, got %d", http.StatusAccepted, first.Code)
	}

	second := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(body), headers)
	if second.Code != http.StatusAccepted {
		t.Fatalf("expected second status %d, got %d", http.StatusAccepted, second.Code)
	}

	var responseBody map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &responseBody); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if reused, _ := responseBody["reused"].(bool); !reused {
		t.Fatalf("expected reused=true response, got %s", second.Body.String())
	}

	items, err := store.ListQueueItems(t.Context(), "", 10)
	if err != nil {
		t.Fatalf("list queue items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 queue item after duplicate request, got %d", len(items))
	}
}

func TestPaperlessWebhookPersistsRequestedStages(t *testing.T) {
	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"document-type"},{"id":3,"name":"tags"}],"next":null}`))
		case "/api/documents/42/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Invoice April","tags":[1,2,3]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	router, store := newWebhookTestRouter(t, "secret")
	if err := store.SaveConfig(t.Context(), BackendConfig{
		Engine: EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process: ProcessConfig{
			ProcessTriggerTag:      "process",
			ProcessDocumentTypeTag: "document-type",
			ProcessDocumentTagsTag: "tags",
		},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
		LLMs:      LLMConfig{OllamaURL: "http://localhost:11434"},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	response := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(`{"document_title":"Invoice April","document_url":"https://paperless.example/documents/42/details"}`), map[string]string{
		"Content-Type":    "application/json",
		"x-shared-secret": "secret",
	})

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, response.Code, response.Body.String())
	}

	items, err := store.ListQueueItems(t.Context(), "", 10)
	if err != nil {
		t.Fatalf("list queue items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 queue item, got %d", len(items))
	}
	if strings.Join(items[0].RequestedStages, ",") != "extract_text,document_type,tags" {
		t.Fatalf("unexpected requested stages: %+v", items[0].RequestedStages)
	}
}

func TestPaperlessWebhookRejectsUnsupportedContentType(t *testing.T) {
	router, _ := newWebhookTestRouter(t, "secret")

	response := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader("document_title=Invoice April&document_url=https://paperless.example/documents/77/details"), map[string]string{
		"Content-Type":    "application/x-www-form-urlencoded",
		"x-shared-secret": "secret",
	})

	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status %d, got %d", http.StatusUnsupportedMediaType, response.Code)
	}
}

func TestPaperlessWebhookRejectsPayloadWithoutDocumentURL(t *testing.T) {
	router, _ := newWebhookTestRouter(t, "secret")

	response := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(`{"document_title":"Invoice April"}`), map[string]string{
		"Content-Type":    "application/json",
		"x-shared-secret": "secret",
	})

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestPaperlessWebhookRejectsDocumentURLWithoutEmbeddedID(t *testing.T) {
	router, _ := newWebhookTestRouter(t, "secret")

	response := performWebhookRequest(t, router, http.MethodPost, "/api/webhooks/paperless", strings.NewReader(`{"document_title":"Invoice April","document_url":"https://paperless.example/documents/details"}`), map[string]string{
		"Content-Type":    "application/json",
		"x-shared-secret": "secret",
	})

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestExtractDocumentIDFromURLSupportsFragmentPaths(t *testing.T) {
	documentID, err := extractDocumentIDFromURL("https://paperless.example/#/documents/55/details")
	if err != nil {
		t.Fatalf("extract document id: %v", err)
	}
	if documentID == nil || *documentID != 55 {
		t.Fatalf("expected document ID 55, got %+v", documentID)
	}
}

func TestStatusEndpointReportsDependencyHealth(t *testing.T) {
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"mistral:latest","model":"mistral:latest","modified_at":"2026-01-01T00:00:00Z","size":1,"digest":"abc","details":{}}]}`))
	}))
	defer ollamaServer.Close()

	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags/" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Token paperless-token" {
			t.Fatalf("expected paperless authorization header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":1,"results":[{"id":1,"name":"process"}]}`))
	}))
	defer paperlessServer.Close()

	router, store := newWebhookTestRouter(t, "secret")
	if err := store.SaveConfig(t.Context(), BackendConfig{
		Engine:    EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "paperless-token"},
		LLMs:      LLMConfig{OllamaURL: ollamaServer.URL},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	response := performWebhookRequest(t, router, http.MethodGet, "/api/status", nil, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	var payload struct {
		Backend struct {
			Healthy bool `json:"healthy"`
		} `json:"backend"`
		Paperless struct {
			Configured bool `json:"configured"`
			Healthy    bool `json:"healthy"`
		} `json:"paperless"`
		Ollama struct {
			Configured bool `json:"configured"`
			Healthy    bool `json:"healthy"`
			ModelCount int  `json:"model_count"`
		} `json:"ollama"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !payload.Backend.Healthy {
		t.Fatalf("expected backend healthy response")
	}
	if !payload.Paperless.Configured || !payload.Paperless.Healthy {
		t.Fatalf("expected healthy paperless status, got %+v", payload.Paperless)
	}
	if !payload.Ollama.Configured || !payload.Ollama.Healthy || payload.Ollama.ModelCount != 1 {
		t.Fatalf("expected healthy ollama status, got %+v", payload.Ollama)
	}
}

func TestProcessQueueItemAllowsRetryForFailedItems(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")

	if err := store.CreateSession(t.Context(), Session{
		Token:        "session-token",
		Username:     "tester",
		CreatedAtMS:  nowMS(),
		ExpiresAtMS:  nowMS() + 60_000,
		LastSeenAtMS: nowMS(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	documentID := int64(42)
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Retry me", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}
	startedAt := nowMS()
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

	response := performWebhookRequest(
		t,
		router,
		http.MethodPost,
		fmt.Sprintf("/api/queue/%d/process", item.ID),
		nil,
		map[string]string{"Authorization": "Bearer session-token"},
	)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	var responseItem QueueItem
	if err := json.Unmarshal(response.Body.Bytes(), &responseItem); err != nil {
		t.Fatalf("decode process response: %v", err)
	}
	if responseItem.Status != "processing" {
		t.Fatalf("expected response item status processing, got %q", responseItem.Status)
	}
	if responseItem.Attempts != failedItem.Attempts+1 {
		t.Fatalf(
			"expected attempts to increment from %d to %d after retry, got %d",
			failedItem.Attempts,
			failedItem.Attempts+1,
			responseItem.Attempts,
		)
	}
}

func TestProcessQueueItemAllowsRetryForPartiallyCompletedItems(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")

	if err := store.CreateSession(t.Context(), Session{
		Token:        "session-token",
		Username:     "tester",
		CreatedAtMS:  nowMS(),
		ExpiresAtMS:  nowMS() + 60_000,
		LastSeenAtMS: nowMS(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	documentID := int64(42)
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Retry me", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}
	startedAt := nowMS()
	partialItem, err := store.MarkQueueItemPartiallyCompleted(
		t.Context(),
		item.ID,
		"Completed document type suggestion.",
		"invalid llm output",
		`{"failure":true}`,
		"llama3.2",
		"llava",
		&startedAt,
	)
	if err != nil {
		t.Fatalf("mark queue item partially completed: %v", err)
	}

	response := performWebhookRequest(
		t,
		router,
		http.MethodPost,
		fmt.Sprintf("/api/queue/%d/process", item.ID),
		nil,
		map[string]string{"Authorization": "Bearer session-token"},
	)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	var responseItem QueueItem
	if err := json.Unmarshal(response.Body.Bytes(), &responseItem); err != nil {
		t.Fatalf("decode process response: %v", err)
	}
	if responseItem.Status != queueItemStatusProcessing {
		t.Fatalf("expected response item status processing, got %q", responseItem.Status)
	}
	if responseItem.Attempts != partialItem.Attempts+1 {
		t.Fatalf(
			"expected attempts to increment from %d to %d after retry, got %d",
			partialItem.Attempts,
			partialItem.Attempts+1,
			responseItem.Attempts,
		)
	}
}

func TestDeleteQueueItemRemovesPendingItems(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")

	if err := store.CreateSession(t.Context(), Session{
		Token:        "session-token",
		Username:     "tester",
		CreatedAtMS:  nowMS(),
		ExpiresAtMS:  nowMS() + 60_000,
		LastSeenAtMS: nowMS(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	documentID := int64(42)
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Delete me", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	response := performWebhookRequest(
		t,
		router,
		http.MethodDelete,
		fmt.Sprintf("/api/queue/%d", item.ID),
		nil,
		map[string]string{"Authorization": "Bearer session-token"},
	)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, response.Code, response.Body.String())
	}

	items, err := store.ListQueueItems(t.Context(), "", 10)
	if err != nil {
		t.Fatalf("list queue items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected queue to be empty after deletion, got %d items", len(items))
	}
}

func TestDeleteQueueItemRejectsProcessingItems(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")

	if err := store.CreateSession(t.Context(), Session{
		Token:        "session-token",
		Username:     "tester",
		CreatedAtMS:  nowMS(),
		ExpiresAtMS:  nowMS() + 60_000,
		LastSeenAtMS: nowMS(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	documentID := int64(42)
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Delete me", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}
	if _, err := store.ClaimQueueItemByID(t.Context(), item.ID); err != nil {
		t.Fatalf("claim queue item: %v", err)
	}

	response := performWebhookRequest(
		t,
		router,
		http.MethodDelete,
		fmt.Sprintf("/api/queue/%d", item.ID),
		nil,
		map[string]string{"Authorization": "Bearer session-token"},
	)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, response.Code, response.Body.String())
	}
}

func TestDocumentProcessHistoryEndpointReturnsDocumentRuns(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")

	if err := store.CreateSession(t.Context(), Session{
		Token:        "session-token",
		Username:     "tester",
		CreatedAtMS:  nowMS(),
		ExpiresAtMS:  nowMS() + 60_000,
		LastSeenAtMS: nowMS(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	documentID := int64(42)
	otherDocumentID := int64(77)
	firstItem, err := store.CreateQueueItem(t.Context(), &documentID, "Invoice April", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create first queue item: %v", err)
	}
	secondItem, err := store.CreateQueueItem(t.Context(), &documentID, "Invoice April", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create second queue item: %v", err)
	}
	if _, err := store.CreateQueueItem(t.Context(), &otherDocumentID, "Other document", "paperless", "webhook", `{}`); err != nil {
		t.Fatalf("create unrelated queue item: %v", err)
	}

	response := performWebhookRequest(
		t,
		router,
		http.MethodGet,
		"/api/documents/42/processes?limit=5",
		nil,
		map[string]string{"Authorization": "Bearer session-token"},
	)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}

	var payload struct {
		DocumentID    int64       `json:"document_id"`
		DocumentTitle string      `json:"document_title"`
		Items         []QueueItem `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if payload.DocumentID != documentID {
		t.Fatalf("expected document id %d, got %d", documentID, payload.DocumentID)
	}
	if payload.DocumentTitle != "Invoice April" {
		t.Fatalf("expected document title Invoice April, got %q", payload.DocumentTitle)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 document items, got %d", len(payload.Items))
	}
	if payload.Items[0].ID != secondItem.ID || payload.Items[1].ID != firstItem.ID {
		t.Fatalf("expected newest-first ordering, got ids [%d, %d]", payload.Items[0].ID, payload.Items[1].ID)
	}
}

func TestApplyQueueItemAppliesSuggestionsAndCompletedTag(t *testing.T) {
	patched := false
	paperlessServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags/" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"process"},{"id":2,"name":"invoice"},{"id":3,"name":"completed"}],"next":null}`))
		case r.URL.Path == "/api/documents/42/" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Invoice April","tags":[1]}`))
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
			_, _ = w.Write([]byte(`{"id":42,"title":"Invoice April","tags":[2,3]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer paperlessServer.Close()

	router, store := newWebhookTestRouter(t, "secret")
	if err := store.CreateSession(t.Context(), Session{
		Token:        "session-token",
		Username:     "tester",
		CreatedAtMS:  nowMS(),
		ExpiresAtMS:  nowMS() + 60_000,
		LastSeenAtMS: nowMS(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SaveConfig(t.Context(), BackendConfig{
		Engine:    EngineConfig{ProcessingMode: ProcessingModeManual, ProcessingIntervalSeconds: 30},
		Process:   ProcessConfig{ProcessTriggerTag: "process", ProcessCompletedTag: "completed"},
		Paperless: PaperlessConfig{PaperlessURL: paperlessServer.URL, PaperlessToken: "token"},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	documentID := int64(42)
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Invoice April", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}
	startedAt := nowMS()
	completedItem, err := store.MarkQueueItemCompleted(
		t.Context(),
		item.ID,
		"Completed tag suggestion.",
		`{"document":{"id":42,"title":"Invoice April"},"tags":{"status":"completed","payload":{"tag_ids":[2],"tag_names":["invoice"],"suggested_new_tags":[],"confidence":"high","reasoning":"matched"}}}`,
		"llama3.2",
		"llava",
		&startedAt,
	)
	if err != nil {
		t.Fatalf("mark queue item completed: %v", err)
	}
	if completedItem.Status != queueItemStatusCompleted {
		t.Fatalf("expected completed item, got %q", completedItem.Status)
	}

	response := performWebhookRequest(
		t,
		router,
		http.MethodPost,
		fmt.Sprintf("/api/queue/%d/apply", item.ID),
		nil,
		map[string]string{"Authorization": "Bearer session-token"},
	)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if !patched {
		t.Fatal("expected document patch request")
	}

	var responseItem QueueItem
	if err := json.Unmarshal(response.Body.Bytes(), &responseItem); err != nil {
		t.Fatalf("decode apply response: %v", err)
	}
	if responseItem.ApplyStatus != "applied" {
		t.Fatalf("expected applied status, got %q", responseItem.ApplyStatus)
	}
	if responseItem.AppliedSummary == "" {
		t.Fatal("expected applied summary in response")
	}
	if responseItem.AppliedAtMS == nil {
		t.Fatal("expected applied_at_ms in response")
	}

	reloaded, err := store.GetQueueItem(t.Context(), item.ID)
	if err != nil {
		t.Fatalf("reload queue item: %v", err)
	}
	if reloaded.ApplyStatus != "applied" {
		t.Fatalf("expected applied status persisted, got %q", reloaded.ApplyStatus)
	}
}

func TestApplyQueueItemRejectsAlreadyAppliedItems(t *testing.T) {
	router, store := newWebhookTestRouter(t, "secret")

	if err := store.CreateSession(t.Context(), Session{
		Token:        "session-token",
		Username:     "tester",
		CreatedAtMS:  nowMS(),
		ExpiresAtMS:  nowMS() + 60_000,
		LastSeenAtMS: nowMS(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	documentID := int64(42)
	item, err := store.CreateQueueItem(t.Context(), &documentID, "Invoice April", "paperless", "webhook", `{}`)
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}
	startedAt := nowMS()
	completedItem, err := store.MarkQueueItemCompleted(
		t.Context(),
		item.ID,
		"Completed tag suggestion.",
		`{"tags":{"status":"completed","payload":{"tag_ids":[2]}}}`,
		"llama3.2",
		"llava",
		&startedAt,
	)
	if err != nil {
		t.Fatalf("mark queue item completed: %v", err)
	}
	if _, err := store.MarkQueueItemApplied(t.Context(), completedItem.ID, "Applied tags."); err != nil {
		t.Fatalf("mark queue item applied: %v", err)
	}

	response := performWebhookRequest(
		t,
		router,
		http.MethodPost,
		fmt.Sprintf("/api/queue/%d/apply", item.ID),
		nil,
		map[string]string{"Authorization": "Bearer session-token"},
	)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, response.Code, response.Body.String())
	}
}

func newWebhookTestRouter(t *testing.T, sharedSecret string) (http.Handler, *Store) {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "paperless-aiext-test.db")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_ = os.Remove(databasePath)
	})

	logger := zerolog.New(io.Discard)
	server := NewServer(store, NewProcessor(store, logger), logger, sharedSecret)
	return server.Router(), store
}

func performWebhookRequest(t *testing.T, router http.Handler, method string, target string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, target, body)
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
