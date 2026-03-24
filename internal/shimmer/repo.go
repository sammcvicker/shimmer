package shimmer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoSet clones the overlay repo into ~/.shimmer/repos/.
func (s *Shimmer) RepoSet(url string) (*RepoInfo, error) {
	owner, name, err := ParseRepoURL(url)
	if err != nil {
		return nil, err
	}

	clonePath := s.Scope.ClonePath(s.Home, owner, name)

	// Check if already set
	if _, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil {
		remote, _ := gitOutput(clonePath, "remote", "get-url", "origin")
		return nil, &ErrRepoAlreadySet{
			RemoteURL: strings.TrimSpace(remote),
			ClonePath: clonePath,
		}
	}

	if err := os.MkdirAll(filepath.Dir(clonePath), 0o755); err != nil {
		return nil, fmt.Errorf("creating directories: %w", err)
	}

	cmd := exec.Command("git", "clone", url, clonePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %s\n%s", err, out)
	}

	return s.repoInfo(clonePath, owner, name)
}

// RepoPath returns the absolute path to the clone for the current scope.
func (s *Shimmer) RepoPath() (string, error) {
	return s.findClone()
}

// RepoRemove deletes the clone for the current scope.
func (s *Shimmer) RepoRemove() error {
	clone, err := s.findClone()
	if err != nil {
		return err
	}

	// Unlink before removing to clean up symlinks.
	// ErrNotLinked is expected if the repo was never linked; all other errors are real.
	if _, err := s.Unlink(); err != nil {
		if _, ok := err.(*ErrNotLinked); !ok {
			return fmt.Errorf("unlinking before removal: %w", err)
		}
	}
	if err := os.RemoveAll(clone); err != nil {
		return fmt.Errorf("removing clone: %w", err)
	}

	cleanEmptyParents(filepath.Dir(clone), s.ReposPath())
	return nil
}

// RepoList walks ~/.shimmer/repos/ and returns info about all clones.
func (s *Shimmer) RepoList() ([]RepoInfo, error) {
	rp := s.ReposPath()
	if _, err := os.Stat(rp); err != nil {
		return nil, nil
	}

	var repos []RepoInfo
	err := filepath.WalkDir(rp, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" && d.IsDir() {
			cloneDir := filepath.Dir(path)
			rel, _ := filepath.Rel(rp, cloneDir)
			segments := strings.SplitN(rel, string(os.PathSeparator), 3)
			if len(segments) < 3 {
				return nil
			}
			owner, name := segments[0], segments[1]
			info, err := s.repoInfo(cloneDir, owner, name)
			if err != nil {
				return nil
			}
			repos = append(repos, *info)
			return filepath.SkipDir
		}
		return nil
	})
	return repos, err
}

// findClone locates the clone directory for the current scope.
func (s *Shimmer) findClone() (string, error) {
	rp := s.ReposPath()
	if _, err := os.Stat(rp); err != nil {
		return "", &ErrNoRepo{ScopeLabel: s.Scope.ScopeLabel()}
	}

	var matches []string
	var walkErr error
	_ = filepath.WalkDir(rp, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			walkErr = err
			return filepath.SkipAll
		}
		if d.Name() == ".git" && d.IsDir() {
			cloneDir := filepath.Dir(path)
			rel, _ := filepath.Rel(rp, cloneDir)
			segments := strings.SplitN(rel, string(os.PathSeparator), 3)
			if len(segments) < 3 {
				return nil
			}
			targetSegment := segments[2]

			if s.Scope.MatchClone(targetSegment) {
				matches = append(matches, cloneDir)
			}
			return filepath.SkipDir
		}
		return nil
	})

	switch len(matches) {
	case 0:
		if walkErr != nil {
			return "", fmt.Errorf("searching repos: %w", walkErr)
		}
		return "", &ErrNoRepo{ScopeLabel: s.Scope.ScopeLabel()}
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple overlay repos found for %s:\n  %s\nRemove extras with: shimmer repo remove",
			s.Scope.ScopeLabel(), strings.Join(matches, "\n  "))
	}
}

// repoInfo builds RepoInfo from a clone directory.
func (s *Shimmer) repoInfo(clonePath, owner, name string) (*RepoInfo, error) {
	remote, _ := gitOutput(clonePath, "remote", "get-url", "origin")
	branch, _ := gitOutput(clonePath, "rev-parse", "--abbrev-ref", "HEAD")

	rp := s.ReposPath()
	rel, _ := filepath.Rel(rp, clonePath)
	segments := strings.SplitN(rel, string(os.PathSeparator), 3)

	targetSegment := ""
	if len(segments) >= 3 {
		targetSegment = segments[2]
	}

	targetPath := ""
	isGlobal := targetSegment == globalSegment
	if !isGlobal {
		targetPath = "/" + targetSegment
	}

	targetExists := true
	if isGlobal {
		// $HOME always exists
	} else if targetPath != "" {
		if _, err := os.Stat(targetPath); err != nil {
			targetExists = false
		}
	}

	info := &RepoInfo{
		Owner:        owner,
		Name:         name,
		RemoteURL:    strings.TrimSpace(remote),
		TargetPath:   targetPath,
		Branch:       strings.TrimSpace(branch),
		ClonePath:    clonePath,
		TargetExists: targetExists,
		IsGlobal:     isGlobal,
	}

	// Check linked status using targeted check (fast for $HOME).
	if overlayFiles, walkErr := WalkOverlay(clonePath); walkErr == nil {
		checkTarget := targetPath
		if isGlobal {
			if home, err := os.UserHomeDir(); err == nil {
				checkTarget = home
			}
		}
		if checkTarget != "" && targetExists {
			links, _ := CheckSymlinks(checkTarget, overlayFiles, s.Home)
			info.Linked = len(links) > 0
		}
	}

	return info, nil
}

// gitOutput runs git in the given directory and returns stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

