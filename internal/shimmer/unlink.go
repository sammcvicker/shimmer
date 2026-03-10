package shimmer

import (
	"fmt"
	"os"
	"path/filepath"
)

// Unlink removes all shimmer symlinks, restores stashed files, clears
// skip-worktree flags, and cleans up .git/info/exclude entries.
// It returns the number of symlinks removed, or ErrNotLinked if nothing is linked.
func (s *Shimmer) Unlink() (int, error) {
	// 1. Find all shimmer symlinks.
	// Uses clone-based targeted check when possible (fast for global scope).
	links, err := s.findShimmerLinks()
	if err != nil {
		return 0, fmt.Errorf("scanning symlinks: %w", err)
	}

	if len(links) == 0 {
		return 0, &ErrNotLinked{}
	}

	// 2. For each symlink: remove it, clear skip-worktree if local, clean empty parents.
	for _, link := range links {
		rel, _ := filepath.Rel(s.Target, link)

		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("removing symlink %s: %w", link, err)
		}

		if !s.Global {
			// Best-effort: clear skip-worktree (may fail if file was never tracked).
			if err := s.setSkipWorktree(rel, false); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not clear skip-worktree for %s: %v\n", rel, err)
			}
		}

		s.cleanEmptyLinkParents(filepath.Dir(link))
	}

	// 3. Restore stashed files.
	if err := s.restoreStash(); err != nil {
		return 0, fmt.Errorf("restoring stash: %w", err)
	}

	// 4. Clear link state: .git/info/exclude (local) or ~/.shimmer/linked (global).
	if s.Global {
		if err := s.writeGlobalLinkedPaths(nil); err != nil {
			return 0, fmt.Errorf("clearing global linked paths: %w", err)
		}
	} else {
		if err := s.updateGitExclude(nil); err != nil {
			return 0, fmt.Errorf("clearing .git/info/exclude: %w", err)
		}
	}

	return len(links), nil
}

// stashDir returns the base directory for stashed files.
// Local scope: .git/shimmer-stash
// Global scope: ~/.shimmer/stash
func (s *Shimmer) stashDir() string {
	if s.Global {
		return filepath.Join(s.Home, "stash")
	}
	return filepath.Join(s.Target, ".git", "shimmer-stash")
}

// restoreStash walks the stash directory and moves each file back to its
// original location, then removes the stash directory.
func (s *Shimmer) restoreStash() error {
	dir := s.stashDir()

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil // no stash to restore
	}

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		dest := filepath.Join(s.Target, rel)

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating parent dirs for %s: %w", rel, err)
		}

		// Remove any existing file/symlink at dest (the symlink we just removed
		// should already be gone, but be safe).
		_ = os.Remove(dest)

		if err := os.Rename(path, dest); err != nil {
			return fmt.Errorf("restoring %s: %w", rel, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Remove the stash directory tree.
	return os.RemoveAll(dir)
}

// StashedFiles returns a list of relative paths of files currently in the stash.
func (s *Shimmer) StashedFiles() ([]string, error) {
	dir := s.stashDir()

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var files []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}
