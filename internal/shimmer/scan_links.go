package shimmer

import (
	"os"
	"path/filepath"
	"strings"
)

// findShimmerLinks finds all shimmer symlinks in the target directory.
//
// For local scope, walks the project directory (always fast and correct).
//
// For global scope, avoids walking all of $HOME by using a targeted check:
// combines the current overlay file list (from clone) with previously linked
// paths (from ~/.shimmer/linked) to know exactly which paths to check.
func (s *Shimmer) findShimmerLinks() ([]string, error) {
	if !s.Global {
		return ScanSymlinks(s.Target, s.Home)
	}

	// Global scope: build the set of paths to check.
	pathSet := make(map[string]bool)

	// Add current overlay files (if clone exists).
	if clonePath, err := s.findClone(); err == nil {
		if overlayFiles, err := WalkOverlay(clonePath); err == nil {
			for _, f := range overlayFiles {
				pathSet[f] = true
			}
		}
	}

	// Add previously linked paths (catches stale links from deleted files).
	for _, p := range s.readGlobalLinkedPaths() {
		pathSet[p] = true
	}

	if len(pathSet) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}
	return CheckSymlinks(s.Target, paths, s.Home)
}

// globalLinkedPathsFile returns the path to the state file that records
// which paths are currently linked for global scope.
// This is the global equivalent of .git/info/exclude for local scope.
func (s *Shimmer) globalLinkedPathsFile() string {
	return filepath.Join(s.Home, "linked")
}

// writeGlobalLinkedPaths saves the currently linked paths for global scope.
func (s *Shimmer) writeGlobalLinkedPaths(paths []string) error {
	if !s.Global {
		return nil
	}
	if len(paths) == 0 {
		if err := os.Remove(s.globalLinkedPathsFile()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(s.globalLinkedPathsFile(), []byte(strings.Join(paths, "\n")+"\n"), 0o644)
}

// readGlobalLinkedPaths reads previously linked paths for global scope.
func (s *Shimmer) readGlobalLinkedPaths() []string {
	data, err := os.ReadFile(s.globalLinkedPathsFile())
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}
