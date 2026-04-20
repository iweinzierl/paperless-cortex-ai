package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultChatTimeout = 10 * time.Minute
const defaultEmbeddingTimeout = 2 * time.Minute

type Message struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Stream   bool           `json:"stream"`
	Messages []Message      `json:"messages"`
	Format   any            `json:"format,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

type chatResponse struct {
	Message Message `json:"message"`
	Error   string  `json:"error"`
}

type embeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Input  string `json:"input"`
}

type embeddingResponse struct {
	Embedding []float64 `json:"embedding"`
	Error     string    `json:"error"`
}

func defaultChatOptions() map[string]any {
	return map[string]any{
		"temperature": 0,
		"top_k":       1,
		"top_p":       0,
		"seed":        1,
	}
}

func Run(parent context.Context, ollamaURL string, model string, message Message) (string, error) {
	return RunWithFormat(parent, ollamaURL, model, []Message{message}, nil)
}

func RunWithFormat(parent context.Context, ollamaURL string, model string, messages []Message, format any) (string, error) {
	ctx, cancel := context.WithTimeout(parent, chatTimeout())
	defer cancel()

	requestBody := chatRequest{
		Model:    model,
		Stream:   false,
		Messages: messages,
		Format:   format,
		Options:  defaultChatOptions(),
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshal ollama request: %w", err)
	}

	endpoint := strings.TrimRight(ollamaURL, "/") + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call ollama chat API: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ollama response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("ollama chat API returned %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}

	if parsed.Error != "" {
		return "", errors.New(parsed.Error)
	}

	if strings.TrimSpace(parsed.Message.Content) == "" {
		return "", errors.New("ollama response did not include message content")
	}

	return parsed.Message.Content, nil
}

func chatTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS"))
	if raw == "" {
		return defaultChatTimeout
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultChatTimeout
	}

	return time.Duration(seconds) * time.Second
}

func Embed(parent context.Context, ollamaURL string, model string, input string) ([]float64, error) {
	trimmedInput := strings.TrimSpace(input)
	if trimmedInput == "" {
		return nil, errors.New("embedding input is empty")
	}

	ctx, cancel := context.WithTimeout(parent, embeddingTimeout())
	defer cancel()

	requestBody := embeddingRequest{
		Model:  model,
		Prompt: trimmedInput,
		Input:  trimmedInput,
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama embedding request: %w", err)
	}

	endpoint := strings.TrimRight(ollamaURL, "/") + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build ollama embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call ollama embeddings API: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ollama embedding response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("ollama embeddings API returned %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode ollama embedding response: %w", err)
	}

	if parsed.Error != "" {
		return nil, errors.New(parsed.Error)
	}
	if len(parsed.Embedding) == 0 {
		return nil, errors.New("ollama embedding response did not include vector values")
	}

	return parsed.Embedding, nil
}

func embeddingTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("PAPERLESS_AIEXT_OLLAMA_EMBED_TIMEOUT_SECONDS"))
	if raw == "" {
		return defaultEmbeddingTimeout
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultEmbeddingTimeout
	}

	return time.Duration(seconds) * time.Second
}
