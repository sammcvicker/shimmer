package shimmer

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitRoot finds the git repository root from the given directory.
func GitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", &ErrNotInGitRepo{}
	}
	return strings.TrimSpace(string(out)), nil
}

// ParseRepoURL extracts owner and repo name from a git URL.
// Supports SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git).
func ParseRepoURL(url string) (owner, name string, err error) {
	url = strings.TrimSuffix(url, ".git")

	// SSH: git@github.com:owner/repo
	if strings.Contains(url, ":") && strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("cannot parse SSH URL: %s", url)
		}
		segments := strings.Split(parts[1], "/")
		if len(segments) < 2 {
			return "", "", fmt.Errorf("cannot parse owner/repo from: %s", url)
		}
		return segments[len(segments)-2], segments[len(segments)-1], nil
	}

	// HTTPS: https://github.com/owner/repo
	segments := strings.Split(strings.TrimRight(url, "/"), "/")
	if len(segments) < 2 {
		return "", "", fmt.Errorf("cannot parse owner/repo from URL: %s", url)
	}
	return segments[len(segments)-2], segments[len(segments)-1], nil
}

// ClonePath computes the filesystem path where a clone should live.
func ClonePath(home, owner, repo, targetPath string, global bool) string {
	if global {
		return filepath.Join(home, "repos", owner, repo, "_global")
	}
	rel := strings.TrimPrefix(targetPath, "/")
	return filepath.Join(home, "repos", owner, repo, rel)
}
