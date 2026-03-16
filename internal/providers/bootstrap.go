package providers

type HistoryConsistency string

const (
	HistoryConsistencyImmediate            HistoryConsistency = "immediate"
	HistoryConsistencyEventuallyConsistent HistoryConsistency = "eventually_consistent"
)

type TranscriptBootstrapMode string

const (
	TranscriptBootstrapModeSnapshotFirst TranscriptBootstrapMode = "snapshot_first"
	TranscriptBootstrapModeDeferSnapshot TranscriptBootstrapMode = "defer_snapshot"
)

type BootstrapProfile struct {
	HistoryConsistency     HistoryConsistency
	SessionStartTranscript TranscriptBootstrapMode
}

func defaultBootstrapProfile() BootstrapProfile {
	return BootstrapProfile{
		HistoryConsistency:     HistoryConsistencyImmediate,
		SessionStartTranscript: TranscriptBootstrapModeSnapshotFirst,
	}
}
