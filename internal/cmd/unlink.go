package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink",
		Short: "Remove all shimmer symlinks and restore original files",
		Long: `Unlink reverses everything link does: removes symlinks, restores stashed
originals, clears skip-worktree flags, and cleans git exclude entries.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromCmd(cmd)
			if err != nil {
				return renderError(err)
			}

			n, err := s.Unlink()
			if err != nil {
				return renderError(err)
			}

			if n == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing to unlink")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "unlinked %d file(s)\n", n)
			}
			return nil
		},
	}
}
