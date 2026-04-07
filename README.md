# Introduction
This project is supposed to become an extension of paperless-ngx (https://github.com/paperless-ngx/paperless-ngx). Whereas paperless-ngx uses internal tools to determine document types, correspondents, tags and others, this project uses large language models (LLMs) to ocr documents and determine such.

# Project Structure
- **backend** - the backend serving a RESTful API to manage settings of this extension
- **cmd** - command line scripts that can be executed using the Makefile. The scripts are supposed to support the development of the backend.
- **webapp* - a web application which interacts with the RESTful API provided by the backend. It's used by users to configure the extension.

## Backend
Users must configure a set of tags. The backend observes document changes and the set tags. If the tag **process_trigger_tag** is set, the backend starts processing the document. The processing steps will be determined through further tags: **force_ocr_tag**, **force_vision_tag**, **process_correspondent_tag**, **process_document_type_tag**, **process_document_tags_tag**. If the processing of a document is completed, the **process_completed_tag** is set and the **process_trigger_tag** is unset.

### Configuration
The backend's engine must be configured by users. The configuration is structured in the following sections:

#### Configuration: Engine
- **processing_mode** - the processing mode can either be `manual` or `auto`. In case of manual mode, documents that requested processing must be triggered manually through the webapp. If it's set to auto, the engine automatically picks and processes a document every X seconds, as configured by the user, based on the timestamp when the processing was requested (FIFO).
- **processing_interval** - the interval used in auto processing mode to schedule the processing of documents.

#### Configuration: Process
- **process_trigger_tag** - A document tag used to trigger the processing in the backend.
- **force_ocr_tag** - A document tag used to enforce an OCR screening in the backend, ignoring the metadata that might be stored with the existing document already.
- **force_vision_tag** - A document tag used to enforce text extraction using a Vision LLM (VLM).
- **process_correspondent_tag** - A document tag to request a fix of the document's correspondent.
- **process_document_type_tag** - A document tag to request a fix of the document's type.
- **process_document_tags_tag** - A document tag to request a fix of the document's tags. 
- **process_completed_tag** - A document tag that shall be set if the processing completed successfully.

#### Configuration: Paperless
- **paperless_url** - The URL to the paperless-ngx instance.
- **paperless_token** - The authentication token used to access the paperless-ngx instance.

#### Configuration: LLMs
- **ollama_url** - The URL under which the ollama is accessible.
- **default_llm** - The default LLM used for recommendation for correspondents, document types and tags.
- **vision_llm** - The vision LLM (VLM) used for text extraction.


### Features
- **OCR Screening** - the ocr screening features uses configured LLMs to read the content of documents, for example PDF documents. After successful screening, the content is returned as text which can be further processed to determine correspondents, document type and others.

## CMD
The command line scripts are written in Golang as well and can be executed using the project's Makefile:
- **ocr_screening** - this command line script accepts the path to a document which shall be screened and the LLM that shall be used for the screening. The script uses ollama's chat API to trigger the screening process. After successful screening, it returns the document's content as plain text.
- **ocr_vision_screening** - this command line script accepts the path to an image or PDF document and the vision LLM that shall be used for screening. Image documents are sent directly to ollama's chat API. PDFs are rendered to page images before they are sent to the model. After successful screening, it returns the document's content as plain text.
- **process_correspondents** - this command line script accepts the path to a document which shall be screened for its correspondents and the LLM that shall be used for screening and suggesting correspondents. The script shall read all existing correspondents from the paperless-ngx instance. The env variable PAPERLESS_TOKEN which contains the authentication token for paperless-ngx will be used for authentication to the paperless-ngx instance. The PAPERLESS_URL contains the url to this instance.
- **process_document_types** - this command line script accepts the path to a document which shall be screened for its document type and the LLM that shall be used for screening and suggesting document types. The script shall read all existing document types from the paperless-ngx instance. The env variable PAPERLESS_TOKEN which contains the authentication token for paperless-ngx will be used for authentication to the paperless-ngx instance. The PAPERLESS_URL contains the url to this instance.
- **process_tags** - this command line script accepts the path to a document which shall be screened and the LLM that shall be used for screening. It first tries a simple OCR/text extraction path and falls back to a vision LLM when no usable text can be extracted. You can also force vision-based screening explicitly. The script reads the existing tags from your paperless-ngx instance and suggests a set of matching existing tags plus optional new tags when none of the existing tags fit well. Authentication is handled via the PAPERLESS_URL and PAPERLESS_TOKEN variables.

## Webapp
The webapp is a management interface for users to configure the backend. Users are able to manually trigger the processing of documents if manual processing mode is selected.

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

### Webapp
- the webapp uses `flutter` as framework
- calls to the server use a paperless-ngx session token for authentication

#### Screens
- **login screen** - if no valid session token is stored in the web browser, the login screen is shown. The screen informs the user about the necessary login and that paperless-ngx credentials are used. Upon successful authentication, the user is forwarded to the dashboard screen.
- **dashboard screen** - the dashboard screen shows some statistics at the top of the screen (number of documents in the queue, average processing time per document, processing success rate). Below, a table shows the past processed documents, including the status, the used models, the timestamp and processing times. Each row can be clicked to show further resutls in a side panel that fades in from the right. The left side of the dashboard screen shows a side menu with menu items to access the other screens.
- **queue screen** - this screen shows a table of the documents in the processing queue. The table shows the document title, timestamp of the request, the ID from the backend assigned to this request and a button to trigger the processing.
- **configuration screen** - the configuration screen allows the user to configure the backend as described above.