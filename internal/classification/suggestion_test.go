package classification

import (
	"testing"

	"paperless-ai-ext/internal/paperless"
)

func TestParseCorrespondentSuggestionPrefersMatchingNameWhenIDConflicts(t *testing.T) {
	correspondents := []paperless.Correspondent{
		{ID: 12, Name: "Fuxtec GmbH"},
		{ID: 21, Name: "Deutsche Telekom"},
	}

	suggestion, err := ParseCorrespondentSuggestion(`{
		"correspondent_id": 12,
		"correspondent_name": "Deutsche Telekom",
		"suggested_new_correspondent": null,
		"confidence": "high",
		"reasoning": "The bill clearly comes from Deutsche Telekom."
	}`, correspondents)
	if err != nil {
		t.Fatalf("parse correspondent suggestion: %v", err)
	}
	if suggestion.CorrespondentID == nil || *suggestion.CorrespondentID != 21 {
		t.Fatalf("expected Deutsche Telekom ID 21, got %+v", suggestion.CorrespondentID)
	}
	if suggestion.CorrespondentName == nil || *suggestion.CorrespondentName != "Deutsche Telekom" {
		t.Fatalf("expected Deutsche Telekom name, got %+v", suggestion.CorrespondentName)
	}
}

func TestParseDocumentTypeSuggestionPrefersMatchingNameWhenIDConflicts(t *testing.T) {
	documentTypes := []paperless.DocumentType{
		{ID: 7, Name: "Invoice"},
		{ID: 11, Name: "Contract"},
	}

	suggestion, err := ParseDocumentTypeSuggestion(`{
		"document_type_id": 7,
		"document_type_name": "Contract",
		"suggested_new_document_type": null,
		"confidence": "medium",
		"reasoning": "This reads like a signed contract."
	}`, documentTypes)
	if err != nil {
		t.Fatalf("parse document type suggestion: %v", err)
	}
	if suggestion.DocumentTypeID == nil || *suggestion.DocumentTypeID != 11 {
		t.Fatalf("expected Contract ID 11, got %+v", suggestion.DocumentTypeID)
	}
	if suggestion.DocumentTypeName == nil || *suggestion.DocumentTypeName != "Contract" {
		t.Fatalf("expected Contract name, got %+v", suggestion.DocumentTypeName)
	}
}

func TestParseCorrespondentSuggestionUsesReasoningToRepairContradiction(t *testing.T) {
	correspondents := []paperless.Correspondent{
		{ID: 12, Name: "Fuxtec GmbH"},
		{ID: 21, Name: "Telekom"},
	}

	suggestion, err := ParseCorrespondentSuggestion(`{
		"correspondent_id": 12,
		"correspondent_name": "Fuxtec GmbH",
		"suggested_new_correspondent": null,
		"confidence": "high",
		"reasoning": "The bill clearly comes from Telekom."
	}`, correspondents)
	if err != nil {
		t.Fatalf("parse correspondent suggestion: %v", err)
	}
	if suggestion.CorrespondentID == nil || *suggestion.CorrespondentID != 21 {
		t.Fatalf("expected Telekom ID 21 after reasoning reconciliation, got %+v", suggestion.CorrespondentID)
	}
	if suggestion.CorrespondentName == nil || *suggestion.CorrespondentName != "Telekom" {
		t.Fatalf("expected Telekom name after reasoning reconciliation, got %+v", suggestion.CorrespondentName)
	}
	if suggestion.Confidence != "medium" {
		t.Fatalf("expected downgraded confidence, got %q", suggestion.Confidence)
	}
}

func TestParseCorrespondentSuggestionRepairsContradictionUsingDistinctivePartialName(t *testing.T) {
	correspondents := []paperless.Correspondent{
		{ID: 12, Name: "Fuxtec GmbH"},
		{ID: 21, Name: "Deutsche Telekom"},
	}

	suggestion, err := ParseCorrespondentSuggestion(`{
		"correspondent_id": 12,
		"correspondent_name": "Fuxtec GmbH",
		"suggested_new_correspondent": null,
		"confidence": "high",
		"reasoning": "The bill clearly comes from Telekom."
	}`, correspondents)
	if err != nil {
		t.Fatalf("parse correspondent suggestion: %v", err)
	}
	if suggestion.CorrespondentID == nil || *suggestion.CorrespondentID != 21 {
		t.Fatalf("expected Deutsche Telekom ID 21 after reasoning reconciliation, got %+v", suggestion.CorrespondentID)
	}
	if suggestion.CorrespondentName == nil || *suggestion.CorrespondentName != "Deutsche Telekom" {
		t.Fatalf("expected Deutsche Telekom name after reasoning reconciliation, got %+v", suggestion.CorrespondentName)
	}
	if suggestion.Confidence != "medium" {
		t.Fatalf("expected downgraded confidence, got %q", suggestion.Confidence)
	}
}

func TestParseCorrespondentSuggestionKeepsSelectionWhenReasoningIsAmbiguous(t *testing.T) {
	correspondents := []paperless.Correspondent{
		{ID: 12, Name: "Fuxtec GmbH"},
		{ID: 21, Name: "Telekom"},
	}

	suggestion, err := ParseCorrespondentSuggestion(`{
		"correspondent_id": 12,
		"correspondent_name": "Fuxtec GmbH",
		"suggested_new_correspondent": null,
		"confidence": "high",
		"reasoning": "The document looks like a vendor invoice."
	}`, correspondents)
	if err != nil {
		t.Fatalf("parse correspondent suggestion: %v", err)
	}
	if suggestion.CorrespondentID == nil || *suggestion.CorrespondentID != 12 {
		t.Fatalf("expected original Fuxtec selection to remain, got %+v", suggestion.CorrespondentID)
	}
	if suggestion.CorrespondentName == nil || *suggestion.CorrespondentName != "Fuxtec GmbH" {
		t.Fatalf("expected original Fuxtec name to remain, got %+v", suggestion.CorrespondentName)
	}
	if suggestion.Confidence != "high" {
		t.Fatalf("expected confidence to remain high, got %q", suggestion.Confidence)
	}
}
