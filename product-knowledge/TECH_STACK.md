<!--
GLaDOS-MANAGED DOCUMENT
Last Updated: 2026-07-11
To modify: Edit directly.
-->

# Tech Stack

Recorded from the established codebase (adopted 2026-07-04) plus the stack implications of the
2026-07-11 complement-positioning roadmap. Guiding constraint throughout: **single static Go
binary, minimal dependencies** — the product is a tool people drop onto a laptop or Pi, not a
deployed service.

## Core

- **Language**: Go (currently 1.26), single module `goaldl`. Standard tooling only: gofmt · vet · `test -race`; no extra linters (see `standards/go/`).
- **Serial I/O**: `go.bug.st/serial` — the only hardware dependency; thin wrapper in `pkg/serial`.
- **TUI**: `charmbracelet/bubbletea` + `lipgloss` (+ `x/ansi` for width-aware truncation) — the default UX (`cmd/goaldl/tui.go`), a consumer of the core API.
- **Core API**: `pkg/stream` `Session`/`Snapshot` facade — plain serializable data, no UI; every front-end (TUI, monitor, future serve/mobile) is a consumer.
- **Persistence**: none. Files only — raw byte captures (ground truth), CSV exports, grid text files. No database; no plans for one.
- **CI/release**: GitHub Actions (gofmt/vet/build/test -race), release-please + GoReleaser (conventional commits; see `standards/release/`).

## Horizon additions (planned)

- **Horizon 1 (data prep)**: XDF parsing via stdlib `encoding/xml` (read-only — axes/scaling, never editing); exports are plain tab-delimited text (TunerPro paste format). No new dependencies expected.
- **Horizon 2 (onboard)**:
  - ESP32-S3 WiFi/serial bridge — **firmware lives outside this repo**; goaldl consumes it as a byte stream via `TCPProvider` (stdlib `net`, spec: `specs/2026-07-06_feature_tcp-provider/`).
  - `serve` adapter: stdlib `net/http` + WebSocket (smallest maintained option when needed, e.g. `nhooyr.io/websocket`/`coder/websocket` — decide at spec time) over the `Snapshot` stream.
  - Headless record mode: pure Go on the existing serial path; Pi Zero-class targets already covered by the GoReleaser matrix (linux/arm64, best-effort armv6).
- **Horizon 3 (interop)**: ADX parsing via stdlib `encoding/xml` → data-only `pkg/ecm` definitions; config persistence via a simple flat file (format decided at spec time — stdlib-parseable preferred).

## Non-stack (permanent)

No GUI toolkit (web front-end via `serve` instead); no database; no CGO where avoidable; no
bin-editing/emulation hardware libraries (out of mission scope).
