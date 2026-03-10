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
		if err := s.writeGlobalLinkedPaths(nil); err != nil {
			return nil, fmt.Errorf("clearing global linked paths: %w", err)
		}
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

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
