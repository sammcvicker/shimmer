package shimmer

import (
	"os"
	"path/filepath"
	"strings"
)

// Status reports the health of all shimmer symlinks for the current scope.
// It returns ErrNotLinked if no shimmer symlinks exist.
func (s *Shimmer) Status() (*LinkStatus, error) {
	// 1. Find shimmer symlinks.
	// Uses clone-based targeted check when possible (fast for global scope).
	links, err := s.findShimmerLinks()
	if err != nil {
		return nil, err
	}

	// 2. If none found, return ErrNotLinked.
	if len(links) == 0 {
		return nil, &ErrNotLinked{}
	}

	// 3. Derive RepoInfo from the first symlink's target path.
	repo, err := s.repoInfoFromSymlink(links[0])
	if err != nil {
		return nil, err
	}

	// 4. Check each symlink's health.
	var files []FileStatus
	for _, link := range links {
		rel, _ := filepath.Rel(s.Scope.Target(), link)
		target, err := os.Readlink(link)
		if err != nil {
			files = append(files, FileStatus{Path: rel, OK: false, Reason: "unreadable"})
			continue
		}
		target = absSymlinkTarget(link, target)
		if _, err := os.Stat(target); err != nil {
			files = append(files, FileStatus{Path: rel, OK: false, Reason: "target missing"})
		} else {
			files = append(files, FileStatus{Path: rel, OK: true})
		}
	}

	// 5. Get stashed files.
	stashed, err := s.StashedFiles()
	if err != nil {
		return nil, err
	}

	return &LinkStatus{
		Repo:    *repo,
		Files:   files,
		Stashed: stashed,
	}, nil
}

// repoInfoFromSymlink derives RepoInfo by walking up from a symlink's target
// to find the clone root (directory containing .git/).
func (s *Shimmer) repoInfoFromSymlink(linkPath string) (*RepoInfo, error) {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}

	// Walk up from the target to find the .git directory marking the clone root.
	cloneRoot := target
	for {
		if _, err := os.Stat(filepath.Join(cloneRoot, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(cloneRoot)
		if parent == cloneRoot {
			// Reached filesystem root without finding .git
			return nil, &ErrNoRepo{ScopeLabel: s.Scope.ScopeLabel()}
		}
		cloneRoot = parent
	}

	// Extract owner/name from the path under ~/.shimmer/repos/<owner>/<name>/...
	reposDir := filepath.Join(s.Home, "repos")
	rel, err := filepath.Rel(reposDir, cloneRoot)
	if err != nil {
		return nil, err
	}
	segments := strings.SplitN(rel, string(os.PathSeparator), 3)
	if len(segments) < 2 {
		return nil, &ErrNoRepo{ScopeLabel: s.Scope.ScopeLabel()}
	}

	owner, name := segments[0], segments[1]
	return s.repoInfo(cloneRoot, owner, name)
}
