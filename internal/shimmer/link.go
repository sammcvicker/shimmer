package shimmer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Link reconciles symlinks between the overlay clone and the target.
// skip: skip conflicting files. overwrite: stash and shadow conflicting files.
func (s *Shimmer) Link(skip, overwrite bool) (*LinkResult, error) {
	// 1. Find the clone for the current scope.
	clonePath, err := s.findClone()
	if err != nil {
		return nil, err
	}

	// 2. Remove stale symlinks from previous link state.
	existing, err := ScanSymlinks(s.Target, s.Home)
	if err != nil {
		return nil, fmt.Errorf("scanning existing links: %w", err)
	}

	var removed []string
	for _, link := range existing {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("removing stale link %s: %w", link, err)
		}
		rel, _ := filepath.Rel(s.Target, link)
		removed = append(removed, rel)
		s.cleanEmptyLinkParents(filepath.Dir(link))
	}

	// 3. Walk overlay to get files to link.
	overlayFiles, err := WalkOverlay(clonePath)
	if err != nil {
		return nil, fmt.Errorf("walking overlay: %w", err)
	}

	// 4. For each file, check if destination exists (and is not a shimmer symlink).
	//    Collect as conflicts.
	var conflicts []Conflict
	conflictSet := make(map[string]bool)
	for _, rel := range overlayFiles {
		dest := filepath.Join(s.Target, rel)
		info, err := os.Lstat(dest)
		if err != nil {
			continue // file doesn't exist, no conflict
		}
		// If it's a symlink pointing into our repos dir, it's ours (already removed above).
		// If it still exists after removal it's something else or a new file.
		if info.Mode()&os.ModeSymlink != 0 {
			target, linkErr := os.Readlink(dest)
			if linkErr == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(dest), target)
				}
				reposDir := filepath.Join(s.Home, "repos")
				if strings.HasPrefix(target, reposDir) {
					continue // it's a shimmer link that we just created or will create
				}
			}
		}
		tracked := s.isTracked(rel)
		conflicts = append(conflicts, Conflict{Path: rel, Tracked: tracked})
		conflictSet[rel] = true
	}

	// 5. If conflicts exist and neither skip nor overwrite: return ErrConflicts.
	if len(conflicts) > 0 && !skip && !overwrite {
		return nil, &ErrConflicts{Conflicts: conflicts}
	}

	// 6. Create symlinks.
	result := &LinkResult{
		Removed: removed,
	}

	for _, rel := range overlayFiles {
		dest := filepath.Join(s.Target, rel)
		src := filepath.Join(clonePath, rel)

		if conflictSet[rel] {
			if skip {
				result.Skipped = append(result.Skipped, rel)
				continue
			}
			if overwrite {
				if err := s.stashFile(rel, dest); err != nil {
					return nil, fmt.Errorf("stashing %s: %w", rel, err)
				}
				result.Stashed = append(result.Stashed, rel)
			if s.isTracked(rel) {
				s.setSkipWorktree(rel, true)
			}
			}
		}

		// Create parent directories if needed.
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, fmt.Errorf("creating parent dirs for %s: %w", rel, err)
		}

		// Create symlink.
		if err := os.Symlink(src, dest); err != nil {
			return nil, fmt.Errorf("creating symlink %s: %w", rel, err)
		}
		result.Linked = append(result.Linked, rel)
	}

	// 7. Update .git/info/exclude with linked paths (local scope only).
	if !s.Global {
		if err := s.updateGitExclude(result.Linked); err != nil {
			return nil, fmt.Errorf("updating .git/info/exclude: %w", err)
		}
	}

	// 8. Filter result.Removed to exclude files that were re-linked.
	result.Removed = filterOut(result.Removed, result.Linked)

	return result, nil
}

// stashFile moves a conflicting file to the stash location.
func (s *Shimmer) stashFile(rel, src string) error {
	dest := s.stashPath(rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dest)
}

// stashPath computes the stash path for a given relative file path.
// For local scope: .git/shimmer-stash/<rel>
// For global scope: ~/.shimmer/stash/<rel>
func (s *Shimmer) stashPath(rel string) string {
	if s.Global {
		return filepath.Join(s.Home, "stash", rel)
	}
	return filepath.Join(s.Target, ".git", "shimmer-stash", rel)
}

// isTracked checks if a file is tracked by git.
func (s *Shimmer) isTracked(rel string) bool {
	cmd := exec.Command("git", "-C", s.Target, "ls-files", "--error-unmatch", rel)
	err := cmd.Run()
	return err == nil
}

// setSkipWorktree sets or clears the skip-worktree flag for a file.
func (s *Shimmer) setSkipWorktree(rel string, set bool) error {
	flag := "--skip-worktree"
	if !set {
		flag = "--no-skip-worktree"
	}
	cmd := exec.Command("git", "-C", s.Target, "update-index", flag, rel)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("skip-worktree %s: %s: %w", rel, out, err)
	}
	return nil
}

// updateGitExclude writes linked paths to .git/info/exclude in a shimmer-managed block.
func (s *Shimmer) updateGitExclude(linkedPaths []string) error {
	excludePath := filepath.Join(s.Target, ".git", "info", "exclude")

	// Ensure the directory exists.
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}

	// Read existing content.
	existing, _ := os.ReadFile(excludePath)
	content := string(existing)

	// Remove old shimmer block if present.
	const startMarker = "# shimmer managed — do not edit"
	const endMarker = "# end shimmer"

	startIdx := strings.Index(content, startMarker)
	if startIdx >= 0 {
		endIdx := strings.Index(content, endMarker)
		if endIdx >= 0 {
			endIdx += len(endMarker)
			// Also consume the trailing newline if present.
			if endIdx < len(content) && content[endIdx] == '\n' {
				endIdx++
			}
			content = content[:startIdx] + content[endIdx:]
		}
	}

	// Remove trailing whitespace/newlines from existing content.
	content = strings.TrimRight(content, "\n\r\t ")

	// Build the shimmer block.
	if len(linkedPaths) > 0 {
		var block strings.Builder
		block.WriteString(startMarker)
		block.WriteString("\n")
		sorted := make([]string, len(linkedPaths))
		copy(sorted, linkedPaths)
		// Sort for deterministic output.
		sortStrings(sorted)
		for _, p := range sorted {
			block.WriteString(p)
			block.WriteString("\n")
		}
		block.WriteString(endMarker)

		if content != "" {
			content += "\n\n"
		}
		content += block.String() + "\n"
	} else if content != "" {
		content += "\n"
	}

	return os.WriteFile(excludePath, []byte(content), 0o644)
}

// cleanEmptyLinkParents removes empty directories up to the target root.
func (s *Shimmer) cleanEmptyLinkParents(dir string) {
	for dir != s.Target && strings.HasPrefix(dir, s.Target) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// filterOut returns items in a that are not in b.
func filterOut(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	var result []string
	for _, s := range a {
		if !set[s] {
			result = append(result, s)
		}
	}
	return result
}

// sortStrings sorts a slice of strings in place.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
