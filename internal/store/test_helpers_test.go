package store

import (
	"io"
	"testing"
)

func closeTestCloser(t testing.TB, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Fatalf("close resource: %v", err)
	}
}
