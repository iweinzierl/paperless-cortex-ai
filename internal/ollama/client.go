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

const defaultChatTimeout = 2 * time.Minute

type Message struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Stream   bool           `json:"stream"`
	Messages []Message      `json:"messages"`
	Options  map[string]any `json:"options,omitempty"`
}

type chatResponse struct {
	Message Message `json:"message"`
	Error   string  `json:"error"`
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
	ctx, cancel := context.WithTimeout(parent, chatTimeout())
	defer cancel()

	requestBody := chatRequest{
		Model:    model,
		Stream:   false,
		Messages: []Message{message},
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
