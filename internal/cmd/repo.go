package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/siimpl/shimmer/internal/shimmer"
	"github.com/spf13/cobra"
)

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
		Use:   "set <url> [project-path]",
		Short: "Clone an overlay repo for the current project",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			if len(args) > 1 {
				target, err := resolveProjectPath(args[1])
				if err != nil {
					return renderError(err)
				}
				s.Target = target
			}

			info, err := s.RepoSet(args[0])
			if err != nil {
				return renderError(err)
			}

			scope := info.TargetPath
			if info.IsGlobal {
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
			home, err := shimmer.DefaultHome()
			if err != nil {
				return err
			}
			s := &shimmer.Shimmer{Home: home}

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
				if r.IsGlobal {
					scope = "global"
				}
				status := ""
				if !r.TargetExists {
					status = " (target missing)"
				} else if r.Linked {
					status = " (linked)"
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
		Use:   "remove [project-path]",
		Short: "Remove the overlay repo clone for the current scope",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			if len(args) > 0 {
				target, err := resolveProjectPath(args[0])
				if err != nil {
					return renderError(err)
				}
				s.Target = target
			}

			if err := s.RepoRemove(); err != nil {
				return renderError(err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Overlay repo removed.")
			return nil
		},
	}
}

func resolveProjectPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return shimmer.GitRoot(abs)
}
