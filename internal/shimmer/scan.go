package shimmer

import (
	"os"
	"path/filepath"
)

// ScanSymlinks finds all symlinks under targetDir that point into shimmerHome/repos/.
// This walks the entire target directory tree — fine for project directories,
// but use CheckSymlinks for large targets like $HOME.
func ScanSymlinks(targetDir, shimmerHome string) ([]string, error) {
	reposDir := filepath.Join(shimmerHome, reposDir)
	var links []string

	err := filepath.WalkDir(targetDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Surface errors for the root target itself; skip unreadable children.
			if path == targetDir {
				return err
			}
			return nil
		}

		// Skip .git directory
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				// Skip symlinks we can't read (e.g. permission denied, race with deletion).
				return nil
			}
			target = absSymlinkTarget(path, target)
			if isSubpath(target, reposDir) {
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
	reposDir := filepath.Join(shimmerHome, reposDir)
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
		if isSubpath(target, reposDir) {
			links = append(links, path)
		}
	}
	return links, nil
}
