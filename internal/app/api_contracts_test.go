package app

import "testing"

func TestClientAPIImplementsUnifiedTranscriptContract(t *testing.T) {
	var _ SessionUnifiedTranscriptAPI = (*ClientAPI)(nil)
}

func TestClientAPIImplementsFileSearchContract(t *testing.T) {
	var _ FileSearchAPI = (*ClientAPI)(nil)
}
