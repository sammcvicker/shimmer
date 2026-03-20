package shimmer_test

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sammcvicker/shimmer/internal/shimmer"
)

func TestLinkBasic(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Claude Config",
		".claude/settings.json":  `{"key": "value"}`,
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	result, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}

	// Should have linked 2 files
	sort.Strings(result.Linked)
	if len(result.Linked) != 2 {
		t.Fatalf("expected 2 linked, got %d: %v", len(result.Linked), result.Linked)
	}
	if result.Linked[0] != ".claude/settings.json" {
		t.Errorf("expected .claude/settings.json, got %s", result.Linked[0])
	}
	if result.Linked[1] != "CLAUDE.md" {
		t.Errorf("expected CLAUDE.md, got %s", result.Linked[1])
	}

	// Verify symlinks actually exist and point to the clone
	for _, rel := range result.Linked {
		linkPath := filepath.Join(project, rel)
		target, err := os.Readlink(linkPath)
		if err != nil {
			t.Errorf("expected symlink at %s: %v", linkPath, err)
			continue
		}
		if !strings.Contains(target, home) {
			t.Errorf("symlink %s points to %s, expected to point into shimmer home", linkPath, target)
		}
	}

	// Verify .git/info/exclude was updated
	excludePath := filepath.Join(project, ".git", "info", "exclude")
	excludeContent, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("reading .git/info/exclude: %v", err)
	}
	content := string(excludeContent)
	if !strings.Contains(content, "# shimmer managed") {
		t.Error(".git/info/exclude missing shimmer managed block")
	}
	if !strings.Contains(content, "CLAUDE.md") {
		t.Error(".git/info/exclude missing CLAUDE.md")
	}
	if !strings.Contains(content, ".claude/settings.json") {
		t.Error(".git/info/exclude missing .claude/settings.json")
	}
}

func TestLinkConflictFails(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Create a conflicting file in the project
	writeFile(t, project, "CLAUDE.md", "existing content")

	_, err := s.Link(false, false)
	var conflictErr *shimmer.ErrConflicts
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected ErrConflicts, got %v", err)
	}
	if len(conflictErr.Conflicts) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(conflictErr.Conflicts))
	}
	if conflictErr.Conflicts[0].Path != "CLAUDE.md" {
		t.Errorf("expected conflict at CLAUDE.md, got %s", conflictErr.Conflicts[0].Path)
	}
}

func TestLinkSkip(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Claude Config",
		".claude/settings.json":  `{"key": "value"}`,
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Create a conflicting file for CLAUDE.md only
	writeFile(t, project, "CLAUDE.md", "existing content")

	result, err := s.Link(true, false)
	if err != nil {
		t.Fatal(err)
	}

	// Should have linked 1 file and skipped 1
	if len(result.Linked) != 1 {
		t.Fatalf("expected 1 linked, got %d: %v", len(result.Linked), result.Linked)
	}
	if result.Linked[0] != ".claude/settings.json" {
		t.Errorf("expected .claude/settings.json linked, got %s", result.Linked[0])
	}

	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped, got %d: %v", len(result.Skipped), result.Skipped)
	}
	if result.Skipped[0] != "CLAUDE.md" {
		t.Errorf("expected CLAUDE.md skipped, got %s", result.Skipped[0])
	}

	// The conflicting file should still have its original content
	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "existing content" {
		t.Errorf("conflicting file was modified: %s", content)
	}
}

func TestLinkOverwrite(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Create a conflicting untracked file
	writeFile(t, project, "CLAUDE.md", "existing content")

	result, err := s.Link(false, true)
	if err != nil {
		t.Fatal(err)
	}

	// Should have linked 1 and stashed 1
	if len(result.Linked) != 1 {
		t.Fatalf("expected 1 linked, got %d: %v", len(result.Linked), result.Linked)
	}
	if len(result.Stashed) != 1 {
		t.Fatalf("expected 1 stashed, got %d: %v", len(result.Stashed), result.Stashed)
	}

	// The destination should now be a symlink
	linkPath := filepath.Join(project, "CLAUDE.md")
	if _, err := os.Readlink(linkPath); err != nil {
		t.Errorf("expected symlink at %s: %v", linkPath, err)
	}

	// The original file should be stashed
	stashPath := filepath.Join(project, ".git", "shimmer-stash", "CLAUDE.md")
	stashedContent, err := os.ReadFile(stashPath)
	if err != nil {
		t.Fatalf("expected stashed file at %s: %v", stashPath, err)
	}
	if string(stashedContent) != "existing content" {
		t.Errorf("stashed content = %q, want %q", stashedContent, "existing content")
	}
}

func TestLinkReconciles(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Claude Config",
		".claude/settings.json":  `{"key": "value"}`,
	})

	s := newTestShimmer(t, home, project, false)
	info, err := s.RepoSet(overlayURL)
	if err != nil {
		t.Fatal(err)
	}

	// First link
	result1, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result1.Linked) != 2 {
		t.Fatalf("first link: expected 2 linked, got %d", len(result1.Linked))
	}

	// Simulate a file being deleted from the clone (e.g., branch switch)
	os.Remove(filepath.Join(info.ClonePath, ".claude/settings.json"))

	// Re-link
	result2, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}

	// Should have linked only 1 file now
	if len(result2.Linked) != 1 {
		t.Fatalf("second link: expected 1 linked, got %d: %v", len(result2.Linked), result2.Linked)
	}
	if result2.Linked[0] != "CLAUDE.md" {
		t.Errorf("expected CLAUDE.md, got %s", result2.Linked[0])
	}

	// The stale symlink should have been removed
	if len(result2.Removed) != 1 {
		t.Fatalf("expected 1 removed, got %d: %v", len(result2.Removed), result2.Removed)
	}

	// Verify the stale symlink no longer exists
	stalePath := filepath.Join(project, ".claude", "settings.json")
	if _, err := os.Lstat(stalePath); !os.IsNotExist(err) {
		t.Errorf("stale symlink should have been removed: %s", stalePath)
	}
}

func TestLinkOverwriteGuardsExistingStash(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Overlay",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Create a conflicting file and link with overwrite.
	writeFile(t, project, "CLAUDE.md", "precious-v1")
	if _, err := s.Link(false, true); err != nil {
		t.Fatal(err)
	}

	// Verify stash has the original.
	stashPath := filepath.Join(project, ".git", "shimmer-stash", "CLAUDE.md")
	content, err := os.ReadFile(stashPath)
	if err != nil {
		t.Fatalf("expected stash entry: %v", err)
	}
	if string(content) != "precious-v1" {
		t.Fatalf("stash content = %q, want %q", content, "precious-v1")
	}

	// Simulate: user deletes the symlink and creates a new real file.
	os.Remove(filepath.Join(project, "CLAUDE.md"))
	writeFile(t, project, "CLAUDE.md", "precious-v2")

	// Second link --overwrite should fail because stash entry exists.
	_, err = s.Link(false, true)
	if err == nil {
		t.Fatal("expected error on second overwrite with existing stash, got nil")
	}
	if !strings.Contains(err.Error(), "stash conflict") {
		t.Errorf("expected 'stash conflict' in error, got: %v", err)
	}

	// Original stash entry should be preserved.
	content, err = os.ReadFile(stashPath)
	if err != nil {
		t.Fatalf("stash entry should still exist: %v", err)
	}
	if string(content) != "precious-v1" {
		t.Errorf("stash was overwritten: got %q, want %q", content, "precious-v1")
	}
}
