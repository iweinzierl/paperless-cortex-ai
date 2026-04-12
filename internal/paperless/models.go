package paperless

import "encoding/json"

type NamedEntity interface {
	GetID() int64
	GetName() string
}

type Tag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (tag Tag) GetID() int64 {
	return tag.ID
}

func (tag Tag) GetName() string {
	return tag.Name
}

type Correspondent struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (correspondent Correspondent) GetID() int64 {
	return correspondent.ID
}

func (correspondent Correspondent) GetName() string {
	return correspondent.Name
}

type DocumentType struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (documentType DocumentType) GetID() int64 {
	return documentType.ID
}

func (documentType DocumentType) GetName() string {
	return documentType.Name
}

type Document struct {
	ID                int64           `json:"id"`
	Title             string          `json:"title"`
	OriginalFileName  string          `json:"original_file_name"`
	ArchivedFileName  string          `json:"archived_file_name"`
	CorrespondentID   *int64          `json:"correspondent,omitempty"`
	DocumentTypeID    *int64          `json:"document_type,omitempty"`
	TagIDs            []int64         `json:"tags,omitempty"`
	Content           string          `json:"content,omitempty"`
	Created           string          `json:"created,omitempty"`
	Added             string          `json:"added,omitempty"`
	Modified          string          `json:"modified,omitempty"`
	DocumentURL       string          `json:"document_url,omitempty"`
	OriginalFileURL   string          `json:"original_file,omitempty"`
	ArchivedFileURL   string          `json:"archived_file,omitempty"`
	Notes             json.RawMessage `json:"notes,omitempty"`
	CorrespondentName string          `json:"-"`
	DocumentTypeName  string          `json:"-"`
}

type DocumentPatch struct {
	Title           *string `json:"title,omitempty"`
	Created         *string `json:"created,omitempty"`
	CorrespondentID *int64  `json:"correspondent,omitempty"`
	DocumentTypeID  *int64  `json:"document_type,omitempty"`
	TagIDs          []int64 `json:"tags,omitempty"`
}

type DownloadedFile struct {
	Path        string
	FileName    string
	ContentType string
	SizeBytes   int64
}
