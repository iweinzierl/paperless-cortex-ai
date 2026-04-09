package ocr

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"paperless-ai-ext/internal/ollama"

	pdf "github.com/ledongthuc/pdf"
)

const DefaultPrompt = "Transcribe the document verbatim. Return only the plain text exactly as it appears in reading order. Do not summarize, interpret, explain, translate, normalize, classify, extract fields, add labels, add key/value formatting, or infer structure. Do not add markdown, bullets, code fences, or commentary."

const VisionPrompt = `Transcribe the following document image with high fidelity.
    Maintain Formatting: Use Markdown for headings, bold text, and lists.
    Tables: Render all tables using Markdown table syntax.
    Fidelity: Do not summarize. Do not correct spelling errors found in the source. If a word is illegible, mark it as [unclear].
    Output: Provide only the transcription, no conversational filler.`

var imageExtensions = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".webp": {},
}

func BuildScreeningMessage(documentPath string, prompt string) (ollama.Message, error) {
	info, err := os.Stat(documentPath)
	if err != nil {
		return ollama.Message{}, fmt.Errorf("read document metadata: %w", err)
	}

	if info.IsDir() {
		return ollama.Message{}, fmt.Errorf("document path %q is a directory", documentPath)
	}

	ext := strings.ToLower(filepath.Ext(documentPath))
	switch {
	case ext == ".pdf":
		text, err := extractPDFText(documentPath)
		if err != nil {
			return ollama.Message{}, err
		}

		return ollama.Message{
			Role:    "user",
			Content: fmt.Sprintf("%s\n\nDocument source: %s\n\nDocument text:\n%s", prompt, filepath.Base(documentPath), text),
		}, nil
	case isImageExtension(ext):
		encoded, err := encodeImage(documentPath)
		if err != nil {
			return ollama.Message{}, err
		}

		return ollama.Message{
			Role:    "user",
			Content: prompt,
			Images:  []string{encoded},
		}, nil
	default:
		text, err := os.ReadFile(documentPath)
		if err != nil {
			return ollama.Message{}, fmt.Errorf("read document: %w", err)
		}

		return ollama.Message{
			Role:    "user",
			Content: fmt.Sprintf("%s\n\nDocument source: %s\n\nDocument text:\n%s", prompt, filepath.Base(documentPath), string(text)),
		}, nil
	}
}

func BuildVisionScreeningMessage(documentPath string, prompt string, maxPages int) (ollama.Message, error) {
	info, err := os.Stat(documentPath)
	if err != nil {
		return ollama.Message{}, fmt.Errorf("read document metadata: %w", err)
	}

	if info.IsDir() {
		return ollama.Message{}, fmt.Errorf("document path %q is a directory", documentPath)
	}

	ext := strings.ToLower(filepath.Ext(documentPath))
	switch {
	case isImageExtension(ext):
		encoded, err := encodeImage(documentPath)
		if err != nil {
			return ollama.Message{}, err
		}

		return ollama.Message{
			Role:    "user",
			Content: fmt.Sprintf("%s\n\nDocument source: %s", prompt, filepath.Base(documentPath)),
			Images:  []string{encoded},
		}, nil
	case ext == ".pdf":
		images, err := renderPDFAsImages(documentPath, maxPages)
		if err != nil {
			return ollama.Message{}, err
		}

		return ollama.Message{
			Role:    "user",
			Content: fmt.Sprintf("%s\n\nDocument source: %s", prompt, filepath.Base(documentPath)),
			Images:  images,
		}, nil
	default:
		return ollama.Message{}, fmt.Errorf("vision OCR supports PDF and image files only, got %q", ext)
	}
}

func ExtractText(documentPath string) (string, error) {
	message, err := BuildScreeningMessage(documentPath, DefaultPrompt)
	if err != nil {
		return "", err
	}

	return extractTextFromMessage(message)
}

func extractTextFromMessage(message ollama.Message) (string, error) {
	const marker = "Document text:\n"
	if len(message.Images) > 0 {
		return "", errors.New("image-based OCR requires an Ollama model and cannot be extracted locally")
	}

	index := strings.Index(message.Content, marker)
	if index < 0 {
		return "", errors.New("screening message does not contain extracted document text")
	}

	text := strings.TrimSpace(message.Content[index+len(marker):])
	if text == "" {
		return "", errors.New("document text is empty")
	}

	return text, nil
}

func extractPDFText(documentPath string) (string, error) {
	file, reader, err := pdf.Open(documentPath)
	if err != nil {
		return "", fmt.Errorf("open PDF document: %w", err)
	}
	defer file.Close()

	var buffer bytes.Buffer
	plainTextReader, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract text from PDF: %w", err)
	}

	if _, err := io.Copy(&buffer, plainTextReader); err != nil {
		return "", fmt.Errorf("read extracted PDF text: %w", err)
	}

	text := strings.TrimSpace(buffer.String())
	if text == "" {
		return "", errors.New("PDF does not contain extractable text")
	}

	return text, nil
}

func encodeImage(documentPath string) (string, error) {
	data, err := os.ReadFile(documentPath)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}

	if len(data) == 0 {
		return "", errors.New("image file is empty")
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

func isImageExtension(ext string) bool {
	_, ok := imageExtensions[ext]
	return ok
}

func renderPDFAsImages(documentPath string, maxPages int) ([]string, error) {
	if maxPages <= 0 {
		return nil, errors.New("max pages must be greater than zero")
	}

	tempDir, err := os.MkdirTemp("", "paperless-ai-ext-vision-*")
	if err != nil {
		return nil, fmt.Errorf("create temp directory for PDF rendering: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var imagePaths []string
	switch {
	case commandExists("pdftoppm"):
		imagePaths, err = renderPDFWithPDFToPPM(documentPath, tempDir, maxPages)
	case commandExists("qlmanage"):
		imagePaths, err = renderPDFWithQuickLook(documentPath, tempDir)
	default:
		return nil, errors.New("vision OCR for PDF requires pdftoppm or qlmanage to be installed")
	}
	if err != nil {
		return nil, err
	}

	encoded := make([]string, 0, len(imagePaths))
	for _, imagePath := range imagePaths {
		base64Image, err := encodeImage(imagePath)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, base64Image)
	}

	if len(encoded) == 0 {
		return nil, errors.New("no PDF pages were rendered for vision OCR")
	}

	return encoded, nil
}

func renderPDFWithPDFToPPM(documentPath string, tempDir string, maxPages int) ([]string, error) {
	prefix := filepath.Join(tempDir, "page")
	command := exec.Command(
		"pdftoppm",
		"-png",
		"-f", "1",
		"-l", strconv.Itoa(maxPages),
		documentPath,
		prefix,
	)

	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("render PDF with pdftoppm: %w: %s", err, strings.TrimSpace(string(output)))
	}

	imagePaths, err := filepath.Glob(filepath.Join(tempDir, "page-*.png"))
	if err != nil {
		return nil, fmt.Errorf("list rendered PDF images: %w", err)
	}
	sort.Strings(imagePaths)

	if len(imagePaths) == 0 {
		return nil, errors.New("pdftoppm did not produce any page images")
	}

	return imagePaths, nil
}

func renderPDFWithQuickLook(documentPath string, tempDir string) ([]string, error) {
	command := exec.Command(
		"qlmanage",
		"-t",
		"-s", "2048",
		"-o", tempDir,
		documentPath,
	)

	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("render PDF with qlmanage: %w: %s", err, strings.TrimSpace(string(output)))
	}

	imagePaths, err := filepath.Glob(filepath.Join(tempDir, "*.png"))
	if err != nil {
		return nil, fmt.Errorf("list Quick Look rendered images: %w", err)
	}
	sort.Strings(imagePaths)

	if len(imagePaths) == 0 {
		return nil, errors.New("qlmanage did not produce any page images")
	}

	return imagePaths, nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
