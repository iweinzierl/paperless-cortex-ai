package classification

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"paperless-ai-ext/internal/ocr"
	"paperless-ai-ext/internal/ollama"
)

const textMarker = "Document text:\n"

type ExtractionOptions struct {
	OllamaURL           string
	OCRModel            string
	VisionModel         string
	VisionMaxPages      int
	ForceVision         bool
	AllowVisionFallback bool
	Prompt              string
}

type ExtractionResult struct {
	Text      string `json:"text"`
	Source    string `json:"source"`
	UsedModel string `json:"used_model,omitempty"`
}

func ExtractDocumentText(ctx context.Context, documentPath string, options ExtractionOptions) (ExtractionResult, error) {
	prompt := strings.TrimSpace(options.Prompt)
	if prompt == "" {
		prompt = ocr.DefaultPrompt
	}
	if options.VisionMaxPages <= 0 {
		options.VisionMaxPages = 5
	}

	if options.ForceVision {
		return extractWithVision(ctx, documentPath, options, errors.New("vision OCR was forced"))
	}

	message, err := ocr.BuildScreeningMessage(documentPath, prompt)
	if err != nil {
		if options.AllowVisionFallback {
			return extractWithVision(ctx, documentPath, options, fmt.Errorf("simple extraction failed: %w", err))
		}
		return ExtractionResult{}, err
	}

	if len(message.Images) == 0 {
		text, err := extractEmbeddedText(message)
		if err == nil {
			return ExtractionResult{Text: text, Source: "document-text"}, nil
		}
		if !options.AllowVisionFallback {
			return ExtractionResult{}, err
		}
		return extractWithVision(ctx, documentPath, options, err)
	}

	if strings.TrimSpace(options.OCRModel) == "" {
		return ExtractionResult{}, errors.New("ocr model is required for image extraction")
	}

	response, err := ollama.Run(ctx, options.OllamaURL, options.OCRModel, message)
	if err != nil {
		if !options.AllowVisionFallback {
			return ExtractionResult{}, err
		}
		return extractWithVision(ctx, documentPath, options, fmt.Errorf("simple OCR model call failed: %w", err))
	}

	text := strings.TrimSpace(response)
	if text == "" {
		if !options.AllowVisionFallback {
			return ExtractionResult{}, errors.New("OCR result is empty")
		}
		return extractWithVision(ctx, documentPath, options, errors.New("OCR result is empty"))
	}

	return ExtractionResult{Text: text, Source: "ocr-llm", UsedModel: strings.TrimSpace(options.OCRModel)}, nil
}

func extractEmbeddedText(message ollama.Message) (string, error) {
	index := strings.Index(message.Content, textMarker)
	if index < 0 {
		return "", errors.New("screening message did not contain document text")
	}

	text := strings.TrimSpace(message.Content[index+len(textMarker):])
	if text == "" {
		return "", errors.New("document text is empty")
	}

	return text, nil
}

func extractWithVision(ctx context.Context, documentPath string, options ExtractionOptions, rootCause error) (ExtractionResult, error) {
	visionModel := strings.TrimSpace(options.VisionModel)
	if visionModel == "" {
		visionModel = strings.TrimSpace(options.OCRModel)
	}
	if visionModel == "" {
		return ExtractionResult{}, rootCause
	}

	message, err := ocr.BuildVisionScreeningMessage(documentPath, options.PromptOrDefault(), options.VisionMaxPages)
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("%v; vision fallback failed: %w", rootCause, err)
	}

	response, err := ollama.Run(ctx, options.OllamaURL, visionModel, message)
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("%v; vision fallback failed: %w", rootCause, err)
	}

	text := strings.TrimSpace(response)
	if text == "" {
		return ExtractionResult{}, fmt.Errorf("%v; vision fallback returned empty text", rootCause)
	}

	return ExtractionResult{Text: text, Source: "vision-llm", UsedModel: visionModel}, nil
}

func (options ExtractionOptions) PromptOrDefault() string {
	if strings.TrimSpace(options.Prompt) == "" {
		return ocr.DefaultPrompt
	}
	return options.Prompt
}
