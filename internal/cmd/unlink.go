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
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			if err := s.Unlink(); err != nil {
				return renderError(err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "unlinked")
			return nil
		},
	}
}
