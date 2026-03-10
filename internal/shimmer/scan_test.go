package shimmer_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestScanSymlinks(t *testing.T) {
	home := setupShimmerHome(t)
	project := setupTestProject(t)

	// Create a fake clone file to point symlinks at
	cloneFile := filepath.Join(home, "repos", "owner", "repo", "CLAUDE.md")
	writeFile(t, filepath.Dir(cloneFile), "CLAUDE.md", "config")

	// Create a shimmer symlink in the project
	if err := os.Symlink(cloneFile, filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	// Create a non-shimmer symlink (should be ignored)
	other := filepath.Join(t.TempDir(), "other.md")
	os.WriteFile(other, []byte("x"), 0o644)
	os.Symlink(other, filepath.Join(project, "other.md"))

	// Create a regular file (should be ignored)
	writeFile(t, project, "readme.md", "hello")

	links, err := shimmer.ScanSymlinks(project, home)
	if err != nil {
		t.Fatal(err)
	}

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %v", len(links), links)
	}
	if links[0] != filepath.Join(project, "CLAUDE.md") {
		t.Errorf("got %q", links[0])
	}
}

func TestScanSymlinksNested(t *testing.T) {
	home := setupShimmerHome(t)
	project := setupTestProject(t)

	// Create nested clone files
	cloneBase := filepath.Join(home, "repos", "owner", "repo")
	writeFile(t, cloneBase, "CLAUDE.md", "config")
	writeFile(t, cloneBase, ".claude/settings.json", "{}")

	// Create matching symlinks
	os.MkdirAll(filepath.Join(project, ".claude"), 0o755)
	os.Symlink(filepath.Join(cloneBase, "CLAUDE.md"), filepath.Join(project, "CLAUDE.md"))
	os.Symlink(filepath.Join(cloneBase, ".claude/settings.json"), filepath.Join(project, ".claude/settings.json"))

	links, err := shimmer.ScanSymlinks(project, home)
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(links)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %v", len(links), links)
	}
}
