package shimmer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	existing, err := s.findShimmerLinks()
	if err != nil {
		return nil, fmt.Errorf("scanning existing links: %w", err)
	}

	// 3. Walk overlay to get files to link.
	overlayFiles, err := WalkOverlay(clonePath)
	if err != nil {
		return nil, fmt.Errorf("walking overlay: %w", err)
	}

	target := s.Scope.Target()

	var removed []string
	for _, link := range existing {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("removing stale link %s: %w", link, err)
		}
		rel, _ := filepath.Rel(target, link)
		removed = append(removed, rel)
		s.cleanEmptyLinkParents(filepath.Dir(link))
	}

	// 4. For each file, check if destination exists (and is not a shimmer symlink).
	//    Collect as conflicts.
	var conflictRels []string
	conflictSet := make(map[string]Conflict)
	for _, rel := range overlayFiles {
		dest := filepath.Join(target, rel)
		info, err := os.Lstat(dest)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("checking %s: %w", rel, err)
		}
		// If it's a symlink pointing into our repos dir, it's ours (already removed above).
		// If it still exists after removal it's something else or a new file.
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, linkErr := os.Readlink(dest)
			if linkErr == nil {
				linkTarget = absSymlinkTarget(dest, linkTarget)
				reposDir := filepath.Join(s.Home, "repos")
				if isSubpath(linkTarget, reposDir) {
					continue // it's a shimmer link that we just created or will create
				}
			}
		}
		conflictRels = append(conflictRels, rel)
	}

	// Batch-check which conflicting files are tracked by git (single subprocess).
	tracked := s.Scope.TrackedFiles(conflictRels)
	var conflicts []Conflict
	for _, rel := range conflictRels {
		c := Conflict{Path: rel, Tracked: tracked[rel]}
		conflicts = append(conflicts, c)
		conflictSet[rel] = c
	}

	// 5. If conflicts and no flags: return ErrConflicts.
	if len(conflicts) > 0 && !skip && !overwrite {
		return nil, &ErrConflicts{Conflicts: conflicts}
	}

	// 6. Create symlinks.
	result := &LinkResult{
		Removed: removed,
	}

	for _, rel := range overlayFiles {
		dest := filepath.Join(target, rel)
		src := filepath.Join(clonePath, rel)

		if c, ok := conflictSet[rel]; ok {
			if skip {
				result.Skipped = append(result.Skipped, rel)
				continue
			}
			if overwrite {
				if err := s.stashFile(rel, dest); err != nil {
					return nil, fmt.Errorf("stashing %s: %w", rel, err)
				}
				result.Stashed = append(result.Stashed, rel)
				if c.Tracked {
					if err := s.Scope.SetSkipWorktree(rel, true); err != nil {
						fmt.Fprintf(os.Stderr, "warning: could not set skip-worktree for %s: %v\n", rel, err)
					}
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

	// 7. Update link state.
	if err := s.Scope.SaveLinkState(result.Linked); err != nil {
		return nil, fmt.Errorf("saving link state: %w", err)
	}

	// 8. Filter result.Removed to exclude files that were re-linked.
	result.Removed = filterOut(result.Removed, result.Linked)

	return result, nil
}

// stashFile moves a conflicting file to the stash location.
func (s *Shimmer) stashFile(rel, src string) error {
	dest := filepath.Join(s.Scope.StashDir(), rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dest)
}

// updateGitExclude writes linked paths to .git/info/exclude in a shimmer-managed block.
func updateGitExclude(target string, linkedPaths []string) error {
	excludePath := filepath.Join(target, ".git", "info", "exclude")

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
		sort.Strings(sorted)
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
	target := s.Scope.Target()
	for dir != target && isSubpath(dir, target) {
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
