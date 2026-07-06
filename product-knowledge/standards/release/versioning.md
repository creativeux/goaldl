<!--
GLaDOS-MANAGED STANDARD
Last Updated: 2026-07-05
-->
# Conventional Commits drive automated semver releases

**Rule**: Commits merged to `main` MUST follow [Conventional Commits](https://www.conventionalcommits.org). The commit type determines the version bump — releasing is automated from the commit history, never hand-edited.

| Prefix | Effect (pre-1.0) | Effect (post-1.0) |
| --- | --- | --- |
| `fix:` | patch bump | patch bump |
| `feat:` | patch bump* | minor bump |
| `feat!:` / `fix!:` / `BREAKING CHANGE:` footer | **minor** bump (never auto-1.0) | major bump |
| `chore:` `docs:` `refactor:` `test:` `ci:` `build:` `perf:` `style:` | no release on their own | same |

\* Pre-1.0 the config sets `bump-minor-pre-major` + `bump-patch-for-minor-pre-major`, so `feat:` is a patch and only a breaking change bumps the minor. No commit auto-promotes to `1.0.0` — that is a deliberate manual decision.

**Release flow** (do not tag or edit `CHANGELOG.md` / the manifest by hand):
1. Land Conventional Commits on `main`.
2. [release-please](https://github.com/googleapis/release-please) maintains an open **release PR** that bumps the version and updates `CHANGELOG.md` (state tracked in `.release-please-manifest.json`; behaviour in `release-please-config.json`).
3. Merging that PR tags `vX.Y.Z` and cuts a GitHub Release.
4. [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`) builds macOS/Linux/Windows × amd64/arm64 binaries with the version injected via ldflags and appends them to the release. Dry-run with `goreleaser release --snapshot --clean`.

All three (release-please, GoReleaser, the `Release` workflow in `.github/workflows/release.yml`) fire from the same push-to-`main` event; the repo needs Actions "Read and write" permissions **and** "Allow GitHub Actions to create and approve pull requests" enabled.

**Version embedding**: `cmd/goaldl/version.go` holds `version`/`commit`/`date`, overwritten at release time by GoReleaser's `-ldflags -X main.*`. A plain `go build` leaves them at defaults and falls back to `runtime/debug.ReadBuildInfo()` (VCS revision + `+dirty`). `goaldl version` / `--version` prints the result — every binary self-identifies, released or from-source.

**Why**: The version a build reports must be derived, not asserted — a hand-typed version drifts from the commit it was cut from. Conventional Commits make the changelog and the semver bump a mechanical function of intent already recorded in each commit message, and the release PR keeps a human gate on *when* to ship without reintroducing manual version bookkeeping. Embedding via ldflags with a build-info fallback means a diagnostic tool in the field can always be traced back to exact source.
