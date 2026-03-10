package shimmer

import (
	"os"
	"path/filepath"
	"strings"
)

// ScanSymlinks finds all symlinks under targetDir that point into shimmerHome/repos/.
// This walks the entire target directory tree — fine for project directories,
// but use CheckSymlinks for large targets like $HOME.
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
			target = absSymlinkTarget(path, target)
			if strings.HasPrefix(target, reposDir) {
				links = append(links, path)
			}
		}
		return nil
	})
	return links, err
}

// CheckSymlinks checks specific relative paths in targetDir for symlinks
// pointing into shimmerHome/repos/. Much faster than ScanSymlinks when the
// target directory is large (e.g. $HOME) and the set of possible paths is known.
func CheckSymlinks(targetDir string, relPaths []string, shimmerHome string) ([]string, error) {
	reposDir := filepath.Join(shimmerHome, "repos")
	var links []string

	for _, rel := range relPaths {
		path := filepath.Join(targetDir, rel)
		fi, err := os.Lstat(path)
		if err != nil {
			continue // doesn't exist
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			continue // not a symlink
		}
		target, err := os.Readlink(path)
		if err != nil {
			continue
		}
		target = absSymlinkTarget(path, target)
		if strings.HasPrefix(target, reposDir) {
			links = append(links, path)
		}
	}
	return links, nil
}
