package cmd

import (
	"github.com/spf13/cobra"
)

func newGitCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "git [args...]",
		Short:              "Run git commands against the overlay repo clone",
		Long:               "Run git commands against the overlay repo clone.\n\nThe -g/--global flag can be placed before or after 'git'.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			global, gitArgs := extractGlobalFlag(args)
			s, err := newShimmerWithGlobal(global)
			if err != nil {
				return renderError(err)
			}
			return s.Git(gitArgs)
		},
	}
}

// extractGlobalFlag scans args for -g or --global, removes them,
// and returns whether the flag was found along with the remaining args.
func extractGlobalFlag(args []string) (bool, []string) {
	var filtered []string
	global := false
	for _, arg := range args {
		if arg == "-g" || arg == "--global" {
			global = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	return global, filtered
}
