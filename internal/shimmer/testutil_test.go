package shimmer_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sammcvicker/shimmer/internal/shimmer"
)

// setupTestProject creates a temp directory with an initialized git repo.
func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

// setupTestOverlay creates a bare git repo, clones it, adds files, and pushes.
// Returns the bare repo URL (for cloning).
func setupTestOverlay(t *testing.T, files map[string]string) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "overlay.git")
	git(t, "", "init", "--bare", bare)

	work := t.TempDir()
	git(t, "", "clone", bare, work)

	for name, content := range files {
		p := filepath.Join(work, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, work, "add", "-A")
	git(t, work, "commit", "-m", "add overlay files")
	git(t, work, "push")

	return bare
}

// setupShimmerHome creates a temp ~/.shimmer equivalent.
func setupShimmerHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "repos"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newTestShimmer creates a Shimmer instance wired to test directories.
func newTestShimmer(t *testing.T, home, target string, global bool) *shimmer.Shimmer {
	t.Helper()
	var scope shimmer.Scope
	if global {
		scope = shimmer.NewGlobalScope(target, home)
	} else {
		scope = shimmer.NewLocalScope(target)
	}
	return &shimmer.Shimmer{
		Home:  home,
		Scope: scope,
	}
}

// writeFile creates a file at base/rel with the given content, creating parent dirs.
func writeFile(t *testing.T, base, rel, content string) {
	t.Helper()
	p := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// git runs a git command in the given directory.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s\n%s", args, err, out)
	}
	return string(out)
}
