# shimmer

Git-backed and hot-swappable symlink overlays for sharing configurations across your team.

## Why

AI coding tools are configured via files in projects and globally (`~/.claude/`, `.cursorrules`). Today there's no good way to share these across a team, try a teammate's setup, or iterate on configurations without committing them to the project repo.

Shimmer keeps overlay files in a separate Git repo and symlinks them into place. Engineers collaborate on configurations through branches and PRs, try each other's setups with a single command, and keep everything versioned and reviewable.

## How it works

An overlay repo mirrors the file tree of your project. `shimmer link` walks the overlay and creates per-file symlinks at the corresponding paths:

```
# Overlay repo                         # Project (after shimmer link)

CLAUDE.md                        →    <project>/CLAUDE.md
.claude/
  settings.json                  →    <project>/.claude/settings.json
  skills/
    review.md                    →    <project>/.claude/skills/review.md
.cursorrules                     →    <project>/.cursorrules
```

Shimmer is tool-agnostic — it doesn't know or care what files it's overlaying.

## Install

Requires Go 1.25+ and `git`.

```bash
go install github.com/sammcvicker/shimmer@latest
```

Or build from source:

```bash
make build    # ./shimmer
make install  # $GOPATH/bin/shimmer
```

## Quick start

```bash
# Set an overlay repo for this project
shimmer repo set git@github.com:your-org/overlay-repo.git

# Link overlay files into the project
shimmer link

# Try a teammate's branch
shimmer git checkout sarah/strict-rules
shimmer link

# Pull latest changes
shimmer git pull
shimmer link

# Check symlink health
shimmer status

# Undo everything — restores the project to its pre-shimmer state
shimmer unlink
```

## Commands

```
shimmer repo set <url> [path]    Clone an overlay repo for the current project
shimmer repo path                Print the overlay clone path
shimmer repo list                List all overlay repos
shimmer repo remove              Remove the overlay repo (unlinks first)

shimmer link [--skip|--overwrite]  Create symlinks from overlay into project
shimmer unlink                     Remove symlinks and restore stashed files
shimmer eject                      Replace symlinks with file copies (keeps repo)
shimmer status                     Show symlink health and repo info

shimmer git <args...>            Run git commands against the overlay clone
```

### Global scope

All commands default to local scope (current project). Use `-g` to target `$HOME` instead:

```bash
shimmer -g repo set git@github.com:your-org/global-config.git
shimmer -g link
shimmer -g status
```

### Conflict handling

When an overlay file would shadow an existing project file, `shimmer link` fails and lists the conflicts. You have three options:

- **`--skip`** — link only non-conflicting files
- **`--overwrite`** — stash existing files and replace them with symlinks
- **`git rm --cached`** — remove tracked files from the project first (recommended for tracked files)

Stashed files are always restored by `shimmer unlink`.

### Ejecting

Use `shimmer eject` to materialize symlinks into real files you can commit. The overlay repo stays intact for future updates.

```bash
# Pull upstream changes, link, and eject into the project
shimmer git pull
shimmer link --overwrite
shimmer eject
git status  # ejected files show up as changes
```

## .shimmerignore

Place a `.shimmerignore` file in the overlay repo root to exclude files from linking. Uses gitignore syntax.

```
# .shimmerignore
README.md
LICENSE
```

`.shimmerignore`, `.git/`, and `.gitignore` are always excluded.

## Design principles

- **Dumb simple.** Git does the hard work. Shimmer is just symlinks.
- **Non-destructive.** Existing files are stashed, never deleted. `unlink` always restores.
- **Git-native collaboration.** Branches, PRs, diffs — no new workflow to learn.
- **Tool-agnostic.** Mirrors files. No knowledge of any specific tool baked in.
- **No config files.** The filesystem is the database. Everything discoverable from `~/.shimmer/`.

## License

MIT
