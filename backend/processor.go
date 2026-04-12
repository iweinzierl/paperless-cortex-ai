package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"paperless-ai-ext/internal/classification"
	"paperless-ai-ext/internal/paperless"

	"github.com/rs/zerolog"
)

type Processor struct {
	store            *Store
	logger           zerolog.Logger
	waitForNextCycle func(context.Context, time.Duration) bool
}

var errQueueItemNotApplyable = errors.New("queue item is not applyable")
var errQueueItemAlreadyApplied = errors.New("queue item suggestions were already applied")

func NewProcessor(store *Store, logger zerolog.Logger) *Processor {
	return &Processor{
		store:            store,
		logger:           logger,
		waitForNextCycle: waitForNextCycle,
	}
}

func (p *Processor) Start(ctx context.Context) {
	go func() {
		for {
			waitInterval := p.loadProcessingWaitInterval(ctx)
			if !p.waitForNextCycle(ctx, waitInterval) {
				return
			}

			cfg, err := p.store.LoadConfig(ctx)
			if err != nil {
				p.logger.Error().Err(err).Msg("failed to load backend config for processor loop")
				continue
			}

			if cfg.Engine.ProcessingMode != ProcessingModeAuto {
				continue
			}

			p.drainAutomaticQueue(ctx)
		}
	}()
}

func (p *Processor) loadProcessingWaitInterval(ctx context.Context) time.Duration {
	cfg, err := p.store.LoadConfig(ctx)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to load backend config for processor wait interval")
		return 5 * time.Second
	}

	return processingWaitInterval(cfg)
}

func (p *Processor) drainAutomaticQueue(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		if _, err := p.ProcessNext(ctx); err != nil {
			if errors.Is(err, errNoPendingQueueItems) {
				return
			}
			p.logger.Error().Err(err).Msg("automatic queue processing failed")
		}

		cfg, err := p.store.LoadConfig(ctx)
		if err != nil {
			p.logger.Error().Err(err).Msg("failed to load backend config while draining automatic queue")
			return
		}
		if cfg.Engine.ProcessingMode != ProcessingModeAuto {
			return
		}
	}
}

func processingWaitInterval(cfg BackendConfig) time.Duration {
	waitInterval := 5 * time.Second
	if cfg.Engine.ProcessingIntervalSeconds > 0 {
		waitInterval = time.Duration(cfg.Engine.ProcessingIntervalSeconds) * time.Second
	}

	return waitInterval
}

func waitForNextCycle(ctx context.Context, waitInterval time.Duration) bool {
	timer := time.NewTimer(waitInterval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
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

func (p *Processor) ApplyByID(ctx context.Context, id int64) (*QueueItem, error) {
	item, err := p.store.GetQueueItem(ctx, id)
	if err != nil {
		return nil, err
	}

	cfg, err := p.store.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	return p.applyQueueItem(ctx, item, cfg)
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
		Bool("process_created_date", plan.CreatedDate).
		Bool("process_correspondent", plan.Correspondent).
		Bool("process_document_type", plan.DocumentType).
		Bool("process_document_tags", plan.DocumentTags).
		Bool("process_title", plan.Title).
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

	usesDefaultModel := plan.CreatedDate || plan.Correspondent || plan.DocumentType || plan.DocumentTags || plan.Title
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

	stageFailures := make([]string, 0, 5)
	recordStageFailure := func(stageLabel string, cause error) error {
		failureMessage := fmt.Sprintf("%s: %v", stageLabel, cause)
		stageFailures = append(stageFailures, failureMessage)
		logger.Error().Err(cause).Str("stage", stageLabel).Msg("processing stage failed; continuing with remaining stages")
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			_, failErr := p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist %s progress: %v", stageLabel, err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
			return failErr
		}
		return nil
	}

	if plan.Correspondent {
		logger.Info().Msg("starting correspondent suggestion stage")
		result.Correspondent = SuggestionStageResult{Status: stageStatusRunning, UsedModel: cfg.LLMs.DefaultLLM}
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist correspondent progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		correspondents, err := client.ListCorrespondents(ctx)
		if err != nil {
			result.Correspondent = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
			if recordErr := recordStageFailure("correspondent suggestion", err); recordErr != nil {
				return nil, recordErr
			}
		} else {
			result.Document.CorrespondentName = resolveNamedEntityName(correspondents, document.CorrespondentID, result.Document.CorrespondentName)
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
				if recordErr := recordStageFailure("correspondent suggestion", err); recordErr != nil {
					return nil, recordErr
				}
			} else {
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
		}
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
			if recordErr := recordStageFailure("document type suggestion", err); recordErr != nil {
				return nil, recordErr
			}
		} else {
			result.Document.DocumentTypeName = resolveNamedEntityName(documentTypes, document.DocumentTypeID, result.Document.DocumentTypeName)
			logger.Info().Int("available_document_type_count", len(documentTypes)).Msg("loaded document types for suggestion stage")

			suggestion, err := classification.SuggestDocumentType(ctx, cfg.LLMs.OllamaURL, cfg.LLMs.DefaultLLM, processorDocumentName(document, downloaded.Path), extraction.Text, documentTypes)
			if err != nil {
				result.DocumentType = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
				if recordErr := recordStageFailure("document type suggestion", err); recordErr != nil {
					return nil, recordErr
				}
			} else {
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
		}
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
			if recordErr := recordStageFailure("tag suggestion", err); recordErr != nil {
				return nil, recordErr
			}
		} else {
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
	}

	if plan.CreatedDate {
		logger.Info().Msg("starting creation date suggestion stage")
		result.CreatedDate = SuggestionStageResult{Status: stageStatusRunning, UsedModel: cfg.LLMs.DefaultLLM}
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist creation date progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		suggestion, err := classification.SuggestCreatedDate(ctx, cfg.LLMs.OllamaURL, cfg.LLMs.DefaultLLM, processorDocumentName(document, downloaded.Path), extraction.Text)
		if err != nil {
			result.CreatedDate = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
			if recordErr := recordStageFailure("creation date suggestion", err); recordErr != nil {
				return nil, recordErr
			}
		} else {
			result.CreatedDate = suggestionStageResult(stageStatusCompleted, cfg.LLMs.DefaultLLM, createdDateStagePayload(suggestion), suggestion.Confidence, suggestion.Reasoning)
			if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
				return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist creation date progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
			}
			logger.Info().
				Str("confidence", suggestion.Confidence).
				Str("reasoning", truncateLogValue(suggestion.Reasoning, 240)).
				Str("created", pointerString(suggestion.Created)).
				Msg("completed creation date suggestion stage")
		}
	}

	if plan.Title {
		logger.Info().Msg("starting title suggestion stage")
		result.Title = SuggestionStageResult{Status: stageStatusRunning, UsedModel: cfg.LLMs.DefaultLLM}
		if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
			return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist title progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
		}
		suggestion, err := classification.SuggestTitle(ctx, cfg.LLMs.OllamaURL, cfg.LLMs.DefaultLLM, processorDocumentName(document, downloaded.Path), extraction.Text)
		if err != nil {
			result.Title = SuggestionStageResult{Status: stageStatusFailed, Error: err.Error(), UsedModel: cfg.LLMs.DefaultLLM}
			if recordErr := recordStageFailure("title suggestion", err); recordErr != nil {
				return nil, recordErr
			}
		} else {
			result.Title = suggestionStageResult(stageStatusCompleted, cfg.LLMs.DefaultLLM, titleStagePayload(suggestion), suggestion.Confidence, suggestion.Reasoning)
			if _, err := p.persistQueueProgress(ctx, logger, item, result, cfg.LLMs.DefaultLLM, usedVisionModel); err != nil {
				return p.failQueueItem(ctx, logger, item, fmt.Sprintf("persist title progress: %v", err), err, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel)
			}
			logger.Info().
				Str("confidence", suggestion.Confidence).
				Str("reasoning", truncateLogValue(suggestion.Reasoning, 240)).
				Str("title", pointerString(suggestion.Title)).
				Msg("completed title suggestion stage")
		}
	}

	resultSummary := summarizeProcessingResult(result)
	logger.Info().
		Str("result_summary", resultSummary).
		Str("result_payload", truncateLogValue(result.Marshal(), 4000)).
		Msg("processed queue item")
	var finalItem *QueueItem
	if len(stageFailures) > 0 {
		finalItem, err = p.store.MarkQueueItemPartiallyCompleted(ctx, item.ID, resultSummary, strings.Join(stageFailures, "; "), result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
	} else {
		finalItem, err = p.store.MarkQueueItemCompleted(ctx, item.ID, resultSummary, result.Marshal(), cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
	}
	if err != nil {
		return nil, err
	}
	if cfg.Engine.ProcessingMode != ProcessingModeAuto {
		return finalItem, nil
	}

	appliedItem, applyErr := p.applyQueueItem(ctx, finalItem, cfg)
	if applyErr != nil {
		logger.Error().Err(applyErr).Msg("automatic suggestion apply failed")
		if appliedItem != nil {
			return appliedItem, nil
		}
		return finalItem, nil
	}
	return appliedItem, nil
}

func (p *Processor) applyQueueItem(ctx context.Context, item *QueueItem, cfg BackendConfig) (*QueueItem, error) {
	logger := p.queueItemLogger(item)
	if item == nil {
		return nil, errQueueItemNotFound
	}
	if item.ApplyStatus == "applied" {
		return nil, errQueueItemAlreadyApplied
	}
	if item.Status != queueItemStatusCompleted && item.Status != queueItemStatusPartiallyCompleted {
		return nil, errQueueItemNotApplyable
	}
	if item.DocumentID == nil || strings.TrimSpace(item.ResultPayload) == "" {
		return nil, errQueueItemNotApplyable
	}

	var result ProcessingResult
	if err := json.Unmarshal([]byte(item.ResultPayload), &result); err != nil {
		failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, fmt.Sprintf("decode processing result: %v", err))
		if markErr != nil {
			return nil, markErr
		}
		return failedItem, fmt.Errorf("decode processing result: %w", err)
	}

	if !hasCompletedSuggestion(result) && strings.TrimSpace(cfg.Process.ProcessCompletedTag) == "" {
		return nil, errQueueItemNotApplyable
	}
	if cfg.Paperless.PaperlessURL == "" || cfg.Paperless.PaperlessToken == "" {
		failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, "paperless configuration is incomplete")
		if markErr != nil {
			return nil, markErr
		}
		return failedItem, errors.New("paperless configuration is incomplete")
	}

	client := paperless.NewClient(cfg.Paperless.PaperlessURL, cfg.Paperless.PaperlessToken)
	liveDocument, err := client.GetDocument(ctx, *item.DocumentID)
	if err != nil {
		applyErr := fmt.Errorf("load paperless document: %w", err)
		failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
		if markErr != nil {
			return nil, markErr
		}
		return failedItem, applyErr
	}

	tags, err := client.ListTags(ctx)
	if err != nil {
		applyErr := fmt.Errorf("list paperless tags: %w", err)
		failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
		if markErr != nil {
			return nil, markErr
		}
		return failedItem, applyErr
	}

	patch := paperless.DocumentPatch{}
	appliedParts := make([]string, 0, 6)

	if result.CreatedDate.Status == stageStatusCompleted {
		created, err := resolveAppliedCreatedDate(result.CreatedDate)
		if err != nil {
			applyErr := fmt.Errorf("resolve created-date suggestion: %w", err)
			failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
			if markErr != nil {
				return nil, markErr
			}
			return failedItem, applyErr
		}
		if strings.TrimSpace(liveDocument.Created) != created {
			patch.Created = stringPointer(created)
			appliedParts = append(appliedParts, fmt.Sprintf("created date %q", created))
		}
	}

	if result.Title.Status == stageStatusCompleted {
		title, err := resolveAppliedTitle(result.Title)
		if err != nil {
			applyErr := fmt.Errorf("resolve title suggestion: %w", err)
			failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
			if markErr != nil {
				return nil, markErr
			}
			return failedItem, applyErr
		}
		if strings.TrimSpace(liveDocument.Title) != title {
			patch.Title = stringPointer(title)
			appliedParts = append(appliedParts, fmt.Sprintf("title %q", title))
		}
	}

	if result.Correspondent.Status == stageStatusCompleted {
		correspondents, err := client.ListCorrespondents(ctx)
		if err != nil {
			applyErr := fmt.Errorf("list correspondents: %w", err)
			failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
			if markErr != nil {
				return nil, markErr
			}
			return failedItem, applyErr
		}

		targetID, resolved, err := resolveAppliedCorrespondent(ctx, client, correspondents, result.Correspondent)
		if err != nil {
			applyErr := fmt.Errorf("resolve correspondent suggestion: %w", err)
			failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
			if markErr != nil {
				return nil, markErr
			}
			return failedItem, applyErr
		}
		if targetID != nil && !sameOptionalInt64(liveDocument.CorrespondentID, targetID) {
			patch.CorrespondentID = targetID
			appliedParts = append(appliedParts, fmt.Sprintf("correspondent %q", resolved))
		}
	}

	if result.DocumentType.Status == stageStatusCompleted {
		documentTypes, err := client.ListDocumentTypes(ctx)
		if err != nil {
			applyErr := fmt.Errorf("list document types: %w", err)
			failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
			if markErr != nil {
				return nil, markErr
			}
			return failedItem, applyErr
		}

		targetID, resolved, err := resolveAppliedDocumentType(ctx, client, documentTypes, result.DocumentType)
		if err != nil {
			applyErr := fmt.Errorf("resolve document type suggestion: %w", err)
			failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
			if markErr != nil {
				return nil, markErr
			}
			return failedItem, applyErr
		}
		if targetID != nil && !sameOptionalInt64(liveDocument.DocumentTypeID, targetID) {
			patch.DocumentTypeID = targetID
			appliedParts = append(appliedParts, fmt.Sprintf("document type %q", resolved))
		}
	}

	finalTagIDs := append([]int64(nil), liveDocument.TagIDs...)
	if result.Tags.Status == stageStatusCompleted {
		resolvedTagIDs, err := resolveAppliedTags(ctx, client, &tags, result.Tags)
		if err != nil {
			applyErr := fmt.Errorf("resolve tag suggestion: %w", err)
			failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
			if markErr != nil {
				return nil, markErr
			}
			return failedItem, applyErr
		}
		finalTagIDs = mergeInt64Lists(finalTagIDs, resolvedTagIDs)
		if len(resolvedTagIDs) > 0 {
			appliedParts = append(appliedParts, "tags")
		}
	}
	if completedTag := strings.TrimSpace(cfg.Process.ProcessCompletedTag); completedTag != "" {
		completedTagID, err := resolveOrCreateTag(ctx, client, &tags, completedTag)
		if err != nil {
			applyErr := fmt.Errorf("resolve completed tag: %w", err)
			failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
			if markErr != nil {
				return nil, markErr
			}
			return failedItem, applyErr
		}
		finalTagIDs = mergeInt64Lists(finalTagIDs, []int64{completedTagID})
		appliedParts = append(appliedParts, fmt.Sprintf("completed tag %q", completedTag))
	}
	finalTagIDs = removeStateTagIDs(finalTagIDs, tags, cfg.Process)
	if !sameInt64Set(liveDocument.TagIDs, finalTagIDs) {
		patch.TagIDs = finalTagIDs
	}

	if patch.Title == nil && patch.Created == nil && patch.CorrespondentID == nil && patch.DocumentTypeID == nil && len(patch.TagIDs) == 0 {
		summary := "Suggestions already matched the live Paperless document."
		if len(appliedParts) > 0 {
			summary = "Apply confirmed live Paperless metadata is already up to date."
		}
		return p.store.MarkQueueItemApplied(ctx, item.ID, summary)
	}

	if _, err := client.PatchDocument(ctx, *item.DocumentID, patch); err != nil {
		applyErr := fmt.Errorf("patch paperless document: %w", err)
		failedItem, markErr := p.store.MarkQueueItemApplyFailed(ctx, item.ID, applyErr.Error())
		if markErr != nil {
			return nil, markErr
		}
		return failedItem, applyErr
	}

	summary := "Applied processing suggestions to the Paperless document."
	if len(appliedParts) > 0 {
		summary = "Applied " + strings.Join(appliedParts, ", ") + " to the Paperless document."
	}
	logger.Info().Str("applied_summary", summary).Msg("applied processing suggestions to paperless")
	return p.store.MarkQueueItemApplied(ctx, item.ID, summary)
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

func hasCompletedSuggestion(result ProcessingResult) bool {
	return result.CreatedDate.Status == stageStatusCompleted ||
		result.Correspondent.Status == stageStatusCompleted ||
		result.DocumentType.Status == stageStatusCompleted ||
		result.Tags.Status == stageStatusCompleted ||
		result.Title.Status == stageStatusCompleted
}

func resolveAppliedCreatedDate(stage SuggestionStageResult) (string, error) {
	var suggestion classification.CreatedDateSuggestion
	if err := decodeSuggestionPayload(stage.Payload, &suggestion); err != nil {
		return "", err
	}
	if suggestion.Created == nil || strings.TrimSpace(*suggestion.Created) == "" {
		return "", errors.New("created-date suggestion did not contain an applyable value")
	}
	return strings.TrimSpace(*suggestion.Created), nil
}

func resolveAppliedTitle(stage SuggestionStageResult) (string, error) {
	var suggestion classification.TitleSuggestion
	if err := decodeSuggestionPayload(stage.Payload, &suggestion); err != nil {
		return "", err
	}
	if suggestion.Title == nil || strings.TrimSpace(*suggestion.Title) == "" {
		return "", errors.New("title suggestion did not contain an applyable value")
	}
	return strings.TrimSpace(*suggestion.Title), nil
}

func resolveAppliedCorrespondent(ctx context.Context, client *paperless.Client, correspondents []paperless.Correspondent, stage SuggestionStageResult) (*int64, string, error) {
	var suggestion classification.CorrespondentSuggestion
	if err := decodeSuggestionPayload(stage.Payload, &suggestion); err != nil {
		return nil, "", err
	}
	if suggestion.CorrespondentID != nil && suggestion.CorrespondentName != nil {
		return suggestion.CorrespondentID, strings.TrimSpace(*suggestion.CorrespondentName), nil
	}
	name := pointerString(suggestion.SuggestedNewCorrespondent)
	if name == "" {
		return nil, "", errors.New("correspondent suggestion did not contain an applyable value")
	}
	trimmed := strings.TrimSpace(name)
	for _, candidate := range correspondents {
		if strings.EqualFold(strings.TrimSpace(candidate.Name), trimmed) {
			return int64Pointer(candidate.ID), candidate.Name, nil
		}
	}
	created, err := client.CreateCorrespondent(ctx, trimmed)
	if err != nil {
		return nil, "", err
	}
	return int64Pointer(created.ID), created.Name, nil
}

func resolveAppliedDocumentType(ctx context.Context, client *paperless.Client, documentTypes []paperless.DocumentType, stage SuggestionStageResult) (*int64, string, error) {
	var suggestion classification.DocumentTypeSuggestion
	if err := decodeSuggestionPayload(stage.Payload, &suggestion); err != nil {
		return nil, "", err
	}
	if suggestion.DocumentTypeID != nil && suggestion.DocumentTypeName != nil {
		return suggestion.DocumentTypeID, strings.TrimSpace(*suggestion.DocumentTypeName), nil
	}
	name := pointerString(suggestion.SuggestedNewDocumentType)
	if name == "" {
		return nil, "", errors.New("document type suggestion did not contain an applyable value")
	}
	trimmed := strings.TrimSpace(name)
	for _, candidate := range documentTypes {
		if strings.EqualFold(strings.TrimSpace(candidate.Name), trimmed) {
			return int64Pointer(candidate.ID), candidate.Name, nil
		}
	}
	created, err := client.CreateDocumentType(ctx, trimmed)
	if err != nil {
		return nil, "", err
	}
	return int64Pointer(created.ID), created.Name, nil
}

func resolveAppliedTags(ctx context.Context, client *paperless.Client, existingTags *[]paperless.Tag, stage SuggestionStageResult) ([]int64, error) {
	var suggestion classification.TagSuggestion
	if err := decodeSuggestionPayload(stage.Payload, &suggestion); err != nil {
		return nil, err
	}
	resolved := append([]int64(nil), suggestion.TagIDs...)
	for _, tagName := range classification.NormalizeStringList(suggestion.SuggestedNewTags) {
		tagID, err := resolveOrCreateTag(ctx, client, existingTags, tagName)
		if err != nil {
			return nil, err
		}
		resolved = mergeInt64Lists(resolved, []int64{tagID})
	}
	return resolved, nil
}

func resolveOrCreateTag(ctx context.Context, client *paperless.Client, tags *[]paperless.Tag, name string) (int64, error) {
	trimmed := strings.TrimSpace(name)
	for _, tag := range *tags {
		if strings.EqualFold(strings.TrimSpace(tag.Name), trimmed) {
			return tag.ID, nil
		}
	}
	created, err := client.CreateTag(ctx, trimmed)
	if err != nil {
		return 0, err
	}
	*tags = append(*tags, *created)
	return created.ID, nil
}

func decodeSuggestionPayload(payload any, target any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal suggestion payload: %w", err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		return fmt.Errorf("decode suggestion payload: %w", err)
	}
	return nil
}

func int64Pointer(value int64) *int64 {
	return &value
}

func stringPointer(value string) *string {
	return &value
}

func sameOptionalInt64(left *int64, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func mergeInt64Lists(base []int64, extra []int64) []int64 {
	seen := make(map[int64]struct{}, len(base)+len(extra))
	merged := make([]int64, 0, len(base)+len(extra))
	for _, value := range append(append([]int64(nil), base...), extra...) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	sort.Slice(merged, func(i int, j int) bool {
		return merged[i] < merged[j]
	})
	return merged
}

func sameInt64Set(left []int64, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]int64(nil), left...)
	rightCopy := append([]int64(nil), right...)
	sort.Slice(leftCopy, func(i int, j int) bool { return leftCopy[i] < leftCopy[j] })
	sort.Slice(rightCopy, func(i int, j int) bool { return rightCopy[i] < rightCopy[j] })
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}
	return true
}

func removeStateTagIDs(tagIDs []int64, tags []paperless.Tag, cfg ProcessConfig) []int64 {
	stateTagNames := processingStateTagNames(cfg)
	if len(stateTagNames) == 0 || len(tagIDs) == 0 {
		return tagIDs
	}

	stateTagIDs := make(map[int64]struct{}, len(tags))
	for _, tag := range tags {
		if _, ok := stateTagNames[strings.ToLower(strings.TrimSpace(tag.Name))]; ok {
			stateTagIDs[tag.ID] = struct{}{}
		}
	}
	if len(stateTagIDs) == 0 {
		return tagIDs
	}

	filtered := make([]int64, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		if _, ok := stateTagIDs[tagID]; ok {
			continue
		}
		filtered = append(filtered, tagID)
	}
	return filtered
}

func processingStateTagNames(cfg ProcessConfig) map[string]struct{} {
	values := []string{
		cfg.ProcessTriggerTag,
		cfg.ForceOCRTag,
		cfg.ForceVisionTag,
		cfg.ProcessCreatedDateTag,
		cfg.ProcessCorrespondentTag,
		cfg.ProcessDocumentTypeTag,
		cfg.ProcessDocumentTagsTag,
		cfg.ProcessTitleTag,
	}

	stateTagNames := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		stateTagNames[trimmed] = struct{}{}
	}
	return stateTagNames
}

func documentNeedsModelExtraction(document *paperless.Document) bool {
	fileName := strings.ToLower(strings.TrimSpace(document.OriginalFileName))
	ext := strings.ToLower(filepath.Ext(fileName))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
}

func resolveNamedEntityName[T paperless.NamedEntity](items []T, id *int64, fallback string) string {
	if trimmedFallback := strings.TrimSpace(fallback); trimmedFallback != "" {
		return trimmedFallback
	}
	if id == nil {
		return ""
	}
	for _, item := range items {
		if item.GetID() == *id {
			return strings.TrimSpace(item.GetName())
		}
	}
	return ""
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
	completed := make([]string, 0, 6)
	if result.Extraction.Status == stageStatusCompleted {
		completed = append(completed, "text extraction")
	}
	if result.CreatedDate.Status == stageStatusCompleted {
		completed = append(completed, "creation date suggestion")
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
	if result.Title.Status == stageStatusCompleted {
		completed = append(completed, "title suggestion")
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
	case result.CreatedDate.Status == stageStatusRunning:
		return "creation date suggestion"
	case result.Correspondent.Status == stageStatusRunning:
		return "correspondent suggestion"
	case result.DocumentType.Status == stageStatusRunning:
		return "document type suggestion"
	case result.Tags.Status == stageStatusRunning:
		return "tag suggestion"
	case result.Title.Status == stageStatusRunning:
		return "title suggestion"
	default:
		return ""
	}
}

func pendingStageLabels(result ProcessingResult) []string {
	labels := make([]string, 0, 6)
	if result.Extraction.Status == stageStatusPending {
		labels = append(labels, "text extraction")
	}
	if result.CreatedDate.Status == stageStatusPending {
		labels = append(labels, "creation date suggestion")
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
	if result.Title.Status == stageStatusPending {
		labels = append(labels, "title suggestion")
	}
	return labels
}
