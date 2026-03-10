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

	// SSH with protocol: ssh://git@github.com/owner/repo or ssh://github.com/owner/repo
	if strings.HasPrefix(url, "ssh://") {
		trimmed := strings.TrimPrefix(url, "ssh://")
		// Remove user@ if present: git@github.com/owner/repo -> github.com/owner/repo
		if idx := strings.Index(trimmed, "@"); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		segments := strings.Split(strings.TrimRight(trimmed, "/"), "/")
		if len(segments) < 3 {
			return "", "", fmt.Errorf("cannot parse owner/repo from SSH URL: %s", url)
		}
		return segments[len(segments)-2], segments[len(segments)-1], nil
	}

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

// absSymlinkTarget resolves a symlink target to an absolute path.
// If target is already absolute, it is returned as-is.
func absSymlinkTarget(linkPath, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(linkPath), target)
}

// isSubpath reports whether path is equal to or nested under dir.
// Unlike strings.HasPrefix, this is path-aware and won't match
// /foo/bar-other when dir is /foo/bar.
func isSubpath(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}
