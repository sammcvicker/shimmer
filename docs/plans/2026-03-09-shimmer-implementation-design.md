# shimmer — Implementation Design

Decisions made before writing the implementation plan. For the full spec, see [shimmer-design.md](../2026-03-09-shimmer-design.md).

## CLI Framework

**Cobra.** The command surface is small but the UX matters — polished help text, shell completions, and consistent flag handling out of the box.

## Project Structure

```
cmd/shimmer/main.go              # Entrypoint, root Cobra command, -g flag
internal/cmd/                     # One file per command group (repo.go, link.go, status.go, git.go)
internal/shimmer/                 # All core logic — one package
internal/shimmer/shimmer_test.go  # (and friends)
docs/                             # Design docs + manual testing guide
```

**Single core package.** `internal/shimmer/` owns all logic. `internal/cmd/` is thin Cobra wiring — parse args, call core, render output. No business logic in the CLI layer.

One package avoids premature abstraction. If a natural seam emerges later, easy to split.

## Core Types

```go
// Central context for any operation
type Shimmer struct {
    Home     string // ~/.shimmer
    Global   bool   // -g flag
    Target   string // project root (git root) or $HOME
}

// What a link operation computes before acting
type LinkPlan struct {
    Links     []Link        // symlinks to create
    Removals  []string      // stale symlinks to clean up
    Conflicts []Conflict    // files that already exist
}

type Conflict struct {
    Path    string
    Tracked bool
}

// What status reports
type LinkStatus struct {
    Repo    RepoInfo
    Files   []FileStatus  // ok or broken, per file
    Stashed []string
}
```

Core functions return these types. The CLI layer renders them.

## Error Handling

**Typed errors + separate rendering.** Core package returns error values (`ErrNoRepo`, `ErrConflicts{...}`, `ErrNotLinked`). A render layer in `internal/cmd/` pattern-matches on error type and prints user-facing messages — including the multi-line conflict listing with resolution instructions from the design doc.

Keeps core logic testable without asserting on string output. Keeps UX polish in one place.

## Testing Strategy

**Real filesystem, real git.** No mocking. `t.TempDir()` for everything.

Shimmer's value proposition is "symlinks work correctly" — mocking that away defeats the purpose. Operations are fast enough with small temp dirs and tiny git repos.

Helper functions in `testutil_test.go`:
- `setupTestProject(t)` — temp dir with a git repo
- `setupTestOverlay(t, files)` — temp overlay repo with given files
- `setupShimmerHome(t)` — temp `~/.shimmer` equivalent

Tests exercise `internal/shimmer` directly. Cobra commands are thin enough to trust.

**Heavy unit + integration, light e2e.** Automated tests cover the core package. A `docs/manual-testing.md` documents scenarios for hands-on UAT: set up a real project, link, switch branches, handle conflicts, unlink.
