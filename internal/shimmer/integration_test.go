package shimmer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullWorkflow(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":                "# Config v1",
		".claude/settings.json":    `{"key": "value"}`,
		".claude/skills/review.md": "# Review Skill",
		".cursorrules":             "rules",
		"README.md":                "overlay readme",
		".shimmerignore":           "README.md\n",
	})

	s := newTestShimmer(t, home, project, false)

	// Step 1: repo set
	info, err := s.RepoSet(overlayURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("clone at %s", info.ClonePath)

	// Step 2: repo path
	path, err := s.RepoPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != info.ClonePath {
		t.Errorf("repo path mismatch: %s vs %s", path, info.ClonePath)
	}

	// Step 3: link
	result, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}
	// README.md should be ignored via .shimmerignore, 4 files linked
	if len(result.Linked) != 4 {
		t.Errorf("expected 4 linked, got %d: %v", len(result.Linked), result.Linked)
	}

	// Verify symlinks work (content readable through them)
	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Config v1" {
		t.Errorf("content through symlink = %q", content)
	}

	// README.md should NOT be linked (shimmerignore)
	if _, err := os.Lstat(filepath.Join(project, "README.md")); !os.IsNotExist(err) {
		t.Error("README.md should not be linked (shimmerignore)")
	}

	// Step 4: status
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Files) != 4 {
		t.Errorf("expected 4 files in status, got %d", len(status.Files))
	}
	for _, f := range status.Files {
		if !f.OK {
			t.Errorf("file %s is broken: %s", f.Path, f.Reason)
		}
	}

	// Step 5: repo list
	repos, err := s.RepoList()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}

	// Step 6: re-link (should be idempotent)
	result2, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result2.Linked) != 4 {
		t.Errorf("re-link: expected 4 linked, got %d", len(result2.Linked))
	}

	// Step 7: unlink
	if _, err := s.Unlink(); err != nil {
		t.Fatal(err)
	}

	// All symlinks should be gone
	for _, rel := range result.Linked {
		p := filepath.Join(project, rel)
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s should not exist after unlink", rel)
		}
	}

	// .git/info/exclude should have no shimmer entries
	exclude, _ := os.ReadFile(filepath.Join(project, ".git", "info", "exclude"))
	if strings.Contains(string(exclude), "shimmer managed") {
		t.Error("exclude file still has shimmer entries")
	}
}
