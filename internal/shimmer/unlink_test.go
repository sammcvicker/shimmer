package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnlink(t *testing.T) {
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

	// Link the files first.
	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Verify symlinks exist before unlinking.
	for _, rel := range []string{"CLAUDE.md", ".claude/settings.json"} {
		p := filepath.Join(project, rel)
		if _, err := os.Readlink(p); err != nil {
			t.Fatalf("expected symlink at %s before unlink: %v", p, err)
		}
	}

	// Unlink.
	if _, err := s.Unlink(); err != nil {
		t.Fatalf("Unlink() error: %v", err)
	}

	// Verify symlinks removed.
	for _, rel := range []string{"CLAUDE.md", ".claude/settings.json"} {
		p := filepath.Join(project, rel)
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed after unlink, but it still exists", p)
		}
	}

	// Verify empty dirs cleaned up (.claude/ should be gone).
	claudeDir := filepath.Join(project, ".claude")
	if _, err := os.Stat(claudeDir); !os.IsNotExist(err) {
		t.Errorf("expected .claude/ dir to be cleaned up, but it still exists")
	}
}

func TestUnlinkRestoresStashed(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Create an original file that will be overwritten.
	writeFile(t, project, "CLAUDE.md", "original content")

	// Link with overwrite to stash the original.
	result, err := s.Link(false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stashed) != 1 {
		t.Fatalf("expected 1 stashed, got %d", len(result.Stashed))
	}

	// Unlink should restore the stashed file.
	if _, err := s.Unlink(); err != nil {
		t.Fatalf("Unlink() error: %v", err)
	}

	// Verify original file restored with original content.
	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("expected CLAUDE.md to be restored: %v", err)
	}
	if string(content) != "original content" {
		t.Errorf("restored content = %q, want %q", string(content), "original content")
	}

	// Verify stash directory cleaned up.
	stashDir := filepath.Join(project, ".git", "shimmer-stash")
	if _, err := os.Stat(stashDir); !os.IsNotExist(err) {
		t.Errorf("expected stash directory to be cleaned up, but it still exists")
	}
}

func TestUnlinkNotLinked(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)

	s := newTestShimmer(t, home, project, false)

	// Unlink when nothing is linked — should be a no-op (no error).
	if _, err := s.Unlink(); err != nil {
		t.Fatalf("Unlink() on unlinked project should be no-op, got error: %v", err)
	}
}
