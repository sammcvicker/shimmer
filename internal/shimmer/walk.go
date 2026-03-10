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
