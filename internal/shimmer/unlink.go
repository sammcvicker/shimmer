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
		// Check for leftover stash from a partial failure.
		stash := s.Scope.StashDir()
		if info, err := os.Stat(stash); err != nil || !info.IsDir() {
			return 0, &ErrNotLinked{}
		}
		// Stash exists but no links — restore it.
		if err := s.restoreStash(); err != nil {
			return 0, fmt.Errorf("restoring stash: %w", err)
		}
		return 0, nil
	}

	target := s.Scope.Target()

	// Collect all rels to batch-check tracked status.
	rels := make([]string, 0, len(links))
	for _, link := range links {
		rel, _ := filepath.Rel(target, link)
		rels = append(rels, rel)
	}
	var tracked map[string]bool
	if ga, ok := s.Scope.(GitAware); ok {
		tracked = ga.TrackedFiles(rels)
	}

	// 2. For each symlink: remove it, clear skip-worktree, clean empty parents.
	for _, link := range links {
		rel, _ := filepath.Rel(target, link)

		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("removing symlink %s: %w", link, err)
		}

		// Only clear skip-worktree for files that are actually tracked.
		if tracked[rel] {
			if ga, ok := s.Scope.(GitAware); ok {
				if err := ga.SetSkipWorktree(rel, false); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not clear skip-worktree for %s: %v\n", rel, err)
				}
			}
		}

		cleanEmptyParents(filepath.Dir(link), s.Scope.Target())
	}

	// 3. Restore stashed files.
	if err := s.restoreStash(); err != nil {
		return 0, fmt.Errorf("restoring stash: %w", err)
	}

	// 4. Clear link state.
	if err := s.Scope.SaveLinkState(nil); err != nil {
		return 0, fmt.Errorf("clearing link state: %w", err)
	}

	return len(links), nil
}

// restoreStash walks the stash directory and moves each file back to its
// original location, then removes the stash directory.
func (s *Shimmer) restoreStash() error {
	dir := s.Scope.StashDir()

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil // no stash to restore
	}

	target := s.Scope.Target()

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

		dest := filepath.Join(target, rel)

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("creating parent dirs for %s: %w", rel, err)
		}

		// Try to move the stashed file into place. os.Rename atomically
		// replaces regular files and symlinks on POSIX, so this handles
		// the common case (dest is a leftover shimmer symlink) safely.
		if err := os.Rename(path, dest); err != nil {
			// If the rename failed and something still exists at dest,
			// move it aside to a temp file, retry, and only delete the
			// temp on success. This avoids losing the original if the
			// second rename also fails.
			tmp := dest + ".shimmer-tmp"
			if renameErr := os.Rename(dest, tmp); renameErr != nil {
				// dest either doesn't exist or can't be moved;
				// return the original error as-is.
				return fmt.Errorf("restoring %s: %w", rel, err)
			}
			if err := os.Rename(path, dest); err != nil {
				// Restore the original from temp before failing.
				_ = os.Rename(tmp, dest)
				return fmt.Errorf("restoring %s: %w", rel, err)
			}
			_ = os.Remove(tmp)
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
	dir := s.Scope.StashDir()

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
