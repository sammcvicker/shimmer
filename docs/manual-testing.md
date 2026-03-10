# shimmer — Manual Testing Guide

Scenarios to walk through by hand for UAT and "does it feel right" validation.

## Prerequisites

- `shimmer` binary built and on PATH (`make install`)
- A git repository to use as a test overlay (or create one on GitHub)
- A git project to overlay onto

## Scenario 1: Basic Link and Unlink

1. Create a test overlay repo on GitHub with a `CLAUDE.md` file
2. Navigate to a project: `cd ~/projects/some-project`
3. Set the overlay: `shimmer repo set <url>`
4. Link: `shimmer link`
5. Verify: `ls -la CLAUDE.md` — should be a symlink
6. Check status: `shimmer status`
7. Unlink: `shimmer unlink`
8. Verify: `ls -la CLAUDE.md` — should not exist

## Scenario 2: Conflict Handling

1. Create a `CLAUDE.md` in the project (not tracked by git)
2. Run `shimmer link` — should fail with conflict error
3. Run `shimmer link --skip` — should link everything except CLAUDE.md
4. Run `shimmer unlink`
5. Run `shimmer link --overwrite` — should stash and shadow
6. Verify stash: check `.git/shimmer-stash/CLAUDE.md`
7. Run `shimmer unlink` — original should be restored

## Scenario 3: Branch Switching

1. `shimmer link`
2. `shimmer git branch -a` — see available branches
3. `shimmer git checkout <other-branch>`
4. `shimmer link` — should reconcile (remove old links, create new ones)
5. `shimmer status` — verify health

## Scenario 4: Global Scope

1. `shimmer -g repo set <url>`
2. `shimmer -g link`
3. `ls -la ~/.claude/` — should contain symlinks
4. `shimmer -g status`
5. `shimmer -g unlink`

## Scenario 5: Repo Management

1. `shimmer repo set <url>`
2. `shimmer repo path` — should print the clone path
3. `shimmer repo list` — should show the repo
4. `shimmer repo remove` — should remove
5. `shimmer repo list` — should be empty

## Scenario 6: Git Passthrough

1. `shimmer repo set <url>`
2. `shimmer git status`
3. `shimmer git log --oneline -5`
4. `shimmer git pull`

## What to Look For

- Error messages are clear and actionable
- Help text is useful (`shimmer --help`, `shimmer link --help`)
- Tab completion works (if installed)
- No leftover files after unlink
- Re-linking is truly idempotent
- Stash/restore preserves file content exactly
