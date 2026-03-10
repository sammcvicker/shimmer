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
	if !s.Scope.IsGlobal() {
		return ScanSymlinks(s.Scope.Target(), s.Home)
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
	for _, p := range readLinkedPaths(s.Home) {
		pathSet[p] = true
	}

	if len(pathSet) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}
	return CheckSymlinks(s.Scope.Target(), paths, s.Home)
}

// readLinkedPaths reads previously linked paths from the global linked state file.
func readLinkedPaths(home string) []string {
	data, err := os.ReadFile(filepath.Join(home, "linked"))
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
