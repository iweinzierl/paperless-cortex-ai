package classification

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"paperless-ai-ext/internal/paperless"
)

type CorrespondentSuggestion struct {
	CorrespondentID           *int64  `json:"correspondent_id"`
	CorrespondentName         *string `json:"correspondent_name"`
	SuggestedNewCorrespondent *string `json:"suggested_new_correspondent"`
	Confidence                string  `json:"confidence"`
	Reasoning                 string  `json:"reasoning"`
}

type DocumentTypeSuggestion struct {
	DocumentTypeID           *int64  `json:"document_type_id"`
	DocumentTypeName         *string `json:"document_type_name"`
	SuggestedNewDocumentType *string `json:"suggested_new_document_type"`
	Confidence               string  `json:"confidence"`
	Reasoning                string  `json:"reasoning"`
}

type TagSuggestion struct {
	TagIDs           []int64  `json:"tag_ids"`
	TagNames         []string `json:"tag_names"`
	SuggestedNewTags []string `json:"suggested_new_tags"`
	Confidence       string   `json:"confidence"`
	Reasoning        string   `json:"reasoning"`
}

func ExtractJSONObject(raw string) (string, error) {
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

func NormalizeConfidence(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "high" || normalized == "medium" || normalized == "low" {
		return normalized
	}
	return "low"
}

func NormalizeStringList(values []string) []string {
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

func ParseCorrespondentSuggestion(raw string, correspondents []paperless.Correspondent) (CorrespondentSuggestion, error) {
	var suggestion CorrespondentSuggestion
	if err := parseSuggestion(raw, &suggestion); err != nil {
		return CorrespondentSuggestion{}, err
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
	if suggestion.CorrespondentID == nil && suggestion.CorrespondentName == nil && suggestion.SuggestedNewCorrespondent == nil {
		return CorrespondentSuggestion{}, errors.New("LLM did not select or suggest any correspondent")
	}

	byID := make(map[int64]paperless.Correspondent, len(correspondents))
	byName := make(map[string]paperless.Correspondent, len(correspondents))
	for _, item := range correspondents {
		byID[item.ID] = item
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}

	if suggestion.CorrespondentID != nil {
		item, ok := byID[*suggestion.CorrespondentID]
		if !ok {
			return CorrespondentSuggestion{}, fmt.Errorf("LLM selected unknown correspondent id %d", *suggestion.CorrespondentID)
		}
		suggestion.CorrespondentName = stringPointer(item.Name)
		suggestion.SuggestedNewCorrespondent = nil
		return suggestion, nil
	}

	if suggestion.CorrespondentName != nil {
		item, ok := byName[strings.ToLower(strings.TrimSpace(*suggestion.CorrespondentName))]
		if ok {
			suggestion.CorrespondentID = int64Pointer(item.ID)
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

	return CorrespondentSuggestion{}, errors.New("LLM response could not be mapped to an existing or new correspondent")
}

func ParseDocumentTypeSuggestion(raw string, documentTypes []paperless.DocumentType) (DocumentTypeSuggestion, error) {
	var suggestion DocumentTypeSuggestion
	if err := parseSuggestion(raw, &suggestion); err != nil {
		return DocumentTypeSuggestion{}, err
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
	if suggestion.DocumentTypeID == nil && suggestion.DocumentTypeName == nil && suggestion.SuggestedNewDocumentType == nil {
		return DocumentTypeSuggestion{}, errors.New("LLM did not select or suggest any document type")
	}

	byID := make(map[int64]paperless.DocumentType, len(documentTypes))
	byName := make(map[string]paperless.DocumentType, len(documentTypes))
	for _, item := range documentTypes {
		byID[item.ID] = item
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}

	if suggestion.DocumentTypeID != nil {
		item, ok := byID[*suggestion.DocumentTypeID]
		if !ok {
			return DocumentTypeSuggestion{}, fmt.Errorf("LLM selected unknown document type id %d", *suggestion.DocumentTypeID)
		}
		suggestion.DocumentTypeName = stringPointer(item.Name)
		suggestion.SuggestedNewDocumentType = nil
		return suggestion, nil
	}

	if suggestion.DocumentTypeName != nil {
		item, ok := byName[strings.ToLower(strings.TrimSpace(*suggestion.DocumentTypeName))]
		if ok {
			suggestion.DocumentTypeID = int64Pointer(item.ID)
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

	return DocumentTypeSuggestion{}, errors.New("LLM response could not be mapped to an existing or new document type")
}

func ParseTagSuggestion(raw string, tags []paperless.Tag) (TagSuggestion, error) {
	var suggestion TagSuggestion
	if err := parseSuggestion(raw, &suggestion); err != nil {
		return TagSuggestion{}, err
	}

	suggestion.TagNames = NormalizeStringList(suggestion.TagNames)
	suggestion.SuggestedNewTags = NormalizeStringList(suggestion.SuggestedNewTags)

	byID := make(map[int64]paperless.Tag, len(tags))
	byName := make(map[string]paperless.Tag, len(tags))
	for _, item := range tags {
		byID[item.ID] = item
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}

	selectedByID := make(map[int64]paperless.Tag)
	for _, tagID := range suggestion.TagIDs {
		item, ok := byID[tagID]
		if !ok {
			return TagSuggestion{}, fmt.Errorf("LLM selected unknown tag id %d", tagID)
		}
		selectedByID[item.ID] = item
	}
	for _, tagName := range suggestion.TagNames {
		item, ok := byName[strings.ToLower(strings.TrimSpace(tagName))]
		if ok {
			selectedByID[item.ID] = item
		}
	}

	selectedTags := make([]paperless.Tag, 0, len(selectedByID))
	for _, item := range selectedByID {
		selectedTags = append(selectedTags, item)
	}
	sort.Slice(selectedTags, func(left int, right int) bool {
		return strings.ToLower(selectedTags[left].Name) < strings.ToLower(selectedTags[right].Name)
	})

	normalized := TagSuggestion{
		TagIDs:           make([]int64, 0, len(selectedTags)),
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

func parseSuggestion(raw string, target any) error {
	jsonObject, err := ExtractJSONObject(raw)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(jsonObject), target); err != nil {
		return fmt.Errorf("decode LLM suggestion: %w", err)
	}

	switch typed := target.(type) {
	case *CorrespondentSuggestion:
		typed.Confidence = NormalizeConfidence(typed.Confidence)
	case *DocumentTypeSuggestion:
		typed.Confidence = NormalizeConfidence(typed.Confidence)
	case *TagSuggestion:
		typed.Confidence = NormalizeConfidence(typed.Confidence)
	}

	return nil
}

func int64Pointer(value int64) *int64 {
	return &value
}

func stringPointer(value string) *string {
	return &value
}
