package classification

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"paperless-ai-ext/internal/paperless"
)

const (
	maxCorrespondentCandidates        = 8
	minShortlistCandidates            = 4
	maxEvidenceCandidates             = 5
	maxHistoricalExamplesPerCandidate = 2
	maxPromptDocumentTextChars        = 12000
	maxHistoricalSnippetChars         = 320
	shortlistThresholdCount           = 12
)

var trivialDocumentTokens = map[string]struct{}{
	"aber":  {},
	"about": {},
	"auch":  {},
	"bei":   {},
	"das":   {},
	"dem":   {},
	"der":   {},
	"die":   {},
	"eine":  {},
	"einer": {},
	"eines": {},
	"einen": {},
	"einem": {},
	"for":   {},
	"from":  {},
	"für":   {},
	"have":  {},
	"hier":  {},
	"mit":   {},
	"oder":  {},
	"the":   {},
	"this":  {},
	"und":   {},
	"vom":   {},
	"von":   {},
	"with":  {},
	"your":  {},
}

type historicalDocumentExample struct {
	Title    string
	FileName string
	Snippet  string
	Overlap  int
}

type correspondentCandidateContext struct {
	Correspondent paperless.Correspondent
	Score         int
	Signals       []string
	Examples      []historicalDocumentExample
}

type correspondentPromptContext struct {
	PromptCorrespondents []paperless.Correspondent
	EvidenceCandidates   []correspondentCandidateContext
	Shortlisted          bool
}

func buildCorrespondentPrompt(documentName string, documentText string, correspondents []paperless.Correspondent, historicalDocuments []paperless.Document) string {
	promptContext := buildCorrespondentPromptContext(documentName, documentText, correspondents, historicalDocuments)
	correspondentHeading := "Existing correspondents:"
	if promptContext.Shortlisted {
		correspondentHeading = "Candidate correspondents shortlisted from library evidence:"
	}

	historicalEvidence := buildHistoricalEvidenceSection(promptContext.EvidenceCandidates)
	rules := strictJSONOutputRules
	if promptContext.Shortlisted {
		rules += "\n- The candidate list is a shortlist based on the current document and similar historical documents."
		rules += "\n- If none of the shortlisted candidates fit, suggest a new correspondent instead of forcing a weak match."
	} else {
		rules += "\n- Use the historical examples as supporting evidence when they align with the current document."
	}

	return fmt.Sprintf(
		correspondentPromptTemplate,
		rules,
		correspondentHeading,
		buildEntityList(promptContext.PromptCorrespondents, "No existing correspondents available"),
		historicalEvidence,
		documentName,
		trimForPrompt(documentText, maxPromptDocumentTextChars),
	)
}

func buildCorrespondentPromptContext(documentName string, documentText string, correspondents []paperless.Correspondent, historicalDocuments []paperless.Document) correspondentPromptContext {
	if len(correspondents) == 0 {
		return correspondentPromptContext{}
	}

	queryText := strings.TrimSpace(documentName + "\n" + documentText)
	queryTerms := orderedUniqueTokens(queryText)
	queryTokens := tokenSet(queryText)
	historyByCorrespondent := make(map[int64][]paperless.Document)
	for _, document := range historicalDocuments {
		if document.CorrespondentID == nil {
			continue
		}
		if sameHistoricalDocument(documentName, document) {
			continue
		}
		historyByCorrespondent[*document.CorrespondentID] = append(historyByCorrespondent[*document.CorrespondentID], document)
	}

	candidates := make([]correspondentCandidateContext, 0, len(correspondents))
	for _, correspondent := range correspondents {
		candidate := scoreCorrespondentCandidate(correspondent, queryText, queryTerms, queryTokens, historyByCorrespondent[correspondent.ID])
		candidates = append(candidates, candidate)
	}

	sort.SliceStable(candidates, func(left int, right int) bool {
		if candidates[left].Score != candidates[right].Score {
			return candidates[left].Score > candidates[right].Score
		}
		if len(candidates[left].Examples) != len(candidates[right].Examples) {
			return len(candidates[left].Examples) > len(candidates[right].Examples)
		}
		return strings.ToLower(candidates[left].Correspondent.Name) < strings.ToLower(candidates[right].Correspondent.Name)
	})

	promptCorrespondents := append([]paperless.Correspondent(nil), correspondents...)
	evidenceCandidates := topEvidenceCandidates(candidates, maxEvidenceCandidates)
	shortlisted := false
	if len(correspondents) > shortlistThresholdCount {
		strongCandidates := filterStrongCandidates(candidates)
		if len(strongCandidates) >= minShortlistCandidates {
			if len(strongCandidates) > maxCorrespondentCandidates {
				strongCandidates = strongCandidates[:maxCorrespondentCandidates]
			}
			promptCorrespondents = correspondentsFromCandidates(strongCandidates)
			evidenceCandidates = strongCandidates
			shortlisted = true
		}
	}

	return correspondentPromptContext{
		PromptCorrespondents: promptCorrespondents,
		EvidenceCandidates:   evidenceCandidates,
		Shortlisted:          shortlisted,
	}
}

func scoreCorrespondentCandidate(correspondent paperless.Correspondent, queryText string, queryTerms []string, queryTokens map[string]struct{}, historicalDocuments []paperless.Document) correspondentCandidateContext {
	candidate := correspondentCandidateContext{Correspondent: correspondent}
	normalizedQuery := normalizeFreeText(queryText)
	normalizedName := normalizeFreeText(correspondent.Name)
	if normalizedName != "" && strings.Contains(normalizedQuery, normalizedName) {
		candidate.Score += 120
		candidate.Signals = append(candidate.Signals, "full correspondent name appears in the document")
	}

	nameTokenMatches := 0
	for _, token := range tokenize(correspondent.Name) {
		if _, ok := queryTokens[token]; !ok {
			continue
		}
		candidate.Score += 18
		nameTokenMatches++
	}
	if nameTokenMatches > 0 {
		candidate.Signals = append(candidate.Signals, fmt.Sprintf("%d correspondent-name token(s) overlap with the document", nameTokenMatches))
	}

	examples := make([]historicalDocumentExample, 0, len(historicalDocuments))
	for _, document := range historicalDocuments {
		sampleText := buildHistoricalSampleText(document)
		overlap := overlapCount(queryTokens, tokenSet(sampleText))
		if overlap == 0 {
			continue
		}
		candidate.Score += minInt(overlap, 12) * 3
		if titleOverlap := overlapCount(queryTokens, tokenSet(document.Title+" "+document.OriginalFileName)); titleOverlap > 0 {
			candidate.Score += minInt(titleOverlap, 4) * 4
		}
		examples = append(examples, historicalDocumentExample{
			Title:    strings.TrimSpace(document.Title),
			FileName: strings.TrimSpace(document.OriginalFileName),
			Snippet:  buildHistoricalSnippet(document, queryTerms),
			Overlap:  overlap,
		})
	}

	if len(examples) > 0 {
		sort.SliceStable(examples, func(left int, right int) bool {
			if examples[left].Overlap != examples[right].Overlap {
				return examples[left].Overlap > examples[right].Overlap
			}
			return examples[left].Title < examples[right].Title
		})
		if len(examples) > maxHistoricalExamplesPerCandidate {
			examples = examples[:maxHistoricalExamplesPerCandidate]
		}
		candidate.Examples = examples
		candidate.Signals = append(candidate.Signals, fmt.Sprintf("%d historical document example(s) share distinctive terms", len(examples)))
	}

	candidate.Signals = normalizeSignals(candidate.Signals)
	return candidate
}

func buildHistoricalEvidenceSection(candidates []correspondentCandidateContext) string {
	if len(candidates) == 0 {
		return "Historical library evidence:\n- No strong historical matches were found."
	}

	var builder strings.Builder
	builder.WriteString("Historical library evidence:\n")
	for _, candidate := range candidates {
		builder.WriteString("- ")
		builder.WriteString(candidate.Correspondent.Name)
		if len(candidate.Signals) > 0 {
			builder.WriteString(" | ")
			builder.WriteString(strings.Join(candidate.Signals, "; "))
		}
		builder.WriteByte('\n')
		for _, example := range candidate.Examples {
			builder.WriteString("  Example: ")
			builder.WriteString(exampleTitle(example))
			if example.Snippet != "" {
				builder.WriteString(" -> ")
				builder.WriteString(example.Snippet)
			}
			builder.WriteByte('\n')
		}
	}

	return strings.TrimSpace(builder.String())
}

func filterStrongCandidates(candidates []correspondentCandidateContext) []correspondentCandidateContext {
	strong := make([]correspondentCandidateContext, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Score <= 0 {
			continue
		}
		if len(candidate.Examples) == 0 && candidate.Score < 36 {
			continue
		}
		strong = append(strong, candidate)
	}
	return strong
}

func topEvidenceCandidates(candidates []correspondentCandidateContext, limit int) []correspondentCandidateContext {
	selected := make([]correspondentCandidateContext, 0, limit)
	for _, candidate := range candidates {
		if candidate.Score <= 0 && len(candidate.Examples) == 0 {
			continue
		}
		selected = append(selected, candidate)
		if len(selected) == limit {
			break
		}
	}
	return selected
}

func correspondentsFromCandidates(candidates []correspondentCandidateContext) []paperless.Correspondent {
	items := make([]paperless.Correspondent, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, candidate.Correspondent)
	}
	return items
}

func buildHistoricalSampleText(document paperless.Document) string {
	return strings.TrimSpace(strings.Join([]string{document.Title, document.OriginalFileName, trimForPrompt(document.Content, 1500)}, "\n"))
}

func buildHistoricalSnippet(document paperless.Document, queryTerms []string) string {
	content := compactWhitespace(document.Content)
	if content == "" {
		return compactWhitespace(strings.TrimSpace(strings.Join([]string{document.Title, document.OriginalFileName}, " ")))
	}

	normalizedContent := normalizeFreeText(content)
	for _, token := range queryTerms {
		if token == "" {
			continue
		}
		index := strings.Index(normalizedContent, token)
		if index < 0 {
			continue
		}
		start := maxInt(index-80, 0)
		end := minInt(len(content), start+maxHistoricalSnippetChars)
		return strings.TrimSpace(content[start:end])
	}

	return trimForPrompt(content, maxHistoricalSnippetChars)
}

func sameHistoricalDocument(documentName string, historicalDocument paperless.Document) bool {
	normalizedDocumentName := normalizeFreeText(documentName)
	if normalizedDocumentName == "" {
		return false
	}
	return normalizedDocumentName == normalizeFreeText(historicalDocument.Title) || normalizedDocumentName == normalizeFreeText(historicalDocument.OriginalFileName)
}

func tokenSet(text string) map[string]struct{} {
	tokens := orderedUniqueTokens(text)
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[token] = struct{}{}
	}
	return set
}

func orderedUniqueTokens(text string) []string {
	parts := tokenize(text)
	seen := make(map[string]struct{}, len(parts))
	unique := make([]string, 0, len(parts))
	for _, token := range parts {
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		unique = append(unique, token)
	}
	return unique
}

func tokenize(text string) []string {
	normalized := normalizeFreeText(text)
	parts := strings.Fields(normalized)
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 3 {
			continue
		}
		if _, ok := trivialDocumentTokens[part]; ok {
			continue
		}
		tokens = append(tokens, part)
	}
	return tokens
}

func normalizeFreeText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(text))
	lastWasSpace := false
	for _, char := range text {
		switch {
		case unicode.IsLetter(char) || unicode.IsDigit(char):
			builder.WriteRune(char)
			lastWasSpace = false
		case unicode.IsSpace(char):
			if !lastWasSpace {
				builder.WriteByte(' ')
				lastWasSpace = true
			}
		default:
			if !lastWasSpace {
				builder.WriteByte(' ')
				lastWasSpace = true
			}
		}
	}

	return strings.TrimSpace(builder.String())
}

func compactWhitespace(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func overlapCount(left map[string]struct{}, right map[string]struct{}) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	count := 0
	for token := range left {
		if _, ok := right[token]; ok {
			count++
		}
	}
	return count
}

func trimForPrompt(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return strings.TrimSpace(text[:limit]) + "..."
}

func normalizeSignals(signals []string) []string {
	seen := make(map[string]struct{}, len(signals))
	normalized := make([]string, 0, len(signals))
	for _, signal := range signals {
		signal = strings.TrimSpace(signal)
		if signal == "" {
			continue
		}
		if _, ok := seen[signal]; ok {
			continue
		}
		seen[signal] = struct{}{}
		normalized = append(normalized, signal)
	}
	return normalized
}

func exampleTitle(example historicalDocumentExample) string {
	parts := make([]string, 0, 2)
	if example.Title != "" {
		parts = append(parts, example.Title)
	}
	if example.FileName != "" && example.FileName != example.Title {
		parts = append(parts, example.FileName)
	}
	if len(parts) == 0 {
		return "untitled document"
	}
	return strings.Join(parts, " / ")
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
