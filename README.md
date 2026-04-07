# Introduction
This project is supposed to become an extension of paperless-ngx (https://github.com/paperless-ngx/paperless-ngx). Whereas paperless-ngx uses internal tools to determine document types, correspondents, tags and others, this project uses large language models (LLMs) to ocr documents and determine such.

# Project Structure
- **backend** - the backend serving a RESTful API to manage settings of this extension
- **cmd** - command line scripts that can be executed using the Makefile. The scripts are supposed to support the development of the backend.
- **webapp* - a web application which interacts with the RESTful API provided by the backend. It's used by users to configure the extension.

## Backend
The backend is written in Golang.

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
The webapp uses `flutter` as framework.