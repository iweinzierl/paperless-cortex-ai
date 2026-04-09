package classification

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"paperless-ai-ext/internal/ocr"
	"paperless-ai-ext/internal/paperless"
)

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
{"correspondent_id":12,"correspondent_name":"Telekom","suggested_new_correspondent":null,"confidence":"high","reasoning":"The bill clearly comes from Telekom."}

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
{"document_type_id":7,"document_type_name":"Invoice","suggested_new_document_type":null,"confidence":"high","reasoning":"The document is a supplier invoice."}

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
{"tag_ids":[3,8],"tag_names":["invoice","telecom"],"suggested_new_tags":[],"confidence":"medium","reasoning":"The document is a telecom invoice and both tags fit."}

Existing tags:
%s

Document source: %s

Document text:
%s`

func SuggestCorrespondent(ctx context.Context, ollamaURL string, model string, documentName string, documentText string, correspondents []paperless.Correspondent, historicalDocuments []paperless.Document) (CorrespondentSuggestion, error) {
	prompt := buildCorrespondentPrompt(documentName, documentText, correspondents, historicalDocuments)
	response, err := ocr.Run(ctx, ollamaURL, model, ocr.Message{Role: "user", Content: prompt})
	if err != nil {
		return CorrespondentSuggestion{}, err
	}
	return ParseCorrespondentSuggestion(response, correspondents)
}

func SuggestDocumentType(ctx context.Context, ollamaURL string, model string, documentName string, documentText string, documentTypes []paperless.DocumentType) (DocumentTypeSuggestion, error) {
	prompt := fmt.Sprintf(documentTypePromptTemplate, strictJSONOutputRules, buildEntityList(documentTypes, "No existing document types available"), documentName, documentText)
	response, err := ocr.Run(ctx, ollamaURL, model, ocr.Message{Role: "user", Content: prompt})
	if err != nil {
		return DocumentTypeSuggestion{}, err
	}
	return ParseDocumentTypeSuggestion(response, documentTypes)
}

func SuggestTags(ctx context.Context, ollamaURL string, model string, documentName string, documentText string, tags []paperless.Tag) (TagSuggestion, error) {
	prompt := fmt.Sprintf(tagPromptTemplate, strictJSONOutputRules, buildEntityList(tags, "No existing tags available"), documentName, documentText)
	response, err := ocr.Run(ctx, ollamaURL, model, ocr.Message{Role: "user", Content: prompt})
	if err != nil {
		return TagSuggestion{}, err
	}
	return ParseTagSuggestion(response, tags)
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
