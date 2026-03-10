# shimmer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the `shimmer` CLI — a Go tool that creates per-file symlinks from git-backed overlay repos into projects.

**Architecture:** Single binary CLI using Cobra. All logic in `internal/shimmer/` (one package). Thin Cobra wiring in `internal/cmd/`. Typed errors rendered at the CLI layer. Real filesystem + real git for all tests.

**Tech Stack:** Go, Cobra, git (on PATH)

**Reference docs:**
- Design spec: `docs/2026-03-09-shimmer-design.md`
- Implementation design: `docs/plans/2026-03-09-shimmer-implementation-design.md`

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/shimmer/main.go`
- Create: `internal/cmd/root.go`

**Step 1: Initialize Go module and install Cobra**

Run:
```bash
cd /Users/sam/projects/shimmer
go mod init github.com/siimpl/shimmer
go get github.com/spf13/cobra@latest
```

**Step 2: Create the entrypoint**

Create `cmd/shimmer/main.go`:

```go
package main

import (
	"os"

	"github.com/siimpl/shimmer/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

**Step 3: Create the root command with -g flag**

Create `internal/cmd/root.go`:

```go
package cmd

import (
	"github.com/spf13/cobra"
)

var globalFlag bool

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shimmer",
		Short: "Transparent git-backed file overlays",
		Long:  "shimmer creates per-file symlinks from a git-backed overlay repo into your project.\nUse -g for global scope ($HOME) instead of the current project.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "use global scope ($HOME)")

	return cmd
}

func Execute() error {
	return NewRootCmd().Execute()
}
```

**Step 4: Verify it builds and runs**

Run:
```bash
go build ./cmd/shimmer && ./shimmer --help
```
Expected: help output showing `shimmer` with `-g` flag.

**Step 5: Commit**

```bash
git add go.mod go.sum cmd/ internal/
git commit -m "scaffold project with cobra root command and -g flag"
```

---

### Task 2: Core Types and Test Helpers

**Files:**
- Create: `internal/shimmer/shimmer.go`
- Create: `internal/shimmer/errors.go`
- Create: `internal/shimmer/testutil_test.go`

**Step 1: Define core types**

Create `internal/shimmer/shimmer.go`:

```go
package shimmer

import (
	"os"
	"path/filepath"
)

// Shimmer is the central context for any operation.
type Shimmer struct {
	Home   string // ~/.shimmer
	Global bool   // -g flag
	Target string // project root (git root) or $HOME
}

// DefaultHome returns the default shimmer home directory.
func DefaultHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".shimmer")
}

// Link represents a single symlink to create.
type Link struct {
	Src  string // path in the clone
	Dest string // path in the project
}

// LinkPlan is what a link operation computes before acting.
type LinkPlan struct {
	Links     []Link
	Removals  []string   // stale symlinks to clean up
	Conflicts []Conflict // files that already exist at destination
}

// Conflict is an existing file that would be shadowed by a link.
type Conflict struct {
	Path    string
	Tracked bool
}

// LinkResult is what link returns after executing.
type LinkResult struct {
	Linked  []string
	Skipped []string
	Removed []string
	Stashed []string
}

// FileStatus is the health of a single linked file.
type FileStatus struct {
	Path   string
	OK     bool
	Reason string // empty if OK, e.g. "target missing" if broken
}

// RepoInfo is metadata about an overlay repo clone.
type RepoInfo struct {
	Owner      string
	Name       string
	RemoteURL  string
	TargetPath string // project path or "_global"
	Branch     string
	ClonePath  string
	Linked     bool
	TargetExists bool
}

// LinkStatus is what shimmer status returns.
type LinkStatus struct {
	Repo    RepoInfo
	Files   []FileStatus
	Stashed []string
}
```

**Step 2: Define error types**

Create `internal/shimmer/errors.go`:

```go
package shimmer

import "fmt"

// ErrNoRepo means no overlay repo is set for this scope.
type ErrNoRepo struct {
	Target string
	Global bool
}

func (e *ErrNoRepo) Error() string {
	if e.Global {
		return "no overlay repo set for global scope"
	}
	return fmt.Sprintf("no overlay repo set for %s", e.Target)
}

// ErrConflicts means existing files would be shadowed.
type ErrConflicts struct {
	Conflicts []Conflict
}

func (e *ErrConflicts) Error() string {
	return fmt.Sprintf("%d file(s) already exist and would be shadowed", len(e.Conflicts))
}

// ErrNotLinked means there are no shimmer symlinks to operate on.
type ErrNotLinked struct{}

func (e *ErrNotLinked) Error() string {
	return "not linked"
}

// ErrNotInGitRepo means a local-scope command was run outside a git repo.
type ErrNotInGitRepo struct{}

func (e *ErrNotInGitRepo) Error() string {
	return "not in a git repository (use -g for global scope)"
}

// ErrRepoAlreadySet means a repo is already configured for this scope.
type ErrRepoAlreadySet struct {
	RemoteURL string
	ClonePath string
}

func (e *ErrRepoAlreadySet) Error() string {
	return fmt.Sprintf("overlay repo already set: %s", e.RemoteURL)
}
```

**Step 3: Create test helpers**

Create `internal/shimmer/testutil_test.go`:

```go
package shimmer_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

// setupTestProject creates a temp directory with an initialized git repo.
// Returns the project path.
func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

// setupTestOverlay creates a bare git repo, clones it, adds the given files,
// and pushes. Returns the bare repo URL (for cloning) and the path where files
// were committed.
func setupTestOverlay(t *testing.T, files map[string]string) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "overlay.git")
	git(t, "", "init", "--bare", bare)

	work := t.TempDir()
	git(t, "", "clone", bare, work)

	for name, content := range files {
		p := filepath.Join(work, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, work, "add", "-A")
	git(t, work, "commit", "-m", "add overlay files")
	git(t, work, "push")

	return bare
}

// setupShimmerHome creates a temp ~/.shimmer equivalent.
func setupShimmerHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "repos"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newTestShimmer creates a Shimmer instance wired to test directories.
func newTestShimmer(t *testing.T, home, target string, global bool) *shimmer.Shimmer {
	t.Helper()
	return &shimmer.Shimmer{
		Home:   home,
		Global: global,
		Target: target,
	}
}

// git runs a git command in the given directory.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s\n%s", args, err, out)
	}
	return string(out)
}
```

**Step 4: Verify tests compile**

Run:
```bash
go test ./internal/shimmer/ -run TestNothing -v
```
Expected: `testing: warning: no tests to run` (compiles fine, no tests yet).

**Step 5: Commit**

```bash
git add internal/shimmer/
git commit -m "add core types, error types, and test helpers"
```

---

### Task 3: Path Utilities — Git Root, URL Parsing, Clone Path

**Files:**
- Create: `internal/shimmer/paths.go`
- Create: `internal/shimmer/paths_test.go`

These are pure utility functions that everything else builds on.

**Step 1: Write failing tests**

Create `internal/shimmer/paths_test.go`:

```go
package shimmer_test

import (
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestGitRoot(t *testing.T) {
	project := setupTestProject(t)

	// From project root
	root, err := shimmer.GitRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	if root != project {
		t.Errorf("got %q, want %q", root, project)
	}

	// From subdirectory
	sub := filepath.Join(project, "a", "b")
	if err := mkdir(sub); err != nil {
		t.Fatal(err)
	}
	root, err = shimmer.GitRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if root != project {
		t.Errorf("got %q, want %q", root, project)
	}

	// Outside git repo
	_, err = shimmer.GitRoot(t.TempDir())
	if err == nil {
		t.Error("expected error outside git repo")
	}
}

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		url          string
		wantOwner    string
		wantName     string
	}{
		{"git@github.com:siimpl/claude-dhi.git", "siimpl", "claude-dhi"},
		{"git@github.com:siimpl/claude-dhi", "siimpl", "claude-dhi"},
		{"https://github.com/siimpl/claude-dhi.git", "siimpl", "claude-dhi"},
		{"https://github.com/siimpl/claude-dhi", "siimpl", "claude-dhi"},
		{"git@github.com:other-org/claude-configs.git", "other-org", "claude-configs"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, name, err := shimmer.ParseRepoURL(tt.url)
			if err != nil {
				t.Fatal(err)
			}
			if owner != tt.wantOwner || name != tt.wantName {
				t.Errorf("got (%q, %q), want (%q, %q)", owner, name, tt.wantOwner, tt.wantName)
			}
		})
	}
}

func TestClonePath(t *testing.T) {
	home := "/home/test/.shimmer"

	// Local scope
	got := shimmer.ClonePath(home, "siimpl", "claude-dhi", "/Users/sam/projects/dhi", false)
	want := "/home/test/.shimmer/repos/siimpl/claude-dhi/Users/sam/projects/dhi"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Global scope
	got = shimmer.ClonePath(home, "siimpl", "claude-global", "", true)
	want = "/home/test/.shimmer/repos/siimpl/claude-global/_global"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func mkdir(path string) error {
	return filepath.WalkDir(path, func(_ string, _ any, _ error) error { return nil })
}
```

Note: the `mkdir` helper won't work correctly — use `os.MkdirAll` directly in the test. Let me correct that in step 3.

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/shimmer/ -v`
Expected: compilation errors — functions don't exist yet.

**Step 3: Implement path utilities**

Create `internal/shimmer/paths.go`:

```go
package shimmer

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitRoot finds the git repository root from the given directory.
func GitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", &ErrNotInGitRepo{}
	}
	return strings.TrimSpace(string(out)), nil
}

// ParseRepoURL extracts owner and repo name from a git URL.
// Supports SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git).
func ParseRepoURL(url string) (owner, name string, err error) {
	// Strip .git suffix
	url = strings.TrimSuffix(url, ".git")

	// SSH: git@github.com:owner/repo
	if strings.Contains(url, ":") && strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("cannot parse SSH URL: %s", url)
		}
		segments := strings.Split(parts[1], "/")
		if len(segments) < 2 {
			return "", "", fmt.Errorf("cannot parse owner/repo from: %s", url)
		}
		return segments[len(segments)-2], segments[len(segments)-1], nil
	}

	// HTTPS: https://github.com/owner/repo
	segments := strings.Split(strings.TrimRight(url, "/"), "/")
	if len(segments) < 2 {
		return "", "", fmt.Errorf("cannot parse owner/repo from URL: %s", url)
	}
	return segments[len(segments)-2], segments[len(segments)-1], nil
}

// ClonePath computes the filesystem path where a clone should live.
func ClonePath(home, owner, repo, targetPath string, global bool) string {
	if global {
		return filepath.Join(home, "repos", owner, repo, "_global")
	}
	// Strip leading slash so it nests cleanly
	rel := strings.TrimPrefix(targetPath, "/")
	return filepath.Join(home, "repos", owner, repo, rel)
}
```

**Step 4: Fix the test (replace mkdir with os.MkdirAll)**

In `paths_test.go`, replace the `mkdir` function and its usage:

```go
import "os"

// In TestGitRoot, replace:
//   if err := mkdir(sub); err != nil {
// with:
//   if err := os.MkdirAll(sub, 0o755); err != nil {
```

Remove the `mkdir` function entirely.

**Step 5: Run tests**

Run: `go test ./internal/shimmer/ -run 'TestGitRoot|TestParseRepoURL|TestClonePath' -v`
Expected: all pass.

**Step 6: Commit**

```bash
git add internal/shimmer/paths.go internal/shimmer/paths_test.go
git commit -m "add git root discovery, URL parsing, and clone path computation"
```

---

### Task 4: .shimmerignore Parsing

**Files:**
- Create: `internal/shimmer/ignore.go`
- Create: `internal/shimmer/ignore_test.go`

Gitignore-syntax file matching. Always implicitly ignores `.shimmerignore`, `.git/`, `.gitignore`.

**Step 1: Write failing tests**

Create `internal/shimmer/ignore_test.go`:

```go
package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestParseShimmerignore(t *testing.T) {
	dir := t.TempDir()
	content := "README.md\nLICENSE\n# comment\n\n*.txt\n"
	if err := os.WriteFile(filepath.Join(dir, ".shimmerignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ignore, err := shimmer.ParseShimmerignore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Explicitly listed
	assertIgnored(t, ignore, "README.md", true)
	assertIgnored(t, ignore, "LICENSE", true)
	assertIgnored(t, ignore, "notes.txt", true)

	// Always implicitly ignored
	assertIgnored(t, ignore, ".shimmerignore", true)
	assertIgnored(t, ignore, ".git/config", true)
	assertIgnored(t, ignore, ".gitignore", true)

	// Not ignored
	assertIgnored(t, ignore, "CLAUDE.md", false)
	assertIgnored(t, ignore, ".claude/settings.json", false)
}

func TestParseShimmerignoreNoFile(t *testing.T) {
	dir := t.TempDir()

	ignore, err := shimmer.ParseShimmerignore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Implicit ignores still work
	assertIgnored(t, ignore, ".shimmerignore", true)
	assertIgnored(t, ignore, ".git/config", true)

	// Everything else is not ignored
	assertIgnored(t, ignore, "CLAUDE.md", false)
}

func assertIgnored(t *testing.T, ignore *shimmer.Ignore, path string, want bool) {
	t.Helper()
	if got := ignore.Match(path); got != want {
		t.Errorf("ignore.Match(%q) = %v, want %v", path, got, want)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/shimmer/ -run TestParseShimmerignore -v`
Expected: compilation error.

**Step 3: Implement .shimmerignore parsing**

Create `internal/shimmer/ignore.go`:

Use a gitignore-compatible matching library or implement basic glob matching. For simplicity and correctness, use `github.com/go-git/go-git/v5/plumbing/format/gitignore` or implement a simpler version. Given "dumb simple" philosophy, let's use `filepath.Match` for basic patterns and prefix matching for directories.

```go
package shimmer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Ignore decides whether a file path should be excluded from linking.
type Ignore struct {
	patterns []string
}

// implicitIgnores are always excluded regardless of .shimmerignore content.
var implicitIgnores = []string{".shimmerignore", ".git", ".gitignore"}

// ParseShimmerignore reads .shimmerignore from the repo root.
// If the file doesn't exist, only implicit ignores apply.
func ParseShimmerignore(repoRoot string) (*Ignore, error) {
	ig := &Ignore{}

	f, err := os.Open(filepath.Join(repoRoot, ".shimmerignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return ig, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ig.patterns = append(ig.patterns, line)
	}
	return ig, scanner.Err()
}

// Match returns true if the path should be ignored.
func (ig *Ignore) Match(path string) bool {
	// Check implicit ignores
	for _, imp := range implicitIgnores {
		if path == imp || strings.HasPrefix(path, imp+"/") {
			return true
		}
	}

	// Check user patterns against the full path and the base name
	base := filepath.Base(path)
	for _, pattern := range ig.patterns {
		// Try matching against the full relative path
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Try matching against just the filename
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Directory prefix match (pattern without trailing slash)
		clean := strings.TrimSuffix(pattern, "/")
		if path == clean || strings.HasPrefix(path, clean+"/") {
			return true
		}
	}
	return false
}
```

**Step 4: Run tests**

Run: `go test ./internal/shimmer/ -run TestParseShimmerignore -v`
Expected: all pass.

**Step 5: Commit**

```bash
git add internal/shimmer/ignore.go internal/shimmer/ignore_test.go
git commit -m "add .shimmerignore parsing with implicit excludes"
```

---

### Task 5: Repo Set (Clone Overlay Into ~/.shimmer/repos/)

**Files:**
- Create: `internal/shimmer/repo.go`
- Create: `internal/shimmer/repo_test.go`
- Create: `internal/cmd/repo.go`

**Step 1: Write failing tests**

Create `internal/shimmer/repo_test.go`:

```go
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

	// Should use _global path segment
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

	// Second set should return ErrRepoAlreadySet
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/shimmer/ -run 'TestRepo' -v`
Expected: compilation errors.

**Step 3: Implement repo operations**

Create `internal/shimmer/repo.go`:

```go
package shimmer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoSet clones the overlay repo into ~/.shimmer/repos/.
// Returns ErrRepoAlreadySet if a clone already exists for this scope.
func (s *Shimmer) RepoSet(url string) (*RepoInfo, error) {
	owner, name, err := ParseRepoURL(url)
	if err != nil {
		return nil, err
	}

	clonePath := ClonePath(s.Home, owner, name, s.Target, s.Global)

	// Check if already set
	if _, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil {
		remote, _ := gitOutput(clonePath, "remote", "get-url", "origin")
		return nil, &ErrRepoAlreadySet{
			RemoteURL: strings.TrimSpace(remote),
			ClonePath: clonePath,
		}
	}

	// Create parent dirs and clone
	if err := os.MkdirAll(filepath.Dir(clonePath), 0o755); err != nil {
		return nil, fmt.Errorf("creating directories: %w", err)
	}

	cmd := exec.Command("git", "clone", url, clonePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %s\n%s", err, out)
	}

	return s.repoInfo(clonePath, owner, name)
}

// RepoPath returns the absolute path to the clone for the current scope.
func (s *Shimmer) RepoPath() (string, error) {
	clone, err := s.findClone()
	if err != nil {
		return "", err
	}
	return clone, nil
}

// RepoRemove unlinks (if linked) and deletes the clone for the current scope.
func (s *Shimmer) RepoRemove() error {
	clone, err := s.findClone()
	if err != nil {
		return err
	}

	// Unlink first if linked (ignore errors — may not be linked)
	_ = s.Unlink()

	// Remove the clone directory
	if err := os.RemoveAll(clone); err != nil {
		return fmt.Errorf("removing clone: %w", err)
	}

	// Clean up empty parent directories
	s.cleanEmptyParents(clone)

	return nil
}

// RepoList walks ~/.shimmer/repos/ and returns info about all clones.
func (s *Shimmer) RepoList() ([]RepoInfo, error) {
	reposDir := filepath.Join(s.Home, "repos")
	if _, err := os.Stat(reposDir); err != nil {
		return nil, nil // no repos directory = no repos
	}

	var repos []RepoInfo
	err := filepath.WalkDir(reposDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" && d.IsDir() {
			cloneDir := filepath.Dir(path)
			rel, _ := filepath.Rel(reposDir, cloneDir)
			segments := strings.SplitN(rel, string(os.PathSeparator), 3)
			if len(segments) < 3 {
				return nil
			}
			owner, name := segments[0], segments[1]
			info, err := s.repoInfo(cloneDir, owner, name)
			if err != nil {
				return nil // skip broken clones
			}
			repos = append(repos, *info)
			return filepath.SkipDir
		}
		return nil
	})
	return repos, err
}

// findClone locates the clone directory for the current scope by scanning
// ~/.shimmer/repos/ for a clone whose target matches.
func (s *Shimmer) findClone() (string, error) {
	reposDir := filepath.Join(s.Home, "repos")
	if _, err := os.Stat(reposDir); err != nil {
		return "", &ErrNoRepo{Target: s.Target, Global: s.Global}
	}

	var found string
	_ = filepath.WalkDir(reposDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipAll
		}
		if d.Name() == ".git" && d.IsDir() {
			cloneDir := filepath.Dir(path)
			rel, _ := filepath.Rel(reposDir, cloneDir)
			segments := strings.SplitN(rel, string(os.PathSeparator), 3)
			if len(segments) < 3 {
				return nil
			}
			targetSegment := segments[2]

			if s.Global && targetSegment == "_global" {
				found = cloneDir
				return filepath.SkipAll
			}
			if !s.Global {
				targetPath := "/" + targetSegment
				if targetPath == s.Target {
					found = cloneDir
					return filepath.SkipAll
				}
			}
			return filepath.SkipDir
		}
		return nil
	})

	if found == "" {
		return "", &ErrNoRepo{Target: s.Target, Global: s.Global}
	}
	return found, nil
}

// repoInfo builds RepoInfo from a clone directory.
func (s *Shimmer) repoInfo(clonePath, owner, name string) (*RepoInfo, error) {
	remote, _ := gitOutput(clonePath, "remote", "get-url", "origin")
	branch, _ := gitOutput(clonePath, "rev-parse", "--abbrev-ref", "HEAD")

	reposDir := filepath.Join(s.Home, "repos")
	rel, _ := filepath.Rel(reposDir, clonePath)
	segments := strings.SplitN(rel, string(os.PathSeparator), 3)

	targetSegment := ""
	if len(segments) >= 3 {
		targetSegment = segments[2]
	}

	targetPath := ""
	isGlobal := targetSegment == "_global"
	if !isGlobal {
		targetPath = "/" + targetSegment
	}

	targetExists := true
	if isGlobal {
		// $HOME always exists
	} else if targetPath != "" {
		if _, err := os.Stat(targetPath); err != nil {
			targetExists = false
		}
	}

	return &RepoInfo{
		Owner:        owner,
		Name:         name,
		RemoteURL:    strings.TrimSpace(remote),
		TargetPath:   targetPath,
		Branch:       strings.TrimSpace(branch),
		ClonePath:    clonePath,
		TargetExists: targetExists,
	}, nil
}

// gitOutput runs git in the given directory and returns stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

// cleanEmptyParents removes empty directories up the tree.
func (s *Shimmer) cleanEmptyParents(path string) {
	reposDir := filepath.Join(s.Home, "repos")
	dir := filepath.Dir(path)
	for dir != reposDir && dir != s.Home {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}
```

**Step 4: Add stub Unlink method** (to satisfy compiler; full implementation in Task 9)

Add to `internal/shimmer/shimmer.go` temporarily:

```go
// Unlink removes all shimmer symlinks. Stub — implemented in Task 9.
func (s *Shimmer) Unlink() error {
	return nil
}
```

**Step 5: Run tests**

Run: `go test ./internal/shimmer/ -run 'TestRepo' -v`
Expected: all pass.

**Step 6: Wire up Cobra commands**

Create `internal/cmd/repo.go`:

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/siimpl/shimmer/internal/shimmer"
	"github.com/spf13/cobra"
)

func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage overlay repositories",
	}

	cmd.AddCommand(newRepoSetCmd())
	cmd.AddCommand(newRepoListCmd())
	cmd.AddCommand(newRepoRemoveCmd())
	cmd.AddCommand(newRepoPathCmd())

	return cmd
}

func newRepoSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <url> [project-path]",
		Short: "Clone an overlay repo for the current scope",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags(args)
			if err != nil {
				return err
			}

			// If a second arg is given, use it as the target (resolve to absolute)
			if len(args) > 1 {
				abs, err := resolveProjectPath(args[1])
				if err != nil {
					return err
				}
				s.Target = abs
			}

			info, err := s.RepoSet(args[0])
			if err != nil {
				return renderError(err)
			}
			fmt.Printf("cloned %s/%s into %s\n", info.Owner, info.Name, info.ClonePath)
			return nil
		},
	}
}

func newRepoListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all overlay repos",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := &shimmer.Shimmer{Home: shimmer.DefaultHome()}
			repos, err := s.RepoList()
			if err != nil {
				return err
			}
			if len(repos) == 0 {
				fmt.Println("no overlay repos configured")
				return nil
			}
			for _, r := range repos {
				scope := r.TargetPath
				if scope == "" {
					scope = "global"
				}
				status := ""
				if !r.TargetExists && scope != "global" {
					status = " (project not found on disk)"
				}
				fmt.Printf("%s/%s @ %s -> %s%s\n", r.Owner, r.Name, r.Branch, scope, status)
			}
			return nil
		},
	}
}

func newRepoRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [project-path]",
		Short: "Remove the overlay repo for the current scope",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags(args)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				abs, err := resolveProjectPath(args[0])
				if err != nil {
					return err
				}
				s.Target = abs
			}
			if err := s.RepoRemove(); err != nil {
				return renderError(err)
			}
			fmt.Println("removed")
			return nil
		},
	}
}

func newRepoPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the path to the overlay repo clone",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags(nil)
			if err != nil {
				return err
			}
			p, err := s.RepoPath()
			if err != nil {
				return renderError(err)
			}
			fmt.Println(p)
			return nil
		},
	}
}

// newShimmerFromFlags creates a Shimmer instance from the -g flag and current directory.
func newShimmerFromFlags(args []string) (*shimmer.Shimmer, error) {
	s := &shimmer.Shimmer{
		Home:   shimmer.DefaultHome(),
		Global: globalFlag,
	}

	if globalFlag {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		s.Target = home
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root, err := shimmer.GitRoot(cwd)
		if err != nil {
			return nil, err
		}
		s.Target = root
	}

	return s, nil
}

// resolveProjectPath resolves a project path argument to an absolute git root.
func resolveProjectPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return shimmer.GitRoot(abs)
}
```

**Step 7: Register repo command and add renderError**

Update `internal/cmd/root.go` to add the repo subcommand and error rendering:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/siimpl/shimmer/internal/shimmer"
	"github.com/spf13/cobra"
)

var globalFlag bool

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shimmer",
		Short: "Transparent git-backed file overlays",
		Long:  "shimmer creates per-file symlinks from a git-backed overlay repo into your project.\nUse -g for global scope ($HOME) instead of the current project.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "use global scope ($HOME)")

	cmd.AddCommand(newRepoCmd())

	return cmd
}

func Execute() error {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

// renderError formats typed errors for user-facing output.
func renderError(err error) error {
	switch e := err.(type) {
	case *shimmer.ErrNoRepo:
		if e.Global {
			return fmt.Errorf("no overlay repo set for global scope\n\n  shimmer -g repo set <url>")
		}
		return fmt.Errorf("no overlay repo set for %s\n\n  shimmer repo set <url>", e.Target)

	case *shimmer.ErrRepoAlreadySet:
		return fmt.Errorf("overlay repo already set: %s\n\nTo replace, remove first:\n  shimmer repo remove", e.RemoteURL)

	case *shimmer.ErrConflicts:
		var b strings.Builder
		b.WriteString("these files already exist and would be shadowed:\n")
		for _, c := range e.Conflicts {
			status := "untracked"
			if c.Tracked {
				status = "tracked"
			}
			fmt.Fprintf(&b, "  %-20s (%s)\n", c.Path, status)
		}
		b.WriteString("\nOptions:\n")
		b.WriteString("  --skip        Link only non-conflicting files, leave existing ones in place\n")
		b.WriteString("  --overwrite   Stash existing files and shadow them (tracked files use\n")
		b.WriteString("                skip-worktree, which is fragile — see docs)\n")
		b.WriteString("\nTo permanently resolve tracked file conflicts (recommended):\n")
		for _, c := range e.Conflicts {
			if c.Tracked {
				fmt.Fprintf(&b, "  git rm --cached %s\n", c.Path)
			}
		}
		b.WriteString("  shimmer link\n")
		b.WriteString("\nTo undo any shimmer operation:\n")
		b.WriteString("  shimmer unlink\n")
		return fmt.Errorf("%s", b.String())

	case *shimmer.ErrNotInGitRepo:
		return fmt.Errorf("not in a git repository (use -g for global scope)")

	case *shimmer.ErrNotLinked:
		return fmt.Errorf("not linked — nothing to do")

	default:
		return err
	}
}
```

**Step 8: Verify build and repo set works**

Run:
```bash
go build ./cmd/shimmer && ./shimmer repo --help
```
Expected: help text for `shimmer repo` with subcommands.

**Step 9: Commit**

```bash
git add internal/shimmer/repo.go internal/shimmer/repo_test.go internal/cmd/repo.go internal/cmd/root.go
git commit -m "add repo set, path, list, and remove commands"
```

---

### Task 6: Repo File Walking (Collect Linkable Files)

**Files:**
- Create: `internal/shimmer/walk.go`
- Create: `internal/shimmer/walk_test.go`

Walks the overlay clone, collects files not in `.shimmerignore`, returns relative paths.

**Step 1: Write failing tests**

Create `internal/shimmer/walk_test.go`:

```go
package shimmer_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestWalkOverlay(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "CLAUDE.md", "config")
	writeFile(t, dir, ".claude/settings.json", "{}")
	writeFile(t, dir, ".claude/skills/review.md", "skill")
	writeFile(t, dir, ".cursorrules", "rules")
	writeFile(t, dir, "README.md", "readme")
	writeFile(t, dir, ".shimmerignore", "README.md\n")
	writeFile(t, dir, ".gitignore", "")
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)

	files, err := shimmer.WalkOverlay(dir)
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(files)
	want := []string{".claude/settings.json", ".claude/skills/review.md", ".cursorrules", "CLAUDE.md"}
	if !slicesEqual(files, want) {
		t.Errorf("got %v, want %v", files, want)
	}
}

func writeFile(t *testing.T, base, rel, content string) {
	t.Helper()
	p := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/shimmer/ -run TestWalkOverlay -v`
Expected: compilation error.

**Step 3: Implement walk**

Create `internal/shimmer/walk.go`:

```go
package shimmer

import (
	"os"
	"path/filepath"
)

// WalkOverlay collects all files in the overlay repo that should be linked.
// Returns paths relative to the repo root.
func WalkOverlay(repoRoot string) ([]string, error) {
	ignore, err := ParseShimmerignore(repoRoot)
	if err != nil {
		return nil, err
	}

	var files []string
	err = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		if ignore.Match(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}
```

**Step 4: Run tests**

Run: `go test ./internal/shimmer/ -run TestWalkOverlay -v`
Expected: pass.

**Step 5: Commit**

```bash
git add internal/shimmer/walk.go internal/shimmer/walk_test.go
git commit -m "add overlay file walking with shimmerignore filtering"
```

---

### Task 7: Symlink Scanning (Find Existing Shimmer Links)

**Files:**
- Create: `internal/shimmer/scan.go`
- Create: `internal/shimmer/scan_test.go`

Scans a target directory for symlinks pointing into `~/.shimmer/repos/`.

**Step 1: Write failing tests**

Create `internal/shimmer/scan_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/shimmer/ -run TestScanSymlinks -v`
Expected: compilation error.

**Step 3: Implement symlink scanning**

Create `internal/shimmer/scan.go`:

```go
package shimmer

import (
	"os"
	"path/filepath"
	"strings"
)

// ScanSymlinks finds all symlinks under targetDir that point into shimmerHome/repos/.
func ScanSymlinks(targetDir, shimmerHome string) ([]string, error) {
	reposDir := filepath.Join(shimmerHome, "repos")
	var links []string

	err := filepath.WalkDir(targetDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		// Skip .git directory
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			// Resolve to absolute
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			if strings.HasPrefix(target, reposDir) {
				links = append(links, path)
			}
		}
		return nil
	})
	return links, err
}
```

**Step 4: Run tests**

Run: `go test ./internal/shimmer/ -run TestScanSymlinks -v`
Expected: all pass.

**Step 5: Commit**

```bash
git add internal/shimmer/scan.go internal/shimmer/scan_test.go
git commit -m "add symlink scanning to find shimmer-managed links"
```

---

### Task 8: Link (The Core Operation)

**Files:**
- Create: `internal/shimmer/link.go`
- Create: `internal/shimmer/link_test.go`
- Create: `internal/cmd/link.go`

This is the biggest task. Link reconciles symlinks from scratch:
1. Find the clone
2. Remove stale symlinks
3. Walk overlay, detect conflicts
4. Create symlinks (or skip/overwrite)
5. Update .git/info/exclude
6. Return summary

**Step 1: Write failing tests**

Create `internal/shimmer/link_test.go`:

```go
package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestLinkBasic(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Config",
		".claude/settings.json":  "{}",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	result, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Linked) != 2 {
		t.Errorf("expected 2 linked, got %d: %v", len(result.Linked), result.Linked)
	}

	// Verify symlinks exist and point to the clone
	for _, rel := range result.Linked {
		p := filepath.Join(project, rel)
		target, err := os.Readlink(p)
		if err != nil {
			t.Errorf("%s is not a symlink: %v", rel, err)
		}
		if target == "" {
			t.Errorf("%s has empty target", rel)
		}
	}

	// Verify .git/info/exclude was updated
	excludePath := filepath.Join(project, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(string(content), "CLAUDE.md") {
		t.Error("exclude file missing CLAUDE.md")
	}
}

func TestLinkConflictFails(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Config",
	})

	// Create a conflicting file
	writeFile(t, project, "CLAUDE.md", "existing")

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	_, err := s.Link(false, false)
	conflicts, ok := err.(*shimmer.ErrConflicts)
	if !ok {
		t.Fatalf("expected ErrConflicts, got %v", err)
	}
	if len(conflicts.Conflicts) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(conflicts.Conflicts))
	}
}

func TestLinkSkip(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Config",
		".claude/settings.json":  "{}",
	})

	// Create a conflicting file for CLAUDE.md only
	writeFile(t, project, "CLAUDE.md", "existing")

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	result, err := s.Link(true, false) // --skip
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Linked) != 1 {
		t.Errorf("expected 1 linked, got %d", len(result.Linked))
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(result.Skipped))
	}

	// Original file should be untouched
	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "existing" {
		t.Error("original file was modified")
	}
}

func TestLinkOverwrite(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Config",
	})

	// Create a conflicting untracked file
	writeFile(t, project, "CLAUDE.md", "existing")

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	result, err := s.Link(false, true) // --overwrite
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Linked) != 1 {
		t.Errorf("expected 1 linked, got %d", len(result.Linked))
	}
	if len(result.Stashed) != 1 {
		t.Errorf("expected 1 stashed, got %d", len(result.Stashed))
	}

	// Stashed file should exist
	stashed := filepath.Join(project, ".git", "shimmer-stash", "CLAUDE.md")
	content, err := os.ReadFile(stashed)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "existing" {
		t.Errorf("stashed content = %q, want %q", content, "existing")
	}

	// Symlink should now exist
	if _, err := os.Readlink(filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Error("CLAUDE.md is not a symlink after overwrite")
	}
}

func TestLinkReconciles(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Config",
		".claude/settings.json":  "{}",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// First link
	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Manually remove a file from the clone to simulate branch switch
	clonePath, _ := s.RepoPath()
	os.Remove(filepath.Join(clonePath, "CLAUDE.md"))

	// Re-link should clean up the stale symlink
	result, err := s.Link(false, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d", len(result.Removed))
	}
	if len(result.Linked) != 1 {
		t.Errorf("expected 1 linked, got %d: %v", len(result.Linked), result.Linked)
	}

	// Stale symlink should be gone
	if _, err := os.Lstat(filepath.Join(project, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("stale symlink CLAUDE.md was not removed")
	}
}

func containsLine(text, line string) bool {
	for _, l := range filepath.SplitList(text) {
		if l == line {
			return true
		}
	}
	// Simple substring check as fallback
	return len(line) > 0 && len(text) > 0 && filepath.Base(line) != "" &&
		contains(text, line)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/shimmer/ -run TestLink -v`
Expected: compilation error.

**Step 3: Implement link**

Create `internal/shimmer/link.go`:

```go
package shimmer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Link reconciles symlinks between the overlay clone and the target.
// skip: skip conflicting files. overwrite: stash and shadow conflicting files.
func (s *Shimmer) Link(skip, overwrite bool) (*LinkResult, error) {
	clonePath, err := s.findClone()
	if err != nil {
		return nil, err
	}

	result := &LinkResult{}

	// Step 1: Remove stale symlinks from previous link state
	existing, err := ScanSymlinks(s.Target, s.Home)
	if err != nil {
		return nil, fmt.Errorf("scanning existing links: %w", err)
	}
	for _, link := range existing {
		os.Remove(link)
		rel, _ := filepath.Rel(s.Target, link)
		result.Removed = append(result.Removed, rel)
		// Clean up empty parent directories
		s.cleanEmptyLinkParents(filepath.Dir(link))
	}

	// Step 2: Walk overlay to get files to link
	overlayFiles, err := WalkOverlay(clonePath)
	if err != nil {
		return nil, fmt.Errorf("walking overlay: %w", err)
	}

	// Step 3: Check for conflicts
	var conflicts []Conflict
	for _, rel := range overlayFiles {
		dest := filepath.Join(s.Target, rel)
		info, err := os.Lstat(dest)
		if err != nil {
			continue // doesn't exist, no conflict
		}
		// If it's a symlink into our repos dir, it was already cleaned up above
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(dest)
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(dest), target)
			}
			if strings.HasPrefix(target, filepath.Join(s.Home, "repos")) {
				continue
			}
		}
		tracked := s.isTracked(rel)
		conflicts = append(conflicts, Conflict{Path: rel, Tracked: tracked})
	}

	// Step 4: Handle conflicts
	if len(conflicts) > 0 && !skip && !overwrite {
		return nil, &ErrConflicts{Conflicts: conflicts}
	}

	// Step 5: Create symlinks
	for _, rel := range overlayFiles {
		dest := filepath.Join(s.Target, rel)
		src := filepath.Join(clonePath, rel)

		// Check if this is a conflict
		isConflict := false
		for _, c := range conflicts {
			if c.Path == rel {
				isConflict = true
				break
			}
		}

		if isConflict {
			if skip {
				result.Skipped = append(result.Skipped, rel)
				continue
			}
			if overwrite {
				if err := s.stashFile(rel, dest); err != nil {
					return nil, fmt.Errorf("stashing %s: %w", rel, err)
				}
				result.Stashed = append(result.Stashed, rel)

				// Set skip-worktree for tracked files
				if s.isTracked(rel) {
					s.setSkipWorktree(rel, true)
				}
			}
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, fmt.Errorf("creating directory for %s: %w", rel, err)
		}

		// Create symlink
		if err := os.Symlink(src, dest); err != nil {
			return nil, fmt.Errorf("creating symlink %s: %w", rel, err)
		}
		result.Linked = append(result.Linked, rel)
	}

	// Step 6: Update .git/info/exclude (local scope only)
	if !s.Global {
		if err := s.updateGitExclude(result.Linked); err != nil {
			return nil, fmt.Errorf("updating git exclude: %w", err)
		}
	}

	// Remove entries from Removed that are also in Linked (re-linked files)
	result.Removed = filterOut(result.Removed, result.Linked)

	return result, nil
}

// stashFile moves a file to the stash location.
func (s *Shimmer) stashFile(rel, src string) error {
	stashPath := s.stashPath(rel)
	if err := os.MkdirAll(filepath.Dir(stashPath), 0o755); err != nil {
		return err
	}
	return os.Rename(src, stashPath)
}

// stashPath returns the stash location for a file.
func (s *Shimmer) stashPath(rel string) string {
	if s.Global {
		return filepath.Join(s.Home, "stash", rel)
	}
	return filepath.Join(s.Target, ".git", "shimmer-stash", rel)
}

// isTracked checks if a file is tracked by git in the target project.
func (s *Shimmer) isTracked(rel string) bool {
	if s.Global {
		return false
	}
	cmd := exec.Command("git", "-C", s.Target, "ls-files", "--error-unmatch", rel)
	return cmd.Run() == nil
}

// setSkipWorktree sets or clears the skip-worktree flag on a tracked file.
func (s *Shimmer) setSkipWorktree(rel string, set bool) {
	flag := "--skip-worktree"
	if !set {
		flag = "--no-skip-worktree"
	}
	exec.Command("git", "-C", s.Target, "update-index", flag, rel).Run()
}

// updateGitExclude writes shimmer-linked paths to .git/info/exclude.
func (s *Shimmer) updateGitExclude(linkedPaths []string) error {
	excludeDir := filepath.Join(s.Target, ".git", "info")
	excludePath := filepath.Join(excludeDir, "exclude")

	if err := os.MkdirAll(excludeDir, 0o755); err != nil {
		return err
	}

	// Read existing content, preserve non-shimmer lines
	existing, _ := os.ReadFile(excludePath)
	var preserved []string
	inShimmerBlock := false
	for _, line := range strings.Split(string(existing), "\n") {
		if line == "# shimmer managed — do not edit" {
			inShimmerBlock = true
			continue
		}
		if line == "# end shimmer" {
			inShimmerBlock = false
			continue
		}
		if !inShimmerBlock {
			preserved = append(preserved, line)
		}
	}

	// Remove trailing empty lines
	for len(preserved) > 0 && preserved[len(preserved)-1] == "" {
		preserved = preserved[:len(preserved)-1]
	}

	// Build new content
	var b strings.Builder
	for _, line := range preserved {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(linkedPaths) > 0 {
		b.WriteString("\n# shimmer managed — do not edit\n")
		for _, p := range linkedPaths {
			b.WriteString(p)
			b.WriteString("\n")
		}
		b.WriteString("# end shimmer\n")
	}

	return os.WriteFile(excludePath, []byte(b.String()), 0o644)
}

// cleanEmptyLinkParents removes empty directories up to the target root.
func (s *Shimmer) cleanEmptyLinkParents(dir string) {
	for dir != s.Target {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// filterOut returns items in a that are not in b.
func filterOut(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	var result []string
	for _, s := range a {
		if !set[s] {
			result = append(result, s)
		}
	}
	return result
}
```

**Step 4: Run tests**

Run: `go test ./internal/shimmer/ -run TestLink -v`
Expected: all pass.

**Step 5: Wire up Cobra command**

Create `internal/cmd/link.go`:

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var skip, overwrite bool

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Create symlinks from the overlay repo into the project",
		Long:  "Reconcile symlinks between the overlay repo and the project.\nSafe to run at any time — cleans up stale links and creates new ones.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags(nil)
			if err != nil {
				return renderError(err)
			}
			result, err := s.Link(skip, overwrite)
			if err != nil {
				return renderError(err)
			}
			renderLinkResult(result)
			return nil
		},
	}

	cmd.Flags().BoolVar(&skip, "skip", false, "skip conflicting files")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "stash and shadow conflicting files")

	return cmd
}

func renderLinkResult(r *shimmer.LinkResult) {
	if len(r.Linked) > 0 {
		fmt.Printf("linked %d file(s)\n", len(r.Linked))
		for _, f := range r.Linked {
			fmt.Printf("  %s\n", f)
		}
	}
	if len(r.Skipped) > 0 {
		fmt.Printf("skipped %d file(s)\n", len(r.Skipped))
		for _, f := range r.Skipped {
			fmt.Printf("  %s\n", f)
		}
	}
	if len(r.Removed) > 0 {
		fmt.Printf("removed %d stale link(s)\n", len(r.Removed))
		for _, f := range r.Removed {
			fmt.Printf("  %s\n", f)
		}
	}
	if len(r.Stashed) > 0 {
		fmt.Printf("stashed %d original(s)\n", len(r.Stashed))
		for _, f := range r.Stashed {
			fmt.Printf("  %s\n", f)
		}
		// Warn about skip-worktree fragility
		fmt.Println("\n⚠ tracked files use skip-worktree, which is fragile.")
		fmt.Println("  git stash, git checkout --force, and git reset can silently undo it.")
	}
}
```

Note: Add `"github.com/siimpl/shimmer/internal/shimmer"` to the import.

**Step 6: Register in root command**

Add `cmd.AddCommand(newLinkCmd())` to `NewRootCmd()` in `internal/cmd/root.go`.

**Step 7: Build and verify**

Run: `go build ./cmd/shimmer && ./shimmer link --help`
Expected: help text with `--skip` and `--overwrite` flags.

**Step 8: Commit**

```bash
git add internal/shimmer/link.go internal/shimmer/link_test.go internal/cmd/link.go internal/cmd/root.go
git commit -m "add link command — symlink creation, conflict detection, stashing"
```

---

### Task 9: Unlink (Restore Pre-Shimmer State)

**Files:**
- Modify: `internal/shimmer/shimmer.go` (replace stub)
- Create: `internal/shimmer/unlink.go`
- Create: `internal/shimmer/unlink_test.go`
- Create: `internal/cmd/unlink.go`

**Step 1: Write failing tests**

Create `internal/shimmer/unlink_test.go`:

```go
package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestUnlink(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Config",
		".claude/settings.json":  "{}",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Verify links exist
	if _, err := os.Readlink(filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Fatal("link should exist before unlink")
	}

	// Unlink
	if err := s.Unlink(); err != nil {
		t.Fatal(err)
	}

	// Symlinks should be gone
	if _, err := os.Lstat(filepath.Join(project, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("CLAUDE.md should not exist after unlink")
	}
	if _, err := os.Lstat(filepath.Join(project, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Error(".claude/settings.json should not exist after unlink")
	}
}

func TestUnlinkRestoresStashed(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Config",
	})

	// Create original file
	writeFile(t, project, "CLAUDE.md", "original content")

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Link(false, true); err != nil { // --overwrite
		t.Fatal(err)
	}

	// Unlink
	if err := s.Unlink(); err != nil {
		t.Fatal(err)
	}

	// Original should be restored
	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original content" {
		t.Errorf("restored content = %q, want %q", content, "original content")
	}

	// Stash should be cleaned up
	stash := filepath.Join(project, ".git", "shimmer-stash")
	if _, err := os.Stat(stash); !os.IsNotExist(err) {
		t.Error("stash directory should be removed after unlink")
	}
}

func TestUnlinkNotLinked(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)

	s := newTestShimmer(t, home, project, false)
	err := s.Unlink()
	// Should be a no-op, not an error
	if err != nil {
		t.Errorf("unlink when not linked should be no-op, got %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/shimmer/ -run TestUnlink -v`

**Step 3: Implement unlink**

Create `internal/shimmer/unlink.go`:

```go
package shimmer

import (
	"os"
	"path/filepath"
	"strings"
)

// Unlink removes all shimmer symlinks from the target, restores stashed files,
// clears skip-worktree flags, and cleans up .git/info/exclude entries.
func (s *Shimmer) Unlink() error {
	// Find and remove shimmer symlinks
	links, err := ScanSymlinks(s.Target, s.Home)
	if err != nil {
		return err
	}

	for _, link := range links {
		rel, _ := filepath.Rel(s.Target, link)

		// Remove the symlink
		os.Remove(link)

		// Clear skip-worktree if it was set
		if !s.Global {
			s.setSkipWorktree(rel, false)
		}

		// Clean up empty parent directories
		s.cleanEmptyLinkParents(filepath.Dir(link))
	}

	// Restore stashed files
	s.restoreStash()

	// Clean up .git/info/exclude (local only)
	if !s.Global {
		s.updateGitExclude(nil)
	}

	return nil
}

// restoreStash moves all stashed files back to their original locations.
func (s *Shimmer) restoreStash() {
	stashDir := s.stashDir()
	if _, err := os.Stat(stashDir); err != nil {
		return
	}

	filepath.WalkDir(stashDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(stashDir, path)
		dest := filepath.Join(s.Target, rel)

		os.MkdirAll(filepath.Dir(dest), 0o755)
		os.Rename(path, dest)
		return nil
	})

	// Clean up stash directory
	os.RemoveAll(stashDir)
}

// stashDir returns the root stash directory.
func (s *Shimmer) stashDir() string {
	if s.Global {
		return filepath.Join(s.Home, "stash")
	}
	return filepath.Join(s.Target, ".git", "shimmer-stash")
}

// hasStashedFiles checks if the stash directory has any files.
func (s *Shimmer) hasStashedFiles() bool {
	stashDir := s.stashDir()
	entries, err := os.ReadDir(stashDir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// StashedFiles returns the list of stashed file paths (relative to target).
func (s *Shimmer) StashedFiles() []string {
	stashDir := s.stashDir()
	var files []string
	filepath.WalkDir(stashDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(stashDir, path)
		files = append(files, rel)
		return nil
	})
	return files
}
```

**Step 4: Remove the Unlink stub from shimmer.go**

Delete the stub `Unlink` method from `internal/shimmer/shimmer.go`.

**Step 5: Run tests**

Run: `go test ./internal/shimmer/ -run TestUnlink -v`
Expected: all pass. Also run all tests: `go test ./internal/shimmer/ -v`

**Step 6: Wire up Cobra command**

Create `internal/cmd/unlink.go`:

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink",
		Short: "Remove all shimmer symlinks and restore originals",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags(nil)
			if err != nil {
				return renderError(err)
			}
			if err := s.Unlink(); err != nil {
				return renderError(err)
			}
			fmt.Println("unlinked")
			return nil
		},
	}
}
```

**Step 7: Register in root command**

Add `cmd.AddCommand(newUnlinkCmd())` in `NewRootCmd()`.

**Step 8: Commit**

```bash
git add internal/shimmer/unlink.go internal/shimmer/unlink_test.go internal/shimmer/shimmer.go internal/cmd/unlink.go internal/cmd/root.go
git commit -m "add unlink command — symlink removal, stash restore, exclude cleanup"
```

---

### Task 10: Status (Symlink Health Check)

**Files:**
- Create: `internal/shimmer/status.go`
- Create: `internal/shimmer/status_test.go`
- Create: `internal/cmd/status.go`

**Step 1: Write failing tests**

Create `internal/shimmer/status_test.go`:

```go
package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestStatusAllOK(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Config",
		".claude/settings.json":  "{}",
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
		t.Fatal(err)
	}

	if len(status.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(status.Files))
	}
	for _, f := range status.Files {
		if !f.OK {
			t.Errorf("%s is broken: %s", f.Path, f.Reason)
		}
	}
}

func TestStatusBroken(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Config",
		".claude/settings.json":  "{}",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Delete a file from the clone to create a broken link
	clonePath, _ := s.RepoPath()
	os.Remove(filepath.Join(clonePath, "CLAUDE.md"))

	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}

	broken := 0
	for _, f := range status.Files {
		if !f.OK {
			broken++
		}
	}
	if broken != 1 {
		t.Errorf("expected 1 broken, got %d", broken)
	}
}

func TestStatusNotLinked(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)

	s := newTestShimmer(t, home, project, false)
	_, err := s.Status()
	if _, ok := err.(*shimmer.ErrNotLinked); !ok {
		t.Errorf("expected ErrNotLinked, got %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/shimmer/ -run TestStatus -v`

**Step 3: Implement status**

Create `internal/shimmer/status.go`:

```go
package shimmer

import (
	"os"
	"path/filepath"
	"strings"
)

// Status returns the symlink health for the current scope.
func (s *Shimmer) Status() (*LinkStatus, error) {
	links, err := ScanSymlinks(s.Target, s.Home)
	if err != nil {
		return nil, err
	}

	if len(links) == 0 {
		return nil, &ErrNotLinked{}
	}

	// Derive repo info from the first symlink's target
	firstTarget, _ := os.Readlink(links[0])
	if !filepath.IsAbs(firstTarget) {
		firstTarget = filepath.Join(filepath.Dir(links[0]), firstTarget)
	}

	reposDir := filepath.Join(s.Home, "repos")
	rel, _ := filepath.Rel(reposDir, firstTarget)
	segments := strings.SplitN(rel, string(os.PathSeparator), 3)

	var repo RepoInfo
	if len(segments) >= 2 {
		cloneDir := filepath.Dir(firstTarget)
		// Walk up to find the .git dir (clone root)
		for cloneDir != reposDir {
			if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err == nil {
				break
			}
			cloneDir = filepath.Dir(cloneDir)
		}
		info, err := s.repoInfo(cloneDir, segments[0], segments[1])
		if err == nil {
			repo = *info
		}
	}

	var files []FileStatus
	for _, link := range links {
		relPath, _ := filepath.Rel(s.Target, link)
		target, _ := os.Readlink(link)
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(link), target)
		}

		fs := FileStatus{Path: relPath, OK: true}
		if _, err := os.Stat(target); err != nil {
			fs.OK = false
			fs.Reason = "target missing"
		}
		files = append(files, fs)
	}

	stashed := s.StashedFiles()

	return &LinkStatus{
		Repo:    repo,
		Files:   files,
		Stashed: stashed,
	}, nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/shimmer/ -run TestStatus -v`
Expected: all pass.

**Step 5: Wire up Cobra command**

Create `internal/cmd/status.go`:

```go
package cmd

import (
	"fmt"

	"github.com/siimpl/shimmer/internal/shimmer"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show symlink health for the current scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags(nil)
			if err != nil {
				return renderError(err)
			}
			status, err := s.Status()
			if err != nil {
				return renderError(err)
			}
			renderStatus(status)
			return nil
		},
	}
}

func renderStatus(st *shimmer.LinkStatus) {
	broken := 0
	for _, f := range st.Files {
		if !f.OK {
			broken++
		}
	}

	header := fmt.Sprintf("linked (%d files", len(st.Files))
	if broken > 0 {
		header += fmt.Sprintf(", %d broken", broken)
	}
	header += ")"
	fmt.Println(header)

	if st.Repo.Owner != "" {
		fmt.Printf("  repo: %s/%s @ %s\n", st.Repo.Owner, st.Repo.Name, st.Repo.Branch)
	}

	for _, f := range st.Files {
		if f.OK {
			fmt.Printf("  ok:      %s\n", f.Path)
		} else {
			fmt.Printf("  BROKEN:  %s (%s — run `shimmer link` to reconcile)\n", f.Path, f.Reason)
		}
	}

	for _, s := range st.Stashed {
		fmt.Printf("  stashed: %s (original in .git/shimmer-stash/)\n", s)
	}
}
```

**Step 6: Register in root command**

Add `cmd.AddCommand(newStatusCmd())` in `NewRootCmd()`.

**Step 7: Commit**

```bash
git add internal/shimmer/status.go internal/shimmer/status_test.go internal/cmd/status.go internal/cmd/root.go
git commit -m "add status command — symlink health reporting"
```

---

### Task 11: Git Passthrough

**Files:**
- Create: `internal/shimmer/git.go`
- Create: `internal/cmd/git.go`

Simple passthrough — runs `git -C <clone-path> <args...>`.

**Step 1: Implement git passthrough in core**

Create `internal/shimmer/git.go`:

```go
package shimmer

import (
	"os"
	"os/exec"
)

// Git runs a git command against the overlay clone, with stdin/stdout/stderr
// connected to the parent process.
func (s *Shimmer) Git(args []string) error {
	clonePath, err := s.findClone()
	if err != nil {
		return err
	}

	fullArgs := append([]string{"-C", clonePath}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

**Step 2: Wire up Cobra command**

Create `internal/cmd/git.go`:

```go
package cmd

import (
	"github.com/spf13/cobra"
)

func newGitCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "git [args...]",
		Short:              "Run git commands against the overlay repo clone",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags(nil)
			if err != nil {
				return renderError(err)
			}
			return s.Git(args)
		},
	}
}
```

**Step 3: Register in root command**

Add `cmd.AddCommand(newGitCmd())` in `NewRootCmd()`.

**Step 4: Build and verify**

Run: `go build ./cmd/shimmer && ./shimmer git --help`

**Step 5: Commit**

```bash
git add internal/shimmer/git.go internal/cmd/git.go internal/cmd/root.go
git commit -m "add git passthrough command"
```

---

### Task 12: Global Scope Integration Tests

**Files:**
- Create: `internal/shimmer/global_test.go`

Test the full workflow with `-g` (global scope targeting `$HOME` equivalent).

**Step 1: Write tests**

Create `internal/shimmer/global_test.go`:

```go
package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestGlobalLinkUnlink(t *testing.T) {
	// Use a temp dir as $HOME to avoid touching real home
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

	// Symlink should be gone
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

	// Create existing file in fake home
	writeFile(t, fakeHome, "config.txt", "original")

	s := &shimmer.Shimmer{
		Home:   shimmerHome,
		Global: true,
		Target: fakeHome,
	}

	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Link with overwrite
	result, err := s.Link(false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stashed) != 1 {
		t.Fatal("expected 1 stashed")
	}

	// Stash should be in ~/.shimmer/stash/
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
```

**Step 2: Run tests**

Run: `go test ./internal/shimmer/ -run TestGlobal -v`
Expected: all pass.

**Step 3: Commit**

```bash
git add internal/shimmer/global_test.go
git commit -m "add global scope integration tests"
```

---

### Task 13: Full Workflow Integration Test

**Files:**
- Create: `internal/shimmer/integration_test.go`

End-to-end test of the complete happy path: repo set -> link -> status -> git -> unlink.

**Step 1: Write test**

Create `internal/shimmer/integration_test.go`:

```go
package shimmer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestFullWorkflow(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md":              "# Config v1",
		".claude/settings.json":  `{"key": "value"}`,
		".claude/skills/review.md": "# Review Skill",
		".cursorrules":           "rules",
		"README.md":              "overlay readme",
		".shimmerignore":         "README.md\n",
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
	// README.md should be ignored, 4 files linked
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
	if err := s.Unlink(); err != nil {
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
	if containsSubstring(string(exclude), "shimmer managed") {
		t.Error("exclude file still has shimmer entries")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}
```

**Step 2: Run tests**

Run: `go test ./internal/shimmer/ -run TestFullWorkflow -v`
Expected: pass.

**Step 3: Run all tests**

Run: `go test ./internal/shimmer/ -v`
Expected: all pass.

**Step 4: Commit**

```bash
git add internal/shimmer/integration_test.go
git commit -m "add full workflow integration test"
```

---

### Task 14: Polish — .gitignore, Build Target, Manual Testing Doc

**Files:**
- Create: `.gitignore`
- Create: `Makefile`
- Create: `docs/manual-testing.md`

**Step 1: Create .gitignore**

```
shimmer
*.exe
```

**Step 2: Create Makefile**

```makefile
.PHONY: build test clean install

build:
	go build -o shimmer ./cmd/shimmer

test:
	go test ./... -v

clean:
	rm -f shimmer

install:
	go install ./cmd/shimmer
```

**Step 3: Create manual testing doc**

Create `docs/manual-testing.md`:

```markdown
# shimmer — Manual Testing Guide

Scenarios to walk through by hand for UAT and "does it feel right" validation.

## Prerequisites

- `shimmer` binary built and on PATH (`make install`)
- A git repository to use as a test overlay (or create one on GitHub)
- A git project to overlay onto

## Scenario 1: Basic Link and Unlink

1. Create a test overlay repo on GitHub with a `CLAUDE.md` file
2. Navigate to a project: `cd ~/projects/some-project`
3. Set the overlay: `shimmer repo set <url>`
4. Link: `shimmer link`
5. Verify: `ls -la CLAUDE.md` — should be a symlink
6. Check status: `shimmer status`
7. Unlink: `shimmer unlink`
8. Verify: `ls -la CLAUDE.md` — should not exist

## Scenario 2: Conflict Handling

1. Create a `CLAUDE.md` in the project (not tracked by git)
2. Run `shimmer link` — should fail with conflict error
3. Run `shimmer link --skip` — should link everything except CLAUDE.md
4. Run `shimmer unlink`
5. Run `shimmer link --overwrite` — should stash and shadow
6. Verify stash: check `.git/shimmer-stash/CLAUDE.md`
7. Run `shimmer unlink` — original should be restored

## Scenario 3: Branch Switching

1. `shimmer link`
2. `shimmer git branch -a` — see available branches
3. `shimmer git checkout <other-branch>`
4. `shimmer link` — should reconcile (remove old links, create new ones)
5. `shimmer status` — verify health

## Scenario 4: Global Scope

1. `shimmer -g repo set <url>`
2. `shimmer -g link`
3. `ls -la ~/.claude/` — should contain symlinks
4. `shimmer -g status`
5. `shimmer -g unlink`

## Scenario 5: Repo Management

1. `shimmer repo set <url>`
2. `shimmer repo path` — should print the clone path
3. `shimmer repo list` — should show the repo
4. `shimmer repo remove` — should prompt and remove
5. `shimmer repo list` — should be empty

## Scenario 6: Git Passthrough

1. `shimmer repo set <url>`
2. `shimmer git status`
3. `shimmer git log --oneline -5`
4. `shimmer git pull`

## What to Look For

- Error messages are clear and actionable
- Help text is useful (`shimmer --help`, `shimmer link --help`)
- Tab completion works (if installed)
- No leftover files after unlink
- Re-linking is truly idempotent
- Stash/restore preserves file content exactly
```

**Step 4: Build and run all tests**

Run:
```bash
make test
make build
./shimmer --help
```

**Step 5: Commit**

```bash
git add .gitignore Makefile docs/manual-testing.md
git commit -m "add gitignore, makefile, and manual testing guide"
```

---

## Task Summary

| Task | What | Tests |
|------|------|-------|
| 1 | Project scaffolding (go mod, cobra root, -g flag) | build + help |
| 2 | Core types, error types, test helpers | compiles |
| 3 | Path utilities (git root, URL parsing, clone path) | 3 test functions |
| 4 | .shimmerignore parsing | 2 test functions |
| 5 | Repo set/path/list/remove + cobra wiring | 5 test functions |
| 6 | Overlay file walking | 1 test function |
| 7 | Symlink scanning | 2 test functions |
| 8 | Link (the big one) + cobra wiring | 5 test functions |
| 9 | Unlink + cobra wiring | 3 test functions |
| 10 | Status + cobra wiring | 3 test functions |
| 11 | Git passthrough + cobra wiring | build + help |
| 12 | Global scope integration tests | 2 test functions |
| 13 | Full workflow integration test | 1 test function |
| 14 | Polish (.gitignore, Makefile, manual testing doc) | make test |
