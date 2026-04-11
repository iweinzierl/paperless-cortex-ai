# Introduction
This project is supposed to become an extension of paperless-ngx (https://github.com/paperless-ngx/paperless-ngx). Whereas paperless-ngx uses internal tools to determine document types, correspondents, tags and others, this project uses large language models (LLMs) to ocr documents and determine such.

# Project Structure
- **backend** - the backend serving a RESTful API to manage settings of this extension
- **cmd** - command line scripts that can be executed using the Makefile. The scripts are supposed to support the development of the backend.
- **webapp* - a web application which interacts with the RESTful API provided by the backend. It's used by users to configure the extension.

## Backend
Users must configure a set of tags. The backend observes document changes and queues matching documents. When a queued item is processed, the backend fetches the live document metadata and downloads the current source file from paperless-ngx. If the tag **process_trigger_tag** is still present on the live document, the backend builds a staged processing plan from the configured tags. Text extraction is always the first stage and is followed by the requested suggestion stages: **force_ocr_tag**, **force_vision_tag**, **process_correspondent_tag**, **process_document_type_tag**, **process_document_tags_tag**. The backend stores structured suggestions and extraction metadata in its queue records and can now write successful suggestions back to Paperless. In **manual** mode, that writeback is triggered from the webapp with an explicit **Apply Suggestions** action. In **auto** mode, the backend applies the successful suggestions automatically after processing finishes. Whenever apply succeeds, the configured **process_completed_tag** is added to the Paperless document and the configured processing state tags are removed.

### Container Build
You can build and push a multi-architecture backend container image for deployment to k3s with:

```sh
make docker_backend_multiarch IMAGE=ghcr.io/<owner>/paperless-ai-ext TAG=latest
```

The target uses Docker Buildx, builds for `linux/amd64` and `linux/arm64` by default, and pushes the resulting manifest list to your registry. You can override the platform set through `DOCKER_PLATFORMS` and the Dockerfile path through `DOCKERFILE`.

The container exposes port `8080` and stores its SQLite database at `/data/paperless-aiext.db`, so for k3s you should mount `/data` on persistent storage.

The backend needs `pdftoppm` for PDF-based vision OCR. The provided backend container image installs `poppler-utils`, which supplies `pdftoppm`, so forced vision extraction and vision fallback work for PDF documents out of the box. If you run the backend outside the container, install either `pdftoppm` or another supported renderer on the host.

You can build and push the webapp container image with:

```sh
make docker_webapp_multiarch IMAGE=ghcr.io/<owner>/paperless-ai-ext-webapp TAG=latest
```

The webapp is built as a static Flutter web bundle and served by nginx. It calls the backend through the same host on `/api`.

### k3s Deployment
The repository includes a k3s manifest set for the backend under `deploy/k3s`.

Before applying it, update these files:

- `deploy/k3s/deployment.yaml` - set the backend image to the registry and tag you pushed.
- `deploy/k3s/webapp-deployment.yaml` - set the webapp image to the registry and tag you pushed.
- `deploy/k3s/configmap.yaml` - set `PAPERLESS_URL` to your paperless-ngx instance URL.
- `deploy/k3s/configmap.yaml` - adjust `PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS` if your Ollama deployment needs more than the default 2 minutes for larger models or prompts.
- `deploy/k3s/secret.yaml` - replace `PAPERLESS_AIEXT_SHARED_SECRET` with a real shared secret.
- `deploy/k3s/ingress.yaml` - set the hostname you want Traefik to expose for both the UI and API.

Apply the manifests with:

```sh
make k3s_apply
```

Or directly with:

```sh
kubectl apply -k deploy/k3s
```

The default PVC uses the `local-path` storage class that ships with k3s. The deployment is intentionally configured as a single replica with a `Recreate` strategy because the backend stores state in SQLite on a `ReadWriteOnce` volume.

If your image registry is private, add an `imagePullSecrets` entry to the pod spec in `deploy/k3s/deployment.yaml`.

The ingress serves the Flutter webapp on `/` and forwards `/api` to the backend service, so the browser talks to the API over the same origin.

### Configuration
The backend's engine must be configured by users. The configuration is structured in the following sections:

#### Configuration: Engine
- **processing_mode** - the processing mode can either be `manual` or `auto`. In case of manual mode, documents that requested processing must be triggered manually through the webapp. If it's set to auto, the engine automatically picks and processes a document every X seconds, as configured by the user, based on the timestamp when the processing was requested (FIFO).
- **processing_interval** - the interval used in auto processing mode to schedule the processing of documents.

#### Configuration: Process
- **process_trigger_tag** - A document tag used to trigger the processing in the backend.
- **force_ocr_tag** - A document tag used to force the extraction stage to run from the downloaded source document.
- **force_vision_tag** - A document tag used to force text extraction using a Vision LLM (VLM).
- **process_correspondent_tag** - A document tag that enables the correspondent suggestion stage.
- **process_document_type_tag** - A document tag that enables the document type suggestion stage.
- **process_document_tags_tag** - A document tag that enables the document tag suggestion stage.
- **process_completed_tag** - A Paperless tag that is added when the apply step succeeds. In manual mode this happens when the user confirms **Apply Suggestions** in the webapp. In auto mode it happens automatically after processing finishes. During the same apply step, the configured processing state tags are removed from the document.

#### Configuration: Paperless
- **paperless_url** - The URL to the paperless-ngx instance.
- **paperless_token** - The authentication token used to access the paperless-ngx instance.

#### Configuration: LLMs
- **ollama_url** - The URL under which the ollama is accessible.
- **default_llm** - The default LLM used for recommendation for correspondents, document types and tags.
- **vision_llm** - The vision LLM (VLM) used for text extraction.

Environment variables:
- **PAPERLESS_AIEXT_OLLAMA_TIMEOUT_SECONDS** - Optional timeout for a single Ollama chat request. Defaults to 120 seconds. Increase this for slower clusters or larger models.


### Features
- **Staged Processing Engine** - queued documents are processed one by one. The backend re-loads the live Paperless document, downloads the current source file, extracts text first, and then runs the requested suggestion stages for correspondents, document types, and tags.
- **OCR Screening** - the OCR screening features use configured LLMs to read the content of documents, for example PDF documents. After successful screening, the content is returned as text which can be further processed to determine correspondents, document types, and tags.

## CMD
The command line scripts are written in Golang as well and can be executed using the project's Makefile:
- **ocr_screening** - this command line script accepts the path to a document which shall be screened and the LLM that shall be used for the screening. The script uses ollama's chat API to trigger the screening process. After successful screening, it returns the document's content as plain text.
- **ocr_vision_screening** - this command line script accepts the path to an image or PDF document and the vision LLM that shall be used for screening. Image documents are sent directly to ollama's chat API. PDFs are rendered to page images before they are sent to the model. After successful screening, it returns the document's content as plain text.
- **process_correspondents** - this command line script accepts the path to a document which shall be screened for its correspondents and the LLM that shall be used for screening and suggesting correspondents. The script shall read all existing correspondents from the paperless-ngx instance. The env variable PAPERLESS_TOKEN which contains the authentication token for paperless-ngx will be used for authentication to the paperless-ngx instance. The PAPERLESS_URL contains the url to this instance.
- **process_document_types** - this command line script accepts the path to a document which shall be screened for its document type and the LLM that shall be used for screening and suggesting document types. The script shall read all existing document types from the paperless-ngx instance. The env variable PAPERLESS_TOKEN which contains the authentication token for paperless-ngx will be used for authentication to the paperless-ngx instance. The PAPERLESS_URL contains the url to this instance.
- **process_tags** - this command line script accepts the path to a document which shall be screened and the LLM that shall be used for screening. It first tries a simple OCR/text extraction path and falls back to a vision LLM when no usable text can be extracted. You can also force vision-based screening explicitly. The script reads the existing tags from your paperless-ngx instance and suggests a set of matching existing tags plus optional new tags when none of the existing tags fit well. Authentication is handled via the PAPERLESS_URL and PAPERLESS_TOKEN variables.

## Webapp
The webapp is a management interface for users to configure the backend. Users are able to manually trigger the processing of documents if manual processing mode is selected. When manual mode is configured, completed processing results also expose an **Apply Suggestions** action in the result drawer so the suggested metadata can be written back to Paperless on demand.

## Architecture

### Backend
- the backend is written in `Golang`
- an `SQLite` database is used to store state
- the `Gin` framework is used to for the RESTful API layer
- an internal `middleware` logs all incoming http requests and the corresponding http responses
- `Zerolog` is used for logging across the backend. The loglevel can be configured using an environment variable (PAPERLESS_AIEXT_LOGLEVEL).
- the backend uses token based authentication to access the paperless-ngx instance to fetch document metadata from paperless-ngx 
- incoming http calls are authenticated using username:password from the paperless-ngx instance in a /api/auth call which returns a paperless-ngx session token upon successful authentication. This session token is most prominently used by the webapp to authenticate further API calls.
- the backend implements a webhook which is called by paperless-ngx once a document is updated. Since the webhook in the workflow in paperless-ngx is called before the document is saved, the request is persisted in the queue and processed later. Authentication for this webhook endpoint is done through a shared secret set as http header (x-shared-secret) which is configured via environment variable in the backend (PAPERLESS_AIEXT_SHARED_SECRET).
- the processor executes queued documents sequentially and evaluates the live document tags at execution time, not only the original webhook payload.
- processing results are stored as structured JSON payloads on queue items so the UI can review extraction metadata and LLM suggestions before writeback, and the queue item also records whether applying those suggestions to Paperless succeeded or failed.

### Backend Webhook Interface
- endpoint: POST /api/webhooks/paperless
- authentication: the x-shared-secret header must match PAPERLESS_AIEXT_SHARED_SECRET
- accepted content type: application/json
- request body: the JSON payload must include document_title and document_url
- document identity: the backend derives the Paperless document ID from document_url so duplicate webhook deliveries for the same document can reuse the active queue item
- behavior: a valid request is queued with HTTP 202 Accepted; if an active queue item already exists for the same document, the existing item is reused and HTTP 202 is returned with reused=true
- invalid requests: unsupported content types return HTTP 415, missing or invalid shared secrets return HTTP 401, malformed payloads, missing document_url, or document_url values without an embedded numeric document ID return HTTP 400

Recommended paperless-ngx workflow configuration:
- send the webhook as JSON
- set document_title to the document title and document_url to the Paperless document URL
- set the x-shared-secret header in the paperless webhook action so the backend can authenticate the request

### Webapp
- the webapp uses `flutter` as framework
- calls to the server use a paperless-ngx session token for authentication

#### Screens
- **login screen** - if no valid session token is stored in the web browser, the login screen is shown. The screen informs the user about the necessary login and that paperless-ngx credentials are used. Upon successful authentication, the user is forwarded to the dashboard screen.
- **dashboard screen** - the dashboard screen shows some statistics at the top of the screen (number of documents in the queue, average processing time per document, processing success rate). Below, a table shows the past processed documents, including the status, the used models, the timestamp and processing times. Each row can be clicked to show further resutls in a side panel that fades in from the right. The left side of the dashboard screen shows a side menu with menu items to access the other screens.
- **queue screen** - this screen shows a table of the documents in the processing queue. The table shows the document title, timestamp of the request, the ID from the backend assigned to this request and a button to trigger the processing.
- **configuration screen** - the configuration screen allows the user to configure the backend as described above.