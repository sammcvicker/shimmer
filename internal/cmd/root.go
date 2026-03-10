package cmd

import (
	"fmt"
	"os"

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
