package daemon

import (
	"reflect"
	"testing"
)

func TestProviderAdditionalDirectoryArgs(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		got, err := providerAdditionalDirectoryArgs("codex", []string{"/tmp/backend", "/tmp/shared"})
		if err != nil {
			t.Fatalf("providerAdditionalDirectoryArgs: %v", err)
		}
		if len(got) != 4 {
			t.Fatalf("expected 4 args, got %#v", got)
		}
		if got[0] != "--add-dir" || got[2] != "--add-dir" {
			t.Fatalf("expected --add-dir flags, got %#v", got)
		}
	})

	t.Run("claude", func(t *testing.T) {
		got, err := providerAdditionalDirectoryArgs("claude", []string{"/tmp/backend"})
		if err != nil {
			t.Fatalf("providerAdditionalDirectoryArgs: %v", err)
		}
		if len(got) != 2 || got[0] != "--add-dir" || got[1] != "/tmp/backend" {
			t.Fatalf("unexpected args: %#v", got)
		}
	})

	t.Run("gemini", func(t *testing.T) {
		got, err := providerAdditionalDirectoryArgs("gemini", []string{"/tmp/backend", "/tmp/shared"})
		if err != nil {
			t.Fatalf("providerAdditionalDirectoryArgs: %v", err)
		}
		if len(got) != 4 {
			t.Fatalf("expected 4 args, got %#v", got)
		}
		if got[0] != "--include-directories" || got[2] != "--include-directories" {
			t.Fatalf("expected include-directories args, got %#v", got)
		}
	})

	t.Run("gemini_limit", func(t *testing.T) {
		_, err := providerAdditionalDirectoryArgs("gemini", []string{"1", "2", "3", "4", "5", "6"})
		if err == nil {
			t.Fatalf("expected gemini include directory limit error")
		}
	})

	t.Run("opencode_and_kilo_noop", func(t *testing.T) {
		for _, provider := range []string{"opencode", "kilocode"} {
			got, err := providerAdditionalDirectoryArgs(provider, []string{"/tmp/backend"})
			if err != nil {
				t.Fatalf("%s providerAdditionalDirectoryArgs: %v", provider, err)
			}
			if len(got) != 0 {
				t.Fatalf("%s expected no args, got %#v", provider, got)
			}
		}
	})

	t.Run("opencode_permission_payload", func(t *testing.T) {
		got, err := providerAdditionalDirectoryPermission("opencode", []string{"/tmp/backend", " /tmp/shared/ ", "", "/tmp/backend"})
		if err != nil {
			t.Fatalf("providerAdditionalDirectoryPermission: %v", err)
		}
		if got == nil {
			t.Fatalf("expected permission payload")
		}
		if gotPermission := got.Permission; gotPermission != "external_directory" {
			t.Fatalf("unexpected permission: %q", gotPermission)
		}
		wantPatterns := []string{"/tmp/backend/*", "/tmp/shared/*"}
		if !reflect.DeepEqual(got.Patterns, wantPatterns) {
			t.Fatalf("unexpected patterns: got %#v want %#v", got.Patterns, wantPatterns)
		}
	})

	t.Run("unknown_provider_is_noop", func(t *testing.T) {
		gotArgs, err := providerAdditionalDirectoryArgs("unknown-provider", []string{"/tmp/backend"})
		if err != nil {
			t.Fatalf("providerAdditionalDirectoryArgs: %v", err)
		}
		if len(gotArgs) != 0 {
			t.Fatalf("expected no args for unknown provider, got %#v", gotArgs)
		}

		gotPermission, err := providerAdditionalDirectoryPermission("unknown-provider", []string{"/tmp/backend"})
		if err != nil {
			t.Fatalf("providerAdditionalDirectoryPermission: %v", err)
		}
		if gotPermission != nil {
			t.Fatalf("expected no permission for unknown provider, got %#v", gotPermission)
		}
	})

	t.Run("flag_providers_permission_is_noop", func(t *testing.T) {
		for _, provider := range []string{"codex", "claude", "gemini"} {
			got, err := providerAdditionalDirectoryPermission(provider, []string{"/tmp/backend"})
			if err != nil {
				t.Fatalf("%s providerAdditionalDirectoryPermission: %v", provider, err)
			}
			if got != nil {
				t.Fatalf("%s expected no permission payload, got %#v", provider, got)
			}
		}
	})

	t.Run("opencode_permission_skips_dot_paths", func(t *testing.T) {
		got, err := providerAdditionalDirectoryPermission("opencode", []string{".", " ./ ", ""})
		if err != nil {
			t.Fatalf("providerAdditionalDirectoryPermission: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil permission payload for dot-only paths, got %#v", got)
		}
	})
}
