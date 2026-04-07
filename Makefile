.PHONY: ocr_screening ocr_vision_screening process_correspondents process_document_types process_tags

OLLAMA_URL ?= http://localhost:11434

ocr_screening:
	@if [ -z "$(DOCUMENT)" ]; then echo "DOCUMENT is required. Usage: make ocr_screening DOCUMENT=/path/to/document MODEL=llama3.2-vision"; exit 1; fi
	@if [ -z "$(MODEL)" ]; then echo "MODEL is required. Usage: make ocr_screening DOCUMENT=/path/to/document MODEL=llama3.2-vision"; exit 1; fi
	go run ./cmd/ocr_screening -document "$(DOCUMENT)" -model "$(MODEL)" -ollama-url "$(OLLAMA_URL)"

ocr_vision_screening:
	@if [ -z "$(DOCUMENT)" ]; then echo "DOCUMENT is required. Usage: make ocr_vision_screening DOCUMENT=/path/to/document MODEL=qwen2.5vl:7b"; exit 1; fi
	@if [ -z "$(MODEL)" ]; then echo "MODEL is required. Usage: make ocr_vision_screening DOCUMENT=/path/to/document MODEL=qwen2.5vl:7b"; exit 1; fi
	go run ./cmd/ocr_vision_screening -document "$(DOCUMENT)" -model "$(MODEL)" -ollama-url "$(OLLAMA_URL)"

process_correspondents:
	@if [ -z "$(DOCUMENT)" ]; then echo "DOCUMENT is required. Usage: make process_correspondents DOCUMENT=/path/to/document MODEL=llama3.2-vision"; exit 1; fi
	@if [ -z "$(MODEL)" ]; then echo "MODEL is required. Usage: make process_correspondents DOCUMENT=/path/to/document MODEL=llama3.2-vision"; exit 1; fi
	go run ./cmd/process_correspondents -document "$(DOCUMENT)" -model "$(MODEL)" -ollama-url "$(OLLAMA_URL)"

process_document_types:
	@if [ -z "$(DOCUMENT)" ]; then echo "DOCUMENT is required. Usage: make process_document_types DOCUMENT=/path/to/document MODEL=llama3.2-vision"; exit 1; fi
	@if [ -z "$(MODEL)" ]; then echo "MODEL is required. Usage: make process_document_types DOCUMENT=/path/to/document MODEL=llama3.2-vision"; exit 1; fi
	go run ./cmd/process_document_types -document "$(DOCUMENT)" -model "$(MODEL)" -ollama-url "$(OLLAMA_URL)"

process_tags:
	@if [ -z "$(DOCUMENT)" ]; then echo "DOCUMENT is required. Usage: make process_tags DOCUMENT=/path/to/document MODEL=llama3.2-vision"; exit 1; fi
	@if [ -z "$(MODEL)" ]; then echo "MODEL is required. Usage: make process_tags DOCUMENT=/path/to/document MODEL=llama3.2-vision"; exit 1; fi
	go run ./cmd/process_tags -document "$(DOCUMENT)" -model "$(MODEL)" -vision-model "$(if $(VISION_MODEL),$(VISION_MODEL),$(MODEL))" -vision-max-pages "$(if $(VISION_MAX_PAGES),$(VISION_MAX_PAGES),3)" $(if $(FORCE_VISION),-force-vision,) -ollama-url "$(OLLAMA_URL)"
