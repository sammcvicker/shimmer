package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var skipFlag bool
	var overwriteFlag bool

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Reconcile symlinks from the overlay repo into your project",
		Long: `Link creates per-file symlinks from the overlay clone into your project.

If files already exist at the destination, use --skip to leave them alone
or --overwrite to stash them and create the symlink.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			result, err := s.Link(skipFlag, overwriteFlag)
			if err != nil {
				return renderError(err)
			}

			out := cmd.OutOrStdout()

			if len(result.Linked) > 0 {
				fmt.Fprintf(out, "Linked (%d):\n", len(result.Linked))
				for _, f := range result.Linked {
					fmt.Fprintf(out, "  %s\n", f)
				}
			}

			if len(result.Skipped) > 0 {
				fmt.Fprintf(out, "Skipped (%d):\n", len(result.Skipped))
				for _, f := range result.Skipped {
					fmt.Fprintf(out, "  %s (conflict)\n", f)
				}
			}

			if len(result.Stashed) > 0 {
				fmt.Fprintf(out, "Stashed (%d):\n", len(result.Stashed))
				for _, f := range result.Stashed {
					fmt.Fprintf(out, "  %s\n", f)
				}
			}

			if len(result.Removed) > 0 {
				fmt.Fprintf(out, "Removed (%d):\n", len(result.Removed))
				for _, f := range result.Removed {
					fmt.Fprintf(out, "  %s (stale)\n", f)
				}
			}

			// Summary line
			if len(result.Linked) == 0 && len(result.Removed) == 0 && len(result.Skipped) == 0 && len(result.Stashed) == 0 {
				fmt.Fprintln(out, "Nothing to do.")
			}

			// Warn about tracked stashed files and skip-worktree fragility
			if len(result.Stashed) > 0 {
				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "Warning: stashed files were replaced with symlinks.")
				fmt.Fprintln(out, "If any were tracked by git, note that skip-worktree is fragile —")
				fmt.Fprintln(out, "operations like `git reset --hard` or `git stash` may revert them.")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&skipFlag, "skip", false, "skip conflicting files")
	cmd.Flags().BoolVar(&overwriteFlag, "overwrite", false, "stash and replace conflicting files")
	cmd.MarkFlagsMutuallyExclusive("skip", "overwrite")

	return cmd
}
