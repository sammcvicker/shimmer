package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestGitRoot(t *testing.T) {
	project := setupTestProject(t)
	// Resolve symlinks so comparison matches git's resolved output
	// (on macOS /var -> /private/var).
	project, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}

	// From project root
	root, err := shimmer.GitRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	if root != project {
		t.Errorf("got %q, want %q", root, project)
	}

	// From subdirectory
	sub := filepath.Join(project, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err = shimmer.GitRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if root != project {
		t.Errorf("got %q, want %q", root, project)
	}

	// Outside git repo
	_, err = shimmer.GitRoot(t.TempDir())
	if err == nil {
		t.Error("expected error outside git repo")
	}
}

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantName  string
	}{
		{"git@github.com:siimpl/claude-dhi.git", "siimpl", "claude-dhi"},
		{"git@github.com:siimpl/claude-dhi", "siimpl", "claude-dhi"},
		{"https://github.com/siimpl/claude-dhi.git", "siimpl", "claude-dhi"},
		{"https://github.com/siimpl/claude-dhi", "siimpl", "claude-dhi"},
		{"git@github.com:other-org/claude-configs.git", "other-org", "claude-configs"},
		{"ssh://git@github.com/siimpl/claude-dhi.git", "siimpl", "claude-dhi"},
		{"ssh://git@github.com/siimpl/claude-dhi", "siimpl", "claude-dhi"},
		{"ssh://github.com/siimpl/claude-dhi.git", "siimpl", "claude-dhi"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, name, err := shimmer.ParseRepoURL(tt.url)
			if err != nil {
				t.Fatal(err)
			}
			if owner != tt.wantOwner || name != tt.wantName {
				t.Errorf("got (%q, %q), want (%q, %q)", owner, name, tt.wantOwner, tt.wantName)
			}
		})
	}
}

func TestClonePath(t *testing.T) {
	home := "/home/test/.shimmer"

	// Local scope
	got := shimmer.ClonePath(home, "siimpl", "claude-dhi", "/Users/sam/projects/dhi", false)
	want := "/home/test/.shimmer/repos/siimpl/claude-dhi/Users/sam/projects/dhi"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Global scope
	got = shimmer.ClonePath(home, "siimpl", "claude-global", "", true)
	want = "/home/test/.shimmer/repos/siimpl/claude-global/_global"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
