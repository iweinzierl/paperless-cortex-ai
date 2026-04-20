package ollama

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestRunSendsDeterministicOptions(t *testing.T) {
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
			Options map[string]any `json:"options"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if payload.Options["temperature"] != float64(0) {
			t.Fatalf("expected temperature 0, got %+v", payload.Options["temperature"])
		}
		if payload.Options["top_k"] != float64(1) {
			t.Fatalf("expected top_k 1, got %+v", payload.Options["top_k"])
		}
		if payload.Options["top_p"] != float64(0) {
			t.Fatalf("expected top_p 0, got %+v", payload.Options["top_p"])
		}
		if payload.Options["seed"] != float64(1) {
			t.Fatalf("expected seed 1, got %+v", payload.Options["seed"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ok"}}`))
	}))
	defer testServer.Close()

	result, err := Run(t.Context(), testServer.URL, "llama3.2", Message{Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("run chat request: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected ok response, got %q", result)
	}
}

func TestRunWithFormatSendsStructuredOutputSchema(t *testing.T) {
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
			Format map[string]any `json:"format"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if payload.Format["type"] != "object" {
			t.Fatalf("expected object schema, got %+v", payload.Format)
		}

		properties, ok := payload.Format["properties"].(map[string]any)
		if !ok {
			t.Fatalf("expected schema properties, got %+v", payload.Format["properties"])
		}
		if _, ok := properties["title"]; !ok {
			t.Fatalf("expected title property in schema, got %+v", properties)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"title\":\"ok\",\"confidence\":\"high\",\"reasoning\":\"test\"}"}}`))
	}))
	defer testServer.Close()

	result, err := RunWithFormat(t.Context(), testServer.URL, "llama3.2", []Message{{Role: "user", Content: "hello"}}, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":      map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "string"},
			"reasoning":  map[string]any{"type": "string"},
		},
	})
	if err != nil {
		t.Fatalf("run structured chat request: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty response")
	}
}

func TestChatTimeoutUsesDefaultWhenUnsetOrInvalid(t *testing.T) {
	t.Setenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS", "")
	if got := chatTimeout(); got != defaultChatTimeout {
		t.Fatalf("expected default timeout when unset, got %s", got)
	}

	t.Setenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS", "invalid")
	if got := chatTimeout(); got != defaultChatTimeout {
		t.Fatalf("expected default timeout when invalid, got %s", got)
	}

	t.Setenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS", "0")
	if got := chatTimeout(); got != defaultChatTimeout {
		t.Fatalf("expected default timeout when non-positive, got %s", got)
	}
}

func TestChatTimeoutUsesConfiguredSeconds(t *testing.T) {
	t.Setenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS", "600")
	if got := chatTimeout(); got != 10*time.Minute {
		t.Fatalf("expected configured timeout, got %s", got)
	}
}

func TestChatTimeoutReadsEnvironmentAtCallTime(t *testing.T) {
	original := os.Getenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS")
	t.Cleanup(func() {
		if original == "" {
			_ = os.Unsetenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS")
			return
		}
		_ = os.Setenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS", original)
	})

	_ = os.Setenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS", "180")
	if got := chatTimeout(); got != 3*time.Minute {
		t.Fatalf("expected 3 minute timeout, got %s", got)
	}
}

func TestEmbedReturnsVector(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if payload["model"] != "nomic-embed-text" {
			t.Fatalf("unexpected model payload: %+v", payload["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":[0.1,0.2,0.3]}`))
	}))
	defer testServer.Close()

	vector, err := Embed(t.Context(), testServer.URL, "nomic-embed-text", "hello world")
	if err != nil {
		t.Fatalf("embed request failed: %v", err)
	}
	if len(vector) != 3 {
		t.Fatalf("expected vector length 3, got %d", len(vector))
	}
}

func TestEmbedRejectsEmptyVectors(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":[]}`))
	}))
	defer testServer.Close()

	if _, err := Embed(t.Context(), testServer.URL, "nomic-embed-text", "hello"); err == nil {
		t.Fatal("expected error for empty embedding vector")
	}
}

func TestEmbeddingTimeoutUsesConfiguredSeconds(t *testing.T) {
	t.Setenv("PAPERLESS_AIEXT_OLLAMA_EMBED_TIMEOUT_SECONDS", "120")
	if got := embeddingTimeout(); got != 2*time.Minute {
		t.Fatalf("expected configured embedding timeout, got %s", got)
	}
}
