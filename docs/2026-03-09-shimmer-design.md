# shimmer — Design Document

Implementation reference for shimmer. For context, motivation, and the high-level pitch, see [shimmer-rfc.md](./2026-03-09-shimmer-rfc.md).

## Design Principles

- **Dumb simple.** Git does the hard work. `shimmer` is just symlinks.
- **One repo per scope.** One overlay repo per project (local), one globally. No disambiguation headaches.
- **Git-native collaboration.** Branching, PRs, diffs — all the stuff engineers already know. No new collaboration model.
- **Non-destructive.** Existing project files are stashed, never deleted. Unlinking restores the original state.
- **Tool-agnostic.** The repo layout mirrors the target. No knowledge of `.claude/`, `.cursor/`, or any specific tool baked in.
- **No config files.** The filesystem is the database. Everything is discoverable from `~/.shimmer/repos/` and symlink scanning.
- **Not a git wrapper.** Shimmer never touches clone git state. If the clone is dirty, detached, or on an unexpected branch, that's what gets linked. Use `shimmer git` to manage it yourself.

## Core Mental Model

A shimmer repo is a transparent overlay on your project (or `$HOME` for global). The repo's file tree mirrors the destination. `shimmer link` walks the repo, skipping anything in `.shimmerignore`, and creates per-file symlinks at the corresponding paths in the project.

```
# Overlay repo                         # Project (after shimmer link)

CLAUDE.md                        ->    <project>/CLAUDE.md
.claude/
  settings.json                  ->    <project>/.claude/settings.json
  skills/
    review.md                    ->    <project>/.claude/skills/review.md
.cursorrules                     ->    <project>/.cursorrules
.shimmerignore                         (skipped)
README.md                              (skipped, listed in .shimmerignore)
```

Per-file symlinks (not directory symlinks) so the project can have its own files alongside the overlaid ones. The overlay repo doesn't need to know what tools it's configuring — it just mirrors the file structure.

### .shimmerignore

Gitignore-syntax file at the repo root. Files matching these patterns are not linked. Always implicitly includes `.shimmerignore`, `.git/`, and `.gitignore`.

```
# .shimmerignore
README.md
LICENSE
```

## Command API

### Scope flag

All commands default to local scope (current project, discovered via git root). The `-g` flag switches to global scope (`$HOME`). It's a top-level flag on `shimmer` itself, not on individual subcommands. Global scope works anywhere — no git repository required.

```
shimmer <command>           # local scope (current project)
shimmer -g <command>        # global scope ($HOME)
```

### Repository management

```
shimmer repo set <url> [<project-path>]
```
Clone an overlay repo into `~/.shimmer/repos/`. `<project-path>` defaults to `.` (discovers the git root). If a repo is already set for this scope, prompts for confirmation before replacing. The repo owner and name are discovered from the remote URL.

```
shimmer repo list
```
Walk `~/.shimmer/repos/` and show everything shimmer knows about: repo name, URL (from clone's git remote), target project path (from clone's filesystem path), linked status (from symlink scan), and current branch. This is always a global view.

```
shimmer repo remove [<project-path>]
```
Unlinks first if linked. Deletes the clone directory for the given project (default `.`) or global scope. Prompts for confirmation.

```
shimmer repo path
```
Print the absolute filesystem path to the clone for the current scope. Useful for `cd $(shimmer repo path)`, opening in an editor, or running git commands directly.

### Linking

```
shimmer link [--skip] [--overwrite]
```
Reconcile symlinks between the overlay repo clone and the project (or `$HOME` with `-g`). This is the only linking operation — it handles both initial linking and updates after branch switches.

**What it does:**
1. Finds the clone for the current scope by looking up the project's git root path in `~/.shimmer/repos/`
2. Scans the project for any existing symlinks pointing into `~/.shimmer/repos/` and removes them (cleaning up the previous link state)
3. Walks the repo clone, collecting all files not excluded by `.shimmerignore`
4. For each file, checks if the destination already exists and is not a symlink into the clone
   - If conflicts are found and neither flag is set: **fails immediately** with an error listing the conflicting files and instructions:
     ```
     Error: these files already exist and would be shadowed:
       CLAUDE.md          (tracked)
       .cursorrules       (untracked)

     Options:
       --skip        Link only non-conflicting files, leave existing ones in place
       --overwrite   Stash existing files and shadow them (tracked files use
                     skip-worktree, which is fragile — see docs)

     To permanently resolve tracked file conflicts (recommended):
       git rm --cached CLAUDE.md
       shimmer link

     To undo any shimmer operation:
       shimmer unlink
     ```
   - **`--skip`**: skip any file that already exists at the destination (tracked or untracked). Links everything else. Good for overlaying only what's missing.
   - **`--overwrite`**: stash existing files and shadow them. Untracked files are simply stashed. Tracked files are stashed and `git update-index --skip-worktree` is set on each — this works but is fragile (`git stash`, `git checkout --force`, and `git reset` can silently undo it). Shimmer warns about this on every overwrite of tracked files.
5. Creates parent directories as needed
6. Creates symlinks: `<project>/path/to/file` -> `<clone>/path/to/file`
7. Updates the project's `.git/info/exclude` with the current set of linked paths (local only) — this is git's built-in per-clone ignore mechanism, never committed, so the project's `.gitignore` is untouched. Prevents `git status` from showing the symlinks as untracked files.
8. Prints a summary: files linked, files skipped, files removed since last link, stashed originals

Because `link` always reconciles from scratch, it's safe to run at any time — after a branch switch, after pulling, or just to verify state.

```
shimmer unlink
```
Remove all symlinks pointing into `~/.shimmer/repos/`. Restore any stashed originals. Remove `.git/info/exclude` entries. Reverse any `--skip-worktree` flags. This is the escape hatch — if anything goes wrong with a link, `shimmer unlink` restores the project to its pre-shimmer state.

### Status

```
shimmer status
```
Show symlink health for the current scope (local by default, global with `-g`). This is purely about the symlinks — are they intact or dangling? Use `shimmer git status` for the clone's git state.

```
linked (3 files)
  repo: siimpl/claude-dhi @ main
  ok:  CLAUDE.md
  ok:  .claude/settings.json
  ok:  .claude/skills/review.md
  stashed: CLAUDE.md (original in .git/shimmer-stash/)
```

Or when something's wrong:

```
linked (3 files, 1 broken)
  repo: siimpl/claude-dhi @ main
  ok:      CLAUDE.md
  ok:      .claude/settings.json
  BROKEN:  .claude/skills/review.md (target missing — run `shimmer link` to reconcile)
```

### Git passthrough

```
shimmer git <args...>
```
Runs `git -C <clone-path> <args...>` against the overlay repo clone. Respects `-g` for global scope. This is the primary way to interact with the clone's git state — no need to `cd` anywhere.

```bash
shimmer git status              # git status of local overlay clone
shimmer git branch -a           # list all branches
shimmer git checkout sam/new    # switch branches
shimmer git pull                # pull latest
shimmer git log --oneline -5    # recent commits
shimmer -g git status           # git status of global overlay clone
```

### Examples

```bash
# Set the global overlay repo
shimmer -g repo set git@github.com:siimpl/claude-global.git

# Set a project's overlay repo (from within the project)
shimmer repo set git@github.com:siimpl/claude-dhi.git

# Or set it from elsewhere, specifying the project path
shimmer repo set git@github.com:siimpl/claude-dhi.git ~/projects/dhi

# Link — symlinks each file from the clone into the project
shimmer link

# Check symlink health
shimmer status

# See what branches are available
shimmer git branch -a

# Try a teammate's branch — just switch and re-link
shimmer git checkout sarah/strict-rules
shimmer link

# Pull latest and re-link
shimmer git pull
shimmer link

# Check the clone's git state
shimmer git status

# Open the overlay repo in your editor
vim $(shimmer repo path)

# Project moved? Re-register from the new location, clean up the old one.
cd ~/projects/new-dhi
shimmer repo set git@github.com:siimpl/claude-dhi.git
shimmer link
shimmer repo remove ~/projects/old-dhi    # clean up orphaned clone
```

## Data Model

### On-disk layout

The filesystem is the entire database. No config files.

```
~/.shimmer/
├── stash/                                          # stashed global originals
│   └── .claude/
│       └── CLAUDE.md
└── repos/
    └── <owner>/
        └── <repo>/
            ├── _global/                            # clone for global scope
            └── <project-path-without-leading-slash>/
                                                    # clone per project
```

Example:

```
~/.shimmer/repos/
├── siimpl/
│   ├── claude-global/
│   │   └── _global/                                # global overlay clone
│   └── claude-dhi/
│       ├── Users/sam/projects/dhi/                 # clone for /Users/sam/projects/dhi
│       └── Users/sam/projects/api-server/          # clone for /Users/sam/projects/api-server
└── other-org/
    └── claude-configs/
        └── Users/sam/projects/foo/                 # clone for /Users/sam/projects/foo
```

Each leaf directory containing a `.git/` is a clone. Everything is derivable:

- **Repo owner/name:** from the path under `~/.shimmer/repos/` (first two segments)
- **Remote URL:** from `git remote get-url origin` in the clone
- **Target project path:** from the path after `<owner>/<repo>/`, prepend `/` (or `_global` = global scope)
- **Current branch:** from `git` in the clone
- **Linked status:** scan the target project for symlinks pointing into this clone

### How discovery works

`shimmer repo list` walks `~/.shimmer/repos/`, finds every directory containing `.git/`, and derives all metadata from the path and the clone's git state. No index to maintain or get stale.

`shimmer link` (and all scoped commands) find the right clone by computing the expected path: `~/.shimmer/repos/<owner>/<repo>/<current-project-path>/` and checking if it exists.

`shimmer status` and `shimmer unlink` find linked files by scanning the project for symlinks whose target is inside `~/.shimmer/repos/`.

### Symlink structure

Per-file symlinks, preserving the repo's directory structure:

```
~/projects/dhi/CLAUDE.md                -> ~/.shimmer/repos/siimpl/claude-dhi/Users/sam/projects/dhi/CLAUDE.md
~/projects/dhi/.claude/settings.json    -> ~/.shimmer/repos/siimpl/claude-dhi/Users/sam/projects/dhi/.claude/settings.json
~/projects/dhi/.claude/skills/review.md -> ~/.shimmer/repos/siimpl/claude-dhi/Users/sam/projects/dhi/.claude/skills/review.md
```

Editors follow symlinks, so diffs show changes relative to the overlay repo. The project can have its own non-overlapping files alongside linked ones.

### Stash location

When `shimmer link` encounters existing files at destination paths, they're stashed preserving their relative path:

- **Local:** `<project>/.git/shimmer-stash/CLAUDE.md`, `<project>/.git/shimmer-stash/.claude/settings.json`, etc.
- **Global:** `~/.shimmer/stash/.claude/CLAUDE.md`, etc.

Stash is inside `.git/` (local) so it's invisible to the project's git status.

## Failure States

### Existing files conflict with overlay

The project repo tracks `CLAUDE.md` or `.claude/` files, or has untracked files at paths shimmer wants to link.

`shimmer link` fails immediately, listing all conflicting files (both tracked and untracked). The user must choose:

- **`--skip`**: link only non-conflicting files, leave existing files in place. Simplest option — the project keeps its own versions.
- **`--overwrite`**: stash all existing files and shadow them. Tracked files additionally get `--skip-worktree`, which is fragile — `git stash`, `git checkout --force`, and `git reset` can silently undo it. Shimmer warns about this on every overwrite of tracked files.
- **Recommended**: `git rm --cached <files>` first to stop tracking the files, then `shimmer link` cleanly. This is the permanent fix.

**On unlink:** always restores — `--no-skip-worktree` where applied, stashed files restored, exclude entries removed.

### Project moved to a new path

The clone lives at `~/.shimmer/repos/siimpl/claude-dhi/Users/sam/projects/dhi/`. If the project moves, symlinks in the new location don't exist. `shimmer repo list` shows the old path with "project not found on disk" status.

**Resolution:** `shimmer repo set <same-url>` from the new location creates a fresh clone. `shimmer repo remove ~/projects/old-dhi` cleans up the orphan. The examples section shows this workflow.

### Everything else

Broken symlinks from a deleted clone are cleaned up by `shimmer link` (which reconciles from scratch) or `shimmer unlink`. Local scope requires being inside a git repo; global scope (`-g`) works anywhere. `shimmer unlink` when not linked is a no-op.

## Implementation Notes

- **Language:** Go (single binary, easy cross-platform distribution, team already uses it)
- **Dependencies:** Just git on PATH. No Docker, no daemon, no network services.
- **Symlink strategy:** Per-file symlinks, not directory symlinks. Allows project files to coexist alongside linked files.
- **Discovery over state:** No config files, no tracking files. The filesystem layout under `~/.shimmer/repos/` encodes all metadata. Symlink scanning discovers link state. Can't go stale.

## Future Work

- `shimmer repo init` — scaffold a new overlay repo from a template. Deferred until we have real overlay repos to learn from.
