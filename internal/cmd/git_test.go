package cmd

import (
	"io"
	"strings"
	"testing"
)

// TestGitSubcommand_GlobalFlagPlacement verifies that the -g/--global flag is
// honored whether it appears before or after the "git" subcommand name.
//
// Both placements must select the global scope. We assert via the resulting
// error message: with no overlay repo set, a global-scope git invocation
// produces "no overlay repo set for global", while a local-scope invocation
// in a non-git-repo cwd produces "not in a git repository".
func TestGitSubcommand_GlobalFlagPlacement(t *testing.T) {
	// Empty HOME → no overlay repos exist anywhere.
	t.Setenv("HOME", t.TempDir())
	// Non-git cwd → local scope will fail with ErrNotInGitRepo.
	t.Chdir(t.TempDir())

	tests := []struct {
		name        string
		args        []string
		wantContain string
	}{
		{
			name:        "flag before git subcommand",
			args:        []string{"-g", "git", "status"},
			wantContain: "no overlay repo set for global",
		},
		{
			name:        "long flag before git subcommand",
			args:        []string{"--global", "git", "status"},
			wantContain: "no overlay repo set for global",
		},
		{
			name:        "flag after git subcommand",
			args:        []string{"git", "-g", "status"},
			wantContain: "no overlay repo set for global",
		},
		{
			name:        "no global flag falls back to local scope",
			args:        []string{"git", "status"},
			wantContain: "not in a git repository",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCmd()
			root.SetArgs(tc.args)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)

			err := root.Execute()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantContain) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantContain, err)
			}
		})
	}
}

func TestExtractGlobalFlag(t *testing.T) {
	tests := []struct {
		name         string
		in           []string
		wantGlobal   bool
		wantFiltered []string
	}{
		{"empty", nil, false, nil},
		{"no flag", []string{"status"}, false, []string{"status"}},
		{"short flag at start", []string{"-g", "status"}, true, []string{"status"}},
		{"long flag at start", []string{"--global", "status"}, true, []string{"status"}},
		{"flag at end", []string{"status", "-g"}, true, []string{"status"}},
		{"flag in middle", []string{"log", "-g", "--oneline"}, true, []string{"log", "--oneline"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotGlobal, gotFiltered := extractGlobalFlag(tc.in)
			if gotGlobal != tc.wantGlobal {
				t.Errorf("global: got %v, want %v", gotGlobal, tc.wantGlobal)
			}
			if !sliceEqual(gotFiltered, tc.wantFiltered) {
				t.Errorf("filtered: got %v, want %v", gotFiltered, tc.wantFiltered)
			}
		})
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
