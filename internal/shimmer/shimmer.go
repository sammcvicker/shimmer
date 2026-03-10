package shimmer

import (
	"os"
	"path/filepath"
)

// Shimmer is the central context for any operation.
type Shimmer struct {
	Home   string // ~/.shimmer
	Global bool   // -g flag
	Target string // project root (git root) or $HOME
}

// DefaultHome returns the default shimmer home directory.
func DefaultHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".shimmer")
}

// Link represents a single symlink to create.
type Link struct {
	Src  string // path in the clone
	Dest string // path in the project
}

// LinkPlan is what a link operation computes before acting.
type LinkPlan struct {
	Links     []Link
	Removals  []string   // stale symlinks to clean up
	Conflicts []Conflict // files that already exist at destination
}

// Conflict is an existing file that would be shadowed by a link.
type Conflict struct {
	Path    string
	Tracked bool
}

// LinkResult is what link returns after executing.
type LinkResult struct {
	Linked  []string
	Skipped []string
	Removed []string
	Stashed []string
}

// FileStatus is the health of a single linked file.
type FileStatus struct {
	Path   string
	OK     bool
	Reason string // empty if OK, e.g. "target missing" if broken
}

// RepoInfo is metadata about an overlay repo clone.
type RepoInfo struct {
	Owner        string
	Name         string
	RemoteURL    string
	TargetPath   string // project path or "_global"
	Branch       string
	ClonePath    string
	Linked       bool
	TargetExists bool
}

// LinkStatus is what shimmer status returns.
type LinkStatus struct {
	Repo    RepoInfo
	Files   []FileStatus
	Stashed []string
}
