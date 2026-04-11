package paperless

import (
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
