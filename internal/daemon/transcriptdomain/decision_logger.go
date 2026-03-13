package transcriptdomain

type TranscriptDecisionLogEntry struct {
	Layer    string
	Decision string
	Reason   string
	Identity MessageIdentity
	Block    TranscriptIdentityBlock
	Context  map[string]string
}

type TranscriptDecisionLogger interface {
	LogDecision(entry TranscriptDecisionLogEntry)
}

type noopTranscriptDecisionLogger struct{}

func NewNoopTranscriptDecisionLogger() TranscriptDecisionLogger {
	return noopTranscriptDecisionLogger{}
}

func (noopTranscriptDecisionLogger) LogDecision(TranscriptDecisionLogEntry) {}
