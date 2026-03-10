package cmd

import (
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
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "use global scope ($HOME)")

	cmd.AddCommand(newRepoCmd())
	cmd.AddCommand(newLinkCmd())
	cmd.AddCommand(newUnlinkCmd())
	cmd.AddCommand(newStatusCmd())

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
	switch e := err.(type) {
	case *shimmer.ErrNoRepo:
		scope := e.Target
		if e.Global {
			scope = "global"
		}
		return fmt.Errorf("no overlay repo set for %s\n\n  shimmer repo set <url>", scope)

	case *shimmer.ErrRepoAlreadySet:
		return fmt.Errorf("overlay repo already set: %s\n  clone: %s\n\n  To change, first run: shimmer repo remove",
			e.RemoteURL, e.ClonePath)

	case *shimmer.ErrConflicts:
		var b strings.Builder
		fmt.Fprintf(&b, "%d file(s) already exist and would be shadowed:\n\n", len(e.Conflicts))
		for _, c := range e.Conflicts {
			tracked := "untracked"
			if c.Tracked {
				tracked = "tracked"
			}
			fmt.Fprintf(&b, "  %s (%s)\n", c.Path, tracked)
		}
		b.WriteString("\nOptions:\n")
		b.WriteString("  shimmer link --skip        skip conflicting files\n")
		b.WriteString("  shimmer link --overwrite    stash and replace conflicting files\n")
		b.WriteString("  git rm --cached <file>      untrack files first\n")
		return fmt.Errorf("%s", b.String())

	case *shimmer.ErrNotInGitRepo:
		return fmt.Errorf("not in a git repository (use -g for global scope)")

	case *shimmer.ErrNotLinked:
		return fmt.Errorf("not linked — nothing to do")

	default:
		return err
	}
}
