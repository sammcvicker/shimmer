package shimmer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sammcvicker/shimmer/internal/shimmer"
)

// ---------------------------------------------------------------------------
// LocalScope
// ---------------------------------------------------------------------------

func TestLocalScope_ClonePath(t *testing.T) {
	scope := shimmer.NewLocalScope("/home/user/projects/myapp")
	got := scope.ClonePath("/home/user/.shimmer", "acme", "overlay")
	want := filepath.Join("/home/user/.shimmer", "repos", "acme", "overlay", "home/user/projects/myapp")
	if got != want {
		t.Errorf("ClonePath = %q, want %q", got, want)
	}
}

func TestLocalScope_MatchClone(t *testing.T) {
	scope := shimmer.NewLocalScope("/home/user/projects/myapp")

	if !scope.MatchClone("home/user/projects/myapp") {
		t.Error("expected MatchClone to return true for matching segment")
	}
	if scope.MatchClone("home/user/projects/other") {
		t.Error("expected MatchClone to return false for non-matching segment")
	}
	if scope.MatchClone("_global") {
		t.Error("expected MatchClone to return false for _global")
	}
}

func TestLocalScope_Target(t *testing.T) {
	scope := shimmer.NewLocalScope("/some/repo")
	if scope.Target() != "/some/repo" {
		t.Errorf("Target() = %q, want %q", scope.Target(), "/some/repo")
	}
}

func TestLocalScope_StashDir(t *testing.T) {
	scope := shimmer.NewLocalScope("/home/user/project")
	got := scope.StashDir()
	want := filepath.Join("/home/user/project", ".git", "shimmer-stash")
	if got != want {
		t.Errorf("StashDir() = %q, want %q", got, want)
	}
}

func TestLocalScope_SaveLinkState(t *testing.T) {
	project := setupTestProject(t)
	scope := shimmer.NewLocalScope(project)

	// Write some paths.
	if err := scope.SaveLinkState([]string{"file-b.txt", "file-a.txt"}); err != nil {
		t.Fatal(err)
	}

	excludePath := filepath.Join(project, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("reading exclude: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "# shimmer managed") {
		t.Error("exclude file should contain shimmer marker start")
	}
	if !strings.Contains(s, "# end shimmer") {
		t.Error("exclude file should contain shimmer marker end")
	}
	if !strings.Contains(s, "file-a.txt") || !strings.Contains(s, "file-b.txt") {
		t.Error("exclude file should contain linked paths")
	}

	// Entries should be sorted.
	aIdx := strings.Index(s, "file-a.txt")
	bIdx := strings.Index(s, "file-b.txt")
	if aIdx > bIdx {
		t.Error("entries should be sorted alphabetically")
	}

	// Saving empty paths should remove the block.
	if err := scope.SaveLinkState(nil); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("reading exclude after clear: %v", err)
	}
	if strings.Contains(string(content), "# shimmer managed") {
		t.Error("shimmer block should be removed when paths are empty")
	}
}

func TestLocalScope_SaveLinkState_PreservesExisting(t *testing.T) {
	project := setupTestProject(t)
	scope := shimmer.NewLocalScope(project)

	// Write pre-existing exclude content.
	excludePath := filepath.Join(project, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludePath, []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := scope.SaveLinkState([]string{"overlay.txt"}); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "*.log") {
		t.Error("pre-existing exclude content should be preserved")
	}
	if !strings.Contains(s, "overlay.txt") {
		t.Error("shimmer entry should be present")
	}
}

func TestLocalScope_SetSkipWorktree(t *testing.T) {
	project := setupTestProject(t)

	// Create and commit a file so update-index has something to work on.
	writeFile(t, project, "tracked.txt", "hello")
	git(t, project, "add", "tracked.txt")
	git(t, project, "commit", "-m", "add tracked file")

	scope := shimmer.NewLocalScope(project)

	// Set skip-worktree.
	if err := scope.SetSkipWorktree("tracked.txt", true); err != nil {
		t.Fatalf("SetSkipWorktree(true): %v", err)
	}

	// Verify via git ls-files -v: skip-worktree shows as 'S'.
	out := git(t, project, "ls-files", "-v", "tracked.txt")
	if !strings.HasPrefix(out, "S ") {
		t.Errorf("expected skip-worktree flag 'S', got: %q", out)
	}

	// Unset skip-worktree.
	if err := scope.SetSkipWorktree("tracked.txt", false); err != nil {
		t.Fatalf("SetSkipWorktree(false): %v", err)
	}

	out = git(t, project, "ls-files", "-v", "tracked.txt")
	if !strings.HasPrefix(out, "H ") {
		t.Errorf("expected normal flag 'H', got: %q", out)
	}
}

func TestLocalScope_TrackedFiles(t *testing.T) {
	project := setupTestProject(t)

	writeFile(t, project, "a.txt", "a")
	writeFile(t, project, "b.txt", "b")
	git(t, project, "add", "a.txt", "b.txt")
	git(t, project, "commit", "-m", "add files")

	// Also create an untracked file.
	writeFile(t, project, "c.txt", "c")

	scope := shimmer.NewLocalScope(project)

	tracked := scope.TrackedFiles([]string{"a.txt", "b.txt", "c.txt"})
	if !tracked["a.txt"] {
		t.Error("a.txt should be tracked")
	}
	if !tracked["b.txt"] {
		t.Error("b.txt should be tracked")
	}
	if tracked["c.txt"] {
		t.Error("c.txt should NOT be tracked")
	}
}

func TestLocalScope_TrackedFiles_Empty(t *testing.T) {
	scope := shimmer.NewLocalScope("/tmp")
	if got := scope.TrackedFiles(nil); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestLocalScope_ImplementsGitAware(t *testing.T) {
	scope := shimmer.NewLocalScope("/tmp")
	if _, ok := interface{}(scope).(shimmer.GitAware); !ok {
		t.Error("LocalScope should implement GitAware")
	}
}

// ---------------------------------------------------------------------------
// GlobalScope
// ---------------------------------------------------------------------------

func TestGlobalScope_ClonePath(t *testing.T) {
	scope := shimmer.NewGlobalScope("/home/user", "/home/user/.shimmer")
	got := scope.ClonePath("/home/user/.shimmer", "acme", "overlay")
	want := filepath.Join("/home/user/.shimmer", "repos", "acme", "overlay", "_global")
	if got != want {
		t.Errorf("ClonePath = %q, want %q", got, want)
	}
}

func TestGlobalScope_MatchClone(t *testing.T) {
	scope := shimmer.NewGlobalScope("/home/user", "/home/user/.shimmer")

	if !scope.MatchClone("_global") {
		t.Error("expected MatchClone to return true for _global")
	}
	if scope.MatchClone("home/user/projects/myapp") {
		t.Error("expected MatchClone to return false for a project path")
	}
}

func TestGlobalScope_Target(t *testing.T) {
	scope := shimmer.NewGlobalScope("/home/user", "/home/user/.shimmer")
	if scope.Target() != "/home/user" {
		t.Errorf("Target() = %q, want %q", scope.Target(), "/home/user")
	}
}

func TestGlobalScope_StashDir(t *testing.T) {
	scope := shimmer.NewGlobalScope("/home/user", "/home/user/.shimmer")
	got := scope.StashDir()
	want := filepath.Join("/home/user/.shimmer", "stash")
	if got != want {
		t.Errorf("StashDir() = %q, want %q", got, want)
	}
}

func TestGlobalScope_SaveLinkState(t *testing.T) {
	shimmerHome := t.TempDir()
	scope := shimmer.NewGlobalScope("/home/user", shimmerHome)

	// Write link state.
	if err := scope.SaveLinkState([]string{".bashrc", ".vimrc"}); err != nil {
		t.Fatal(err)
	}

	linkedFile := filepath.Join(shimmerHome, "linked")
	content, err := os.ReadFile(linkedFile)
	if err != nil {
		t.Fatalf("reading linked file: %v", err)
	}
	want := ".bashrc\n.vimrc\n"
	if string(content) != want {
		t.Errorf("linked file = %q, want %q", content, want)
	}

	// Clear link state.
	if err := scope.SaveLinkState(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(linkedFile); !os.IsNotExist(err) {
		t.Error("linked file should be removed when paths are empty")
	}
}

func TestGlobalScope_SaveLinkState_RemoveNonExistent(t *testing.T) {
	shimmerHome := t.TempDir()
	scope := shimmer.NewGlobalScope("/home/user", shimmerHome)

	// Removing when file doesn't exist should not error.
	if err := scope.SaveLinkState(nil); err != nil {
		t.Errorf("SaveLinkState(nil) on missing file should not error, got: %v", err)
	}
}

func TestGlobalScope_DoesNotImplementGitAware(t *testing.T) {
	scope := shimmer.NewGlobalScope("/home/user", "/home/user/.shimmer")
	if _, ok := interface{}(scope).(shimmer.GitAware); ok {
		t.Error("GlobalScope should NOT implement GitAware")
	}
}
