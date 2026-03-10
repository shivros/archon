package app

import "testing"

func TestClientAPIImplementsUnifiedTranscriptContract(t *testing.T) {
	var _ SessionUnifiedTranscriptAPI = (*ClientAPI)(nil)
}
