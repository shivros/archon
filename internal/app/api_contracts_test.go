package app

import "testing"

func TestClientAPIImplementsUnifiedAndLegacySessionContracts(t *testing.T) {
	var _ SessionUnifiedTranscriptAPI = (*ClientAPI)(nil)
	var _ SessionLegacyStreamAPI = (*ClientAPI)(nil)
}
