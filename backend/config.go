package main

import (
	"os"
	"strings"
)

type ProcessingMode string

const (
	ProcessingModeManual ProcessingMode = "manual"
	ProcessingModeAuto   ProcessingMode = "auto"
)

type BackendConfig struct {
	Engine    EngineConfig    `json:"engine"`
	Process   ProcessConfig   `json:"process"`
	Paperless PaperlessConfig `json:"paperless"`
	LLMs      LLMConfig       `json:"llms"`
}

type EngineConfig struct {
	ProcessingMode            ProcessingMode `json:"processing_mode"`
	ProcessingIntervalSeconds int            `json:"processing_interval"`
}

type ProcessConfig struct {
	ProcessTriggerTag       string `json:"process_trigger_tag"`
	ForceOCRTag             string `json:"force_ocr_tag"`
	ForceVisionTag          string `json:"force_vision_tag"`
	ProcessCreatedDateTag   string `json:"process_created_date_tag"`
	ProcessCorrespondentTag string `json:"process_correspondent_tag"`
	ProcessDocumentTypeTag  string `json:"process_document_type_tag"`
	ProcessDocumentTagsTag  string `json:"process_document_tags_tag"`
	ProcessTitleTag         string `json:"process_title_tag"`
	ProcessCompletedTag     string `json:"process_completed_tag"`
}

type PaperlessConfig struct {
	PaperlessURL   string `json:"paperless_url"`
	PaperlessToken string `json:"paperless_token"`
}

type LLMConfig struct {
	OllamaURL  string `json:"ollama_url"`
	DefaultLLM string `json:"default_llm"`
	VisionLLM  string `json:"vision_llm"`
}

func DefaultBackendConfig() BackendConfig {
	return BackendConfig{
		Engine: EngineConfig{
			ProcessingMode:            ProcessingModeManual,
			ProcessingIntervalSeconds: 30,
		},
		Paperless: PaperlessConfig{
			PaperlessURL: os.Getenv("PAPERLESS_URL"),
		},
		LLMs: LLMConfig{
			OllamaURL: "http://localhost:11434",
		},
	}
}

func (cfg *BackendConfig) Normalize() {
	if cfg.Engine.ProcessingMode != ProcessingModeManual && cfg.Engine.ProcessingMode != ProcessingModeAuto {
		cfg.Engine.ProcessingMode = ProcessingModeManual
	}

	if cfg.Engine.ProcessingIntervalSeconds < 5 {
		cfg.Engine.ProcessingIntervalSeconds = 5
	}

	cfg.Process.ProcessTriggerTag = strings.TrimSpace(cfg.Process.ProcessTriggerTag)
	cfg.Process.ForceOCRTag = strings.TrimSpace(cfg.Process.ForceOCRTag)
	cfg.Process.ForceVisionTag = strings.TrimSpace(cfg.Process.ForceVisionTag)
	cfg.Process.ProcessCreatedDateTag = strings.TrimSpace(cfg.Process.ProcessCreatedDateTag)
	cfg.Process.ProcessCorrespondentTag = strings.TrimSpace(cfg.Process.ProcessCorrespondentTag)
	cfg.Process.ProcessDocumentTypeTag = strings.TrimSpace(cfg.Process.ProcessDocumentTypeTag)
	cfg.Process.ProcessDocumentTagsTag = strings.TrimSpace(cfg.Process.ProcessDocumentTagsTag)
	cfg.Process.ProcessTitleTag = strings.TrimSpace(cfg.Process.ProcessTitleTag)
	cfg.Process.ProcessCompletedTag = strings.TrimSpace(cfg.Process.ProcessCompletedTag)

	cfg.Paperless.PaperlessURL = strings.TrimSpace(cfg.Paperless.PaperlessURL)
	if cfg.Paperless.PaperlessURL == "" {
		cfg.Paperless.PaperlessURL = strings.TrimSpace(os.Getenv("PAPERLESS_URL"))
	}
	cfg.Paperless.PaperlessToken = strings.TrimSpace(cfg.Paperless.PaperlessToken)

	cfg.LLMs.OllamaURL = strings.TrimSpace(cfg.LLMs.OllamaURL)
	if cfg.LLMs.OllamaURL == "" {
		cfg.LLMs.OllamaURL = "http://localhost:11434"
	}

	cfg.LLMs.DefaultLLM = strings.TrimSpace(cfg.LLMs.DefaultLLM)
	cfg.LLMs.VisionLLM = strings.TrimSpace(cfg.LLMs.VisionLLM)
}
