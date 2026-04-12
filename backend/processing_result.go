package main

import (
	"encoding/json"
	"strings"

	"paperless-ai-ext/internal/classification"
	"paperless-ai-ext/internal/paperless"
)

const (
	stageStatusPending   = "pending"
	stageStatusRunning   = "running"
	stageStatusSkipped   = "skipped"
	stageStatusCompleted = "completed"
	stageStatusFailed    = "failed"
)

type ProcessingPlan struct {
	TriggerTagPresent  bool     `json:"trigger_tag_present"`
	ForceOCR           bool     `json:"force_ocr"`
	ForceVision        bool     `json:"force_vision"`
	CreatedDate        bool     `json:"process_created_date"`
	Correspondent      bool     `json:"process_correspondent"`
	DocumentType       bool     `json:"process_document_type"`
	DocumentTags       bool     `json:"process_document_tags"`
	Title              bool     `json:"process_title"`
	RequestedStageList []string `json:"requested_stages"`
}

func (plan ProcessingPlan) HasRequestedWork() bool {
	return plan.ForceOCR || plan.ForceVision || plan.CreatedDate || plan.Correspondent || plan.DocumentType || plan.DocumentTags || plan.Title
}

type ProcessingResult struct {
	Document      ProcessingDocumentSummary `json:"document"`
	Plan          ProcessingPlan            `json:"plan"`
	Extraction    ExtractionStageResult     `json:"extraction"`
	CreatedDate   SuggestionStageResult     `json:"created_date"`
	Correspondent SuggestionStageResult     `json:"correspondent"`
	DocumentType  SuggestionStageResult     `json:"document_type"`
	Tags          SuggestionStageResult     `json:"tags"`
	Title         SuggestionStageResult     `json:"title"`
	Notes         []string                  `json:"notes,omitempty"`
}

type ProcessingDocumentSummary struct {
	ID                int64    `json:"id"`
	Title             string   `json:"title"`
	TagIDs            []int64  `json:"tag_ids,omitempty"`
	TagNames          []string `json:"tag_names,omitempty"`
	OriginalFileName  string   `json:"original_file_name,omitempty"`
	Created           string   `json:"created,omitempty"`
	Added             string   `json:"added,omitempty"`
	CorrespondentID   *int64   `json:"correspondent_id,omitempty"`
	CorrespondentName string   `json:"correspondent_name,omitempty"`
	DocumentTypeID    *int64   `json:"document_type_id,omitempty"`
	DocumentTypeName  string   `json:"document_type_name,omitempty"`
}

type ExtractionStageResult struct {
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	Source      string `json:"source,omitempty"`
	UsedModel   string `json:"used_model,omitempty"`
	TextLength  int    `json:"text_length"`
	TextPreview string `json:"text_preview,omitempty"`
}

type SuggestionStageResult struct {
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	UsedModel  string `json:"used_model,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Reasoning  string `json:"reasoning,omitempty"`
	Payload    any    `json:"payload,omitempty"`
}

func newProcessingResult(document *paperless.Document, tagNames []string, plan ProcessingPlan) ProcessingResult {
	result := ProcessingResult{
		Plan:          plan,
		Extraction:    ExtractionStageResult{Status: stageStatusSkipped},
		CreatedDate:   SuggestionStageResult{Status: stageStatusSkipped},
		Correspondent: SuggestionStageResult{Status: stageStatusSkipped},
		DocumentType:  SuggestionStageResult{Status: stageStatusSkipped},
		Tags:          SuggestionStageResult{Status: stageStatusSkipped},
		Title:         SuggestionStageResult{Status: stageStatusSkipped},
	}
	if len(plan.RequestedStageList) > 0 {
		result.Extraction.Status = stageStatusPending
	}
	if plan.CreatedDate {
		result.CreatedDate.Status = stageStatusPending
	}
	if plan.Correspondent {
		result.Correspondent.Status = stageStatusPending
	}
	if plan.DocumentType {
		result.DocumentType.Status = stageStatusPending
	}
	if plan.DocumentTags {
		result.Tags.Status = stageStatusPending
	}
	if plan.Title {
		result.Title.Status = stageStatusPending
	}
	if document != nil {
		result.Document = ProcessingDocumentSummary{
			ID:                document.ID,
			Title:             document.Title,
			TagIDs:            append([]int64(nil), document.TagIDs...),
			TagNames:          append([]string(nil), tagNames...),
			OriginalFileName:  document.OriginalFileName,
			Created:           document.Created,
			Added:             document.Added,
			CorrespondentID:   document.CorrespondentID,
			CorrespondentName: strings.TrimSpace(document.CorrespondentName),
			DocumentTypeID:    document.DocumentTypeID,
			DocumentTypeName:  strings.TrimSpace(document.DocumentTypeName),
		}
	}
	return result
}

func (result ProcessingResult) Marshal() string {
	payload, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	return string(payload)
}

func buildProcessingPlan(cfg ProcessConfig, tagNameSet map[string]struct{}) ProcessingPlan {
	plan := ProcessingPlan{
		TriggerTagPresent: hasNamedTag(tagNameSet, cfg.ProcessTriggerTag),
		ForceOCR:          hasNamedTag(tagNameSet, cfg.ForceOCRTag),
		ForceVision:       hasNamedTag(tagNameSet, cfg.ForceVisionTag),
		CreatedDate:       hasNamedTag(tagNameSet, cfg.ProcessCreatedDateTag),
		Correspondent:     hasNamedTag(tagNameSet, cfg.ProcessCorrespondentTag),
		DocumentType:      hasNamedTag(tagNameSet, cfg.ProcessDocumentTypeTag),
		DocumentTags:      hasNamedTag(tagNameSet, cfg.ProcessDocumentTagsTag),
		Title:             hasNamedTag(tagNameSet, cfg.ProcessTitleTag),
	}
	if plan.HasRequestedWork() {
		plan.RequestedStageList = append(plan.RequestedStageList, "extract_text")
	}
	if plan.CreatedDate {
		plan.RequestedStageList = append(plan.RequestedStageList, "created_date")
	}
	if plan.Correspondent {
		plan.RequestedStageList = append(plan.RequestedStageList, "correspondent")
	}
	if plan.DocumentType {
		plan.RequestedStageList = append(plan.RequestedStageList, "document_type")
	}
	if plan.DocumentTags {
		plan.RequestedStageList = append(plan.RequestedStageList, "tags")
	}
	if plan.Title {
		plan.RequestedStageList = append(plan.RequestedStageList, "title")
	}
	return plan
}

func buildTagNameSet(tags []paperless.Tag, documentTagIDs []int64) (map[string]struct{}, []string) {
	byID := make(map[int64]string, len(tags))
	for _, tag := range tags {
		byID[tag.ID] = strings.TrimSpace(tag.Name)
	}

	tagNameSet := make(map[string]struct{}, len(documentTagIDs))
	tagNames := make([]string, 0, len(documentTagIDs))
	for _, tagID := range documentTagIDs {
		name := strings.TrimSpace(byID[tagID])
		if name == "" {
			continue
		}
		normalized := strings.ToLower(name)
		if _, ok := tagNameSet[normalized]; ok {
			continue
		}
		tagNameSet[normalized] = struct{}{}
		tagNames = append(tagNames, name)
	}

	return tagNameSet, tagNames
}

func hasNamedTag(tagNameSet map[string]struct{}, tagName string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(tagName))
	if trimmed == "" {
		return false
	}
	_, ok := tagNameSet[trimmed]
	return ok
}

func extractedTextPreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= 2000 {
		return trimmed
	}
	return trimmed[:2000]
}

func suggestionStageResult(status string, usedModel string, payload any, confidence string, reasoning string) SuggestionStageResult {
	return SuggestionStageResult{
		Status:     status,
		UsedModel:  strings.TrimSpace(usedModel),
		Confidence: strings.TrimSpace(confidence),
		Reasoning:  strings.TrimSpace(reasoning),
		Payload:    payload,
	}
}

func correspondentStagePayload(suggestion classification.CorrespondentSuggestion) any {
	return suggestion
}

func documentTypeStagePayload(suggestion classification.DocumentTypeSuggestion) any {
	return suggestion
}

func tagsStagePayload(suggestion classification.TagSuggestion) any {
	return suggestion
}

func createdDateStagePayload(suggestion classification.CreatedDateSuggestion) any {
	return suggestion
}

func titleStagePayload(suggestion classification.TitleSuggestion) any {
	return suggestion
}
