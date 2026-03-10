package cmd

import (
	"fmt"

	"github.com/siimpl/shimmer/internal/shimmer"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show symlink health for the current scope",
		Long: `Status reports whether shimmer symlinks are intact or broken (dangling).
This is purely diagnostic — no files are created or removed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := newShimmerFromFlags()
			if err != nil {
				return renderError(err)
			}

			status, err := s.Status()
			if err != nil {
				return renderError(err)
			}

			out := cmd.OutOrStdout()
			renderStatus(out, status)
			return nil
		},
	}
}

func renderStatus(w interface{ Write([]byte) (int, error) }, status *shimmer.LinkStatus) {
	total := len(status.Files)
	var broken int
	for _, f := range status.Files {
		if !f.OK {
			broken++
		}
	}

	// Header line
	if broken > 0 {
		fmt.Fprintf(w, "linked (%d files, %d broken)\n", total, broken)
	} else {
		fmt.Fprintf(w, "linked (%d files)\n", total)
	}

	// Repo info
	fmt.Fprintf(w, "  repo: %s/%s @ %s\n", status.Repo.Owner, status.Repo.Name, status.Repo.Branch)

	// File status lines
	for _, f := range status.Files {
		if f.OK {
			fmt.Fprintf(w, "  ok:      %s\n", f.Path)
		} else {
			fmt.Fprintf(w, "  BROKEN:  %s (%s — run `shimmer link` to reconcile)\n", f.Path, f.Reason)
		}
	}

	// Stashed files
	for _, s := range status.Stashed {
		fmt.Fprintf(w, "  stashed: %s (original in .git/shimmer-stash/)\n", s)
	}
}
