# Multi-Repo Exploration

Status: **Exploring** — not committed to. Captured here so we can come back to it.

## The Idea

Shimmer becomes a general-purpose version-tracked file overlay tool, not just for AI configs. Your home directory has layers: personal dotfiles, org standards, team Claude config, customer-specific settings. Each layer is its own repo with its own update cadence and ownership. Shimmer composes them.

## Key Changes from v1

- **`-C <path>` replaces `-g`** — target is any directory, not just "current git root" or "$HOME"
- **Multiple repos per target** — `repo add` instead of `repo set`, ordered list
- **Local repo sources** — `shimmer repo add ~/configurations/project-name` (not just git URLs)
- **Non-git targets** — `-C ~` works without `~` being a git repo
- **Priority ordering** — repos are an ordered list, later repos override earlier (or first = highest priority, TBD)

## Proposed API

```bash
shimmer [-C <path>] repo add [--first] [--after <repo>] [--before <repo>] <repo>
shimmer [-C <path>] repo list        # shows repos in priority order
shimmer [-C <path>] repo remove <repo>
shimmer [-C <path>] link
shimmer [-C <path>] unlink
shimmer [-C <path>] eject
shimmer [-C <path>] status           # shows which repo owns each file
shimmer [-C <path>] git <repo> <args...>   # git passthrough requires repo name when multiple
```

Reordering: remove + re-add with `--first`/`--before`/`--after`. Dedicated `repo order` not needed since reordering is rare.

## Conflict Resolution Between Repos

Ordered list, position = priority. Most common pattern in prior art:
- Docker Compose: `-f base.yml -f override.yml` — later wins
- Helm: `--values base.yaml --values override.yaml` — last wins
- Nix overlays: ordered list, later override earlier

`shimmer status` shows which repo owns each file.

## Arguments For

- Shimmer's API is already clean enough for general use — why limit it to one repo?
- `~` isn't a git repo, but people care deeply about what's in it
- Layers map to real organizational boundaries (personal / org / team / customer)
- The eject workflow enables pulling from upstream overlays into your own repos

## Arguments Against

- Significant rearchitecture: Shimmer struct, clone layout, state tracking all change
- "One repo per scope" eliminates an entire class of complexity
- Single-repo shimmer already works if you put everything in one overlay repo
- Multi-repo conflicts add cognitive overhead

## Prior Art

| Tool | Approach |
|------|----------|
| GNU Stow | Conflict = error (no multi-package overlap) |
| Docker Compose | Ordered file list, later wins |
| Git config | Fixed hierarchy (system < global < local) |
| Nix overlays | Ordered list, later overrides earlier |
| Helm values | Last file wins |

## Decision

Letting this marinate. Get more real-world usage with v1 to see which assumptions hold before committing to the rearchitecture.
