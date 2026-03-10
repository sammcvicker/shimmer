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
// Uses a diff-based approach: only creates/removes symlinks that actually changed,
// avoiding unnecessary filesystem churn on re-link.
func (s *Shimmer) Link(skip, overwrite bool) (*LinkResult, error) {
	clonePath, err := s.findClone()
	if err != nil {
		return nil, err
	}

	existing, err := s.findShimmerLinks()
	if err != nil {
		return nil, fmt.Errorf("scanning existing links: %w", err)
	}

	overlayFiles, err := WalkOverlay(clonePath)
	if err != nil {
		return nil, fmt.Errorf("walking overlay: %w", err)
	}

	target := s.Scope.Target()

	// Build sets for diff computation.
	desiredSet := make(map[string]string, len(overlayFiles)) // rel -> src
	for _, rel := range overlayFiles {
		desiredSet[rel] = filepath.Join(clonePath, rel)
	}

	existingByRel := make(map[string]string, len(existing)) // rel -> linkPath
	for _, link := range existing {
		rel, _ := filepath.Rel(target, link)
		existingByRel[rel] = link
	}

	result := &LinkResult{}

	// Remove stale links (exist but not desired).
	for rel, link := range existingByRel {
		if _, desired := desiredSet[rel]; !desired {
			if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("removing stale link %s: %w", link, err)
			}
			result.Removed = append(result.Removed, rel)
			s.cleanEmptyLinkParents(filepath.Dir(link))
		}
	}

	// Determine which desired files already have correct symlinks.
	alreadyCurrent := make(map[string]bool)
	for rel, src := range desiredSet {
		if linkPath, exists := existingByRel[rel]; exists {
			linkTarget, err := os.Readlink(linkPath)
			if err == nil {
				linkTarget = absSymlinkTarget(linkPath, linkTarget)
				if linkTarget == src {
					alreadyCurrent[rel] = true
				}
			}
		}
	}

	// Check for conflicts among files that need creating.
	var conflictRels []string
	conflictSet := make(map[string]Conflict)
	for _, rel := range overlayFiles {
		if alreadyCurrent[rel] {
			continue // already correctly linked
		}
		dest := filepath.Join(target, rel)
		info, err := os.Lstat(dest)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("checking %s: %w", rel, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, linkErr := os.Readlink(dest)
			if linkErr == nil {
				linkTarget = absSymlinkTarget(dest, linkTarget)
				reposDir := filepath.Join(s.Home, "repos")
				if isSubpath(linkTarget, reposDir) {
					// Stale shimmer link with wrong target — remove it.
					os.Remove(dest)
					continue
				}
			}
		}
		conflictRels = append(conflictRels, rel)
	}

	tracked := s.Scope.TrackedFiles(conflictRels)
	var conflicts []Conflict
	for _, rel := range conflictRels {
		c := Conflict{Path: rel, Tracked: tracked[rel]}
		conflicts = append(conflicts, c)
		conflictSet[rel] = c
	}

	if len(conflicts) > 0 && !skip && !overwrite {
		return nil, &ErrConflicts{Conflicts: conflicts}
	}

	// Create symlinks for files that need it.
	for _, rel := range overlayFiles {
		if alreadyCurrent[rel] {
			result.Linked = append(result.Linked, rel)
			continue
		}

		dest := filepath.Join(target, rel)
		src := desiredSet[rel]

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

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, fmt.Errorf("creating parent dirs for %s: %w", rel, err)
		}

		if err := os.Symlink(src, dest); err != nil {
			return nil, fmt.Errorf("creating symlink %s: %w", rel, err)
		}
		result.Linked = append(result.Linked, rel)
	}

	if err := s.Scope.SaveLinkState(result.Linked); err != nil {
		return nil, fmt.Errorf("saving link state: %w", err)
	}

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
	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", excludePath, err)
	}
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

