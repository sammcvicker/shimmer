package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

// ---------------------------------------------------------------------------
// renderError tests
// ---------------------------------------------------------------------------

func TestRenderError_NoRepo_Local(t *testing.T) {
	err := renderError(&shimmer.ErrNoRepo{Target: "/home/user/project", Global: false})
	got := err.Error()

	if !strings.Contains(got, "/home/user/project") {
		t.Errorf("expected target path in output, got: %s", got)
	}
	if !strings.Contains(got, "shimmer repo set <url>") {
		t.Errorf("expected 'shimmer repo set <url>' hint, got: %s", got)
	}
}

func TestRenderError_NoRepo_Global(t *testing.T) {
	err := renderError(&shimmer.ErrNoRepo{Target: "/home/user", Global: true})
	got := err.Error()

	if !strings.Contains(got, "global") {
		t.Errorf("expected 'global' in output, got: %s", got)
	}
	if !strings.Contains(got, "shimmer repo set <url>") {
		t.Errorf("expected 'shimmer repo set <url>' hint, got: %s", got)
	}
}

func TestRenderError_RepoAlreadySet(t *testing.T) {
	err := renderError(&shimmer.ErrRepoAlreadySet{
		RemoteURL: "git@github.com:org/repo.git",
		ClonePath: "/home/.shimmer/repos/x",
	})
	got := err.Error()

	if !strings.Contains(got, "git@github.com:org/repo.git") {
		t.Errorf("expected remote URL in output, got: %s", got)
	}
	if !strings.Contains(got, "shimmer repo remove") {
		t.Errorf("expected 'shimmer repo remove' hint, got: %s", got)
	}
}

func TestRenderError_Conflicts_AllUntracked(t *testing.T) {
	err := renderError(&shimmer.ErrConflicts{
		Conflicts: []shimmer.Conflict{
			{Path: ".eslintrc", Tracked: false},
			{Path: ".prettierrc.json", Tracked: false},
		},
	})
	got := err.Error()

	if !strings.Contains(got, "(untracked)") {
		t.Errorf("expected '(untracked)' label, got: %s", got)
	}
	if strings.Contains(got, "git rm --cached") {
		t.Errorf("should not suggest 'git rm --cached' when all files are untracked, got: %s", got)
	}
}

func TestRenderError_Conflicts_WithTracked(t *testing.T) {
	err := renderError(&shimmer.ErrConflicts{
		Conflicts: []shimmer.Conflict{
			{Path: ".eslintrc", Tracked: true},
			{Path: ".prettierrc.json", Tracked: false},
		},
	})
	got := err.Error()

	if !strings.Contains(got, "(tracked)") {
		t.Errorf("expected '(tracked)' label, got: %s", got)
	}
	if !strings.Contains(got, "(untracked)") {
		t.Errorf("expected '(untracked)' label, got: %s", got)
	}
	if !strings.Contains(got, "git rm --cached .eslintrc") {
		t.Errorf("expected 'git rm --cached .eslintrc' for tracked file, got: %s", got)
	}
	// Should NOT suggest git rm --cached for the untracked file.
	if strings.Contains(got, "git rm --cached .prettierrc.json") {
		t.Errorf("should not suggest 'git rm --cached' for untracked file, got: %s", got)
	}
}

func TestRenderError_Conflicts_ColumnAlignment(t *testing.T) {
	err := renderError(&shimmer.ErrConflicts{
		Conflicts: []shimmer.Conflict{
			{Path: "a", Tracked: false},
			{Path: "longer-name.json", Tracked: true},
		},
	})
	got := err.Error()
	lines := strings.Split(got, "\n")

	// Find the two file-listing lines (start with "  " and contain a label).
	var labelCols []int
	for _, line := range lines {
		idx := strings.Index(line, "(untracked)")
		if idx < 0 {
			idx = strings.Index(line, "(tracked)")
		}
		if idx >= 0 {
			labelCols = append(labelCols, idx)
		}
	}
	if len(labelCols) != 2 {
		t.Fatalf("expected 2 label lines, found %d in:\n%s", len(labelCols), got)
	}
	if labelCols[0] != labelCols[1] {
		t.Errorf("labels are not column-aligned: columns %d and %d in:\n%s",
			labelCols[0], labelCols[1], got)
	}
}

func TestRenderError_NotInGitRepo(t *testing.T) {
	err := renderError(&shimmer.ErrNotInGitRepo{})
	got := err.Error()

	if !strings.Contains(got, "not in a git repository") {
		t.Errorf("expected 'not in a git repository', got: %s", got)
	}
	if !strings.Contains(got, "-g") {
		t.Errorf("expected '-g' hint, got: %s", got)
	}
}

func TestRenderError_NotLinked(t *testing.T) {
	err := renderError(&shimmer.ErrNotLinked{})
	got := err.Error()

	if !strings.Contains(got, "not linked") {
		t.Errorf("expected 'not linked', got: %s", got)
	}
}

func TestRenderError_UnknownError(t *testing.T) {
	original := fmt.Errorf("something")
	got := renderError(original)

	if got != original {
		t.Errorf("expected passthrough of unknown error, got different error: %v", got)
	}
}

// ---------------------------------------------------------------------------
// renderStatus tests
// ---------------------------------------------------------------------------

func TestRenderStatus_AllOK(t *testing.T) {
	var buf bytes.Buffer
	status := &shimmer.LinkStatus{
		Repo: shimmer.RepoInfo{
			Owner:  "org",
			Name:   "dotfiles",
			Branch: "main",
		},
		Files: []shimmer.FileStatus{
			{Path: ".editorconfig", OK: true},
			{Path: ".gitignore", OK: true},
		},
	}

	renderStatus(&buf, status)
	got := buf.String()

	if !strings.Contains(got, "linked (2 files)") {
		t.Errorf("expected 'linked (2 files)' header, got: %s", got)
	}
	if strings.Contains(got, "broken") {
		t.Errorf("should not mention 'broken' when all OK, got: %s", got)
	}
	if !strings.Contains(got, "repo: org/dotfiles @ main") {
		t.Errorf("expected repo line, got: %s", got)
	}
	if !strings.Contains(got, "ok:") {
		t.Errorf("expected 'ok:' lines, got: %s", got)
	}
}

func TestRenderStatus_WithBroken(t *testing.T) {
	var buf bytes.Buffer
	status := &shimmer.LinkStatus{
		Repo: shimmer.RepoInfo{
			Owner:  "org",
			Name:   "dotfiles",
			Branch: "main",
		},
		Files: []shimmer.FileStatus{
			{Path: ".editorconfig", OK: true},
			{Path: ".gitignore", OK: false, Reason: "target missing"},
		},
	}

	renderStatus(&buf, status)
	got := buf.String()

	if !strings.Contains(got, "linked (2 files, 1 broken)") {
		t.Errorf("expected 'linked (2 files, 1 broken)' header, got: %s", got)
	}
	if !strings.Contains(got, "BROKEN:") {
		t.Errorf("expected 'BROKEN:' line, got: %s", got)
	}
	if !strings.Contains(got, "target missing") {
		t.Errorf("expected reason in BROKEN line, got: %s", got)
	}
	if !strings.Contains(got, "shimmer link") {
		t.Errorf("expected reconcile hint, got: %s", got)
	}
}

func TestRenderStatus_StashedLocal(t *testing.T) {
	var buf bytes.Buffer
	status := &shimmer.LinkStatus{
		Repo: shimmer.RepoInfo{
			Owner:    "org",
			Name:     "dotfiles",
			Branch:   "main",
			IsGlobal: false,
		},
		Files: []shimmer.FileStatus{
			{Path: ".editorconfig", OK: true},
		},
		Stashed: []string{".eslintrc"},
	}

	renderStatus(&buf, status)
	got := buf.String()

	if !strings.Contains(got, ".git/shimmer-stash/") {
		t.Errorf("expected local stash path '.git/shimmer-stash/', got: %s", got)
	}
}

func TestRenderStatus_StashedGlobal(t *testing.T) {
	var buf bytes.Buffer
	status := &shimmer.LinkStatus{
		Repo: shimmer.RepoInfo{
			Owner:    "org",
			Name:     "dotfiles",
			Branch:   "main",
			IsGlobal: true,
		},
		Files: []shimmer.FileStatus{
			{Path: ".bashrc", OK: true},
		},
		Stashed: []string{".bashrc"},
	}

	renderStatus(&buf, status)
	got := buf.String()

	if !strings.Contains(got, "~/.shimmer/stash/") {
		t.Errorf("expected global stash path '~/.shimmer/stash/', got: %s", got)
	}
}
