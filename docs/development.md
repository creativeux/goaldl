# Development

## Build & run

```bash
go build ./cmd/goaldl      # build the binary
go run ./cmd/goaldl        # or run directly; prints available commands
go vet ./...               # static checks
go fmt ./...               # format
```

Pure Go, no CGO — one codebase cross-compiles to every supported platform.

## Project layout

```
cmd/goaldl/            binary: main.go (dispatch: command word → that command,
                       else → dashboard) + tui.go + capture/monitor/blm/csv
pkg/decoder/           The decoder (byte-value state machine) + synthetic encoder + tests
    testdata/          Real raw captures + golden frame dumps — the root of the test suite
pkg/stream/            Core engine: Session → Snapshot (the reusable API any front-end drives)
                       + Provider abstraction (replay/serial) + terminal view builders
pkg/blm/               BLM fuel-trim accumulator (RPM × MAP grid, averages, correction)
pkg/ecm/               ECM definitions, frame parsing, and fuel-trim extraction (GM 1227747 per A033.ads)
pkg/serial/            Thin serial-port wrapper (open/read/flush/list) — no decoding
pkg/aldl/              Shared Frame type
pkg/errors/            Error types
data/                  Reference captures and A033.ads ECM definition
docs/history/          Superseded debugging notes, kept for context
```

## Testing

```bash
go test ./...
```

The suite is rooted in real captures under `pkg/decoder/testdata/`:
`TestDecodeRealCapture` asserts exact decode stats and 100% PROM-ID match on the
idle and drive recordings, and `TestGolden` pins the exact decoded frame bytes.
After an intentional decoder change, regenerate the golden files with:

```bash
go test ./pkg/decoder -run TestGolden -update   # then review the diff before committing
```

## Releases & versioning

Every binary self-reports its build: `goaldl version` (or `--version`). Released
builds carry a semantic version + commit; a plain `go build` from source falls
back to the VCS revision the Go toolchain stamps in.

Versioning is automated from [Conventional Commits](https://www.conventionalcommits.org)
— **don't tag, bump versions, or edit `CHANGELOG.md` by hand:**

- Commit with `feat:` / `fix:` / `feat!:` (breaking) prefixes on `main`.
- [release-please](https://github.com/googleapis/release-please) keeps an open
  "release PR" that bumps the version and updates `CHANGELOG.md`. Merging it
  tags `vX.Y.Z` and cuts a GitHub Release.
- [GoReleaser](https://goreleaser.com) then builds the macOS/Linux/Windows
  (amd64 + arm64) binaries — version baked in via ldflags — and attaches them to
  that release. Dry-run locally with `goreleaser release --snapshot --clean`.

Pre-1.0, breaking changes bump the minor (never 1.0) per the config. The full
standard is in `product-knowledge/standards/release/versioning.md`.

## Platform support

The full tiered platform matrix (core / best-effort / non-targets) and the
embedded-microcontroller story live in
[`../product-knowledge/standards/release/platform-support.md`](../product-knowledge/standards/release/platform-support.md).
The supported-platform matrix and per-OS driver notes are in the
[README → Platform support](../README.md#platform-support).
