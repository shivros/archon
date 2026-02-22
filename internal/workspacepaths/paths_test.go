package workspacepaths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeSubpath(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty", raw: "", want: ""},
		{name: "dot", raw: ".", want: ""},
		{name: "relative", raw: "packages/pennies/", want: filepath.Join("packages", "pennies")},
		{name: "absolute", raw: filepath.Join(string(filepath.Separator), "tmp", "abs"), wantErr: true},
		{name: "parent", raw: "..", wantErr: true},
		{name: "escape", raw: filepath.Join("..", "outside"), wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeSubpath(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeSubpath(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestResolveSessionPath(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveSessionPath(root, filepath.Join("packages", "pennies"))
	if err != nil {
		t.Fatalf("ResolveSessionPath: %v", err)
	}
	want := filepath.Join(root, "packages", "pennies")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveSessionPathRequiresRoot(t *testing.T) {
	if _, err := ResolveSessionPath("", "packages/pennies"); err == nil {
		t.Fatalf("expected root required error")
	}
}

func TestValidateDirectoryRejectsFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := ValidateDirectory(filePath, nil); err == nil {
		t.Fatalf("expected non-directory error")
	}
}

func TestValidateRootAndSessionPath(t *testing.T) {
	root := t.TempDir()
	sessionPath := filepath.Join(root, "packages", "pennies")
	if err := os.MkdirAll(sessionPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := ValidateRootAndSessionPath(root, filepath.Join("packages", "pennies"), nil); err != nil {
		t.Fatalf("ValidateRootAndSessionPath should pass: %v", err)
	}
	if err := ValidateRootAndSessionPath(root, filepath.Join("packages", "missing"), nil); err == nil {
		t.Fatalf("expected missing session path error")
	}
}

func TestValidateRootAndSessionPathRejectsEmptyRoot(t *testing.T) {
	if err := ValidateRootAndSessionPath("", "packages/pennies", nil); err == nil {
		t.Fatalf("expected root required error")
	}
}

func TestNormalizeAdditionalDirectories(t *testing.T) {
	got, err := NormalizeAdditionalDirectories([]string{
		" ../backend ",
		"./shared/..//shared",
		"../backend",
	})
	if err != nil {
		t.Fatalf("NormalizeAdditionalDirectories: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected deduped directories, got %#v", got)
	}
	if got[0] != filepath.Clean("../backend") {
		t.Fatalf("unexpected first value: %q", got[0])
	}
	if got[1] != filepath.Clean("./shared/..//shared") {
		t.Fatalf("unexpected second value: %q", got[1])
	}
}

func TestNormalizeAdditionalDirectoriesRejectsEmptyEntry(t *testing.T) {
	if _, err := NormalizeAdditionalDirectories([]string{"../backend", " "}); err == nil {
		t.Fatalf("expected empty entry error")
	}
}

func TestResolveAdditionalDirectories(t *testing.T) {
	base := t.TempDir()
	backend := filepath.Join(base, "..", "backend")
	shared := filepath.Join(base, "..", "shared")
	if err := os.MkdirAll(filepath.Clean(backend), 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	if err := os.MkdirAll(filepath.Clean(shared), 0o755); err != nil {
		t.Fatalf("mkdir shared: %v", err)
	}

	got, err := ResolveAdditionalDirectories(base, []string{"../backend", "../shared"}, nil)
	if err != nil {
		t.Fatalf("ResolveAdditionalDirectories: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 directories, got %#v", got)
	}
	if !filepath.IsAbs(got[0]) || !filepath.IsAbs(got[1]) {
		t.Fatalf("expected absolute paths, got %#v", got)
	}
}

func TestResolveAdditionalDirectoriesRejectsMissingBaseForRelative(t *testing.T) {
	if _, err := ResolveAdditionalDirectories("", []string{"../backend"}, nil); err == nil {
		t.Fatalf("expected base path error")
	}
}

func TestResolveAdditionalDirectoriesRejectsMissingDirectory(t *testing.T) {
	base := t.TempDir()
	_, err := ResolveAdditionalDirectories(base, []string{"../missing"}, nil)
	if err == nil {
		t.Fatalf("expected missing directory error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing file error, got %v", err)
	}
}
