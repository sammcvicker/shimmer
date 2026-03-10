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
	if err := s.Unlink(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(linked); !os.IsNotExist(err) {
		t.Error("symlink should be removed after unlink")
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
	if err := s.Unlink(); err != nil {
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
