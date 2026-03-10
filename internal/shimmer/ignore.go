package shimmer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Ignore decides whether a file path should be excluded from linking.
type Ignore struct {
	patterns []string
}

// implicitIgnores are always excluded regardless of .shimmerignore content.
var implicitIgnores = []string{".shimmerignore", ".git", ".gitignore"}

// ParseShimmerignore reads .shimmerignore from the repo root.
// If the file doesn't exist, only implicit ignores apply.
func ParseShimmerignore(repoRoot string) (*Ignore, error) {
	ig := &Ignore{}

	f, err := os.Open(filepath.Join(repoRoot, ".shimmerignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return ig, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := filepath.Match(line, ""); err != nil {
			return nil, fmt.Errorf("bad .shimmerignore pattern %q: %w", line, err)
		}
		ig.patterns = append(ig.patterns, line)
	}
	return ig, scanner.Err()
}

// Match returns true if the path should be ignored.
func (ig *Ignore) Match(path string) bool {
	// Check implicit ignores
	for _, imp := range implicitIgnores {
		if path == imp || strings.HasPrefix(path, imp+string(filepath.Separator)) {
			return true
		}
	}

	base := filepath.Base(path)
	for _, pattern := range ig.patterns {
		clean := strings.TrimSuffix(pattern, "/")
		hasDirSlash := strings.HasSuffix(pattern, "/")

		// If pattern contains a slash (after trimming trailing /),
		// match against full path only.
		if strings.Contains(clean, "/") {
			if matched, _ := filepath.Match(clean, path); matched {
				return true
			}
			// Directory prefix match
			if path == clean || strings.HasPrefix(path, clean+"/") {
				return true
			}
			continue
		}

		// Trailing-slash patterns (e.g. "docs/") are directory-only:
		// match only as a directory prefix, not as a base name.
		if hasDirSlash {
			if path == clean || strings.HasPrefix(path, clean+"/") {
				return true
			}
			continue
		}

		// No slash in pattern: match against base name (gitignore convention)
		if matched, _ := filepath.Match(clean, base); matched {
			return true
		}
		// Also check as directory prefix for non-glob patterns
		if !strings.ContainsAny(clean, "*?[") {
			if path == clean || strings.HasPrefix(path, clean+"/") {
				return true
			}
		}
	}
	return false
}
