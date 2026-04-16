package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"paperless-ai-ext/internal/classification"
	"paperless-ai-ext/internal/ollama"
	"paperless-ai-ext/internal/paperless"
)

type SimilarDocument struct {
	Record     DocumentEmbeddingRecord
	Similarity float64
}

type RetrievalService struct {
	store *Store
}

func NewRetrievalService(store *Store) *RetrievalService {
	return &RetrievalService{store: store}
}

func (service *RetrievalService) FindSimilarDocuments(ctx context.Context, cfg BackendConfig, client *paperless.Client, currentDocument *paperless.Document, queryText string, correspondentsByID map[int64]string, documentTypesByID map[int64]string, tagsByID map[int64]string) ([]SimilarDocument, error) {
	if !cfg.LLMs.Embeddings.Enabled {
		return nil, nil
	}
	if strings.TrimSpace(cfg.LLMs.Embeddings.Model) == "" {
		return nil, nil
	}
	if strings.TrimSpace(cfg.LLMs.OllamaURL) == "" {
		return nil, nil
	}

	queryPayload := buildEmbeddingInput(*currentDocument, queryText, correspondentsByID, documentTypesByID, tagsByID)
	queryVector, err := ollama.Embed(ctx, cfg.LLMs.OllamaURL, cfg.LLMs.Embeddings.Model, queryPayload)
	if err != nil {
		return nil, fmt.Errorf("embed query document: %w", err)
	}

	if _, _, err := service.SyncEmbeddings(ctx, cfg, client, correspondentsByID, documentTypesByID, tagsByID); err != nil {
		return nil, err
	}

	records, err := service.store.ListDocumentEmbeddings(ctx, cfg.LLMs.Embeddings.HistoricalDocumentLimit)
	if err != nil {
		return nil, fmt.Errorf("list indexed embeddings: %w", err)
	}

	similar := make([]SimilarDocument, 0, cfg.LLMs.Embeddings.TopK)
	for _, record := range records {
		if record.DocumentID == currentDocument.ID {
			continue
		}
		score := cosineSimilarity(queryVector, record.Embedding)
		if score < cfg.LLMs.Embeddings.SimilarityThreshold {
			continue
		}
		similar = append(similar, SimilarDocument{Record: record, Similarity: score})
	}

	sort.SliceStable(similar, func(left int, right int) bool {
		if similar[left].Similarity != similar[right].Similarity {
			return similar[left].Similarity > similar[right].Similarity
		}
		return similar[left].Record.DocumentID > similar[right].Record.DocumentID
	})

	if len(similar) > cfg.LLMs.Embeddings.TopK {
		similar = similar[:cfg.LLMs.Embeddings.TopK]
	}

	return similar, nil
}

func (service *RetrievalService) SyncEmbeddings(ctx context.Context, cfg BackendConfig, client *paperless.Client, correspondentsByID map[int64]string, documentTypesByID map[int64]string, tagsByID map[int64]string) (int, int, error) {
	if !cfg.LLMs.Embeddings.Enabled {
		return 0, 0, nil
	}
	if strings.TrimSpace(cfg.LLMs.Embeddings.Model) == "" {
		return 0, 0, nil
	}
	if strings.TrimSpace(cfg.LLMs.OllamaURL) == "" {
		return 0, 0, nil
	}

	historicalDocuments, err := client.ListDocuments(ctx, paperless.DocumentFilter{Limit: cfg.LLMs.Embeddings.HistoricalDocumentLimit, Ordering: "-created"})
	if err != nil {
		return 0, 0, fmt.Errorf("load historical documents: %w", err)
	}

	indexed := 0
	considered := 0
	for _, historicalDocument := range historicalDocuments {
		if indexed >= cfg.LLMs.Embeddings.MaxDocumentsPerRun {
			break
		}
		considered++

		record, err := service.store.GetDocumentEmbedding(ctx, historicalDocument.ID)
		if err != nil {
			return 0, 0, fmt.Errorf("lookup indexed embedding for document %d: %w", historicalDocument.ID, err)
		}

		isStale := record == nil || strings.TrimSpace(record.SourceModified) != strings.TrimSpace(historicalDocument.Modified)
		if !isStale {
			continue
		}

		embeddingInput := buildEmbeddingInput(historicalDocument, historicalDocument.Content, correspondentsByID, documentTypesByID, tagsByID)
		if strings.TrimSpace(embeddingInput) == "" {
			continue
		}

		vector, err := ollama.Embed(ctx, cfg.LLMs.OllamaURL, cfg.LLMs.Embeddings.Model, embeddingInput)
		if err != nil {
			return 0, 0, fmt.Errorf("embed historical document %d: %w", historicalDocument.ID, err)
		}

		record = &DocumentEmbeddingRecord{
			DocumentID:        historicalDocument.ID,
			Title:             strings.TrimSpace(historicalDocument.Title),
			OriginalFileName:  strings.TrimSpace(historicalDocument.OriginalFileName),
			Created:           strings.TrimSpace(historicalDocument.Created),
			SourceModified:    strings.TrimSpace(historicalDocument.Modified),
			CorrespondentName: normalizeIndexedName(correspondentsByID[readOptionalID(historicalDocument.CorrespondentID)]),
			DocumentTypeName:  normalizeIndexedName(documentTypesByID[readOptionalID(historicalDocument.DocumentTypeID)]),
			TagNames:          resolveTagNames(historicalDocument.TagIDs, tagsByID),
			Snippet:           historicalSnippet(historicalDocument),
			Embedding:         vector,
		}
		if err := service.store.UpsertDocumentEmbedding(ctx, *record); err != nil {
			return 0, 0, fmt.Errorf("store embedding for document %d: %w", historicalDocument.ID, err)
		}

		indexed++
	}
	return indexed, considered, nil
}

func (service *RetrievalService) ClearAllEmbeddings(ctx context.Context) error {
	return service.store.ClearAllEmbeddings(ctx)
}

func BuildClassificationEvidence(items []SimilarDocument) []classification.SimilarDocumentEvidence {
	evidence := make([]classification.SimilarDocumentEvidence, 0, len(items))
	for _, item := range items {
		record := item.Record
		evidence = append(evidence, classification.SimilarDocumentEvidence{
			DocumentID:       record.DocumentID,
			Title:            record.Title,
			OriginalFileName: record.OriginalFileName,
			Created:          record.Created,
			Correspondent:    record.CorrespondentName,
			DocumentType:     record.DocumentTypeName,
			TagNames:         append([]string(nil), record.TagNames...),
			Snippet:          record.Snippet,
			Similarity:       item.Similarity,
		})
	}
	return evidence
}

func buildEmbeddingInput(document paperless.Document, extractedText string, correspondentsByID map[int64]string, documentTypesByID map[int64]string, tagsByID map[int64]string) string {
	parts := make([]string, 0, 8)
	if value := strings.TrimSpace(document.Title); value != "" {
		parts = append(parts, "title: "+value)
	}
	if value := strings.TrimSpace(document.OriginalFileName); value != "" {
		parts = append(parts, "file: "+value)
	}
	if value := normalizeIndexedName(correspondentsByID[readOptionalID(document.CorrespondentID)]); value != "" {
		parts = append(parts, "correspondent: "+value)
	}
	if value := normalizeIndexedName(documentTypesByID[readOptionalID(document.DocumentTypeID)]); value != "" {
		parts = append(parts, "document_type: "+value)
	}
	if tags := resolveTagNames(document.TagIDs, tagsByID); len(tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(tags, ", "))
	}
	text := strings.TrimSpace(extractedText)
	if text == "" {
		text = strings.TrimSpace(document.Content)
	}
	if text != "" {
		parts = append(parts, "content: "+trimForEmbedding(text, 2500))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func trimForEmbedding(value string, maxChars int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= maxChars {
		return trimmed
	}
	if maxChars <= 3 {
		return trimmed[:maxChars]
	}
	return trimmed[:maxChars-3] + "..."
}

func historicalSnippet(document paperless.Document) string {
	base := strings.TrimSpace(document.Content)
	if base == "" {
		base = strings.TrimSpace(document.Title + " " + document.OriginalFileName)
	}
	return trimForEmbedding(base, 320)
}

func readOptionalID(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func resolveTagNames(tagIDs []int64, tagsByID map[int64]string) []string {
	if len(tagIDs) == 0 {
		return nil
	}
	names := make([]string, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		name := normalizeIndexedName(tagsByID[tagID])
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func normalizeIndexedName(value string) string {
	return strings.TrimSpace(value)
}

func cosineSimilarity(left []float64, right []float64) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	if len(left) != len(right) {
		return 0
	}

	var dotProduct float64
	var leftNorm float64
	var rightNorm float64
	for index := range left {
		dotProduct += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}
