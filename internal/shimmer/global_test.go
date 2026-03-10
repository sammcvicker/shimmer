package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestGlobalLinkUnlink(t *testing.T) {
	fakeHome := t.TempDir()
	shimmerHome := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		".claude/CLAUDE.md": "# Global Config",
	})

	s := &shimmer.Shimmer{
		Home:   shimmerHome,
		Global: true,
		Target: fakeHome,
	}

	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	result, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Linked) != 1 {
		t.Fatalf("expected 1 linked, got %d", len(result.Linked))
	}

	// Symlink should exist in fake home
	linked := filepath.Join(fakeHome, ".claude", "CLAUDE.md")
	if _, err := os.Readlink(linked); err != nil {
		t.Errorf("expected symlink at %s", linked)
	}

	// Status should report healthy
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Files) != 1 || !status.Files[0].OK {
		t.Error("status should show 1 healthy file")
	}

	// Unlink
	if _, err := s.Unlink(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(linked); !os.IsNotExist(err) {
		t.Error("symlink should be removed after unlink")
	}
}

func TestGlobalReconcilesStalLinks(t *testing.T) {
	fakeHome := t.TempDir()
	shimmerHome := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"file-a.txt": "a",
		"file-b.txt": "b",
	})

	s := &shimmer.Shimmer{
		Home:   shimmerHome,
		Global: true,
		Target: fakeHome,
	}

	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Link both files
	result, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Linked) != 2 {
		t.Fatalf("expected 2 linked, got %d", len(result.Linked))
	}

	// Delete file-a.txt from the clone (simulates branch switch removing a file)
	clonePath, _ := s.RepoPath()
	os.Remove(filepath.Join(clonePath, "file-a.txt"))

	// Re-link — should remove stale symlink for file-a.txt
	result, err = s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Linked) != 1 {
		t.Errorf("expected 1 linked, got %d: %v", len(result.Linked), result.Linked)
	}
	if len(result.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d: %v", len(result.Removed), result.Removed)
	}

	// Stale symlink should be gone
	if _, err := os.Lstat(filepath.Join(fakeHome, "file-a.txt")); !os.IsNotExist(err) {
		t.Error("stale symlink file-a.txt should have been removed")
	}
}

func TestGlobalOverwriteRestoresStash(t *testing.T) {
	fakeHome := t.TempDir()
	shimmerHome := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"config.txt": "overlay",
	})

	writeFile(t, fakeHome, "config.txt", "original")

	s := &shimmer.Shimmer{
		Home:   shimmerHome,
		Global: true,
		Target: fakeHome,
	}

	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	result, err := s.Link(false, true) // --overwrite
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stashed) != 1 {
		t.Fatal("expected 1 stashed")
	}

	// Stash should be in shimmerHome/stash/
	stashed := filepath.Join(shimmerHome, "stash", "config.txt")
	if _, err := os.Stat(stashed); err != nil {
		t.Errorf("stash file not found at %s", stashed)
	}

	// Unlink should restore
	if _, err := s.Unlink(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(fakeHome, "config.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original" {
		t.Errorf("restored content = %q, want %q", content, "original")
	}
}
