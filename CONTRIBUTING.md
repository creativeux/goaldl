# Contributing to goaldl

Thanks for your interest in goaldl — a cross-platform Go ALDL scanner/datalogger for
pre-OBD2 GM ECMs. This guide covers the development setup and the review flow.

## Development setup

- **Go 1.26+** (the module pins the toolchain via `go.mod`).
- Build & run: `go build ./cmd/goaldl`, then `go run ./cmd/goaldl help`.
- The bare `goaldl` command launches the interactive TUI dashboard; see `README.md` and
  `docs/development.md` for the full command set.

### The gate — run this before every push

```
go fmt ./... && go vet ./... && go build ./... && go test -race ./...
```

This is exactly what CI enforces (`.github/workflows/ci.yml`). `gofmt` must produce zero
diffs. No other linters or build tooling are used — keep contributions reproducible with
nothing but the Go toolchain.

**Golden fixtures.** The decoder test suite is rooted in real-hardware captures
(`pkg/decoder/testdata/*.raw`) and their `.golden` frame dumps. These must stay
byte-identical. Only regenerate them (`go test ./pkg/decoder -run TestGolden -update`)
for a *deliberate* decoder change, and review the diff before committing.

## Commits

Commits on `main` **must** follow [Conventional Commits](https://www.conventionalcommits.org)
(`feat:`, `fix:`, `feat!:` / `BREAKING CHANGE:`, or `docs:`/`refactor:`/`test:`/`chore:`/
`ci:`/`build:`). The type drives automated versioning — releases are cut by release-please
from commit history, so **don't** hand-edit `CHANGELOG.md`, version files, or tags. Details:
`product-knowledge/standards/release/versioning.md`.

## Pull request flow

1. Branch off `main` and open a PR. `main` is protected: direct pushes are rejected, and
   a merge needs **one human approval** plus a passing agent review.
2. Fill in the PR template completely — the automated reviewer checks it.
3. An **automated agent reviews your PR first** (usually within a minute) and posts a
   structured comment covering:
   - description completeness,
   - compliance with the project's standards and philosophies
     (`product-knowledge/standards/`, `product-knowledge/philosophies/`),
   - alignment with the relevant `specs/<work-unit>/` criteria, if any,
   - a PM / Architect / QA quick-take.
4. The agent publishes a `claude-review` status check. It **only blocks the merge on
   `must`-severity findings** (e.g. a raw-data-policy violation in the decode path);
   warnings and nits are advisory. A human still gives the final approval.

### Contributing from a fork

Fork PRs don't get the review automatically (GitHub withholds the credentials the agent
needs on fork runs). A maintainer will kick off the review by commenting
**`@claude review`** on your PR. Feel free to ping if it hasn't happened.

## What the reviewer cares about most

These are the load-bearing invariants — worth checking yourself before opening a PR:

- **Raw-data policy** — no plausibility filtering, outlier rejection, or smoothing in the
  decode path. Emit every structurally-aligned frame; quality signals ride alongside as
  fields, never as gates that drop frames.
- **Session/API layering** — front-ends consume `pkg/stream.Session` → `Snapshot`; a
  consumer never reaches into `pkg/decoder` or `pkg/ecm`. For consumer-only work, that
  seam (plus `go.mod`) stays untouched.
- **Byte-value decoding** — ALDL bits come from UART byte *values*, never host-side timing.

See `product-knowledge/standards/index.yml` for the full catalog and the
`product-knowledge/personas/` review lenses.
