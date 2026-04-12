package classification

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"paperless-ai-ext/internal/paperless"
)

var genericEntityNameTokens = map[string]struct{}{
	"ag":   {},
	"co":   {},
	"gmbh": {},
	"inc":  {},
	"kg":   {},
	"llc":  {},
	"ltd":  {},
	"mbh":  {},
	"sa":   {},
	"se":   {},
	"the":  {},
}

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

type CreatedDateSuggestion struct {
	Created    *string `json:"created"`
	Confidence string  `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

type TitleSuggestion struct {
	Title      *string `json:"title"`
	Confidence string  `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
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

	var matchedByID *paperless.Correspondent
	if suggestion.CorrespondentID != nil {
		item, ok := byID[*suggestion.CorrespondentID]
		if !ok {
			return CorrespondentSuggestion{}, fmt.Errorf("LLM selected unknown correspondent id %d", *suggestion.CorrespondentID)
		}
		matchedByID = &item
	}

	var matchedByName *paperless.Correspondent
	if suggestion.CorrespondentName != nil {
		item, ok := byName[strings.ToLower(strings.TrimSpace(*suggestion.CorrespondentName))]
		if ok {
			matchedByName = &item
		}
	}

	if matchedByID != nil && matchedByName != nil && matchedByID.ID != matchedByName.ID {
		suggestion.CorrespondentID = int64Pointer(matchedByName.ID)
		suggestion.CorrespondentName = stringPointer(matchedByName.Name)
		suggestion.SuggestedNewCorrespondent = nil
		suggestion = reconcileCorrespondentReasoning(suggestion, correspondents)
		return suggestion, nil
	}

	if matchedByID != nil {
		suggestion.CorrespondentName = stringPointer(matchedByID.Name)
		suggestion.SuggestedNewCorrespondent = nil
		suggestion = reconcileCorrespondentReasoning(suggestion, correspondents)
		return suggestion, nil
	}

	if matchedByName != nil {
		suggestion.CorrespondentID = int64Pointer(matchedByName.ID)
		suggestion.CorrespondentName = stringPointer(matchedByName.Name)
		suggestion.SuggestedNewCorrespondent = nil
		suggestion = reconcileCorrespondentReasoning(suggestion, correspondents)
		return suggestion, nil
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

	var matchedByID *paperless.DocumentType
	if suggestion.DocumentTypeID != nil {
		item, ok := byID[*suggestion.DocumentTypeID]
		if !ok {
			return DocumentTypeSuggestion{}, fmt.Errorf("LLM selected unknown document type id %d", *suggestion.DocumentTypeID)
		}
		matchedByID = &item
	}

	var matchedByName *paperless.DocumentType
	if suggestion.DocumentTypeName != nil {
		item, ok := byName[strings.ToLower(strings.TrimSpace(*suggestion.DocumentTypeName))]
		if ok {
			matchedByName = &item
		}
	}

	if matchedByID != nil && matchedByName != nil && matchedByID.ID != matchedByName.ID {
		suggestion.DocumentTypeID = int64Pointer(matchedByName.ID)
		suggestion.DocumentTypeName = stringPointer(matchedByName.Name)
		suggestion.SuggestedNewDocumentType = nil
		return suggestion, nil
	}

	if matchedByID != nil {
		suggestion.DocumentTypeName = stringPointer(matchedByID.Name)
		suggestion.SuggestedNewDocumentType = nil
		return suggestion, nil
	}

	if matchedByName != nil {
		suggestion.DocumentTypeID = int64Pointer(matchedByName.ID)
		suggestion.DocumentTypeName = stringPointer(matchedByName.Name)
		suggestion.SuggestedNewDocumentType = nil
		return suggestion, nil
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

func ParseCreatedDateSuggestion(raw string) (CreatedDateSuggestion, error) {
	var suggestion CreatedDateSuggestion
	if err := parseSuggestion(raw, &suggestion); err != nil {
		return CreatedDateSuggestion{}, err
	}

	if suggestion.Created == nil {
		return CreatedDateSuggestion{}, errors.New("LLM did not suggest a document creation date")
	}

	trimmed := strings.TrimSpace(*suggestion.Created)
	if trimmed == "" {
		return CreatedDateSuggestion{}, errors.New("LLM did not suggest a document creation date")
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return CreatedDateSuggestion{}, fmt.Errorf("LLM suggested invalid document creation date %q", trimmed)
	}

	suggestion.Created = stringPointer(trimmed)
	return suggestion, nil
}

func ParseTitleSuggestion(raw string) (TitleSuggestion, error) {
	var suggestion TitleSuggestion
	if err := parseSuggestion(raw, &suggestion); err != nil {
		return TitleSuggestion{}, err
	}

	if suggestion.Title == nil {
		return TitleSuggestion{}, errors.New("LLM did not suggest a document title")
	}

	trimmed := strings.TrimSpace(*suggestion.Title)
	if trimmed == "" {
		return TitleSuggestion{}, errors.New("LLM did not suggest a document title")
	}

	suggestion.Title = stringPointer(trimmed)
	return suggestion, nil
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
	case *CreatedDateSuggestion:
		typed.Confidence = NormalizeConfidence(typed.Confidence)
	case *TitleSuggestion:
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

func reconcileCorrespondentReasoning(suggestion CorrespondentSuggestion, correspondents []paperless.Correspondent) CorrespondentSuggestion {
	if suggestion.CorrespondentID == nil || suggestion.CorrespondentName == nil {
		return suggestion
	}

	mentioned := findCorrespondentMentionInReasoning(suggestion.Reasoning, correspondents)
	if mentioned == nil || mentioned.ID == *suggestion.CorrespondentID {
		return suggestion
	}

	suggestion.CorrespondentID = int64Pointer(mentioned.ID)
	suggestion.CorrespondentName = stringPointer(mentioned.Name)
	suggestion.SuggestedNewCorrespondent = nil
	suggestion.Confidence = lowerConfidence(suggestion.Confidence)
	return suggestion
}

func findCorrespondentMentionInReasoning(reasoning string, correspondents []paperless.Correspondent) *paperless.Correspondent {
	normalizedReasoning := normalizeSuggestionText(reasoning)
	if normalizedReasoning == "" {
		return nil
	}

	bestScore := 0
	var best *paperless.Correspondent
	ambiguous := false
	for _, correspondent := range correspondents {
		score := reasoningMentionScore(normalizedReasoning, correspondent.Name)
		if score == 0 {
			continue
		}
		if score > bestScore {
			item := correspondent
			best = &item
			bestScore = score
			ambiguous = false
			continue
		}
		if score == bestScore {
			ambiguous = true
		}
	}

	if ambiguous || best == nil {
		return nil
	}

	return best
}

func reasoningMentionScore(normalizedReasoning string, name string) int {
	normalizedName := normalizeSuggestionText(name)
	if normalizedName == "" {
		return 0
	}
	if containsWholePhrase(normalizedReasoning, normalizedName) {
		return 100 + len(normalizedName)
	}

	nameTokens := distinctiveSuggestionTokens(name)
	if len(nameTokens) == 0 {
		nameTokens = suggestionTokens(name)
		if len(nameTokens) == 0 {
			return 0
		}
	}
	matches := 0
	longestMatch := 0
	for _, token := range nameTokens {
		if containsWholePhrase(normalizedReasoning, token) {
			matches++
			if len(token) > longestMatch {
				longestMatch = len(token)
			}
		}
	}
	if matches == 0 {
		return 0
	}
	if matches == len(nameTokens) {
		return 50 + matches
	}
	if len(nameTokens) == 1 {
		return 20 + longestMatch
	}
	if longestMatch >= 6 {
		return 25 + longestMatch + matches
	}
	return 0
}

func lowerConfidence(confidence string) string {
	switch NormalizeConfidence(confidence) {
	case "high":
		return "medium"
	case "medium":
		return "low"
	default:
		return "low"
	}
}

func normalizeSuggestionText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(text))
	lastWasSpace := true
	for _, char := range text {
		switch {
		case unicode.IsLetter(char):
			builder.WriteRune(char)
			lastWasSpace = false
		case unicode.IsDigit(char):
			builder.WriteRune(char)
			lastWasSpace = false
		default:
			if !lastWasSpace {
				builder.WriteByte(' ')
				lastWasSpace = true
			}
		}
	}

	return strings.TrimSpace(builder.String())
}

func suggestionTokens(text string) []string {
	normalized := normalizeSuggestionText(text)
	if normalized == "" {
		return nil
	}
	parts := strings.Fields(normalized)
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 3 {
			continue
		}
		tokens = append(tokens, part)
	}
	return tokens
}

func distinctiveSuggestionTokens(text string) []string {
	parts := suggestionTokens(text)
	if len(parts) == 0 {
		return nil
	}
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if _, ok := genericEntityNameTokens[part]; ok {
			continue
		}
		tokens = append(tokens, part)
	}
	return tokens
}

func containsWholePhrase(text string, phrase string) bool {
	if text == "" || phrase == "" {
		return false
	}
	haystack := " " + text + " "
	needle := " " + phrase + " "
	return strings.Contains(haystack, needle)
}
