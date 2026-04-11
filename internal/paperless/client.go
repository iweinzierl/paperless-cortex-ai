package paperless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type DocumentFilter struct {
	CorrespondentID *int64
	Limit           int
	PageSize        int
	Ordering        string
}

func NewClient(baseURL string, token string) *Client {
	return &Client{
		baseURL: strings.TrimSpace(baseURL),
		token:   strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (client *Client) WithHTTPClient(httpClient *http.Client) *Client {
	if httpClient != nil {
		client.httpClient = httpClient
	}
	return client
}

func (client *Client) ListTags(ctx context.Context) ([]Tag, error) {
	return listEntities[Tag](ctx, client, "tags")
}

func (client *Client) ListCorrespondents(ctx context.Context) ([]Correspondent, error) {
	return listEntities[Correspondent](ctx, client, "correspondents")
}

func (client *Client) ListDocumentTypes(ctx context.Context) ([]DocumentType, error) {
	return listEntities[DocumentType](ctx, client, "document_types")
}

func (client *Client) ListDocuments(ctx context.Context, filter DocumentFilter) ([]Document, error) {
	endpoint, err := client.buildEndpointURL("documents/")
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse paperless endpoint: %w", err)
	}

	query := parsed.Query()
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	if filter.Limit > 0 && filter.Limit < pageSize {
		pageSize = filter.Limit
	}
	query.Set("page_size", strconv.Itoa(pageSize))
	if filter.CorrespondentID != nil {
		query.Set("correspondent", strconv.FormatInt(*filter.CorrespondentID, 10))
	}
	if ordering := strings.TrimSpace(filter.Ordering); ordering != "" {
		query.Set("ordering", ordering)
	}
	parsed.RawQuery = query.Encode()

	documents := make([]Document, 0, minPositive(filter.Limit, 32))
	nextURL := parsed.String()
	for nextURL != "" {
		pageItems, pageNext, err := getPage[Document](ctx, client, nextURL)
		if err != nil {
			return nil, err
		}
		documents = append(documents, pageItems...)
		if filter.Limit > 0 && len(documents) >= filter.Limit {
			return documents[:filter.Limit], nil
		}
		nextURL = ResolveNextURL(parsed.String(), pageNext)
	}

	return documents, nil
}

func (client *Client) GetDocument(ctx context.Context, documentID int64) (*Document, error) {
	endpoint, err := client.buildEndpointURL(fmt.Sprintf("documents/%d/", documentID))
	if err != nil {
		return nil, err
	}

	var document Document
	if err := client.getJSON(ctx, endpoint, &document); err != nil {
		return nil, err
	}

	return &document, nil
}

func (client *Client) CreateTag(ctx context.Context, name string) (*Tag, error) {
	var created Tag
	if err := client.postJSON(ctx, "tags/", map[string]any{"name": strings.TrimSpace(name)}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (client *Client) CreateCorrespondent(ctx context.Context, name string) (*Correspondent, error) {
	var created Correspondent
	if err := client.postJSON(ctx, "correspondents/", map[string]any{"name": strings.TrimSpace(name)}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (client *Client) CreateDocumentType(ctx context.Context, name string) (*DocumentType, error) {
	var created DocumentType
	if err := client.postJSON(ctx, "document_types/", map[string]any{"name": strings.TrimSpace(name)}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (client *Client) PatchDocument(ctx context.Context, documentID int64, patch DocumentPatch) (*Document, error) {
	endpoint, err := client.buildEndpointURL(fmt.Sprintf("documents/%d/", documentID))
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("marshal document patch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build paperless patch request: %w", err)
	}
	client.decorateRequest(req)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("patch paperless document: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read paperless response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, os.ErrNotExist
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("paperless API returned %s for %s: %s", resp.Status, endpoint, strings.TrimSpace(string(responseBody)))
	}

	var document Document
	if err := json.Unmarshal(responseBody, &document); err != nil {
		return nil, fmt.Errorf("decode paperless response: %w", err)
	}
	return &document, nil
}

func (client *Client) DownloadDocument(ctx context.Context, documentID int64, destinationDir string) (*DownloadedFile, error) {
	endpoint, err := client.buildEndpointURL(fmt.Sprintf("documents/%d/download/", documentID))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build paperless download request: %w", err)
	}
	client.decorateRequest(req)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download paperless document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, os.ErrNotExist
	}
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("paperless download API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	fileName := downloadFileName(resp)
	fileName = normalizeDownloadFileName(fileName, strings.TrimSpace(resp.Header.Get("Content-Type")), documentID)

	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}

	destinationPath := filepath.Join(destinationDir, filepath.Base(fileName))
	file, err := os.Create(destinationPath)
	if err != nil {
		return nil, fmt.Errorf("create downloaded document: %w", err)
	}

	sizeBytes, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		os.Remove(destinationPath)
		return nil, fmt.Errorf("write downloaded document: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(destinationPath)
		return nil, fmt.Errorf("close downloaded document: %w", closeErr)
	}

	return &DownloadedFile{
		Path:        destinationPath,
		FileName:    filepath.Base(destinationPath),
		ContentType: strings.TrimSpace(resp.Header.Get("Content-Type")),
		SizeBytes:   sizeBytes,
	}, nil
}

func listEntities[T NamedEntity](ctx context.Context, client *Client, resource string) ([]T, error) {
	endpoint, err := client.buildEndpointURL(resource + "/")
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse paperless endpoint: %w", err)
	}
	query := parsed.Query()
	if query.Get("page_size") == "" {
		query.Set("page_size", "100")
	}
	parsed.RawQuery = query.Encode()

	items := make([]T, 0, 32)
	nextURL := parsed.String()
	for nextURL != "" {
		pageItems, pageNext, err := getPage[T](ctx, client, nextURL)
		if err != nil {
			return nil, err
		}
		items = append(items, pageItems...)
		nextURL = ResolveNextURL(parsed.String(), pageNext)
	}

	sort.Slice(items, func(left int, right int) bool {
		return strings.ToLower(strings.TrimSpace(items[left].GetName())) < strings.ToLower(strings.TrimSpace(items[right].GetName()))
	})

	return items, nil
}

func getPage[T any](ctx context.Context, client *Client, endpoint string) ([]T, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build paperless request: %w", err)
	}
	client.decorateRequest(req)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch paperless resource: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read paperless response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, "", fmt.Errorf("paperless API returned %s for %s: %s", resp.Status, endpoint, strings.TrimSpace(string(body)))
	}

	var paged struct {
		Results []T     `json:"results"`
		Next    *string `json:"next"`
	}
	if err := json.Unmarshal(body, &paged); err == nil && (paged.Results != nil || paged.Next != nil) {
		next := ""
		if paged.Next != nil {
			next = *paged.Next
		}
		return paged.Results, next, nil
	}

	var flat []T
	if err := json.Unmarshal(body, &flat); err == nil {
		return flat, "", nil
	}

	return nil, "", fmt.Errorf("decode paperless response from %s: %s", endpoint, strings.TrimSpace(string(body)))
}

func (client *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build paperless request: %w", err)
	}
	client.decorateRequest(req)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call paperless API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read paperless response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return os.ErrNotExist
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("paperless API returned %s for %s: %s", resp.Status, endpoint, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode paperless response: %w", err)
	}
	return nil
}

func (client *Client) postJSON(ctx context.Context, resourcePath string, body any, target any) error {
	endpoint, err := client.buildEndpointURL(resourcePath)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal paperless request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build paperless request: %w", err)
	}
	client.decorateRequest(req)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call paperless API: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read paperless response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("paperless API returned %s for %s: %s", resp.Status, endpoint, strings.TrimSpace(string(responseBody)))
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return fmt.Errorf("decode paperless response: %w", err)
	}
	return nil
}

func (client *Client) decorateRequest(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	if client.token != "" {
		req.Header.Set("Authorization", "Token "+client.token)
	}
	if req.Method != http.MethodGet && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
}

func (client *Client) buildEndpointURL(resourcePath string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(client.baseURL))
	if err != nil {
		return "", fmt.Errorf("parse paperless URL: %w", err)
	}

	cleanResourcePath := strings.TrimPrefix(strings.TrimSpace(resourcePath), "/")
	basePath := strings.TrimRight(parsed.Path, "/")
	switch {
	case basePath == "":
		parsed.Path = path.Join("/api", cleanResourcePath)
	case strings.HasSuffix(basePath, "/api"):
		parsed.Path = path.Join(basePath, cleanResourcePath)
	case strings.Contains(basePath, "/api/") || strings.HasSuffix(basePath, "/api/"):
		parsed.Path = path.Join(basePath, cleanResourcePath)
	default:
		parsed.Path = path.Join(basePath, "/api", cleanResourcePath)
	}

	if strings.HasSuffix(resourcePath, "/") && !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}

	return parsed.String(), nil
}

func ResolveNextURL(baseURL string, next string) string {
	if next == "" {
		return ""
	}

	parsed, err := url.Parse(next)
	if err != nil {
		return next
	}

	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return next
	}

	if parsed.IsAbs() {
		if strings.EqualFold(parsed.Host, base.Host) || parsed.Host == "" {
			parsed.Scheme = base.Scheme
			parsed.Host = base.Host
			if parsed.Path == "" {
				parsed.Path = base.Path
			}
		}

		return parsed.String()
	}

	return base.ResolveReference(parsed).String()
}

func downloadFileName(resp *http.Response) string {
	contentDisposition := strings.TrimSpace(resp.Header.Get("Content-Disposition"))
	if contentDisposition != "" {
		_, params, err := mime.ParseMediaType(contentDisposition)
		if err == nil {
			if fileName := strings.TrimSpace(params["filename"]); fileName != "" {
				return fileName
			}
			if fileName := strings.TrimSpace(params["filename*"]); fileName != "" {
				parts := strings.SplitN(fileName, "''", 2)
				if len(parts) == 2 {
					decoded, err := url.QueryUnescape(parts[1])
					if err == nil && decoded != "" {
						return decoded
					}
				}
			}
		}
	}

	if resp.Request != nil && resp.Request.URL != nil {
		base := path.Base(strings.TrimSpace(resp.Request.URL.Path))
		if base != "" && base != "." && base != "/" {
			if _, err := strconv.ParseInt(base, 10, 64); err != nil {
				return base
			}
		}
	}

	return ""
}

func normalizeDownloadFileName(fileName string, contentType string, documentID int64) string {
	trimmedName := filepath.Base(strings.TrimSpace(fileName))
	inferredExtension := inferredFileExtension(contentType)
	if trimmedName == "" || trimmedName == "." || trimmedName == string(filepath.Separator) {
		if inferredExtension == "" {
			inferredExtension = ".bin"
		}
		return fmt.Sprintf("document-%d%s", documentID, inferredExtension)
	}

	if filepath.Ext(trimmedName) == "" && inferredExtension != "" {
		return trimmedName + inferredExtension
	}

	return trimmedName
}

func inferredFileExtension(contentType string) string {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if mediaType == "" {
		return ""
	}
	if parsedMediaType, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsedMediaType
	}

	switch mediaType {
	case "application/pdf":
		return ".pdf"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "text/plain":
		return ".txt"
	default:
		return ""
	}
}

func minPositive(left int, right int) int {
	if left <= 0 {
		return right
	}
	if right <= 0 || left < right {
		return left
	}
	return right
}
