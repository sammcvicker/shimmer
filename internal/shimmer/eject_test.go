package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"
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
