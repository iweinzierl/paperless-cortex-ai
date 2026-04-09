package ocr

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
