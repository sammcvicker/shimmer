package shimmer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// linkPlan holds the computed diff between desired and existing symlinks.
type linkPlan struct {
	target       string
	overlayFiles []string            // ordered list of relative paths from overlay
	desiredSet   map[string]string   // rel -> absolute source path
	existingByRel map[string]string  // rel -> absolute link path in target
	alreadyCurrent map[string]bool   // rel paths that already point to the correct source
	conflicts    []Conflict          // files that conflict with non-shimmer content
	conflictSet  map[string]Conflict // same, keyed by rel for fast lookup
	staleLinks   []string            // rel paths of links to remove
}

// planLinks computes what needs to be added, removed, or resolved before linking.
func (s *Shimmer) planLinks(clonePath string) (*linkPlan, error) {
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
	desiredSet := make(map[string]string, len(overlayFiles))
	for _, rel := range overlayFiles {
		desiredSet[rel] = filepath.Join(clonePath, rel)
	}

	existingByRel := make(map[string]string, len(existing))
	for _, link := range existing {
		rel, _ := filepath.Rel(target, link)
		existingByRel[rel] = link
	}

	// Find stale links (exist but not desired).
	var staleLinks []string
	for rel := range existingByRel {
		if _, desired := desiredSet[rel]; !desired {
			staleLinks = append(staleLinks, rel)
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
	for _, rel := range overlayFiles {
		if alreadyCurrent[rel] {
			continue
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
				if isSubpath(linkTarget, s.ReposPath()) {
					// Stale shimmer link with wrong target — remove it.
					os.Remove(dest)
					continue
				}
			}
		}
		conflictRels = append(conflictRels, rel)
	}

	var tracked map[string]bool
	if ga, ok := s.Scope.(GitAware); ok {
		tracked = ga.TrackedFiles(conflictRels)
	}
	conflictSet := make(map[string]Conflict, len(conflictRels))
	conflicts := make([]Conflict, 0, len(conflictRels))
	for _, rel := range conflictRels {
		c := Conflict{Path: rel, Tracked: tracked[rel]}
		conflicts = append(conflicts, c)
		conflictSet[rel] = c
	}

	return &linkPlan{
		target:         target,
		overlayFiles:   overlayFiles,
		desiredSet:     desiredSet,
		existingByRel:  existingByRel,
		alreadyCurrent: alreadyCurrent,
		conflicts:      conflicts,
		conflictSet:    conflictSet,
		staleLinks:     staleLinks,
	}, nil
}

// executeLinks applies the plan: removes stale links, resolves conflicts, and creates new symlinks.
func (s *Shimmer) executeLinks(plan *linkPlan, skip, overwrite bool) (*LinkResult, error) {
	result := &LinkResult{}

	// Remove stale links.
	for _, rel := range plan.staleLinks {
		link := plan.existingByRel[rel]
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("removing stale link %s: %w", link, err)
		}
		result.Removed = append(result.Removed, rel)
		cleanEmptyParents(filepath.Dir(link), plan.target)
	}

	// Create symlinks for files that need it.
	for _, rel := range plan.overlayFiles {
		if plan.alreadyCurrent[rel] {
			result.Linked = append(result.Linked, rel)
			continue
		}

		dest := filepath.Join(plan.target, rel)
		src := plan.desiredSet[rel]

		if c, ok := plan.conflictSet[rel]; ok {
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
					if ga, ok := s.Scope.(GitAware); ok {
						if err := ga.SetSkipWorktree(rel, true); err != nil {
							fmt.Fprintf(os.Stderr, "warning: could not set skip-worktree for %s: %v\n", rel, err)
						}
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

	return result, nil
}

// Link reconciles symlinks between the overlay clone and the target.
// skip: skip conflicting files. overwrite: stash and shadow conflicting files.
// Uses a diff-based approach: only creates/removes symlinks that actually changed,
// avoiding unnecessary filesystem churn on re-link.
func (s *Shimmer) Link(skip, overwrite bool) (*LinkResult, error) {
	clonePath, err := s.findClone()
	if err != nil {
		return nil, err
	}

	plan, err := s.planLinks(clonePath)
	if err != nil {
		return nil, err
	}

	if len(plan.conflicts) > 0 && !skip && !overwrite {
		return nil, &ErrConflicts{Conflicts: plan.conflicts}
	}

	result, err := s.executeLinks(plan, skip, overwrite)
	if err != nil {
		return nil, err
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
	// Guard against overwriting an existing stash entry.
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("stash conflict for %s: a previously stashed copy exists — run 'shimmer unlink' first to restore it", rel)
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
	startIdx := strings.Index(content, excludeMarkerStart)
	if startIdx >= 0 {
		endIdx := strings.Index(content, excludeMarkerEnd)
		if endIdx >= 0 {
			endIdx += len(excludeMarkerEnd)
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
		block.WriteString(excludeMarkerStart)
		block.WriteString("\n")
		sorted := make([]string, len(linkedPaths))
		copy(sorted, linkedPaths)
		// Sort for deterministic output.
		sort.Strings(sorted)
		for _, p := range sorted {
			block.WriteString(p)
			block.WriteString("\n")
		}
		block.WriteString(excludeMarkerEnd)

		if content != "" {
			content += "\n\n"
		}
		content += block.String() + "\n"
	} else if content != "" {
		content += "\n"
	}

	return os.WriteFile(excludePath, []byte(content), 0o644)
}


