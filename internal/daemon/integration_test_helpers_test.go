package daemon

import (
	"os"
	"testing"
	"time"
)

func newDaemonIntegrationTempDir(t *testing.T, pattern string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("mkdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		const attempts = 5
		for i := 0; i < attempts; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Logf("warning: best-effort cleanup could not remove %s", dir)
	})
	return dir
}
