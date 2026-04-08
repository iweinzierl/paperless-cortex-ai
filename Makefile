.PHONY: backend docker_backend_multiarch docker_webapp_multiarch k3s_apply k3s_delete ocr_screening ocr_vision_screening process_correspondents process_document_types process_tags

OLLAMA_URL ?= http://localhost:11434
DOCKER_PLATFORMS ?= linux/amd64,linux/arm64
DOCKERFILE ?= Dockerfile
WEBAPP_DOCKERFILE ?= webapp/Dockerfile
K3S_MANIFEST_DIR ?= deploy/k3s
KUBECTL ?= kubectl

backend:
	go run ./backend

docker_backend_multiarch:
	@if [ -z "$(IMAGE)" ]; then echo "IMAGE is required. Usage: make docker_backend_multiarch IMAGE=ghcr.io/iweinzierl/paperless-ai-ext-backend TAG=latest [DOCKER_PLATFORMS=linux/amd64,linux/arm64]"; exit 1; fi
	docker buildx build \
		--platform "$(DOCKER_PLATFORMS)" \
		--file "$(DOCKERFILE)" \
		--tag "$(IMAGE):$(if $(TAG),$(TAG),latest)" \
		--push \
		.

docker_webapp_multiarch:
	@if [ -z "$(IMAGE)" ]; then echo "IMAGE is required. Usage: make docker_webapp_multiarch IMAGE=ghcr.io/iweinzierl/paperless-ai-ext-webapp TAG=latest [DOCKER_PLATFORMS=linux/amd64,linux/arm64]"; exit 1; fi
	docker buildx build \
		--platform "$(DOCKER_PLATFORMS)" \
		--file "$(WEBAPP_DOCKERFILE)" \
		--tag "$(IMAGE):$(if $(TAG),$(TAG),latest)" \
		--push \
		webapp

k3s_apply:
	$(KUBECTL) apply -k "$(K3S_MANIFEST_DIR)"

k3s_delete:
	$(KUBECTL) delete -k "$(K3S_MANIFEST_DIR)"

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
