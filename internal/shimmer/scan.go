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
