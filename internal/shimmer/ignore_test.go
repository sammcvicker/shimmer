package shimmer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sammcvicker/shimmer/internal/shimmer"
)

func TestParseShimmerignore(t *testing.T) {
	dir := t.TempDir()
	content := "README.md\nLICENSE\n# comment\n\n*.txt\n"
	if err := os.WriteFile(filepath.Join(dir, ".shimmerignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ignore, err := shimmer.ParseShimmerignore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Explicitly listed
	assertIgnored(t, ignore, "README.md", true)
	assertIgnored(t, ignore, "LICENSE", true)
	assertIgnored(t, ignore, "notes.txt", true)

	// Always implicitly ignored
	assertIgnored(t, ignore, ".shimmerignore", true)
	assertIgnored(t, ignore, ".git/config", true)
	assertIgnored(t, ignore, ".gitignore", true)

	// Not ignored
	assertIgnored(t, ignore, "CLAUDE.md", false)
	assertIgnored(t, ignore, ".claude/settings.json", false)
}

func TestParseShimmerignoreNoFile(t *testing.T) {
	dir := t.TempDir()

	ignore, err := shimmer.ParseShimmerignore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Implicit ignores still work
	assertIgnored(t, ignore, ".shimmerignore", true)
	assertIgnored(t, ignore, ".git/config", true)

	// Everything else is not ignored
	assertIgnored(t, ignore, "CLAUDE.md", false)
}

func TestParseShimmerignoreMalformedPattern(t *testing.T) {
	dir := t.TempDir()
	content := "[invalid\n"
	if err := os.WriteFile(filepath.Join(dir, ".shimmerignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := shimmer.ParseShimmerignore(dir)
	if err == nil {
		t.Fatal("expected error for malformed pattern, got nil")
	}
	if !strings.Contains(err.Error(), "[invalid") {
		t.Errorf("error should mention the bad pattern, got: %v", err)
	}
}

func TestIgnorePathSeparatorSemantics(t *testing.T) {
	dir := t.TempDir()
	content := "docs/\nREADME.md\n"
	if err := os.WriteFile(filepath.Join(dir, ".shimmerignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ignore, err := shimmer.ParseShimmerignore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// "docs/" contains a slash — matches only as directory prefix
	assertIgnored(t, ignore, "docs", true)
	assertIgnored(t, ignore, "docs/file.txt", true)

	// "README.md" has no slash — matches base name anywhere
	assertIgnored(t, ignore, "README.md", true)
	assertIgnored(t, ignore, "some/deep/README.md", true)

	// A bare name like "docs" without trailing slash would match as base name
	// But our pattern is "docs/" so it should NOT match a file called "docs" nested deeply
	// unless it's a directory prefix
	assertIgnored(t, ignore, "other/docs", false) // "docs/" pattern doesn't match base name
}

func TestIgnoreLeadingSlashAnchorsToRoot(t *testing.T) {
	dir := t.TempDir()
	content := "/CLAUDE.md\n/docs/\n/build\n"
	if err := os.WriteFile(filepath.Join(dir, ".shimmerignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ignore, err := shimmer.ParseShimmerignore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// "/CLAUDE.md" matches only the root-level file
	assertIgnored(t, ignore, "CLAUDE.md", true)
	assertIgnored(t, ignore, "nested/CLAUDE.md", false)
	assertIgnored(t, ignore, "dhi/CLAUDE.md", false)

	// "/docs/" matches the root-level directory and its contents
	assertIgnored(t, ignore, "docs", true)
	assertIgnored(t, ignore, "docs/file.md", true)
	assertIgnored(t, ignore, "nested/docs", false)
	assertIgnored(t, ignore, "nested/docs/file.md", false)

	// "/build" matches as a root-anchored prefix (file or directory)
	assertIgnored(t, ignore, "build", true)
	assertIgnored(t, ignore, "build/output", true)
	assertIgnored(t, ignore, "nested/build", false)
}

func TestIgnoreLeadingSlashCoexistsWithBaseNamePatterns(t *testing.T) {
	dir := t.TempDir()
	// /CLAUDE.md is root-anchored; LICENSE is unanchored (base-name match anywhere).
	content := "/CLAUDE.md\nLICENSE\n"
	if err := os.WriteFile(filepath.Join(dir, ".shimmerignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ignore, err := shimmer.ParseShimmerignore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Root-anchored pattern unaffected by deeper path
	assertIgnored(t, ignore, "CLAUDE.md", true)
	assertIgnored(t, ignore, "dhi/CLAUDE.md", false)

	// Unanchored pattern still matches by base name at any depth
	assertIgnored(t, ignore, "LICENSE", true)
	assertIgnored(t, ignore, "vendor/foo/LICENSE", true)
}

func assertIgnored(t *testing.T, ignore *shimmer.Ignore, path string, want bool) {
	t.Helper()
	if got := ignore.Match(path); got != want {
		t.Errorf("ignore.Match(%q) = %v, want %v", path, got, want)
	}
}
