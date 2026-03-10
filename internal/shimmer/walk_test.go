package shimmer_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/siimpl/shimmer/internal/shimmer"
)

func TestWalkOverlay(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "CLAUDE.md", "config")
	writeFile(t, dir, ".claude/settings.json", "{}")
	writeFile(t, dir, ".claude/skills/review.md", "skill")
	writeFile(t, dir, ".cursorrules", "rules")
	writeFile(t, dir, "README.md", "readme")
	writeFile(t, dir, ".shimmerignore", "README.md\n")
	writeFile(t, dir, ".gitignore", "")
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)

	files, err := shimmer.WalkOverlay(dir)
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(files)
	want := []string{".claude/settings.json", ".claude/skills/review.md", ".cursorrules", "CLAUDE.md"}
	if !slices.Equal(files, want) {
		t.Errorf("got %v, want %v", files, want)
	}
}

func writeFile(t *testing.T, base, rel, content string) {
	t.Helper()
	p := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

