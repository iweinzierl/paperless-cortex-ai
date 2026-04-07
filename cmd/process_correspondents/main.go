package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"paperless-ai-ext/internal/ocr"
)

const correspondentPromptTemplate = `You are classifying a document for paperless-ngx.
Choose the best matching existing correspondent if one clearly matches the document.
If none match, suggest a concise new correspondent name.

Return strict JSON with exactly these keys:
{
  "correspondent_id": number|null,
  "correspondent_name": string|null,
  "suggested_new_correspondent": string|null,
  "confidence": "high"|"medium"|"low",
  "reasoning": string
}

Rules:
- Prefer an existing correspondent whenever the document clearly belongs to one.
- If you choose an existing correspondent, use its ID exactly as listed and set suggested_new_correspondent to null.
- If no existing correspondent fits, set correspondent_id and correspondent_name to null.
- Return JSON only. No markdown, no code fences, no extra text.

Existing correspondents:
%s

Document source: %s

Document text:
%s`

type correspondent struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type correspondentSuggestion struct {
	CorrespondentID           *int    `json:"correspondent_id"`
	CorrespondentName         *string `json:"correspondent_name"`
	SuggestedNewCorrespondent *string `json:"suggested_new_correspondent"`
	Confidence                string  `json:"confidence"`
	Reasoning                 string  `json:"reasoning"`
}

func main() {
	var documentPath string
	var model string
	var ollamaURL string
	var paperlessURL string
	var paperlessToken string

	flag.StringVar(&documentPath, "document", "", "path to the document to process")
	flag.StringVar(&model, "model", "", "Ollama model to use")
	flag.StringVar(&ollamaURL, "ollama-url", "http://localhost:11434", "base URL of the Ollama server")
	flag.StringVar(&paperlessURL, "paperless-url", os.Getenv("PAPERLESS_URL"), "base URL of the paperless-ngx instance")
	flag.StringVar(&paperlessToken, "paperless-token", os.Getenv("PAPERLESS_TOKEN"), "API token for the paperless-ngx instance")
	flag.Parse()

	if documentPath == "" {
		exitWithError(errors.New("-document is required"))
	}

	if model == "" {
		exitWithError(errors.New("-model is required"))
	}

	if paperlessURL == "" {
		exitWithError(errors.New("-paperless-url is required or PAPERLESS_URL must be set"))
	}

	if paperlessToken == "" {
		exitWithError(errors.New("-paperless-token is required or PAPERLESS_TOKEN must be set"))
	}

	ctx := context.Background()

	correspondents, err := fetchCorrespondents(ctx, paperlessURL, paperlessToken)
	if err != nil {
		exitWithError(err)
	}

	documentText, err := screenDocument(ctx, documentPath, model, ollamaURL)
	if err != nil {
		exitWithError(err)
	}

	prompt := buildCorrespondentPrompt(filepath.Base(documentPath), documentText, correspondents)
	response, err := ocr.Run(ctx, ollamaURL, model, ocr.Message{
		Role:    "user",
		Content: prompt,
	})
	if err != nil {
		exitWithError(err)
	}

	suggestion, err := parseSuggestion(response)
	if err != nil {
		exitWithError(err)
	}

	normalized, err := normalizeSuggestion(suggestion, correspondents)
	if err != nil {
		exitWithError(err)
	}

	output, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		exitWithError(fmt.Errorf("marshal suggestion: %w", err))
	}

	fmt.Println(string(output))
}

func screenDocument(ctx context.Context, documentPath string, model string, ollamaURL string) (string, error) {
	message, err := ocr.BuildScreeningMessage(documentPath, ocr.DefaultPrompt)
	if err != nil {
		return "", err
	}

	if len(message.Images) == 0 {
		const marker = "Document text:\n"
		index := strings.Index(message.Content, marker)
		if index < 0 {
			return "", errors.New("screening message did not contain document text")
		}

		text := strings.TrimSpace(message.Content[index+len(marker):])
		if text == "" {
			return "", errors.New("document text is empty")
		}

		return text, nil
	}

	response, err := ocr.Run(ctx, ollamaURL, model, message)
	if err != nil {
		return "", err
	}

	text := strings.TrimSpace(response)
	if text == "" {
		return "", errors.New("OCR result is empty")
	}

	return text, nil
}

func fetchCorrespondents(parent context.Context, paperlessURL string, token string) ([]correspondent, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	apiBaseURL, err := buildCorrespondentsURL(paperlessURL)
	if err != nil {
		return nil, err
	}
	nextURL := apiBaseURL

	var correspondents []correspondent

	for nextURL != "" {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build paperless request: %w", err)
		}

		request.Header.Set("Authorization", "Token "+token)
		request.Header.Set("Accept", "application/json")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("fetch paperless correspondents: %w", err)
		}

		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read paperless response: %w", readErr)
		}

		if response.StatusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("paperless correspondents API returned %s for %s: %s", response.Status, nextURL, strings.TrimSpace(string(body)))
		}

		pageItems, pageNext, err := decodeCorrespondentPage(body)
		if err != nil {
			return nil, err
		}

		correspondents = append(correspondents, pageItems...)
		nextURL = resolveNextURL(apiBaseURL, pageNext)
	}

	sort.Slice(correspondents, func(left int, right int) bool {
		return strings.ToLower(correspondents[left].Name) < strings.ToLower(correspondents[right].Name)
	})

	return correspondents, nil
}

func buildCorrespondentsURL(rawBaseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return "", fmt.Errorf("parse paperless URL: %w", err)
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/api/correspondents/"
	case strings.HasSuffix(path, "/api/correspondents"):
		parsed.Path = path + "/"
	case strings.HasSuffix(path, "/api"):
		parsed.Path = path + "/correspondents/"
	default:
		parsed.Path = path + "/api/correspondents/"
	}

	query := parsed.Query()
	if query.Get("page_size") == "" {
		query.Set("page_size", "100")
	}
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func decodeCorrespondentPage(body []byte) ([]correspondent, string, error) {
	var paged struct {
		Results []correspondent `json:"results"`
		Next    *string         `json:"next"`
	}

	if err := json.Unmarshal(body, &paged); err == nil && (paged.Results != nil || paged.Next != nil) {
		next := ""
		if paged.Next != nil {
			next = *paged.Next
		}

		return paged.Results, next, nil
	}

	var flat []correspondent
	if err := json.Unmarshal(body, &flat); err == nil {
		return flat, "", nil
	}

	return nil, "", fmt.Errorf("decode paperless correspondents response: %s", strings.TrimSpace(string(body)))
}

func resolveNextURL(baseURL string, next string) string {
	if next == "" {
		return ""
	}

	parsed, err := url.Parse(next)
	if err != nil {
		return next
	}

	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return next
	}

	if parsed.IsAbs() {
		if strings.EqualFold(parsed.Host, base.Host) || parsed.Host == "" {
			parsed.Scheme = base.Scheme
			parsed.Host = base.Host
			if parsed.Path == "" {
				parsed.Path = base.Path
			}
		}

		return parsed.String()
	}

	return base.ResolveReference(parsed).String()
}

func buildCorrespondentPrompt(documentName string, documentText string, correspondents []correspondent) string {
	list := "- No existing correspondents available"
	if len(correspondents) > 0 {
		var builder strings.Builder
		for _, item := range correspondents {
			builder.WriteString("- ")
			builder.WriteString(strconv.Itoa(item.ID))
			builder.WriteString(": ")
			builder.WriteString(item.Name)
			builder.WriteByte('\n')
		}
		list = strings.TrimSpace(builder.String())
	}

	return fmt.Sprintf(correspondentPromptTemplate, list, documentName, documentText)
}

func parseSuggestion(raw string) (correspondentSuggestion, error) {
	jsonObject, err := extractJSONObject(raw)
	if err != nil {
		return correspondentSuggestion{}, err
	}

	var suggestion correspondentSuggestion
	if err := json.Unmarshal([]byte(jsonObject), &suggestion); err != nil {
		return correspondentSuggestion{}, fmt.Errorf("decode LLM suggestion: %w", err)
	}

	if suggestion.CorrespondentName != nil {
		trimmed := strings.TrimSpace(*suggestion.CorrespondentName)
		if trimmed == "" {
			suggestion.CorrespondentName = nil
		} else {
			suggestion.CorrespondentName = stringPointer(trimmed)
		}
	}

	if suggestion.SuggestedNewCorrespondent != nil {
		trimmed := strings.TrimSpace(*suggestion.SuggestedNewCorrespondent)
		if trimmed == "" {
			suggestion.SuggestedNewCorrespondent = nil
		} else {
			suggestion.SuggestedNewCorrespondent = stringPointer(trimmed)
		}
	}

	suggestion.Confidence = strings.ToLower(strings.TrimSpace(suggestion.Confidence))
	if suggestion.Confidence == "" {
		suggestion.Confidence = "low"
	}

	if suggestion.CorrespondentID == nil && suggestion.CorrespondentName == nil && suggestion.SuggestedNewCorrespondent == nil {
		return correspondentSuggestion{}, errors.New("LLM did not select or suggest any correspondent")
	}

	return suggestion, nil
}

func extractJSONObject(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	start := strings.Index(text, "{")
	if start < 0 {
		return "", fmt.Errorf("LLM response did not contain JSON: %s", text)
	}

	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(text); index++ {
		char := text[index]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}

		if char == '"' {
			inString = true
			continue
		}

		if char == '{' {
			depth++
		}

		if char == '}' {
			depth--
			if depth == 0 {
				return text[start : index+1], nil
			}
		}
	}

	return "", errors.New("LLM response contained incomplete JSON")
}

func normalizeSuggestion(suggestion correspondentSuggestion, correspondents []correspondent) (correspondentSuggestion, error) {
	byID := make(map[int]correspondent, len(correspondents))
	byName := make(map[string]correspondent, len(correspondents))
	for _, item := range correspondents {
		byID[item.ID] = item
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}

	if suggestion.CorrespondentID != nil {
		item, ok := byID[*suggestion.CorrespondentID]
		if !ok {
			return correspondentSuggestion{}, fmt.Errorf("LLM selected unknown correspondent id %d", *suggestion.CorrespondentID)
		}

		suggestion.CorrespondentName = stringPointer(item.Name)
		suggestion.SuggestedNewCorrespondent = nil
		return suggestion, nil
	}

	if suggestion.CorrespondentName != nil {
		item, ok := byName[strings.ToLower(strings.TrimSpace(*suggestion.CorrespondentName))]
		if ok {
			suggestion.CorrespondentID = intPointer(item.ID)
			suggestion.CorrespondentName = stringPointer(item.Name)
			suggestion.SuggestedNewCorrespondent = nil
			return suggestion, nil
		}
	}

	if suggestion.SuggestedNewCorrespondent != nil {
		suggestion.CorrespondentID = nil
		suggestion.CorrespondentName = nil
		return suggestion, nil
	}

	return correspondentSuggestion{}, errors.New("LLM response could not be mapped to an existing or new correspondent")
}

func intPointer(value int) *int {
	return &value
}

func stringPointer(value string) *string {
	return &value
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func init() {
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
}
