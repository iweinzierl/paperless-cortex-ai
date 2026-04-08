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
