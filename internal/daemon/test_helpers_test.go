package daemon

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func closeTestCloser(t testing.TB, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Fatalf("close resource: %v", err)
	}
}

// writeTestWrapperScript creates a platform-appropriate wrapper script.
// On non-Windows platforms it writes a .sh file with shellContent.
// On Windows it writes a .bat file with batchContent instead.
func writeTestWrapperScript(t testing.TB, dir, name string, shellContent, batchContent string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, name+".bat")
		if err := os.WriteFile(path, []byte(batchContent), 0o755); err != nil {
			t.Fatalf("write wrapper batch: %v", err)
		}
		return path
	}
	path := filepath.Join(dir, name+".sh")
	if err := os.WriteFile(path, []byte(shellContent), 0o755); err != nil {
		t.Fatalf("write wrapper shell: %v", err)
	}
	return path
}

// claudeTestWrapper creates a wrapper that execs the test binary with the
// given -test.run pattern, suitable for the current platform.
func claudeTestWrapper(t testing.TB, testRunPattern string) string {
	t.Helper()
	testBin := os.Args[0]
	tmpDir := t.TempDir()
	shellScript := "#!/bin/sh\nexec \"" + testBin + "\" -test.run=" + testRunPattern + " -- \"$@\"\n"
	batchScript := "@echo off\n\"" + testBin + "\" -test.run=" + testRunPattern + " -- %*\n"
	return writeTestWrapperScript(t, tmpDir, "claude-wrapper", shellScript, batchScript)
}
