package shimmer_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestEject(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":             "# Claude Config",
		".claude/settings.json": `{"key": "value"}`,
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Link first.
	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Eject.
	result, err := s.Eject()
	if err != nil {
		t.Fatalf("Eject() error: %v", err)
	}

	if len(result.Ejected) != 2 {
		t.Fatalf("expected 2 ejected, got %d: %v", len(result.Ejected), result.Ejected)
	}

	// Verify each file is a regular file (not a symlink) with correct content.
	for _, rel := range []string{"CLAUDE.md", ".claude/settings.json"} {
		p := filepath.Join(project, rel)
		info, err := os.Lstat(p)
		if err != nil {
			t.Fatalf("expected file at %s: %v", p, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s is still a symlink after eject", rel)
		}
	}

	// Verify content of both files.
	for rel, want := range map[string]string{
		"CLAUDE.md":             "# Claude Config",
		".claude/settings.json": `{"key": "value"}`,
	} {
		got, err := os.ReadFile(filepath.Join(project, rel))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		if string(got) != want {
			t.Errorf("%s content = %q, want %q", rel, got, want)
		}
	}
}

func TestEjectClearsStash(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Create a conflicting file, link with overwrite to create a stash.
	writeFile(t, project, "CLAUDE.md", "original content")
	if _, err := s.Link(false, true); err != nil {
		t.Fatal(err)
	}

	// Verify stash exists.
	stashDir := filepath.Join(project, ".git", "shimmer-stash")
	if _, err := os.Stat(stashDir); os.IsNotExist(err) {
		t.Fatal("expected stash to exist after overwrite link")
	}

	// Eject.
	result, err := s.Eject()
	if err != nil {
		t.Fatalf("Eject() error: %v", err)
	}

	if !result.StashCleared {
		t.Error("expected StashCleared to be true")
	}

	// Stash directory should be gone.
	if _, err := os.Stat(stashDir); !os.IsNotExist(err) {
		t.Error("expected stash directory to be removed after eject")
	}

	// Ejected file should have overlay content, not the stashed original.
	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Claude Config" {
		t.Errorf("content = %q, want overlay content %q", content, "# Claude Config")
	}
}

func TestEjectClearsExclude(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Verify exclude has shimmer block.
	excludePath := filepath.Join(project, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "# shimmer managed") {
		t.Fatal("expected shimmer block in exclude before eject")
	}

	// Eject.
	if _, err := s.Eject(); err != nil {
		t.Fatalf("Eject() error: %v", err)
	}

	// Exclude should no longer have shimmer block.
	content, err = os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "# shimmer managed") {
		t.Error("expected shimmer block to be cleared from exclude after eject")
	}
}

func TestEjectNothingLinked(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)

	s := newTestShimmer(t, home, project, false)

	// Eject when nothing is linked — should return ErrNotLinked.
	_, err := s.Eject()
	if err == nil {
		t.Fatal("Eject() on unlinked project should return ErrNotLinked, got nil")
	}
	var notLinked *shimmer.ErrNotLinked
	if !errors.As(err, &notLinked) {
		t.Fatalf("expected ErrNotLinked, got: %v", err)
	}
}

func TestEjectBrokenSymlink(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Break the symlink by deleting the target file in the clone.
	clonePath, err := s.RepoPath()
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(clonePath, "CLAUDE.md"))

	// Eject should fail during pre-flight validation, before any mutations.
	_, err = s.Eject()
	if err == nil {
		t.Fatal("expected error on broken symlink, got nil")
	}

	if !strings.Contains(err.Error(), "broken symlink") {
		t.Errorf("expected 'broken symlink' in error, got: %v", err)
	}

	// The symlink should still be in place — no mutations should have occurred.
	info, statErr := os.Lstat(filepath.Join(project, "CLAUDE.md"))
	if statErr != nil {
		t.Fatalf("expected CLAUDE.md to still exist after failed eject: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("CLAUDE.md should still be a symlink after pre-flight validation failure")
	}
}

func TestEjectClearsSkipWorktree(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Overlay",
	})

	// Create a tracked file that will conflict with the overlay.
	writeFile(t, project, "CLAUDE.md", "original")
	git(t, project, "add", "CLAUDE.md")
	git(t, project, "commit", "-m", "add CLAUDE.md")

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Link with overwrite — this sets skip-worktree on the tracked file.
	if _, err := s.Link(false, true); err != nil {
		t.Fatal(err)
	}

	// Verify skip-worktree is set: 'S' prefix in git ls-files -v means skip-worktree.
	out := git(t, project, "ls-files", "-v", "CLAUDE.md")
	if !strings.HasPrefix(out, "S") {
		t.Fatalf("expected skip-worktree to be set (S prefix), got: %q", out)
	}

	// Eject — should clear skip-worktree.
	result, err := s.Eject()
	if err != nil {
		t.Fatalf("Eject() error: %v", err)
	}
	if len(result.Ejected) != 1 {
		t.Fatalf("expected 1 ejected, got %d", len(result.Ejected))
	}

	// Verify file is a regular file with overlay content.
	info, err := os.Lstat(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("CLAUDE.md is still a symlink after eject")
	}
	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Overlay" {
		t.Errorf("content = %q, want %q", content, "# Overlay")
	}

	// Verify skip-worktree is cleared: 'H' prefix means regular tracked file.
	out = git(t, project, "ls-files", "-v", "CLAUDE.md")
	if strings.HasPrefix(out, "S") {
		t.Errorf("skip-worktree still set after eject, got: %q", out)
	}
	if !strings.HasPrefix(out, "H") {
		t.Errorf("expected 'H' prefix (normal tracked), got: %q", out)
	}
}
