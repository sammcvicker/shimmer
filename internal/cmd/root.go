package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/siimpl/shimmer/internal/shimmer"
	"github.com/spf13/cobra"
)

var globalFlag bool

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shimmer",
		Short: "Transparent git-backed file overlays",
		Long:  "shimmer creates per-file symlinks from a git-backed overlay repo into your project.\nUse -g for global scope ($HOME) instead of the current project.",
		TraverseChildren: true,
		SilenceUsage:     true,
		SilenceErrors:    true,
	}

	cmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "use global scope ($HOME)")

	cmd.AddCommand(newRepoCmd())
	cmd.AddCommand(newLinkCmd())
	cmd.AddCommand(newUnlinkCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newGitCmd())
	cmd.AddCommand(newEjectCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

func Execute() error {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

// renderError translates typed errors into user-friendly messages.
func renderError(err error) error {
	var noRepo *shimmer.ErrNoRepo
	if errors.As(err, &noRepo) {
		return fmt.Errorf("no overlay repo set for %s\n\n  shimmer repo set <url>", noRepo.ScopeLabel)
	}

	var alreadySet *shimmer.ErrRepoAlreadySet
	if errors.As(err, &alreadySet) {
		return fmt.Errorf("overlay repo already set: %s\n  clone: %s\n\n  To change, first run: shimmer repo remove",
			alreadySet.RemoteURL, alreadySet.ClonePath)
	}

	var conflicts *shimmer.ErrConflicts
	if errors.As(err, &conflicts) {
		var b strings.Builder
		b.WriteString("these files already exist and would be shadowed:\n")
		maxLen := 0
		for _, c := range conflicts.Conflicts {
			if len(c.Path) > maxLen {
				maxLen = len(c.Path)
			}
		}
		for _, c := range conflicts.Conflicts {
			tracked := "untracked"
			if c.Tracked {
				tracked = "tracked"
			}
			fmt.Fprintf(&b, "  %-*s (%s)\n", maxLen+1, c.Path, tracked)
		}
		b.WriteString("\nOptions:\n")
		b.WriteString("  --skip        Link only non-conflicting files, leave existing ones in place\n")
		b.WriteString("  --overwrite   Stash existing files and shadow them (tracked files use\n")
		b.WriteString("                skip-worktree, which is fragile — see docs)\n")
		hasTracked := false
		var trackedFiles []string
		for _, c := range conflicts.Conflicts {
			if c.Tracked {
				hasTracked = true
				trackedFiles = append(trackedFiles, c.Path)
			}
		}
		if hasTracked {
			b.WriteString("\nTo permanently resolve tracked file conflicts (recommended):\n")
			for _, f := range trackedFiles {
				fmt.Fprintf(&b, "  git rm --cached %s\n", f)
			}
			b.WriteString("  shimmer link\n")
		}
		b.WriteString("\nTo undo any shimmer operation:\n")
		b.WriteString("  shimmer unlink\n")
		return fmt.Errorf("%s", b.String())
	}

	var notInGit *shimmer.ErrNotInGitRepo
	if errors.As(err, &notInGit) {
		return fmt.Errorf("not in a git repository (use -g for global scope)")
	}

	var notLinked *shimmer.ErrNotLinked
	if errors.As(err, &notLinked) {
		return fmt.Errorf("not linked — nothing to do")
	}

	return err
}
