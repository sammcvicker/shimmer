package cmd

import (
	"github.com/spf13/cobra"
)

func newGitCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "git [args...]",
		Short:              "Run git commands against the overlay repo clone",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}
			return s.Git(args)
		},
	}
}
