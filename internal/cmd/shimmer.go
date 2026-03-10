package cmd

import (
	"fmt"
	"os"

	"github.com/siimpl/shimmer/internal/shimmer"
	"github.com/spf13/cobra"
)

// newShimmerFromCmd creates a Shimmer from the cobra command's -g flag.
func newShimmerFromCmd(cmd *cobra.Command) (*shimmer.Shimmer, error) {
	global, _ := cmd.Root().PersistentFlags().GetBool("global")
	return newShimmerWithGlobal(global)
}

// newShimmerWithGlobal creates a Shimmer with an explicit global flag value.
func newShimmerWithGlobal(global bool) (*shimmer.Shimmer, error) {
	home, err := shimmer.DefaultHome()
	if err != nil {
		return nil, err
	}

	if global {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		return &shimmer.Shimmer{
			Home:  home,
			Scope: shimmer.NewGlobalScope(userHome, home),
		}, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	root, err := shimmer.GitRoot(cwd)
	if err != nil {
		return nil, err
	}

	return &shimmer.Shimmer{
		Home:  home,
		Scope: shimmer.NewLocalScope(root),
	}, nil
}
