package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestRepoSet(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	info, err := s.RepoSet(overlayURL)
	if err != nil {
		t.Fatal(err)
	}

	// Clone should exist at the expected path
	clonePath := info.ClonePath
	if _, err := os.Stat(filepath.Join(clonePath, ".git")); err != nil {
		t.Errorf("clone not found at %s", clonePath)
	}

	// Overlay file should be in the clone
	content, err := os.ReadFile(filepath.Join(clonePath, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Claude Config" {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestRepoSetGlobal(t *testing.T) {
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Global Config",
	})

	s := newTestShimmer(t, home, os.Getenv("HOME"), true)
	info, err := s.RepoSet(overlayURL)
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Base(info.ClonePath) != "_global" {
		t.Errorf("expected _global in path, got %s", info.ClonePath)
	}
}

func TestRepoSetAlreadyExists(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	_, err := s.RepoSet(overlayURL)
	if _, ok := err.(*shimmer.ErrRepoAlreadySet); !ok {
		t.Errorf("expected ErrRepoAlreadySet, got %v", err)
	}
}

func TestRepoPath(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Config",
	})

	s := newTestShimmer(t, home, project, false)
	info, err := s.RepoSet(overlayURL)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.RepoPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != info.ClonePath {
		t.Errorf("got %q, want %q", got, info.ClonePath)
	}
}

func TestRepoPathNoRepo(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)

	s := newTestShimmer(t, home, project, false)
	_, err := s.RepoPath()
	if _, ok := err.(*shimmer.ErrNoRepo); !ok {
		t.Errorf("expected ErrNoRepo, got %v", err)
	}
}

func TestRepoList(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	repos, err := s.RepoList()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].ClonePath == "" {
		t.Error("expected non-empty clone path")
	}
}

func TestRepoListEmpty(t *testing.T) {
	home := setupShimmerHome(t)
	s := newTestShimmer(t, home, "/tmp", false)

	repos, err := s.RepoList()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestRepoRemove(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Config",
	})

	s := newTestShimmer(t, home, project, false)
	info, err := s.RepoSet(overlayURL)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.RepoRemove(); err != nil {
		t.Fatal(err)
	}

	// Clone directory should be gone
	if _, err := os.Stat(info.ClonePath); !os.IsNotExist(err) {
		t.Errorf("clone directory should have been removed: %s", info.ClonePath)
	}

	// Should get ErrNoRepo now
	_, err = s.RepoPath()
	if _, ok := err.(*shimmer.ErrNoRepo); !ok {
		t.Errorf("expected ErrNoRepo after remove, got %v", err)
	}
}

func TestRepoRemoveNoRepo(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)

	s := newTestShimmer(t, home, project, false)
	err := s.RepoRemove()
	if _, ok := err.(*shimmer.ErrNoRepo); !ok {
		t.Errorf("expected ErrNoRepo, got %v", err)
	}
}
