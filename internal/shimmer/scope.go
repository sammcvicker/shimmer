package shimmer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Scope encapsulates behavior that differs between local (project) and global ($HOME) mode.
type Scope interface {
	Target() string
	IsGlobal() bool
	ScopeLabel() string
	StashDir() string
	ClonePath(home, owner, repo string) string
	MatchClone(targetSegment string) bool
	SaveLinkState(paths []string) error
	TrackedFiles(rels []string) map[string]bool
	SetSkipWorktree(rel string, set bool) error
}

// LocalScope operates against a single git repository.
type LocalScope struct {
	target string // git repo root
}

func NewLocalScope(target string) *LocalScope {
	return &LocalScope{target: target}
}

func (l *LocalScope) Target() string     { return l.target }
func (l *LocalScope) IsGlobal() bool     { return false }
func (l *LocalScope) ScopeLabel() string { return l.target }

func (l *LocalScope) StashDir() string {
	return filepath.Join(l.target, ".git", "shimmer-stash")
}

func (l *LocalScope) ClonePath(home, owner, repo string) string {
	rel := strings.TrimPrefix(l.target, "/")
	return filepath.Join(home, "repos", owner, repo, rel)
}

func (l *LocalScope) MatchClone(targetSegment string) bool {
	return "/"+targetSegment == l.target
}

func (l *LocalScope) SaveLinkState(paths []string) error {
	return updateGitExclude(l.target, paths)
}

func (l *LocalScope) TrackedFiles(rels []string) map[string]bool {
	if len(rels) == 0 {
		return nil
	}
	args := append([]string{"-C", l.target, "ls-files"}, rels...)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	tracked := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			tracked[line] = true
		}
	}
	return tracked
}

func (l *LocalScope) SetSkipWorktree(rel string, set bool) error {
	flag := "--skip-worktree"
	if !set {
		flag = "--no-skip-worktree"
	}
	cmd := exec.Command("git", "-C", l.target, "update-index", flag, rel)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("skip-worktree %s: %s: %w", rel, out, err)
	}
	return nil
}

// GlobalScope operates against $HOME.
type GlobalScope struct {
	target string // $HOME
	home   string // ~/.shimmer
}

func NewGlobalScope(target, home string) *GlobalScope {
	return &GlobalScope{target: target, home: home}
}

func (g *GlobalScope) Target() string     { return g.target }
func (g *GlobalScope) IsGlobal() bool     { return true }
func (g *GlobalScope) ScopeLabel() string { return "global" }

func (g *GlobalScope) StashDir() string {
	return filepath.Join(g.home, "stash")
}

func (g *GlobalScope) ClonePath(home, owner, repo string) string {
	return filepath.Join(home, "repos", owner, repo, "_global")
}

func (g *GlobalScope) MatchClone(targetSegment string) bool {
	return targetSegment == "_global"
}

func (g *GlobalScope) SaveLinkState(paths []string) error {
	file := filepath.Join(g.home, "linked")
	if len(paths) == 0 {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(file, []byte(strings.Join(paths, "\n")+"\n"), 0o644)
}

func (g *GlobalScope) TrackedFiles(rels []string) map[string]bool {
	return nil // no git repo at $HOME
}

func (g *GlobalScope) SetSkipWorktree(rel string, set bool) error {
	return nil // no-op for global scope
}
