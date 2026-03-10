package cmd

import (
	"fmt"
	"os"

	"github.com/siimpl/shimmer/internal/shimmer"
)

// newShimmerFromFlags creates a *shimmer.Shimmer from the -g flag and current
// working directory (discovers git root for local scope).
func newShimmerFromFlags() (*shimmer.Shimmer, error) {
	home, err := shimmer.DefaultHome()
	if err != nil {
		return nil, err
	}

	if globalFlag {
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
