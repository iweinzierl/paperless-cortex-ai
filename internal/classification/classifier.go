package classification

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"paperless-ai-ext/internal/ollama"
	"paperless-ai-ext/internal/paperless"
)

var correspondentSuggestionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"correspondent_id":            map[string]any{"type": []string{"integer", "null"}},
		"correspondent_name":          map[string]any{"type": []string{"string", "null"}},
		"suggested_new_correspondent": map[string]any{"type": []string{"string", "null"}},
		"confidence":                  map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
		"reasoning":                   map[string]any{"type": "string"},
	},
	"required":             []string{"correspondent_id", "correspondent_name", "suggested_new_correspondent", "confidence", "reasoning"},
	"additionalProperties": false,
}

var documentTypeSuggestionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"document_type_id":            map[string]any{"type": []string{"integer", "null"}},
		"document_type_name":          map[string]any{"type": []string{"string", "null"}},
		"suggested_new_document_type": map[string]any{"type": []string{"string", "null"}},
		"confidence":                  map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
		"reasoning":                   map[string]any{"type": "string"},
	},
	"required":             []string{"document_type_id", "document_type_name", "suggested_new_document_type", "confidence", "reasoning"},
	"additionalProperties": false,
}

var tagSuggestionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"tag_ids":            map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"tag_names":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"suggested_new_tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"confidence":         map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
		"reasoning":          map[string]any{"type": "string"},
	},
	"required":             []string{"tag_ids", "tag_names", "suggested_new_tags", "confidence", "reasoning"},
	"additionalProperties": false,
}

var createdDateSuggestionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"created":    map[string]any{"type": []string{"string", "null"}},
		"confidence": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
		"reasoning":  map[string]any{"type": "string"},
	},
	"required":             []string{"created", "confidence", "reasoning"},
	"additionalProperties": false,
}

var titleSuggestionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title":      map[string]any{"type": "string"},
		"confidence": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
		"reasoning":  map[string]any{"type": "string"},
	},
	"required":             []string{"title", "confidence", "reasoning"},
	"additionalProperties": false,
}

type SimilarDocumentEvidence struct {
	DocumentID       int64
	Title            string
	OriginalFileName string
	Created          string
	Correspondent    string
	DocumentType     string
	TagNames         []string
	Snippet          string
	Similarity       float64
}

const strictJSONOutputRules = `Output contract:
- Return exactly one valid JSON object.
- The first character of your response must be {.
- The last character of your response must be }.
- Do not wrap the JSON in markdown or code fences.
- Do not add any text before or after the JSON.
- Do not omit required keys.
- Use null for unknown single-value fields.
- Use [] for empty list fields.
- Keep the reasoning short and factual.
- Before responding, verify that your full response is valid JSON.`

const germanResponseRules = `Language rules:
- Write all explanatory text in German.
- Write the reasoning field in German.
- If you suggest a new correspondent, document type, or tag, prefer a concise German label unless the document clearly uses an established non-German term.
- Preserve any existing correspondent, document type, and tag names exactly as listed. Do not translate or rewrite existing names.`

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
- If you return both correspondent_id and correspondent_name, they must refer to the same listed correspondent.
- If you choose an existing correspondent, your reasoning must explicitly name that same correspondent or an obvious normalized form of it.
- If no existing correspondent fits, set correspondent_id and correspondent_name to null.
- If you are uncertain, still return a valid JSON object and lower the confidence.

%s

Valid example:
{"correspondent_id":12,"correspondent_name":"Telekom","suggested_new_correspondent":null,"confidence":"high","reasoning":"Die Rechnung stammt eindeutig von Telekom."}

%s
%s

%s

Document source: %s

Document text:
%s`

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
- If you return both document_type_id and document_type_name, they must refer to the same listed document type.
- If no existing document type fits, set document_type_id and document_type_name to null.
- If you are uncertain, still return a valid JSON object and lower the confidence.

%s

Valid example:
{"document_type_id":7,"document_type_name":"Invoice","suggested_new_document_type":null,"confidence":"high","reasoning":"Das Dokument ist eine Lieferantenrechnung."}

%s

Existing document types:
%s

Document source: %s

Document text:
%s`

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
- If you are uncertain, still return a valid JSON object and lower the confidence.

%s

Valid example:
{"tag_ids":[3,8],"tag_names":["invoice","telecom"],"suggested_new_tags":[],"confidence":"medium","reasoning":"Das Dokument ist eine Telekommunikationsrechnung und beide Tags passen."}

%s

Existing tags:
%s

Document source: %s

Document text:
%s`

const createdDatePromptTemplate = `You are extracting the document creation date for paperless-ngx.
Identify the date the document itself was created or issued, not the date it was added to paperless-ngx.

Return strict JSON with exactly these keys:
{
	"created": "YYYY-MM-DD"|null,
	"confidence": "high"|"medium"|"low",
	"reasoning": string
}

Rules:
- Use the document's own creation, issue, invoice, statement, or letter date if it is clearly supported by the text.
- Do not use due dates, booking dates, debit dates, service periods, or payment dates unless the text clearly indicates that this is also the document's own date.
- If no reliable single date can be determined, return null for created.
- The created value must be an ISO date in the format YYYY-MM-DD when present.

%s

Valid example:
{"created":"2026-03-15","confidence":"high","reasoning":"Das Schreiben ist auf den 15.03.2026 datiert."}

Document source: %s

Document text:
%s`

const titlePromptTemplate = `You are generating a document title for paperless-ngx.
Suggest a concise, human-readable title that will help identify the document later.

Return strict JSON with exactly these keys:
{
	"title": string,
	"confidence": "high"|"medium"|"low",
	"reasoning": string
}

Rules:
- Prefer a concise title, usually 3 to 10 words.
- Include the most identifying information available, such as issuer, document type, and month or date when clearly supported.
- Do not include file extensions or generic prefixes like "Scan" unless they are truly part of the document identity.
- If the current title is already appropriate, you may return the same title.

%s

Valid example:
{"title":"Telekom Rechnung Maerz 2026","confidence":"high","reasoning":"Der Aussteller, der Dokumenttyp und der Monat sind klar erkennbar."}

Current title or source name: %s

Document text:
%s`

const structuredRetryPromptTemplate = `Your previous answer for this same task did not satisfy the required JSON contract.

Validation error:
%s

Re-answer the original task now.

Rules:
- Return exactly one valid JSON object that satisfies the schema for this request.
- Do not include markdown, code fences, or any text before or after the JSON object.
- Follow the original task instructions, candidate lists, and response-language rules.
- If you are uncertain, keep confidence low and use null or [] where the schema permits it.
- Do not summarize the document outside the JSON object.`

func runStructuredSuggestion[T any](ctx context.Context, ollamaURL string, model string, prompt string, schema any, parser func(string) (T, error)) (T, error) {
	messages := []ollama.Message{{Role: "user", Content: prompt}}
	response, err := ollama.RunWithFormat(ctx, ollamaURL, model, messages, schema)
	if err != nil {
		var zero T
		return zero, err
	}

	parsed, err := parser(response)
	if err == nil {
		return parsed, nil
	}

	repairMessages := append(messages,
		ollama.Message{Role: "assistant", Content: response},
		ollama.Message{Role: "user", Content: fmt.Sprintf(structuredRetryPromptTemplate, err.Error())},
	)
	repairedResponse, repairErr := ollama.RunWithFormat(ctx, ollamaURL, model, repairMessages, schema)
	if repairErr != nil {
		var zero T
		return zero, fmt.Errorf("parse structured LLM response: %v; repair retry failed: %w", err, repairErr)
	}

	repaired, repairParseErr := parser(repairedResponse)
	if repairParseErr != nil {
		var zero T
		return zero, fmt.Errorf("parse structured LLM response: %v; repaired response invalid: %w", err, repairParseErr)
	}

	return repaired, nil
}

func SuggestCorrespondent(ctx context.Context, ollamaURL string, model string, documentName string, documentText string, correspondents []paperless.Correspondent, historicalDocuments []paperless.Document) (CorrespondentSuggestion, error) {
	prompt := buildCorrespondentPrompt(documentName, documentText, correspondents, historicalDocuments)
	return runStructuredSuggestion(ctx, ollamaURL, model, prompt, correspondentSuggestionSchema, func(raw string) (CorrespondentSuggestion, error) {
		return ParseCorrespondentSuggestion(raw, correspondents)
	})
}

func SuggestDocumentType(ctx context.Context, ollamaURL string, model string, documentName string, documentText string, documentTypes []paperless.DocumentType, similarDocuments []SimilarDocumentEvidence) (DocumentTypeSuggestion, error) {
	prompt := fmt.Sprintf(documentTypePromptTemplate, strictJSONOutputRules+"\n"+germanResponseRules, buildSimilarDocumentsSection(similarDocuments), buildEntityList(documentTypes, "No existing document types available"), documentName, documentText)
	return runStructuredSuggestion(ctx, ollamaURL, model, prompt, documentTypeSuggestionSchema, func(raw string) (DocumentTypeSuggestion, error) {
		return ParseDocumentTypeSuggestion(raw, documentTypes)
	})
}

func SuggestTags(ctx context.Context, ollamaURL string, model string, documentName string, documentText string, tags []paperless.Tag, similarDocuments []SimilarDocumentEvidence) (TagSuggestion, error) {
	prompt := fmt.Sprintf(tagPromptTemplate, strictJSONOutputRules+"\n"+germanResponseRules, buildSimilarDocumentsSection(similarDocuments), buildEntityList(tags, "No existing tags available"), documentName, documentText)
	return runStructuredSuggestion(ctx, ollamaURL, model, prompt, tagSuggestionSchema, func(raw string) (TagSuggestion, error) {
		return ParseTagSuggestion(raw, tags)
	})
}

func SuggestCreatedDate(ctx context.Context, ollamaURL string, model string, documentName string, documentText string) (CreatedDateSuggestion, error) {
	prompt := fmt.Sprintf(createdDatePromptTemplate, strictJSONOutputRules+"\n"+germanResponseRules, documentName, documentText)
	return runStructuredSuggestion(ctx, ollamaURL, model, prompt, createdDateSuggestionSchema, ParseCreatedDateSuggestion)
}

func SuggestTitle(ctx context.Context, ollamaURL string, model string, documentName string, documentText string) (TitleSuggestion, error) {
	prompt := fmt.Sprintf(titlePromptTemplate, strictJSONOutputRules+"\n"+germanResponseRules, documentName, documentText)
	return runStructuredSuggestion(ctx, ollamaURL, model, prompt, titleSuggestionSchema, ParseTitleSuggestion)
}

func buildEntityList[T paperless.NamedEntity](items []T, emptyLabel string) string {
	if len(items) == 0 {
		return "- " + emptyLabel
	}

	var builder strings.Builder
	for _, item := range items {
		builder.WriteString("- ")
		builder.WriteString(strconv.FormatInt(item.GetID(), 10))
		builder.WriteString(": ")
		builder.WriteString(item.GetName())
		builder.WriteByte('\n')
	}

	return strings.TrimSpace(builder.String())
}

func buildSimilarDocumentsSection(items []SimilarDocumentEvidence) string {
	if len(items) == 0 {
		return "Similar library documents:\n- No strong semantic matches were found in the indexed library."
	}

	var builder strings.Builder
	builder.WriteString("Similar library documents:\n")
	for _, item := range items {
		builder.WriteString("- similarity=")
		builder.WriteString(fmt.Sprintf("%.3f", item.Similarity))
		builder.WriteString(" | id=")
		builder.WriteString(strconv.FormatInt(item.DocumentID, 10))

		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = strings.TrimSpace(item.OriginalFileName)
		}
		if title != "" {
			builder.WriteString(" | title=")
			builder.WriteString(title)
		}

		if correspondent := strings.TrimSpace(item.Correspondent); correspondent != "" {
			builder.WriteString(" | correspondent=")
			builder.WriteString(correspondent)
		}
		if documentType := strings.TrimSpace(item.DocumentType); documentType != "" {
			builder.WriteString(" | document_type=")
			builder.WriteString(documentType)
		}
		if len(item.TagNames) > 0 {
			builder.WriteString(" | tags=")
			builder.WriteString(strings.Join(item.TagNames, ", "))
		}
		if created := strings.TrimSpace(item.Created); created != "" {
			builder.WriteString(" | created=")
			builder.WriteString(created)
		}
		builder.WriteByte('\n')

		if snippet := strings.TrimSpace(item.Snippet); snippet != "" {
			builder.WriteString("  Snippet: ")
			builder.WriteString(snippet)
			builder.WriteByte('\n')
		}
	}

	return strings.TrimSpace(builder.String())
}
