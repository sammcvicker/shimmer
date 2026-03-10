# RFC: shimmer — Transparent Git-Backed File Overlays

## The Ask

Build `shimmer`, a small Go CLI that lets Siimpl engineers share and switch between AI tool configurations via git-backed symlink overlays. Estimated scope: a few focused days for the core commands, internal use only.

## Why This, Specifically

Siimpl puts smart technical people on teams that need them. If a client needed AI automation, they already have the domain expertise to build it better than we could — they hired us for the people, not the bots. And if we built the automation anyway, we'd be handing them a reason to end the engagement. What Siimpl actually needs is more of what it already sells: engineers who are exceptionally good, now amplified by AI. That means collaborating on how we use these tools — sharing what works, learning from each other, and iterating together — not building autonomous systems that replace us.

## The Problem

AI coding tools are configured via files in projects (`.claude/`, `.cursorrules`) and globally (`~/.claude/`). Today there are two options for managing these, and both are bad:

- **Repos you own:** commit AI config to the repo. Now everyone's on the same config and experimentation is impossible — you can't try a different CLAUDE.md without either branching the entire project or stepping on your teammates' setup. And the config is coupled to the project's own review process, so iterating on a skill means PRs into the main codebase.
- **Repos you don't own:** there is no option. You maintain config locally, it lives only on your machine, and sharing means pasting markdown in Slack.

In both cases, there's no way to try a teammate's setup, share what's working, or build institutional knowledge about effective AI configuration.

## The Solution

Think GNU Stow, but project-aware, GitHub-native, and with much better ergonomics.

Dumb simple. Git does the hard work. Shimmer is just symlinks.

A shimmer overlay repo is a git repository whose file tree mirrors your project. `shimmer link` walks that repo and creates per-file symlinks at the corresponding paths in the project — invisible to the project's own git history. Engineers share configurations through branches, try each other's setups with a single command, and keep everything versioned and reviewable.

```
# Overlay repo                         # Project (after shimmer link)

CLAUDE.md                        ->    <project>/CLAUDE.md
.claude/
  settings.json                  ->    <project>/.claude/settings.json
  skills/
    review.md                    ->    <project>/.claude/skills/review.md
.cursorrules                     ->    <project>/.cursorrules
```

It's a completely generic tool — it doesn't know or care what files it's overlaying. It happens to be exactly what we need for AI configurations right now. As long as these tools can be configured through files at all, shimmer remains useful.

## What It Looks Like

```bash
# Set an overlay repo for this project
shimmer repo set git@github.com:siimpl/claude-dhi.git

# Link — symlinks each file from the overlay into the project
shimmer link

# Try a teammate's branch
shimmer git checkout sarah/strict-rules
shimmer link

# Pull latest
shimmer git pull
shimmer link

# Check symlink health
shimmer status

# Undo everything — restores the project to its pre-shimmer state
shimmer unlink
```

Two scopes: local (per-project, default) and global (`shimmer -g`, targets `$HOME`). `shimmer git` passes commands through to the overlay clone so you never need to `cd` anywhere.

## Design Principles

- **Dumb simple.** Git does the hard work. Shimmer is just symlinks.
- **One repo per scope.** One overlay per project, one globally. No disambiguation.
- **Git-native collaboration.** Branching, PRs, diffs. No new collaboration model to learn.
- **Non-destructive.** Existing files are stashed, never deleted. `shimmer unlink` always restores.
- **Tool-agnostic.** Mirrors files. No knowledge of Claude, Cursor, or any specific tool.
- **No config files.** The filesystem is the database. Everything is discoverable from `~/.shimmer/repos/` and symlink scanning. Nothing to get out of sync.
- **Not a git wrapper.** Shimmer never touches clone git state. You manage the clone; shimmer links whatever's there.

## Failure States

We've thought through the non-happy paths. Two are worth highlighting:

**Existing files conflict with overlay.** If the project already has files at paths shimmer wants to link (tracked or untracked), `shimmer link` fails immediately and shows the user their options: `--skip` (link only non-conflicting files), `--overwrite` (stash and shadow, with clear warnings about skip-worktree fragility for tracked files), or `git rm --cached` first (the recommended permanent fix). No silent clobbering.

**Project moved.** Shimmer encodes the project path in its clone directory structure. If a project moves, `shimmer repo list` shows the old path as "project not found on disk." Resolution: `shimmer repo set` from the new location, clean up the old clone.

Everything else (broken symlinks, unlink when not linked, branch switches changing the file set) is handled by `shimmer link` reconciling from scratch every time it runs.

## Prior Art

The closest existing tool is **GNU Stow** — a symlink farm manager from the 90s that mirrors a directory tree into a target via per-file symlinks. If you know Stow, shimmer's mental model is immediately familiar.

What's genuinely new: per-project scope alongside global (Stow only targets one directory), team workflow via git branches (Stow has no opinion about git), and stash-and-restore (Stow refuses or clobbers).

Dotfile managers (chezmoi, yadm, dotbot) solve a different problem — managing one person's `$HOME` environment, not enabling a team to collaborate on project-level overlays.

## What shimmer is NOT

- **Not a sync tool.** No auto-pull, auto-push, or file watching.
- **Not a config generator.** Engineers write the files.
- **Not a package manager.** No dependency resolution, no versioning scheme, no lock files.
- **Not tool-specific.** It doesn't know about Claude Code, Cursor, or Copilot.

## Implementation

- **Language:** Go. Single binary, easy distribution, team already uses it.
- **Dependencies:** Just git on PATH. No Docker, no daemon, no network services.
- **Scope:** Internal to Siimpl. Open source later if it proves valuable.
