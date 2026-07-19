<!-- SDA: v1.0 -->
# Evaluation Brief: TCPProvider

Self-contained brief for a fresh evaluator. Everything you need is referenced by path from the
repo root (`/Users/aaronstone/Development/aldl/goaldl`).

## Section A: What Was Requested

Read `specs/2026-07-06_feature_tcp-provider/requirements.md` in full — goal, functional
requirements **R1–R12**, non-goals, and success criteria 1–6. These are the contract.

## Section B: What Was Agreed To

Read `specs/2026-07-06_feature_tcp-provider/spec.md` — the technical spec. Key agreed points:

- `pkg/stream/tcp.go` (new): `TCPProvider{Addr, Config, Sink, DialTimeout, ReadTimeout}` with
  `Name() = "tcp:"+Addr`, `Bytes()`, `Reconnecting()`, `ReconnectAttempt()`, `Run(ctx, emit)` —
  method set mirrors `SerialProvider` exactly (§2).
- Run loop mirrors serial; redial loop retries a **fixed Addr** (no port rescan) at
  `reconnectInterval`; decoder rebuilt after a gap (§3.1–3.2).
- Cancellation: `DialContext` + rolling read deadline + a per-connection cancel-closer goroutine;
  cancel latency ~0, tested (§3.3, T4).
- **Decision §3.4**: the read/reconnect loop is **duplicated** in tcp.go, NOT extracted from the
  hardware-validated serial.go. Consolidation happens consumer-side instead.
- Consumer-side `liveSource` interface in `cmd/goaldl` (Bytes/Reconnecting/ReconnectAttempt);
  model field `serial`→`live`; six read sites read through it (§4).
- `-tcp host:port` flag at both dispatch sites (TUI + monitor), mutual exclusion between
  `-p`/`-tcp`/capture-file with clear errors; TCP live source gets a `RecordSink` in the TUI and
  honors `-o` in monitor (§5).
- Constants §6: dial 5s, read 3s, `tcpHalfOpenWindows` 4 (~12s to declare half-open).
- Test matrix §7: T1 happy path (golden fixture), T2 sink fidelity (byte-for-byte), T3
  reconnect-on-drop, T4 context cancel bounded, T5 half-open redial with an upper bound, T6
  dial-refused start, T7 diagnostics race, T8 Name — all in-process `127.0.0.1:0`, `-race` clean.
  Plus consumer-side: flag exclusion tests and the waiting-screen path via a fake `liveSource`.
- §10 forbidden seam — must be untouched: `pkg/stream/session.go`, `pkg/decoder/**`,
  `pkg/ecm/**`, `pkg/blm/**`, `go.mod`, `go.sum`.

## Section C: What Changed

Uncommitted working-tree changes on `main` (verify with `git status --short` / `git diff --stat`):

| File | Change |
|---|---|
| `pkg/stream/tcp.go` | **new** — TCPProvider + Run + redial + cancel-closer + diagnostics |
| `pkg/stream/tcp_test.go` | **new** — T1–T8 + `replayTCPServer` test helper |
| `cmd/goaldl/tcp_flags_test.go` | **new** — liveSource interface asserts + source-exclusion tests |
| `cmd/goaldl/tui.go` | `byteSource`→`liveSource`, `m.serial`→`m.live`, `-tcp` flag/branch, source mutual exclusion, help text |
| `cmd/goaldl/tui_test.go` | mechanical renames for the above |
| `cmd/goaldl/monitor.go` | `-tcp` flag/branch/title, source mutual exclusion, sink declared as `io.Writer` (typed-nil fix) |
| `cmd/goaldl/main.go` | usage text gains the `-tcp` example |
| `CLAUDE.md`, `README.md`, `docs/mobile-ui.md` | document the `-tcp` source; Stage 0 delivered note |
| `specs/2026-07-06_feature_tcp-provider/*` | trace/tasks (not code — skim only) |
| `product-knowledge/observations/observed-standards.md` | two observations logged |

## Section D: How to Verify

- Build/lint: `go build ./...` · `go vet ./...` · `gofmt -l .` (expect no output)
- Tests: `go test -race ./...` (full suite; TCP tests are `go test -race -run TestTCP ./pkg/stream/`)
- Goldens: `go test ./pkg/decoder` must pass unchanged (byte-identical fixtures)
- Forbidden seam: `git diff --name-only -- pkg/stream/session.go pkg/decoder pkg/ecm pkg/blm go.mod go.sum` must print nothing
- End-to-end (no hardware): serve `pkg/decoder/testdata/drive_4800.raw` on a local socket
  (e.g. a short python/nc one-liner), then
  `go run ./cmd/goaldl monitor -tcp 127.0.0.1:<port> -csv /tmp/out.csv` — expect **635 frames**
  (the drive fixture's known count) and a clean stop on SIGINT. You may also exercise
  `-o <file>.raw` and compare the recorded bytes to the fixture (interchangeability proof), and
  the flag-error paths (`-p X -tcp Y`, `-tcp Y file.raw`, `monitor` with both).
- The dashboard itself is a full-screen TUI — do not try to drive it in a non-interactive shell;
  the model-level tests in `cmd/goaldl/tui_test.go` stand in for it.

## Section E: Standards to Enforce

- `product-knowledge/standards/architecture/session-api-layering.md` (must)
- `product-knowledge/standards/decoder/byte-value-decoding.md` (must)
- `product-knowledge/standards/decoder/raw-data-policy.md` (must)
- `product-knowledge/standards/release/platform-support.md` (must — pure Go, no CGO/build tags)
- `product-knowledge/standards/testing/golden-fixtures.md` (should)
- `product-knowledge/standards/go/tooling.md` (should — no new deps; fmt/vet/race gates)
- Core philosophies referenced by the spec: consolidate-over-accrete, ground-truth-first (note:
  real-bridge validation is explicitly deferred to the hardware stage — do not fail the
  evaluation for it; it is tracked in the trace).

## Section F: Personas to Consult

- `/Users/aaronstone/.claude/plugins/cache/crux-marketplace/glados/1.3.0/src/personas/architect.md`
- `/Users/aaronstone/.claude/plugins/cache/crux-marketplace/glados/1.3.0/src/personas/qa.md`
- `/Users/aaronstone/.claude/plugins/cache/crux-marketplace/glados/1.3.0/src/personas/product-manager.md`
