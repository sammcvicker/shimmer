package shimmer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		return nil, &ErrNotLinked{}
	}

	target := s.Scope.Target()

	// 2. Pre-flight: validate all symlink targets exist before mutating anything.
	targets := make(map[string]string, len(links)) // link path -> resolved target
	for _, link := range links {
		linkTarget, err := os.Readlink(link)
		if err != nil {
			return nil, fmt.Errorf("reading symlink %s: %w", link, err)
		}
		linkTarget = absSymlinkTarget(link, linkTarget)
		if _, err := os.Stat(linkTarget); err != nil {
			rel, _ := filepath.Rel(target, link)
			return nil, fmt.Errorf("broken symlink %s: target %s does not exist — fix with shimmer link", rel, linkTarget)
		}
		targets[link] = linkTarget
	}

	// 3. Replace each symlink with a copy of its target (all targets verified).
	result := &EjectResult{}
	for _, link := range links {
		if err := os.Remove(link); err != nil {
			return nil, fmt.Errorf("removing symlink %s: %w", link, err)
		}
		if err := copyFile(targets[link], link); err != nil {
			return nil, fmt.Errorf("copying %s: %w", link, err)
		}
		rel, _ := filepath.Rel(target, link)

		// Clear skip-worktree so git sees the real file again.
		// For local scope, failure is non-fatal: the file has already been
		// ejected, and the user can manually run
		// "git update-index --no-skip-worktree" if needed.
		if ga, ok := s.Scope.(GitAware); ok {
			if err := ga.SetSkipWorktree(rel, false); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not clear skip-worktree for %s: %v\n", rel, err)
			}
		}

		result.Ejected = append(result.Ejected, rel)
	}

	// 4. Delete the stash.
	stash := s.Scope.StashDir()
	if info, err := os.Stat(stash); err == nil && info.IsDir() {
		if err := os.RemoveAll(stash); err != nil {
			return nil, fmt.Errorf("clearing stash: %w", err)
		}
		result.StashCleared = true
	}

	// 5. Clear link state.
	if err := s.Scope.SaveLinkState(nil); err != nil {
		return nil, fmt.Errorf("clearing link state: %w", err)
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

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
