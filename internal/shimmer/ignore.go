package shimmer

import (
	"bufio"
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
		ig.patterns = append(ig.patterns, line)
	}
	return ig, scanner.Err()
}

// Match returns true if the path should be ignored.
func (ig *Ignore) Match(path string) bool {
	// Check implicit ignores
	for _, imp := range implicitIgnores {
		if path == imp || strings.HasPrefix(path, imp+"/") {
			return true
		}
	}

	// Check user patterns against the full path and the base name
	base := filepath.Base(path)
	for _, pattern := range ig.patterns {
		// Try matching against the full relative path
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Try matching against just the filename
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Directory prefix match (pattern without trailing slash)
		clean := strings.TrimSuffix(pattern, "/")
		if path == clean || strings.HasPrefix(path, clean+"/") {
			return true
		}
	}
	return false
}
