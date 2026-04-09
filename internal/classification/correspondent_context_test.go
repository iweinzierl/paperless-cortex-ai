package classification

import (
	"strings"
	"testing"

	"paperless-ai-ext/internal/paperless"
)

func TestBuildCorrespondentPromptContextShortlistsUsingHistoricalEvidence(t *testing.T) {
	correspondents := []paperless.Correspondent{
		{ID: 1, Name: "Telekom"},
		{ID: 2, Name: "Vodafone"},
		{ID: 3, Name: "Stadtwerke"},
		{ID: 4, Name: "Allianz"},
		{ID: 5, Name: "Filler A"},
		{ID: 6, Name: "Filler B"},
		{ID: 7, Name: "Filler C"},
		{ID: 8, Name: "Filler D"},
		{ID: 9, Name: "Filler E"},
		{ID: 10, Name: "Filler F"},
		{ID: 11, Name: "Filler G"},
		{ID: 12, Name: "Filler H"},
		{ID: 13, Name: "Filler I"},
	}
	historicalDocuments := []paperless.Document{
		{ID: 101, Title: "Telekom Rechnung April", OriginalFileName: "telekom-april.pdf", CorrespondentID: int64Pointer(1), Content: "Telekom Rechnung Kundennummer Anschluss Rechnungsnummer Monatsbetrag"},
		{ID: 102, Title: "Vodafone Rechnung", OriginalFileName: "vodafone.pdf", CorrespondentID: int64Pointer(2), Content: "Vodafone Rechnung Kundennummer Tarif Vertragsnummer"},
		{ID: 103, Title: "Stadtwerke Abschlag", OriginalFileName: "stadtwerke.pdf", CorrespondentID: int64Pointer(3), Content: "Stadtwerke Rechnung Abschlag Zaehler Vertragskonto"},
		{ID: 104, Title: "Allianz Beitrag", OriginalFileName: "allianz.pdf", CorrespondentID: int64Pointer(4), Content: "Allianz Versicherung Beitrag Vertragsnummer Policennummer"},
	}

	context := buildCorrespondentPromptContext(
		"telekom-mai.pdf",
		"Ihre Telekom Rechnung fuer Mai enthaelt die Kundennummer 123 und eine neue Rechnungsnummer.",
		correspondents,
		historicalDocuments,
	)

	if !context.Shortlisted {
		t.Fatalf("expected shortlist to be enabled")
	}
	if len(context.PromptCorrespondents) == 0 || len(context.PromptCorrespondents) > maxCorrespondentCandidates {
		t.Fatalf("unexpected shortlisted candidate count: %d", len(context.PromptCorrespondents))
	}
	if context.PromptCorrespondents[0].Name != "Telekom" {
		t.Fatalf("expected Telekom to rank first, got %+v", context.PromptCorrespondents[0])
	}
	if len(context.EvidenceCandidates) == 0 || len(context.EvidenceCandidates[0].Examples) == 0 {
		t.Fatalf("expected historical evidence candidates, got %+v", context.EvidenceCandidates)
	}

	prompt := buildCorrespondentPrompt(
		"telekom-mai.pdf",
		"Ihre Telekom Rechnung fuer Mai enthaelt die Kundennummer 123 und eine neue Rechnungsnummer.",
		correspondents,
		historicalDocuments,
	)
	if !strings.Contains(prompt, "Historical library evidence") {
		t.Fatalf("expected prompt to include historical evidence, got %q", prompt)
	}
	if !strings.Contains(prompt, "Telekom Rechnung April") {
		t.Fatalf("expected Telekom historical example in prompt, got %q", prompt)
	}
	if strings.Contains(prompt, "Filler I") {
		t.Fatalf("expected shortlist prompt to exclude unrelated filler correspondents, got %q", prompt)
	}
}

func TestBuildCorrespondentPromptIsDeterministic(t *testing.T) {
	correspondents := []paperless.Correspondent{
		{ID: 1, Name: "Deutsche Telekom"},
		{ID: 2, Name: "Vodafone"},
	}
	historicalDocuments := []paperless.Document{
		{ID: 101, Title: "Telekom Rechnung April", OriginalFileName: "telekom-april.pdf", CorrespondentID: int64Pointer(1), Content: "Kundennummer 123 Rechnungsnummer 999 Deutsche Telekom Monatsbetrag"},
	}

	first := buildCorrespondentPrompt(
		"telekom-mai.pdf",
		"Deutsche Telekom Rechnung fuer Mai mit Kundennummer 123 und Rechnungsnummer 1000.",
		correspondents,
		historicalDocuments,
	)
	for iteration := 0; iteration < 5; iteration++ {
		next := buildCorrespondentPrompt(
			"telekom-mai.pdf",
			"Deutsche Telekom Rechnung fuer Mai mit Kundennummer 123 und Rechnungsnummer 1000.",
			correspondents,
			historicalDocuments,
		)
		if next != first {
			t.Fatalf("expected deterministic prompt output across runs")
		}
	}
}
