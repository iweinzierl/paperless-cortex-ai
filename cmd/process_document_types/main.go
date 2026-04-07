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

const documentTypePromptTemplate = `You are classifying a document for paperless-ngx.
Choose the best matching existing document type if one clearly matches the document.
If none match, suggest a concise new document type name.

Return strict JSON with exactly these keys:
{
  "document_type_id": number|null,
  "document_type_name": string|null,
  "suggested_new_document_type": string|null,
  "confidence": "high"|"medium"|"low",
  "reasoning": string
}

Rules:
- Prefer an existing document type whenever the document clearly belongs to one.
- If you choose an existing document type, use its ID exactly as listed and set suggested_new_document_type to null.
- If no existing document type fits, set document_type_id and document_type_name to null.
- Return JSON only. No markdown, no code fences, no extra text.

Existing document types:
%s

Document source: %s

Document text:
%s`

type documentType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type documentTypeSuggestion struct {
	DocumentTypeID           *int    `json:"document_type_id"`
	DocumentTypeName         *string `json:"document_type_name"`
	SuggestedNewDocumentType *string `json:"suggested_new_document_type"`
	Confidence               string  `json:"confidence"`
	Reasoning                string  `json:"reasoning"`
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

	documentTypes, err := fetchDocumentTypes(ctx, paperlessURL, paperlessToken)
	if err != nil {
		exitWithError(err)
	}

	documentText, err := screenDocument(ctx, documentPath, model, ollamaURL)
	if err != nil {
		exitWithError(err)
	}

	prompt := buildDocumentTypePrompt(filepath.Base(documentPath), documentText, documentTypes)
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

	normalized, err := normalizeSuggestion(suggestion, documentTypes)
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

func fetchDocumentTypes(parent context.Context, paperlessURL string, token string) ([]documentType, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	apiBaseURL, err := buildDocumentTypesURL(paperlessURL)
	if err != nil {
		return nil, err
	}
	nextURL := apiBaseURL

	var documentTypes []documentType

	for nextURL != "" {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build paperless request: %w", err)
		}

		request.Header.Set("Authorization", "Token "+token)
		request.Header.Set("Accept", "application/json")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("fetch paperless document types: %w", err)
		}

		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read paperless response: %w", readErr)
		}

		if response.StatusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("paperless document types API returned %s for %s: %s", response.Status, nextURL, strings.TrimSpace(string(body)))
		}

		pageItems, pageNext, err := decodeDocumentTypePage(body)
		if err != nil {
			return nil, err
		}

		documentTypes = append(documentTypes, pageItems...)
		nextURL = resolveNextURL(apiBaseURL, pageNext)
	}

	sort.Slice(documentTypes, func(left int, right int) bool {
		return strings.ToLower(documentTypes[left].Name) < strings.ToLower(documentTypes[right].Name)
	})

	return documentTypes, nil
}

func buildDocumentTypesURL(rawBaseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return "", fmt.Errorf("parse paperless URL: %w", err)
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/api/document_types/"
	case strings.HasSuffix(path, "/api/document_types"):
		parsed.Path = path + "/"
	case strings.HasSuffix(path, "/api"):
		parsed.Path = path + "/document_types/"
	default:
		parsed.Path = path + "/api/document_types/"
	}

	query := parsed.Query()
	if query.Get("page_size") == "" {
		query.Set("page_size", "100")
	}
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func decodeDocumentTypePage(body []byte) ([]documentType, string, error) {
	var paged struct {
		Results []documentType `json:"results"`
		Next    *string        `json:"next"`
	}

	if err := json.Unmarshal(body, &paged); err == nil && (paged.Results != nil || paged.Next != nil) {
		next := ""
		if paged.Next != nil {
			next = *paged.Next
		}

		return paged.Results, next, nil
	}

	var flat []documentType
	if err := json.Unmarshal(body, &flat); err == nil {
		return flat, "", nil
	}

	return nil, "", fmt.Errorf("decode paperless document types response: %s", strings.TrimSpace(string(body)))
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

func buildDocumentTypePrompt(documentName string, documentText string, documentTypes []documentType) string {
	list := "- No existing document types available"
	if len(documentTypes) > 0 {
		var builder strings.Builder
		for _, item := range documentTypes {
			builder.WriteString("- ")
			builder.WriteString(strconv.Itoa(item.ID))
			builder.WriteString(": ")
			builder.WriteString(item.Name)
			builder.WriteByte('\n')
		}
		list = strings.TrimSpace(builder.String())
	}

	return fmt.Sprintf(documentTypePromptTemplate, list, documentName, documentText)
}

func parseSuggestion(raw string) (documentTypeSuggestion, error) {
	jsonObject, err := extractJSONObject(raw)
	if err != nil {
		return documentTypeSuggestion{}, err
	}

	var suggestion documentTypeSuggestion
	if err := json.Unmarshal([]byte(jsonObject), &suggestion); err != nil {
		return documentTypeSuggestion{}, fmt.Errorf("decode LLM suggestion: %w", err)
	}

	if suggestion.DocumentTypeName != nil {
		trimmed := strings.TrimSpace(*suggestion.DocumentTypeName)
		if trimmed == "" {
			suggestion.DocumentTypeName = nil
		} else {
			suggestion.DocumentTypeName = stringPointer(trimmed)
		}
	}

	if suggestion.SuggestedNewDocumentType != nil {
		trimmed := strings.TrimSpace(*suggestion.SuggestedNewDocumentType)
		if trimmed == "" {
			suggestion.SuggestedNewDocumentType = nil
		} else {
			suggestion.SuggestedNewDocumentType = stringPointer(trimmed)
		}
	}

	suggestion.Confidence = strings.ToLower(strings.TrimSpace(suggestion.Confidence))
	if suggestion.Confidence == "" {
		suggestion.Confidence = "low"
	}

	if suggestion.DocumentTypeID == nil && suggestion.DocumentTypeName == nil && suggestion.SuggestedNewDocumentType == nil {
		return documentTypeSuggestion{}, errors.New("LLM did not select or suggest any document type")
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

func normalizeSuggestion(suggestion documentTypeSuggestion, documentTypes []documentType) (documentTypeSuggestion, error) {
	byID := make(map[int]documentType, len(documentTypes))
	byName := make(map[string]documentType, len(documentTypes))
	for _, item := range documentTypes {
		byID[item.ID] = item
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}

	if suggestion.DocumentTypeID != nil {
		item, ok := byID[*suggestion.DocumentTypeID]
		if !ok {
			return documentTypeSuggestion{}, fmt.Errorf("LLM selected unknown document type id %d", *suggestion.DocumentTypeID)
		}

		suggestion.DocumentTypeName = stringPointer(item.Name)
		suggestion.SuggestedNewDocumentType = nil
		return suggestion, nil
	}

	if suggestion.DocumentTypeName != nil {
		item, ok := byName[strings.ToLower(strings.TrimSpace(*suggestion.DocumentTypeName))]
		if ok {
			suggestion.DocumentTypeID = intPointer(item.ID)
			suggestion.DocumentTypeName = stringPointer(item.Name)
			suggestion.SuggestedNewDocumentType = nil
			return suggestion, nil
		}
	}

	if suggestion.SuggestedNewDocumentType != nil {
		suggestion.DocumentTypeID = nil
		suggestion.DocumentTypeName = nil
		return suggestion, nil
	}

	return documentTypeSuggestion{}, errors.New("LLM response could not be mapped to an existing or new document type")
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
