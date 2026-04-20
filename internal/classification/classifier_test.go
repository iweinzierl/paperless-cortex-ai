package classification

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paperless-ai-ext/internal/paperless"
)

func TestSuggestDocumentTypeRetriesWhenModelReturnsProse(t *testing.T) {
	requestCount := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		var payload struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}

		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"This is clearly an invoice from the supplier and should use the existing invoice type."}}`))
			return
		}

		if len(payload.Messages) != 3 {
			t.Fatalf("expected retry request with 3 messages, got %d", len(payload.Messages))
		}
		if payload.Messages[1]["role"] != "assistant" {
			t.Fatalf("expected prior assistant response in retry payload, got %+v", payload.Messages[1])
		}
		if !strings.Contains(payload.Messages[2]["content"].(string), "did not satisfy the required JSON contract") {
			t.Fatalf("expected repair instruction in retry payload, got %+v", payload.Messages[2])
		}

		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"document_type_id\":7,\"document_type_name\":\"Invoice\",\"suggested_new_document_type\":null,\"confidence\":\"medium\",\"reasoning\":\"Das Dokument ist klar als Rechnung erkennbar.\"}"}}`))
	}))
	defer testServer.Close()

	suggestion, err := SuggestDocumentType(
		t.Context(),
		testServer.URL,
		"llama3.2",
		"invoice.pdf",
		"Rechnung Nummer 123 vom Lieferanten",
		[]paperless.DocumentType{{ID: 7, Name: "Invoice"}},
		nil,
	)
	if err != nil {
		t.Fatalf("suggest document type: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 ollama requests, got %d", requestCount)
	}
	if suggestion.DocumentTypeID == nil || *suggestion.DocumentTypeID != 7 {
		t.Fatalf("expected document type id 7, got %+v", suggestion.DocumentTypeID)
	}
	if suggestion.DocumentTypeName == nil || *suggestion.DocumentTypeName != "Invoice" {
		t.Fatalf("expected Invoice, got %+v", suggestion.DocumentTypeName)
	}
}