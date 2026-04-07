package main

import (
	"context"
	"fmt"
	"time"

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

func (p *Processor) execute(ctx context.Context, item *QueueItem) (*QueueItem, error) {
	cfg, err := p.store.LoadConfig(ctx)
	if err != nil {
		return p.store.MarkQueueItemFailed(ctx, item.ID, fmt.Sprintf("load backend config: %v", err), "", "", item.StartedAtMS)
	}

	usedVisionModel := cfg.LLMs.VisionLLM
	if usedVisionModel == "" {
		usedVisionModel = cfg.LLMs.DefaultLLM
	}

	if cfg.Paperless.PaperlessURL == "" || cfg.Paperless.PaperlessToken == "" {
		return p.store.MarkQueueItemFailed(ctx, item.ID, "paperless configuration is incomplete", cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
	}

	resultSummary := "Initial backend queue processing is wired up, but document enrichment and paperless updates are not implemented yet."
	p.logger.Info().Int64("queue_item_id", item.ID).Msg("processed queue item with bootstrap processor")
	return p.store.MarkQueueItemCompleted(ctx, item.ID, resultSummary, cfg.LLMs.DefaultLLM, usedVisionModel, item.StartedAtMS)
}
