package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newEjectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "eject",
		Short: "Replace symlinks with file copies, keeping the overlay repo",
		Long: `Eject materializes all shimmer symlinks into regular files. The overlay
repo stays intact for future use. Stashed files are discarded.

After eject, the files are real and visible to git status.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromCmd(cmd)
			if err != nil {
				return renderError(err)
			}

			result, err := s.Eject()
			if err != nil {
				return renderError(err)
			}

			out := cmd.OutOrStdout()

			if len(result.Ejected) == 0 {
				fmt.Fprintln(out, "Nothing to eject.")
				return nil
			}

			fmt.Fprintf(out, "Ejected (%d):\n", len(result.Ejected))
			for _, f := range result.Ejected {
				fmt.Fprintf(out, "  %s\n", f)
			}

			if result.StashCleared {
				fmt.Fprintln(out, "Stash cleared.")
			}

			return nil
		},
	}
}
