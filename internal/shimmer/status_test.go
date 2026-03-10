package shimmer_test

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestStatusAllOK(t *testing.T) {
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

	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	status, err := s.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}

	// Should report 2 files
	if len(status.Files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(status.Files), status.Files)
	}

	// All should be OK
	for _, f := range status.Files {
		if !f.OK {
			t.Errorf("expected file %s to be OK, got broken: %s", f.Path, f.Reason)
		}
	}

	// Verify paths are relative
	paths := make([]string, len(status.Files))
	for i, f := range status.Files {
		paths[i] = f.Path
	}
	sort.Strings(paths)
	if paths[0] != ".claude/settings.json" {
		t.Errorf("expected .claude/settings.json, got %s", paths[0])
	}
	if paths[1] != "CLAUDE.md" {
		t.Errorf("expected CLAUDE.md, got %s", paths[1])
	}
}

func TestStatusBroken(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":             "# Claude Config",
		".claude/settings.json": `{"key": "value"}`,
	})

	s := newTestShimmer(t, home, project, false)
	info, err := s.RepoSet(overlayURL)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Delete one file from the clone to create a broken symlink
	os.Remove(filepath.Join(info.ClonePath, ".claude/settings.json"))

	status, err := s.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}

	// Should still report 2 files
	if len(status.Files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(status.Files), status.Files)
	}

	// Count broken
	var broken int
	for _, f := range status.Files {
		if !f.OK {
			broken++
			if f.Reason != "target missing" {
				t.Errorf("expected reason 'target missing', got %q", f.Reason)
			}
		}
	}
	if broken != 1 {
		t.Errorf("expected 1 broken file, got %d", broken)
	}
}

func TestStatusNotLinked(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)

	s := newTestShimmer(t, home, project, false)

	_, err := s.Status()
	var notLinked *shimmer.ErrNotLinked
	if !errors.As(err, &notLinked) {
		t.Fatalf("expected ErrNotLinked, got %v", err)
	}
}
