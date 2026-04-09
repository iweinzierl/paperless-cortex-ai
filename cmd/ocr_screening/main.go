package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"paperless-ai-ext/internal/ocr"
	"paperless-ai-ext/internal/ollama"
)

func main() {
	var documentPath string
	var model string
	var ollamaURL string
	var prompt string

	flag.StringVar(&documentPath, "document", "", "path to the document to screen")
	flag.StringVar(&model, "model", "", "Ollama model to use")
	flag.StringVar(&ollamaURL, "ollama-url", "http://localhost:11434", "base URL of the Ollama server")
	flag.StringVar(&prompt, "prompt", ocr.DefaultPrompt, "prompt sent to the model")
	flag.Parse()

	if documentPath == "" {
		exitWithError(errors.New("-document is required"))
	}

	if model == "" {
		exitWithError(errors.New("-model is required"))
	}

	content, err := ocr.BuildScreeningMessage(documentPath, prompt)
	if err != nil {
		exitWithError(err)
	}

	result, err := ollama.Run(context.Background(), ollamaURL, model, content)
	if err != nil {
		exitWithError(err)
	}

	fmt.Println(strings.TrimSpace(result))
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func init() {
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
}
