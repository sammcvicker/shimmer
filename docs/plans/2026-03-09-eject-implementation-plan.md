# Eject Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `shimmer eject` command that replaces symlinks with file copies, clears the stash and exclude entries, while keeping the overlay repo intact.

**Architecture:** New `Eject()` method on `Shimmer` struct, reusing `findShimmerLinks()` for discovery and existing cleanup helpers. New `eject.go` in `internal/shimmer/`, new `eject.go` in `internal/cmd/`, wired into the root command. Tests follow existing patterns in `unlink_test.go`.

**Tech Stack:** Go, Cobra CLI, `os` package for symlink/file operations.

---

### Task 1: Core eject logic — failing test for basic eject

**Files:**
- Create: `internal/shimmer/eject_test.go`

**Step 1: Write the failing test**

```go
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

	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Claude Config" {
		t.Errorf("content = %q, want %q", content, "# Claude Config")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/shimmer/ -run TestEject -v`
Expected: FAIL — `s.Eject` does not exist.

---

### Task 2: Core eject logic — minimal implementation

**Files:**
- Create: `internal/shimmer/eject.go`

**Step 3: Write minimal implementation**

```go
package shimmer

import (
	"fmt"
	"io"
	"os"
)

// EjectResult is what eject returns after executing.
type EjectResult struct {
	Ejected      []string
	StashCleared bool
}

// Eject replaces all shimmer symlinks with copies of the files they point to.
// It clears the stash and exclude/linked-paths entries.
// The overlay repo is left intact.
func (s *Shimmer) Eject() (*EjectResult, error) {
	// 1. Find all shimmer symlinks.
	links, err := s.findShimmerLinks()
	if err != nil {
		return nil, fmt.Errorf("scanning symlinks: %w", err)
	}

	if len(links) == 0 {
		return &EjectResult{}, nil
	}

	// 2. Replace each symlink with a copy of its target.
	result := &EjectResult{}
	for _, link := range links {
		target, err := os.Readlink(link)
		if err != nil {
			return nil, fmt.Errorf("reading symlink %s: %w", link, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(link), target)
		}

		// Verify target exists.
		if _, err := os.Stat(target); err != nil {
			rel, _ := filepath.Rel(s.Target, link)
			return nil, fmt.Errorf("broken symlink %s: target %s does not exist — fix with shimmer status", rel, target)
		}

		// Remove symlink.
		if err := os.Remove(link); err != nil {
			return nil, fmt.Errorf("removing symlink %s: %w", link, err)
		}

		// Copy file contents.
		if err := copyFile(target, link); err != nil {
			return nil, fmt.Errorf("copying %s: %w", link, err)
		}

		rel, _ := filepath.Rel(s.Target, link)
		result.Ejected = append(result.Ejected, rel)
	}

	// 3. Delete the stash.
	stash := s.stashDir()
	if info, err := os.Stat(stash); err == nil && info.IsDir() {
		if err := os.RemoveAll(stash); err != nil {
			return nil, fmt.Errorf("clearing stash: %w", err)
		}
		result.StashCleared = true
	}

	// 4. Clear exclude/linked-paths entries.
	if s.Global {
		s.writeGlobalLinkedPaths(nil)
	} else {
		if err := s.updateGitExclude(nil); err != nil {
			return nil, fmt.Errorf("clearing .git/info/exclude: %w", err)
		}
	}

	return result, nil
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
```

Add the missing import to `eject.go`:

```go
import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/shimmer/ -run TestEject -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/shimmer/eject.go internal/shimmer/eject_test.go
git commit -m "feat: add Eject() core logic with basic test"
```

---

### Task 3: Add EjectResult to shimmer.go

**Files:**
- Modify: `internal/shimmer/shimmer.go`

Note: `EjectResult` is defined in `eject.go` (Task 2). No changes to `shimmer.go` needed — the type lives alongside the method. This task is a no-op; proceed to Task 4.

---

### Task 4: Test eject clears stash

**Files:**
- Modify: `internal/shimmer/eject_test.go`

**Step 1: Write the failing test**

Append to `eject_test.go`:

```go
func TestEjectClearsStash(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	// Create a conflicting file, link with overwrite to create a stash.
	writeFile(t, project, "CLAUDE.md", "original content")
	if _, err := s.Link(false, true); err != nil {
		t.Fatal(err)
	}

	// Verify stash exists.
	stashDir := filepath.Join(project, ".git", "shimmer-stash")
	if _, err := os.Stat(stashDir); os.IsNotExist(err) {
		t.Fatal("expected stash to exist after overwrite link")
	}

	// Eject.
	result, err := s.Eject()
	if err != nil {
		t.Fatalf("Eject() error: %v", err)
	}

	if !result.StashCleared {
		t.Error("expected StashCleared to be true")
	}

	// Stash directory should be gone.
	if _, err := os.Stat(stashDir); !os.IsNotExist(err) {
		t.Error("expected stash directory to be removed after eject")
	}

	// Ejected file should have overlay content, not the stashed original.
	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Claude Config" {
		t.Errorf("content = %q, want overlay content %q", content, "# Claude Config")
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/shimmer/ -run TestEjectClearsStash -v`
Expected: PASS (implementation from Task 2 already handles this)

**Step 3: Commit**

```bash
git add internal/shimmer/eject_test.go
git commit -m "test: eject clears stash after overwrite link"
```

---

### Task 5: Test eject clears git exclude entries

**Files:**
- Modify: `internal/shimmer/eject_test.go`

**Step 1: Write the failing test**

Append to `eject_test.go`:

```go
func TestEjectClearsExclude(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Verify exclude has shimmer block.
	excludePath := filepath.Join(project, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "# shimmer managed") {
		t.Fatal("expected shimmer block in exclude before eject")
	}

	// Eject.
	if _, err := s.Eject(); err != nil {
		t.Fatalf("Eject() error: %v", err)
	}

	// Exclude should no longer have shimmer block.
	content, err = os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "# shimmer managed") {
		t.Error("expected shimmer block to be cleared from exclude after eject")
	}
}
```

Add `"strings"` to the imports in `eject_test.go`.

**Step 2: Run test to verify it passes**

Run: `go test ./internal/shimmer/ -run TestEjectClearsExclude -v`
Expected: PASS (implementation from Task 2 already handles this)

**Step 3: Commit**

```bash
git add internal/shimmer/eject_test.go
git commit -m "test: eject clears git exclude entries"
```

---

### Task 6: Test eject with nothing linked

**Files:**
- Modify: `internal/shimmer/eject_test.go`

**Step 1: Write the test**

Append to `eject_test.go`:

```go
func TestEjectNothingLinked(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)

	s := newTestShimmer(t, home, project, false)

	result, err := s.Eject()
	if err != nil {
		t.Fatalf("Eject() on unlinked project should succeed, got error: %v", err)
	}

	if len(result.Ejected) != 0 {
		t.Errorf("expected 0 ejected, got %d", len(result.Ejected))
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/shimmer/ -run TestEjectNothingLinked -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/shimmer/eject_test.go
git commit -m "test: eject no-op when nothing is linked"
```

---

### Task 7: Test eject with broken symlink

**Files:**
- Modify: `internal/shimmer/eject_test.go`

**Step 1: Write the test**

Append to `eject_test.go`:

```go
func TestEjectBrokenSymlink(t *testing.T) {
	project := setupTestProject(t)
	home := setupShimmerHome(t)
	overlayURL := setupTestOverlay(t, map[string]string{
		"CLAUDE.md": "# Claude Config",
	})

	s := newTestShimmer(t, home, project, false)
	if _, err := s.RepoSet(overlayURL); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Link(false, false); err != nil {
		t.Fatal(err)
	}

	// Break the symlink by deleting the target file in the clone.
	clonePath, _ := s.RepoPath()
	os.Remove(filepath.Join(clonePath, "CLAUDE.md"))

	// Eject should fail.
	_, err := s.Eject()
	if err == nil {
		t.Fatal("expected error on broken symlink, got nil")
	}

	if !strings.Contains(err.Error(), "broken symlink") {
		t.Errorf("expected 'broken symlink' in error, got: %v", err)
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/shimmer/ -run TestEjectBrokenSymlink -v`
Expected: PASS

Note: this test uses `s.RepoPath()`. Verify this method exists — it's in `internal/shimmer/repo.go`. If the method signature differs, adjust accordingly.

**Step 3: Commit**

```bash
git add internal/shimmer/eject_test.go
git commit -m "test: eject fails on broken symlink"
```

---

### Task 8: CLI command

**Files:**
- Create: `internal/cmd/eject.go`
- Modify: `internal/cmd/root.go`

**Step 1: Create the CLI command**

`internal/cmd/eject.go`:

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newEjectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "eject",
		Short: "Replace symlinks with file copies, keeping the overlay repo",
		Long: `Eject materializes all shimmer symlinks into regular files. The overlay
repo stays intact for future use. Stashed files are discarded.

After eject, the files are real and visible to git status.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			result, err := s.Eject()
			if err != nil {
				return renderError(err)
			}

			out := cmd.OutOrStdout()

			if len(result.Ejected) == 0 {
				fmt.Fprintln(out, "Nothing to eject.")
				return nil
			}

			fmt.Fprintf(out, "Ejected (%d):\n", len(result.Ejected))
			for _, f := range result.Ejected {
				fmt.Fprintf(out, "  %s\n", f)
			}

			if result.StashCleared {
				fmt.Fprintln(out, "Stash cleared.")
			}

			return nil
		},
	}
}
```

**Step 2: Wire into root command**

In `internal/cmd/root.go`, add the eject command registration:

```go
cmd.AddCommand(newEjectCmd())
```

Add it after the existing `cmd.AddCommand(newGitCmd())` line.

**Step 3: Verify it builds**

Run: `go build ./cmd/shimmer`
Expected: builds without errors.

**Step 4: Commit**

```bash
git add internal/cmd/eject.go internal/cmd/root.go
git commit -m "feat: add shimmer eject CLI command"
```

---

### Task 9: Run all tests

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: all tests pass, including new eject tests.

**Step 2: Manual smoke test**

Run: `go build -o shimmer ./cmd/shimmer && ./shimmer eject --help`
Expected: shows eject help text.

**Step 3: Commit (if any fixes were needed)**

---

### Task 10: Update README

**Files:**
- Modify: `README.md`

**Step 1: Add eject to the commands section**

In the commands block, add after the `shimmer unlink` line:

```
shimmer eject                      Replace symlinks with file copies (keeps repo)
```

**Step 2: Add an eject workflow example**

Add a new section after "Conflict handling" and before ".shimmerignore":

```markdown
### Ejecting

Use `shimmer eject` to materialize symlinks into real files you can commit. The overlay repo stays intact for future updates.

\```bash
# Pull upstream changes, link, and eject into the project
shimmer git pull
shimmer link --overwrite
shimmer eject
git status  # ejected files show up as changes
\```
```

(Remove the backslashes before the triple backticks — they are escaping for this plan document only.)

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add eject to README"
```
