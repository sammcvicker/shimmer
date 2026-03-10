package cmd

import (
	"fmt"
	"os"

	"github.com/siimpl/shimmer/internal/shimmer"
	"github.com/spf13/cobra"
)

// newShimmerFromFlags creates a *shimmer.Shimmer from the -g flag and current
// working directory (discovers git root for local scope).
func newShimmerFromFlags() (*shimmer.Shimmer, error) {
	home := shimmer.DefaultHome()

	if globalFlag {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		return &shimmer.Shimmer{
			Home:   home,
			Global: true,
			Target: userHome,
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
		Home:   home,
		Global: false,
		Target: root,
	}, nil
}

func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage overlay repos",
	}

	cmd.AddCommand(newRepoSetCmd())
	cmd.AddCommand(newRepoPathCmd())
	cmd.AddCommand(newRepoListCmd())
	cmd.AddCommand(newRepoRemoveCmd())

	return cmd
}

func newRepoSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <url>",
		Short: "Clone an overlay repo for the current project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			info, err := s.RepoSet(args[0])
			if err != nil {
				return renderError(err)
			}

			scope := info.TargetPath
			if info.TargetPath == "" {
				scope = "global"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Cloned %s/%s → %s (scope: %s)\n",
				info.Owner, info.Name, info.ClonePath, scope)
			return nil
		},
	}
}

func newRepoPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the clone path for the current scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			path, err := s.RepoPath()
			if err != nil {
				return renderError(err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func newRepoListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all overlay repo clones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			repos, err := s.RepoList()
			if err != nil {
				return renderError(err)
			}

			if len(repos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No overlay repos configured.")
				return nil
			}

			for _, r := range repos {
				scope := r.TargetPath
				if r.TargetPath == "" {
					scope = "global"
				}
				status := ""
				if !r.TargetExists {
					status = " (target missing)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s/%s  branch:%s  scope:%s  %s%s\n",
					r.Owner, r.Name, r.Branch, scope, r.ClonePath, status)
			}
			return nil
		},
	}
}

func newRepoRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove the overlay repo clone for the current scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			if err := s.RepoRemove(); err != nil {
				return renderError(err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Overlay repo removed.")
			return nil
		},
	}
}
