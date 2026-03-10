package shimmer

import "fmt"

// ErrNoRepo means no overlay repo is set for this scope.
type ErrNoRepo struct {
	Target string
	Global bool
}

func (e *ErrNoRepo) Error() string {
	if e.Global {
		return "no overlay repo set for global scope"
	}
	return fmt.Sprintf("no overlay repo set for %s", e.Target)
}

// ErrConflicts means existing files would be shadowed.
type ErrConflicts struct {
	Conflicts []Conflict
}

func (e *ErrConflicts) Error() string {
	return fmt.Sprintf("%d file(s) already exist and would be shadowed", len(e.Conflicts))
}

// ErrNotLinked means there are no shimmer symlinks to operate on.
type ErrNotLinked struct{}

func (e *ErrNotLinked) Error() string {
	return "not linked"
}

// ErrNotInGitRepo means a local-scope command was run outside a git repo.
type ErrNotInGitRepo struct{}

func (e *ErrNotInGitRepo) Error() string {
	return "not in a git repository (use -g for global scope)"
}

// ErrRepoAlreadySet means a repo is already configured for this scope.
type ErrRepoAlreadySet struct {
	RemoteURL string
	ClonePath string
}

func (e *ErrRepoAlreadySet) Error() string {
	return fmt.Sprintf("overlay repo already set: %s", e.RemoteURL)
}
