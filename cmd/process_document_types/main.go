package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"paperless-ai-ext/internal/classification"
	"paperless-ai-ext/internal/paperless"
)

func main() {
	var documentPath string
	var model string
	var ollamaURL string
	var paperlessURL string
	var paperlessToken string

	flag.StringVar(&documentPath, "document", "", "path to the document to process")
	flag.StringVar(&model, "model", "", "Ollama model to use")
	flag.StringVar(&ollamaURL, "ollama-url", "http://localhost:11434", "base URL of the Ollama server")
	flag.StringVar(&paperlessURL, "paperless-url", os.Getenv("PAPERLESS_URL"), "base URL of the paperless-ngx instance")
	flag.StringVar(&paperlessToken, "paperless-token", os.Getenv("PAPERLESS_TOKEN"), "API token for the paperless-ngx instance")
	flag.Parse()

	if documentPath == "" {
		exitWithError(errors.New("-document is required"))
	}
	if model == "" {
		exitWithError(errors.New("-model is required"))
	}
	if paperlessURL == "" {
		exitWithError(errors.New("-paperless-url is required or PAPERLESS_URL must be set"))
	}
	if paperlessToken == "" {
		exitWithError(errors.New("-paperless-token is required or PAPERLESS_TOKEN must be set"))
	}

	ctx := context.Background()
	client := paperless.NewClient(paperlessURL, paperlessToken)

	documentTypes, err := client.ListDocumentTypes(ctx)
	if err != nil {
		exitWithError(err)
	}

	extracted, err := classification.ExtractDocumentText(ctx, documentPath, classification.ExtractionOptions{
		OllamaURL:      ollamaURL,
		OCRModel:       model,
		VisionMaxPages: 3,
	})
	if err != nil {
		exitWithError(err)
	}

	suggestion, err := classification.SuggestDocumentType(ctx, ollamaURL, model, filepath.Base(documentPath), extracted.Text, documentTypes, nil)
	if err != nil {
		exitWithError(err)
	}

	output, err := json.MarshalIndent(suggestion, "", "  ")
	if err != nil {
		exitWithError(fmt.Errorf("marshal suggestion: %w", err))
	}

	fmt.Println(string(output))
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func init() {
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
}
