# shimmer eject — Design

## Summary

`shimmer eject` replaces all shimmer symlinks with copies of the files they point to, turning a live overlay into real files. The overlay repo stays intact for future use.

## Steps

1. Find all shimmer symlinks (same as `unlink` does via `findShimmerLinks()`)
2. For each symlink: read the target, remove the symlink, write a regular file copy in its place
3. Delete the stash directory (stashed files are no longer meaningful)
4. Clear `.git/info/exclude` entries (local) or `~/.shimmer/linked` (global) so ejected files are visible to `git status`

## What it doesn't do

- Doesn't touch the overlay repo or repo association
- Doesn't re-link — it operates on whatever symlinks exist right now
- No flags needed

## CLI

```
shimmer eject       # eject local scope
shimmer -g eject    # eject global scope
```

## Typical workflows

```bash
# One-shot template application
shimmer repo set git@github.com:org/template.git
shimmer link
shimmer eject

# Periodic upstream sync
shimmer git pull
shimmer link --overwrite
shimmer eject
git status  # shows changes from upstream

# Selective update (skip conflicts, eject only new files)
shimmer git pull
shimmer link --skip
shimmer eject
```

## Output

```
Ejected (3):
  CLAUDE.md
  .claude/settings.json
  .claude/skills/review.md
Stash cleared.
```

## Edge cases

- No symlinks found → "Nothing to eject."
- Broken symlink (target missing) → error, refuse to eject partially. User should fix with `shimmer status` first.
- No repo set but symlinks exist → still works (eject doesn't need the repo, just the symlinks)
