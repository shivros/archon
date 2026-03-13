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
