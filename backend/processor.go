package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"paperless-ai-ext/internal/classification"
	"paperless-ai-ext/internal/paperless"

	"github.com/rs/zerolog"
)

type Processor struct {
	store  *Store
	logger zerolog.Logger
}

func NewProcessor(store *Store, logger zerolog.Logger) *Processor {
	return &Processor{store: store, logger: logger}
}

func (p *Processor) Start(ctx context.Context) {
	go func() {
		for {
			cfg, err := p.store.LoadConfig(ctx)
			if err != nil {
				p.logger.Error().Err(err).Msg("failed to load backend config for processor loop")
			}

			waitInterval := 5 * time.Second
			if cfg.Engine.ProcessingIntervalSeconds > 0 {
				waitInterval = time.Duration(cfg.Engine.ProcessingIntervalSeconds) * time.Second
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(waitInterval):
			}

			if cfg.Engine.ProcessingMode != ProcessingModeAuto {
				continue
			}

			if _, err := p.ProcessNext(ctx); err != nil && err != errNoPendingQueueItems {
				p.logger.Error().Err(err).Msg("automatic queue processing failed")
			}
		}
	}()
}

func (p *Processor) ProcessNext(ctx context.Context) (*QueueItem, error) {
	item, err := p.store.ClaimNextPendingQueueItem(ctx)
	if err != nil {
		return nil, err
	}

	return p.execute(ctx, item)
}

func (p *Processor) ProcessByID(ctx context.Context, id int64) (*QueueItem, error) {
	item, err := p.store.ClaimQueueItemByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return p.execute(ctx, item)
}

func (p *Processor) StartProcessByID(ctx context.Context, id int64) (*QueueItem, error) {
	item, err := p.store.ClaimQueueItemByID(ctx, id)
	if err != nil {
		return nil, err
	}

	go func(queueItem *QueueItem) {
		runCtx := context.WithoutCancel(ctx)
		if _, executeErr := p.execute(runCtx, queueItem); executeErr != nil {
			queueLogger := p.queueItemLogger(queueItem)
			queueLogger.Error().Err(executeErr).Msg("manual queue processing failed")
		}
	}(item)

	return item, nil
}

func (p *Processor) execute(ctx context.Context, item *QueueItem) (*QueueItem, error) {
	logger := p.queueItemLogger(item)
	logger.Info().Msg("starting queue item processing")

	cfg, err := p.store.LoadConfig(ctx)
	if err != nil {
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("load backend config: %v", err), err, "", "", "")
	}

	usedVisionModel := cfg.LLMs.VisionLLM
	if usedVisionModel == "" {
		usedVisionModel = cfg.LLMs.DefaultLLM
	}

	if cfg.Paperless.PaperlessURL == "" || cfg.Paperless.PaperlessToken == "" {
		return p.failQueueItem(ctx, logger, item, "paperless configuration is incomplete", nil, "", cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	if item.DocumentID == nil {
		return p.failQueueItem(ctx, logger, item, "queue item does not contain a paperless document ID", nil, "", cfg.LLMs.DefaultLLM, usedVisionModel)
	}

	logger.Info().
		Str("ollama_url", cfg.LLMs.OllamaURL).
		Str("default_llm", cfg.LLMs.DefaultLLM).
		Str("vision_llm", usedVisionModel).
		Msg("loaded processor configuration")

	client := paperless.NewClient(cfg.Paperless.PaperlessURL, cfg.Paperless.PaperlessToken)

	logger.Info().Msg("loading paperless tags")
	tags, err := client.ListTags(ctx)
	if err != nil {
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("list paperless tags: %v", err), err, "", cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	logger.Info().Int("available_tag_count", len(tags)).Msg("loaded paperless tags")

	logger.Info().Msg("loading paperless document")
	document, err := client.GetDocument(ctx, *item.DocumentID)
	if errors.Is(err, os.ErrNotExist) {
		result := ProcessingResult{
			Notes: []string{"document no longer exists in paperless"},
		}
		logger.Warn().Msg("skipping queue item because document no longer exists in paperless")
		return p.store.MarkQueueItemCompleted(ctx, item.ID, "Skipped processing because the document no longer exists in Paperless.", result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
	}
	if err != nil {
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("load paperless document: %v", err), err, "", cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	logger.Info().
		Str("document_title", document.Title).
		Str("original_file_name", document.OriginalFileName).
		Int("document_tag_count", len(document.TagIDs)).
		Msg("loaded paperless document")

	tagNameSet, tagNames := buildTagNameSet(tags, document.TagIDs)
	plan := buildProcessingPlan(cfg.Process, tagNameSet)
	result := newProcessingResult(document, tagNames, plan)
	logger.Info().
		Strs("document_tags", tagNames).
		Bool("trigger_tag_present", plan.TriggerTagPresent).
		Bool("force_ocr", plan.ForceOCR).
		Bool("force_vision", plan.ForceVision).
		Bool("process_correspondent", plan.Correspondent).
		Bool("process_document_type", plan.DocumentType).
		Bool("process_document_tags", plan.DocumentTags).
		Msg("built processing plan")
	if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist initial progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
	}

	if !plan.TriggerTagPresent {
		result.Notes = append(result.Notes, "trigger tag is no longer present on the live document")
		logger.Warn().Strs("document_tags", tagNames).Msg("skipping queue item because trigger tag is missing on live document")
		return p.store.MarkQueueItemCompleted(ctx, item.ID, "Skipped processing because the trigger tag is no longer present on the document.", result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
	}
	if !plan.HasRequestedWork() {
		result.Notes = append(result.Notes, "no actionable processing tags were present on the live document")
		logger.Warn().Strs("document_tags", tagNames).Msg("skipping queue item because no actionable processing tags are present")
		return p.store.MarkQueueItemCompleted(ctx, item.ID, "Skipped processing because no actionable processing tags were present on the document.", result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
	}

	usesDefaultModel := plan.Correspondent || plan.DocumentType || plan.DocumentTags
	requiresModelForExtraction := plan.ForceVision || documentNeedsModelExtraction(document)
	if (usesDefaultModel || requiresModelForExtraction) && strings.TrimSpace(cfg.LLMs.DefaultLLM) == "" {
		return p.failQueueItem(ctx, logger, item, "default_llm must be configured for the requested processing stages", nil, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	if plan.ForceVision && strings.TrimSpace(usedVisionModel) == "" {
		return p.failQueueItem(ctx, logger, item, "vision_llm or default_llm must be configured for forced vision extraction", nil, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
	}

	tempDir, err := os.MkdirTemp("", "paperless-ai-ext-backend-*")
	if err != nil {
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("create temp directory: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	defer os.RemoveAll(tempDir)

	logger.Info().Str("temp_dir", tempDir).Msg("created temporary processing directory")
	logger.Info().Msg("downloading paperless document")
	downloaded, err := client.DownloadDocument(ctx, document.ID, tempDir)
	if errors.Is(err, os.ErrNotExist) {
		result.Notes = append(result.Notes, "document download endpoint returned not found")
		logger.Warn().Msg("skipping queue item because document download endpoint returned not found")
		return p.store.MarkQueueItemCompleted(ctx, item.ID, "Skipped processing because the Paperless download endpoint no longer had the document.", result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
	}
	if err != nil {
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("download paperless document: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	logger.Info().
		Str("download_path", downloaded.Path).
		Str("download_file_name", downloaded.FileName).
		Str("download_content_type", downloaded.ContentType).
		Int64("download_size_bytes", downloaded.SizeBytes).
		Msg("downloaded paperless document")

	logger.Info().
		Bool("force_vision", plan.ForceVision).
		Bool("allow_vision_fallback", true).
		Msg("starting extraction stage")
	result.Extraction = ExtractionStageResult{Status: stageStatusRunning}
	if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist extraction progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	extraction, err := classification.ExtractDocumentText(ctx, downloaded.Path, classification.ExtractionOptions{
		OllamaURL:           cfg.LLMs.OllamaURL,
		OCRModel:            cfg.LLMs.DefaultLLM,
		VisionModel:         usedVisionModel,
		VisionMaxPages:      5,
		ForceVision:         plan.ForceVision,
		AllowVisionFallback: true,
	})
	if err != nil {
		result.Extraction = ExtractionStageResult{Status: stageStatusFailed, Error: err.Error()}
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("extract document text: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	result.Extraction = ExtractionStageResult{
		Status:      stageStatusCompleted,
		Source:      extraction.Source,
		UsedModel:   extraction.UsedModel,
		TextLength:  len(extraction.Text),
		TextPreview: extractedTextPreview(extraction.Text),
	}
	if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
		return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist extraction progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
	}
	logger.Info().
		Str("extraction_source", extraction.Source).
		Str("extraction_model", extraction.UsedModel).
		Int("text_length", len(extraction.Text)).
		Msg("completed extraction stage")

	if plan.Correspondent {
		logger.Info().Msg("starting correspondent suggestion stage")
		result.Correspondent = SuggestionStageResult{Status: stageStatusRunning, UsedModel: cfg.LLMs.DefaultLLM}
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist correspondent progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		correspondents, err := client.ListCorrespondents(ctx)
		if err != nil {
			result.Correspondent = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("list correspondents: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		logger.Info().Int("available_correspondent_count", len(correspondents)).Msg("loaded correspondents for suggestion stage")

		historicalDocuments, err := client.ListDocuments(ctx, paperless.DocumentFilter{Limit: 200, Ordering: "-created"})
		if err != nil {
			logger.Warn().Err(err).Msg("failed to load historical documents for correspondent ranking; continuing without library evidence")
			historicalDocuments = nil
		} else if len(historicalDocuments) > 0 {
			filteredDocuments := historicalDocuments[:0]
			for _, historicalDocument := range historicalDocuments {
				if historicalDocument.ID == document.ID {
					continue
				}
				filteredDocuments = append(filteredDocuments, historicalDocument)
			}
			historicalDocuments = filteredDocuments
			logger.Info().Int("historical_document_count", len(historicalDocuments)).Msg("loaded historical documents for correspondent ranking")
		}

		suggestion, err := classification.SuggestCorrespondent(ctx, cfg.LLMs.OllamaURL, cfg.LLMs.DefaultLLM, processorDocumentName(document, downloaded.Path), extraction.Text, correspondents, historicalDocuments)
		if err != nil {
			result.Correspondent = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("suggest correspondent: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}

		result.Correspondent = suggestionStageResult(stageStatusCompleted, cfg.LLMs.DefaultLLM, correspondentStagePayload(suggestion), suggestion.Confidence, suggestion.Reasoning)
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist correspondent progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		logger.Info().
			Str("confidence", suggestion.Confidence).
			Str("reasoning", truncateLogValue(suggestion.Reasoning, 240)).
			Str("correspondent_name", pointerString(suggestion.CorrespondentName)).
			Str("suggested_new_correspondent", pointerString(suggestion.SuggestedNewCorrespondent)).
			Msg("completed correspondent suggestion stage")
	}

	if plan.DocumentType {
		logger.Info().Msg("starting document type suggestion stage")
		result.DocumentType = SuggestionStageResult{Status: stageStatusRunning, UsedModel: cfg.LLMs.DefaultLLM}
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist document type progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		documentTypes, err := client.ListDocumentTypes(ctx)
		if err != nil {
			result.DocumentType = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("list document types: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		logger.Info().Int("available_document_type_count", len(documentTypes)).Msg("loaded document types for suggestion stage")

		suggestion, err := classification.SuggestDocumentType(ctx, cfg.LLMs.OllamaURL, cfg.LLMs.DefaultLLM, processorDocumentName(document, downloaded.Path), extraction.Text, documentTypes)
		if err != nil {
			result.DocumentType = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("suggest document type: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}

		result.DocumentType = suggestionStageResult(stageStatusCompleted, cfg.LLMs.DefaultLLM, documentTypeStagePayload(suggestion), suggestion.Confidence, suggestion.Reasoning)
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist document type progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		logger.Info().
			Str("confidence", suggestion.Confidence).
			Str("reasoning", truncateLogValue(suggestion.Reasoning, 240)).
			Str("document_type_name", pointerString(suggestion.DocumentTypeName)).
			Str("suggested_new_document_type", pointerString(suggestion.SuggestedNewDocumentType)).
			Msg("completed document type suggestion stage")
	}

	if plan.DocumentTags {
		logger.Info().Int("available_tag_count", len(tags)).Msg("starting tag suggestion stage")
		result.Tags = SuggestionStageResult{Status: stageStatusRunning, UsedModel: cfg.LLMs.DefaultLLM}
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist tag progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		suggestion, err := classification.SuggestTags(ctx, cfg.LLMs.OllamaURL, cfg.LLMs.DefaultLLM, processorDocumentName(document, downloaded.Path), extraction.Text, tags)
		if err != nil {
			result.Tags = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("suggest tags: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}

		result.Tags = suggestionStageResult(stageStatusCompleted, cfg.LLMs.DefaultLLM, tagsStagePayload(suggestion), suggestion.Confidence, suggestion.Reasoning)
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist tag progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		logger.Info().
			Str("confidence", suggestion.Confidence).
			Str("reasoning", truncateLogValue(suggestion.Reasoning, 240)).
			Strs("selected_tags", suggestion.TagNames).
			Strs("suggested_new_tags", suggestion.SuggestedNewTags).
			Msg("completed tag suggestion stage")
	}

	resultSummary := summarizeProcessingResult(result)
	logger.Info().
		Str("result_summary", resultSummary).
		Str("result_payload", truncateLogValue(result.Marshal(), 4000)).
		Msg("processed queue item")
	return p.store.MarkQueueItemCompleted(ctx, item.ID, resultSummary, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
}

func (p *Processor) queueItemLogger(item *QueueItem) zerolog.Logger {
	contextLogger := p.logger.With().Int64("queue_item_id", item.ID)
	if item.DocumentID != nil {
		contextLogger = contextLogger.Int64("document_id", *item.DocumentID)
	}
	if strings.TrimSpace(item.DocumentTitle) != "" {
		contextLogger = contextLogger.Str("queue_document_title", item.DocumentTitle)
	}
	return contextLogger.Logger()
}

func (p *Processor) persistQueueProgress(ctx context.Context, logger zerolog.Logger, item *QueueItem, result ProcessingResult, usedLLM string, usedVisionLLM string) (*QueueItem, error) {
	summary := summarizeProcessingProgress(result)
	updatedItem, err := p.store.UpdateQueueItemProgress(ctx, item.ID, summary, result.Marshal(), usedLLM, usedVisionLLM)
	if err != nil {
		return nil, err
	}
	logger.Debug().Str("progress_summary", summary).Msg("persisted queue item progress")
	return updatedItem, nil
}

func (p *Processor) failQueueItem(ctx context.Context, logger zerolog.Logger, item *QueueItem, lastError string, cause error, resultPayload string, usedLLM string, usedVisionLLM string) (*QueueItem, error) {
	event := logger.Error().Str("last_error", lastError)
	if cause != nil {
		event = event.Err(cause)
	}
	if resultPayload != "" {
		event = event.Str("result_payload", truncateLogValue(resultPayload, 4000))
	}
	event.Msg("queue item processing failed")
	return p.store.MarkQueueItemFailed(ctx, item.ID, lastError, resultPayload, usedLLM, usedVisionLLM, item.StartedAtMS)
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func truncateLogValue(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if maxLen <= 0 || len(trimmed) <= maxLen {
		return trimmed
	}
	if maxLen <= 3 {
		return trimmed[:maxLen]
	}
	return trimmed[:maxLen-3] + "..."
}

func documentNeedsModelExtraction(document *paperless.Document) bool {
	fileName := strings.ToLower(strings.TrimSpace(document.OriginalFileName))
	ext := strings.ToLower(filepath.Ext(fileName))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
}

func processorDocumentName(document *paperless.Document, downloadedPath string) string {
	if document != nil {
		if strings.TrimSpace(document.Title) != "" {
			return document.Title
		}
		if strings.TrimSpace(document.OriginalFileName) != "" {
			return document.OriginalFileName
		}
	}
	return filepath.Base(downloadedPath)
}

func summarizeProcessingResult(result ProcessingResult) string {
	completed := make([]string, 0, 4)
	if result.Extraction.Status == stageStatusCompleted {
		completed = append(completed, "text extraction")
	}
	if result.Correspondent.Status == stageStatusCompleted {
		completed = append(completed, "correspondent suggestion")
	}
	if result.DocumentType.Status == stageStatusCompleted {
		completed = append(completed, "document type suggestion")
	}
	if result.Tags.Status == stageStatusCompleted {
		completed = append(completed, "tag suggestion")
	}
	if len(completed) == 0 {
		return "Processed queue item without any completed suggestion stages."
	}
	return "Completed " + strings.Join(completed, ", ") + "."
}

func summarizeProcessingProgress(result ProcessingResult) string {
	if stageLabel := currentRunningStageLabel(result); stageLabel != "" {
		return "Running " + stageLabel + "."
	}

	pending := pendingStageLabels(result)
	if len(pending) > 0 {
		return "Pending " + strings.Join(pending, ", ") + "."
	}

	return summarizeProcessingResult(result)
}

func currentRunningStageLabel(result ProcessingResult) string {
	switch {
	case result.Extraction.Status == stageStatusRunning:
		return "text extraction"
	case result.Correspondent.Status == stageStatusRunning:
		return "correspondent suggestion"
	case result.DocumentType.Status == stageStatusRunning:
		return "document type suggestion"
	case result.Tags.Status == stageStatusRunning:
		return "tag suggestion"
	default:
		return ""
	}
}

func pendingStageLabels(result ProcessingResult) []string {
	labels := make([]string, 0, 4)
	if result.Extraction.Status == stageStatusPending {
		labels = append(labels, "text extraction")
	}
	if result.Correspondent.Status == stageStatusPending {
		labels = append(labels, "correspondent suggestion")
	}
	if result.DocumentType.Status == stageStatusPending {
		labels = append(labels, "document type suggestion")
	}
	if result.Tags.Status == stageStatusPending {
		labels = append(labels, "tag suggestion")
	}
	return labels
}
