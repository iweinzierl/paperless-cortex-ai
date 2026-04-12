package paperless

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

func TestClientListDocumentsAppliesFiltersAndLimit(t *testing.T) {
	correspondentID := int64(7)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/documents/" {
			http.NotFound(w, r)
			return
		}

		requests++
		query := r.URL.Query()
		if query.Get("correspondent") != strconv.FormatInt(correspondentID, 10) {
			t.Fatalf("expected correspondent filter, got %q", query.Get("correspondent"))
		}
		if query.Get("ordering") != "-created" {
			t.Fatalf("expected ordering filter, got %q", query.Get("ordering"))
		}
		if query.Get("page_size") != "2" {
			t.Fatalf("expected page size 2, got %q", query.Get("page_size"))
		}

		w.Header().Set("Content-Type", "application/json")
		switch query.Get("page") {
		case "2":
			_, _ = w.Write([]byte(`{"results":[{"id":3,"title":"Doc 3","correspondent":7,"content":"gamma"},{"id":4,"title":"Doc 4","correspondent":7,"content":"delta"}],"next":null}`))
		default:
			_, _ = w.Write([]byte(`{"results":[{"id":1,"title":"Doc 1","correspondent":7,"content":"alpha"},{"id":2,"title":"Doc 2","correspondent":7,"content":"beta"}],"next":"/api/documents/?page=2&page_size=2&ordering=-created&correspondent=7"}`))
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	documents, err := client.ListDocuments(t.Context(), DocumentFilter{
		CorrespondentID: &correspondentID,
		Limit:           3,
		PageSize:        2,
		Ordering:        "-created",
	})
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}
	if len(documents) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(documents))
	}
	if documents[2].ID != 3 {
		t.Fatalf("expected third document to be id 3, got %+v", documents[2])
	}
}

func TestClientDownloadDocumentInfersExtensionFromContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/documents/42/download/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.4\n")
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	downloaded, err := client.DownloadDocument(t.Context(), 42, t.TempDir())
	if err != nil {
		t.Fatalf("download document: %v", err)
	}

	if filepath.Ext(downloaded.Path) != ".pdf" {
		t.Fatalf("expected inferred .pdf extension, got %q", downloaded.Path)
	}
	if downloaded.FileName != "download.pdf" {
		t.Fatalf("expected normalized filename download.pdf, got %q", downloaded.FileName)
	}
	if downloaded.ContentType != "application/pdf" {
		t.Fatalf("expected application/pdf content type, got %q", downloaded.ContentType)
	}
}

func TestClientCreateTagPostsJSONWithAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Token token" {
			t.Fatalf("expected authorization header, got %q", got)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["name"] != "processed" {
			t.Fatalf("expected tag name processed, got %+v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":9,"name":"processed"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	tag, err := client.CreateTag(t.Context(), " processed ")
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if tag.ID != 9 || tag.Name != "processed" {
		t.Fatalf("unexpected created tag: %+v", tag)
	}
}

func TestClientPatchDocumentSendsPatchPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/documents/42/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH request, got %s", r.Method)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["title"].(string) != "Patched title" {
			t.Fatalf("expected title Patched title, got %+v", payload)
		}
		if payload["created"].(string) != "2026-03-15" {
			t.Fatalf("expected created 2026-03-15, got %+v", payload)
		}
		if payload["correspondent"].(float64) != 7 {
			t.Fatalf("expected correspondent 7, got %+v", payload)
		}
		if payload["document_type"].(float64) != 5 {
			t.Fatalf("expected document type 5, got %+v", payload)
		}
		tags := payload["tags"].([]any)
		if len(tags) != 2 || tags[0].(float64) != 3 || tags[1].(float64) != 4 {
			t.Fatalf("expected tags [3,4], got %+v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"title":"Patched","correspondent":7,"document_type":5,"tags":[3,4]}`))
	}))
	defer server.Close()

	correspondentID := int64(7)
	documentTypeID := int64(5)
	title := "Patched title"
	created := "2026-03-15"
	client := NewClient(server.URL, "token")
	document, err := client.PatchDocument(t.Context(), 42, DocumentPatch{
		Title:           &title,
		Created:         &created,
		CorrespondentID: &correspondentID,
		DocumentTypeID:  &documentTypeID,
		TagIDs:          []int64{3, 4},
	})
	if err != nil {
		t.Fatalf("patch document: %v", err)
	}
	if document.ID != 42 || document.Title != "Patched" {
		t.Fatalf("unexpected patched document: %+v", document)
	}
}
