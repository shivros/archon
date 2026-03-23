package transcriptadapters

import "control/internal/daemon/transcriptdomain"

// TranscriptTextExtractor extracts provider text without imposing formatting normalization.
type TranscriptTextExtractor interface {
	Extract(raw any) string
}

func firstNonEmptyExtracted(extractor TranscriptTextExtractor, values ...any) string {
	if extractor == nil {
		return ""
	}
	for _, value := range values {
		text := transcriptdomain.PreserveText(extractor.Extract(value))
		if transcriptdomain.IsSemanticallyEmpty(text) {
			continue
		}
		return text
	}
	return ""
}

func firstPresentExtracted(extractor TranscriptTextExtractor, values ...any) (string, bool) {
	if extractor == nil {
		return "", false
	}
	for _, value := range values {
		if value == nil {
			continue
		}
		text := transcriptdomain.PreserveText(extractor.Extract(value))
		if text == "" {
			continue
		}
		return text, true
	}
	return "", false
}
