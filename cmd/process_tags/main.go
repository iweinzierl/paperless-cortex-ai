package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
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

const tagPromptTemplate = `You are classifying a document for paperless-ngx.
Choose zero or more existing tags that clearly match the document.
If important tags are missing from the existing list, suggest concise new tags.

Return strict JSON with exactly these keys:
{
  "tag_ids": [number],
  "tag_names": [string],
  "suggested_new_tags": [string],
  "confidence": "high"|"medium"|"low",
  "reasoning": string
}

Rules:
- Select only tags that are genuinely relevant to the document.
- Prefer existing tags whenever they fit.
- Use existing tag IDs exactly as listed.
- Do not duplicate tags.
- If no tags apply, return empty arrays.
- Return JSON only. No markdown, no code fences, no extra text.

Existing tags:
%s

Document text:
%s`

type tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type tagSuggestion struct {
	TagIDs           []int    `json:"tag_ids"`
	TagNames         []string `json:"tag_names"`
	SuggestedNewTags []string `json:"suggested_new_tags"`
	Confidence       string   `json:"confidence"`
	Reasoning        string   `json:"reasoning"`
}

func main() {
	var documentPath string
	var model string
	var visionModel string
	var ollamaURL string
	var paperlessURL string
	var paperlessToken string
	var visionMaxPages int
	var forceVision bool

	flag.StringVar(&documentPath, "document", "", "path to the document to process")
	flag.StringVar(&model, "model", "", "Ollama model to use")
	flag.StringVar(&visionModel, "vision-model", "", "Vision model to use for OCR fallback; defaults to -model")
	flag.StringVar(&ollamaURL, "ollama-url", "http://localhost:11434", "base URL of the Ollama server")
	flag.StringVar(&paperlessURL, "paperless-url", os.Getenv("PAPERLESS_URL"), "base URL of the paperless-ngx instance")
	flag.StringVar(&paperlessToken, "paperless-token", os.Getenv("PAPERLESS_TOKEN"), "API token for the paperless-ngx instance")
	flag.IntVar(&visionMaxPages, "vision-max-pages", 3, "maximum number of pages to render when falling back to vision OCR for PDFs")
	flag.BoolVar(&forceVision, "force-vision", false, "skip simple OCR and force document screening with the vision model")
	flag.Parse()

	if documentPath == "" {
		exitWithError(errors.New("-document is required"))
	}

	if model == "" {
		exitWithError(errors.New("-model is required"))
	}

	if visionModel == "" {
		visionModel = model
	}

	if paperlessURL == "" {
		exitWithError(errors.New("-paperless-url is required or PAPERLESS_URL must be set"))
	}

	if paperlessToken == "" {
		exitWithError(errors.New("-paperless-token is required or PAPERLESS_TOKEN must be set"))
	}

	ctx := context.Background()

	tags, err := fetchTags(ctx, paperlessURL, paperlessToken)
	if err != nil {
		exitWithError(err)
	}

	documentText, err := screenDocument(ctx, documentPath, model, visionModel, ollamaURL, visionMaxPages, forceVision)
	if err != nil {
		exitWithError(err)
	}

	prompt := buildTagPrompt(filepath.Base(documentPath), documentText, tags)
	log.Printf("Generated prompt for document %q:\n%s", documentPath, prompt)
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

	normalized, err := normalizeSuggestion(suggestion, tags)
	if err != nil {
		exitWithError(err)
	}

	output, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		exitWithError(fmt.Errorf("marshal suggestion: %w", err))
	}

	fmt.Println(string(output))
}

func screenDocument(ctx context.Context, documentPath string, model string, visionModel string, ollamaURL string, visionMaxPages int, forceVision bool) (string, error) {
	if forceVision {
		return screenDocumentWithVisionFallback(ctx, documentPath, visionModel, ollamaURL, visionMaxPages, errors.New("vision OCR was forced"))
	}

	message, err := ocr.BuildScreeningMessage(documentPath, ocr.DefaultPrompt)
	if err != nil {
		return screenDocumentWithVisionFallback(ctx, documentPath, visionModel, ollamaURL, visionMaxPages, fmt.Errorf("simple OCR failed: %w", err))
	}

	if len(message.Images) == 0 {
		const marker = "Document text:\n"
		index := strings.Index(message.Content, marker)
		if index < 0 {
			return screenDocumentWithVisionFallback(ctx, documentPath, visionModel, ollamaURL, visionMaxPages, errors.New("screening message did not contain document text"))
		}

		text := strings.TrimSpace(message.Content[index+len(marker):])
		if text == "" {
			return screenDocumentWithVisionFallback(ctx, documentPath, visionModel, ollamaURL, visionMaxPages, errors.New("document text is empty"))
		}

		return text, nil
	}

	response, err := ocr.Run(ctx, ollamaURL, model, message)
	if err != nil {
		return screenDocumentWithVisionFallback(ctx, documentPath, visionModel, ollamaURL, visionMaxPages, fmt.Errorf("simple OCR model call failed: %w", err))
	}

	text := strings.TrimSpace(response)
	if text == "" {
		return screenDocumentWithVisionFallback(ctx, documentPath, visionModel, ollamaURL, visionMaxPages, errors.New("OCR result is empty"))
	}

	return text, nil
}

func screenDocumentWithVisionFallback(ctx context.Context, documentPath string, visionModel string, ollamaURL string, visionMaxPages int, rootCause error) (string, error) {
	visionMessage, err := ocr.BuildVisionScreeningMessage(documentPath, ocr.DefaultPrompt, visionMaxPages)
	if err != nil {
		return "", fmt.Errorf("%v; vision fallback failed: %w", rootCause, err)
	}

	response, err := ocr.Run(ctx, ollamaURL, visionModel, visionMessage)
	if err != nil {
		return "", fmt.Errorf("%v; vision fallback failed: %w", rootCause, err)
	}

	text := strings.TrimSpace(response)
	if text == "" {
		return "", fmt.Errorf("%v; vision fallback returned empty text", rootCause)
	}

	return text, nil
}

func fetchTags(parent context.Context, paperlessURL string, token string) ([]tag, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	apiBaseURL, err := buildTagsURL(paperlessURL)
	if err != nil {
		return nil, err
	}
	nextURL := apiBaseURL

	var tags []tag

	for nextURL != "" {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build paperless request: %w", err)
		}

		request.Header.Set("Authorization", "Token "+token)
		request.Header.Set("Accept", "application/json")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("fetch paperless tags: %w", err)
		}

		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read paperless response: %w", readErr)
		}

		if response.StatusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("paperless tags API returned %s for %s: %s", response.Status, nextURL, strings.TrimSpace(string(body)))
		}

		pageItems, pageNext, err := decodeTagPage(body)
		if err != nil {
			return nil, err
		}

		tags = append(tags, pageItems...)
		nextURL = resolveNextURL(apiBaseURL, pageNext)
	}

	sort.Slice(tags, func(left int, right int) bool {
		return strings.ToLower(tags[left].Name) < strings.ToLower(tags[right].Name)
	})

	return tags, nil
}

func buildTagsURL(rawBaseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return "", fmt.Errorf("parse paperless URL: %w", err)
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/api/tags/"
	case strings.HasSuffix(path, "/api/tags"):
		parsed.Path = path + "/"
	case strings.HasSuffix(path, "/api"):
		parsed.Path = path + "/tags/"
	default:
		parsed.Path = path + "/api/tags/"
	}

	query := parsed.Query()
	if query.Get("page_size") == "" {
		query.Set("page_size", "100")
	}
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func decodeTagPage(body []byte) ([]tag, string, error) {
	var paged struct {
		Results []tag   `json:"results"`
		Next    *string `json:"next"`
	}

	if err := json.Unmarshal(body, &paged); err == nil && (paged.Results != nil || paged.Next != nil) {
		next := ""
		if paged.Next != nil {
			next = *paged.Next
		}

		return paged.Results, next, nil
	}

	var flat []tag
	if err := json.Unmarshal(body, &flat); err == nil {
		return flat, "", nil
	}

	return nil, "", fmt.Errorf("decode paperless tags response: %s", strings.TrimSpace(string(body)))
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

func buildTagPrompt(documentName string, documentText string, tags []tag) string {
	list := "- No existing tags available"
	if len(tags) > 0 {
		var builder strings.Builder
		for _, item := range tags {
			builder.WriteString("- ")
			builder.WriteString(strconv.Itoa(item.ID))
			builder.WriteString(": ")
			builder.WriteString(item.Name)
			builder.WriteByte('\n')
		}
		list = strings.TrimSpace(builder.String())
	}

	return fmt.Sprintf(tagPromptTemplate, list, documentName, documentText)
}

func parseSuggestion(raw string) (tagSuggestion, error) {
	jsonObject, err := extractJSONObject(raw)
	if err != nil {
		return tagSuggestion{}, err
	}

	var suggestion tagSuggestion
	if err := json.Unmarshal([]byte(jsonObject), &suggestion); err != nil {
		return tagSuggestion{}, fmt.Errorf("decode LLM suggestion: %w", err)
	}

	suggestion.Confidence = strings.ToLower(strings.TrimSpace(suggestion.Confidence))
	if suggestion.Confidence == "" {
		suggestion.Confidence = "low"
	}

	suggestion.TagNames = normalizeStringList(suggestion.TagNames)
	suggestion.SuggestedNewTags = normalizeStringList(suggestion.SuggestedNewTags)

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

func normalizeSuggestion(suggestion tagSuggestion, tags []tag) (tagSuggestion, error) {
	byID := make(map[int]tag, len(tags))
	byName := make(map[string]tag, len(tags))
	for _, item := range tags {
		byID[item.ID] = item
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}

	selectedByID := make(map[int]tag)
	for _, tagID := range suggestion.TagIDs {
		item, ok := byID[tagID]
		if !ok {
			return tagSuggestion{}, fmt.Errorf("LLM selected unknown tag id %d", tagID)
		}
		selectedByID[item.ID] = item
	}

	for _, tagName := range suggestion.TagNames {
		item, ok := byName[strings.ToLower(tagName)]
		if ok {
			selectedByID[item.ID] = item
		}
	}

	selectedTags := make([]tag, 0, len(selectedByID))
	for _, item := range selectedByID {
		selectedTags = append(selectedTags, item)
	}

	sort.Slice(selectedTags, func(left int, right int) bool {
		return strings.ToLower(selectedTags[left].Name) < strings.ToLower(selectedTags[right].Name)
	})

	normalized := tagSuggestion{
		TagIDs:           make([]int, 0, len(selectedTags)),
		TagNames:         make([]string, 0, len(selectedTags)),
		SuggestedNewTags: suggestion.SuggestedNewTags,
		Confidence:       suggestion.Confidence,
		Reasoning:        suggestion.Reasoning,
	}

	for _, item := range selectedTags {
		normalized.TagIDs = append(normalized.TagIDs, item.ID)
		normalized.TagNames = append(normalized.TagNames, item.Name)
	}

	return normalized, nil
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	return normalized
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func init() {
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
}
