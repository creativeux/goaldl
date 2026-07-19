<!-- SDA: v1.0 -->
# Trace: TCPProvider (ESP32 wireless bridge transport)

**Workflow**: plan-feature
**Started**: 2026-07-06
**Feature**: tcp-provider — a `stream.Provider` that dials a TCP socket and streams decoded ALDL frames from an ESP32-S3 wireless bridge (WiFi or wired Ethernet), mirroring `SerialProvider`. Adds a `-tcp host:port` source alongside `-p` in the TUI and `monitor`. Gives desktop `goaldl` a network transport and is the shared byte pipe a future iOS/mobile front-end would consume.

## Scope guard
**Plan + spec only this session — NO implementation.** User has ordered an Adafruit ESP32-S3; wants the provider designed and specced ahead of hardware arrival, not built.

## Active Personas
- **Architect** (primary) — keep the addition on the Provider seam; Session/Snapshot/decoder/ecm/blm untouched; reconcile with the existing SerialProvider reconnect model.
- **QA** (primary) — verification strategy with no ECM and no network hardware: fake in-process TCP server replaying a committed fixture; byte-for-byte `.raw` interchangeability proof against a serial capture.
- Product Manager (lighter) — sequencing vs the mobile-UI goal; what desktop ships independently of iOS.

## Active Capabilities
- Bash/Go toolchain — build/test against committed real captures (`pkg/decoder/testdata/`); `net` stdlib in-process listener for tests (no external hardware).
- Existing Provider abstraction — `stream.Provider` interface + `SerialProvider` is a near-template; `ReplayProvider`/`SerialProvider` tests show the fake-source pattern (`open`/`sleep`/`listPorts` injectables).
- Reference design doc — `docs/mobile-ui.md` (transport analysis: WiFi-TCP / Ethernet-TCP / BLE / MFi; TCP chosen as the primary shared transport).
- Subagents — available for context-isolated evaluation in a later verify workflow.

## Log
- 2026-07-06: Session start. Feature `tcp-provider`. Scope fixed to plan + spec (no build) per user. Personas defaulted to Architect + QA (primary) + PM (lighter) — matches the repo trio, weighted to this transport/layering task.
- 2026-07-06: Grounded the plan in the live code: `Provider` interface (`pkg/stream/stream.go:32`), `SerialProvider` read loop + reconnect (`pkg/stream/serial.go`), wiring points `cmd/goaldl/tui.go:129-143` and `cmd/goaldl/monitor.go:50-81`. Confirmed the addition is one new file + flag plumbing at two dispatch sites; no facade change.
- 2026-07-06: Wrote [requirements.md](requirements.md) (goal, functional requirements R1–R12, non-goals, success criteria) and [plan.md](plan.md) (approach, seam analysis, reconnect-model decision, test strategy, open questions for spec). Handoff to spec-feature pending.
- 2026-07-06: **spec-feature.** Grounded the spec in the live code (`FrameEvent` shape, `tuiFlags`/provider construction at `tui.go:52-196`, `monitor.go:26-83`). **Key finding:** the TUI reads live-source diagnostics off the *concrete* `*SerialProvider` at six sites (`tui.go:675,1444,1481,1486,1580,2011`) — so a TCP source needs a consumer-side `liveSource` interface (`Bytes`/`Reconnecting`/`ReconnectAttempt`) and a `m.serial`→`m.live` rename to drive the same waiting/reconnecting chrome without forking it. Wrote [spec.md](spec.md). Resolved all 5 plan open-questions: (1) **duplicate** the provider reconnect loop (serial rescans a re-enumerated port / calls ResetInputBuffer; TCP redials a fixed Addr — genuinely different; consolidation happens on the consumer side instead); (2) cancel via `DialContext` + rolling read-deadline + a per-conn cancel-closer goroutine; (3) `-tcp host:port` single-token flag, mutual-exclusion guard vs `-p`/file; (4) replay-over-TCP stub stays a **test helper**, not a committed command; (5) `Name()` returns configured `Addr` (stable across reconnects).

## Persona Review (spec.md)
- **Architect**: New dependency? No — `net`/`context`/`time`/`io`/`sync/atomic` are all stdlib (key question answered). Fits the architecture: TCPProvider is a new `Provider` behind the stable seam, `Session`/`Snapshot` untouched, forbidden seam enumerated. Pattern-consistent: method set mirrors `SerialProvider` exactly, so both satisfy `liveSource` with no adapter. The duplicate-vs-extract call is the right one and is *justified in-spec* against both core philosophies rather than asserted — duplicating a 40-line loop behind a stable interface is not the accretion the philosophy targets; forking the hardware-validated serial reconnect path to serve TCP's different needs would be. The consumer-side `liveSource` unification is the genuine consolidation. No API-contract breakage; `m.serial`→`m.live` is internal to `cmd/goaldl`. **Approve.**
- **QA**: Unhappy paths are first-class: §8 edge-case table + T3/T4/T5/T6 cover drop, cancel, half-open, and dial-refused — the "how do we handle network failures" question is answered concretely, not hand-waved. Verifiable outputs defined: every test row names an oracle (golden frames, byte-buffer equality, bounded cancel deadline). No-hardware/no-network in-process listener strategy is sound and `-race`-gated. Two additions requested and folded in: (a) T5 must assert an *upper bound* on redial latency (`~tcpHalfOpenWindows·ReadTimeout`) so "detects half-open" can't silently regress to "eventually"; (b) a consumer test that drives the waiting-screen byte-rate path through a fake `liveSource` so the `m.serial`→`m.live` refactor is regression-covered, not just compile-checked. Both now in §7. **Approve.**
- **Product Manager**: Problem/Who clear — desktop `goaldl` consuming the ESP32 bridge, and the shared byte pipe for a future mobile UI (per `docs/mobile-ui.md`). Scope guarded well: BLE/USB-gadget/MFi/iOS-binding/firmware/TLS/mDNS all explicitly out, TCP-only in. Success criteria testable (byte-for-byte `.raw` interchangeability is a particularly crisp, measurable acceptance test). Sequencing honest: desktop ships network transport independently of iOS. One scope note (non-blocking): the spec correctly resists building the dev stub as a product; keep it that way. **Approve.**

**Synthesis: all three approve. Proceed to standards gate.**

## Standards Gate Report (pre-implementation)
| Standard | Scope | Severity | Verdict |
|---|---|---|---|
| architecture/session-api-layering | architecture | must | ✅ PASSES — TCPProvider is a `Provider`; consumers stay on `Session`/`Snapshot`; `liveSource` reads diagnostic scalars only, not pipeline internals |
| decoder/byte-value-decoding | decoder | must | ✅ PASSES — decode path untouched; TCP delivers the same UART byte values; timing-independence is why it works |
| decoder/raw-data-policy | decoder | must | ✅ PASSES — faithful byte transport; liveness timeouts gate the connection, never frame content; every aligned frame emitted |
| release/platform-support | release | must | ✅ PASSES — `net` pure-Go stdlib, `CGO_ENABLED=0`, no build tags / OS-conditional code; `pkg/serial`+VT seams untouched; TinyGo door unaffected (lives in `pkg/stream`, already Tier-3-excluded) |
| testing/golden-fixtures | testing | should | ✅ PASSES — decoder goldens byte-identical; new tests reuse the drive fixture as oracle |
| go/tooling | go | should | ✅ PASSES — no new dependency; gofmt/vet/build/`-race` gate |
| release/versioning | release | should | ℹ️ NOTE — when implemented, the commit is a `feat:` (new `-tcp` source); pre-1.0 bumps a patch. No action at spec time |
| philosophy: consolidate-over-accrete | core | must | ✅ PASSES — grows as a Provider consumer; consumer diagnostics unified via one interface; loop duplicated only where paths truly diverge, reasoned in §3.4 |
| philosophy: ground-truth-first | core | must | ⚠️ WARNING (by design) — in-process tests use a loopback socket, which shares the decoder's assumptions and cannot falsify them. Spec §9/§11 explicitly require real ESP32-S3 + car validation as a post-implementation step. Not blocking at spec time; **this is the reason implementation is deferred until the S3 arrives.** |

**Gate decision: PROCEED** (no `❌ VIOLATION`; one intentional `⚠️ WARNING` that is the very reason for the deferral, one `ℹ️ NOTE`). Implementation is nonetheless **held by user direction** until the ESP32-S3 arrives.

## Implementation (2026-07-18)

- 2026-07-18: **implement-feature resumed** — the ESP32-S3 arrived (Adafruit QT Py ESP32-S3 No
  PSRAM, enumerates as `/dev/cu.usbmodem2101`); hold released by user. Firmware track decided:
  CircuitPython for bring-up. Task breakdown: [tasks.md](tasks.md).
- Active capabilities: Go toolchain (build/vet/test -race), committed drive fixture as oracle,
  in-process `net` listener for all tests (no hardware in the loop this stage).
- 2026-07-18: **Implementation complete.** All tasks in [tasks.md](tasks.md) done; full suite
  green under `-race`; forbidden seam untouched (`git diff` on `pkg/stream/session.go`,
  `pkg/decoder/**`, `pkg/ecm/**`, `pkg/blm/**`, `go.mod` is empty). End-to-end smoke: a local
  socket replaying `drive_4800.raw` through `goaldl monitor -tcp 127.0.0.1:33333 -csv …` decoded
  **635/635 frames** — the drive capture's exact frame count.
- Files changed: `pkg/stream/tcp.go` (new), `pkg/stream/tcp_test.go` (new, T1–T8 +
  `replayTCPServer` helper), `cmd/goaldl/tui.go` (`byteSource`→`liveSource`, `m.serial`→`m.live`,
  `-tcp` flag + branch + source mutual-exclusion + help), `cmd/goaldl/tui_test.go` (renames),
  `cmd/goaldl/tcp_flags_test.go` (new, interface-satisfaction asserts + source-exclusion tests),
  `cmd/goaldl/monitor.go` (`-tcp` flag/branch/title + mutual exclusion), `cmd/goaldl/main.go`
  (usage), `CLAUDE.md`, `README.md`, `docs/mobile-ui.md` (Stage 0 delivered note).
- Notable finds during implementation: (1) `monitor.go` had a latent typed-nil bug — `var sink
  *os.File` assigned into the provider's `Sink io.Writer` meant a live monitor *without* `-o`
  carried a non-nil interface wrapping nil and the first tee write would kill the stream; fixed
  by declaring the variable as `io.Writer` (logged in observed-standards). (2) The source rules
  now also reject `-p <port> <file>` (previously the file was silently ignored).
- Deviations from spec: none of substance. `tcpHalfOpenWindows` implemented as consecutive empty
  read windows inside `Run` (spec §3.1/§6); the consumer interface kept the spec's `liveSource`
  name, renaming the pre-existing `byteSource`.
- **Remaining (ground-truth-first, spec §9)**: real-bridge validation — ESP32-S3 (arrived,
  Adafruit QT Py S3) running UART→TCP firmware (CircuitPython chosen), bench test with the
  PL2303 replaying the drive capture into the S3's UART, then the car. Next: `verify-feature`.

## Verification (2026-07-18)

- **verify-feature session.** Evaluation brief assembled
  ([evaluation-brief.md](evaluation-brief.md)); fresh context-isolated evaluator spawned
  (filesystem-only handoff, no implementation context).
- **Evaluator verdict: PASS** ([evaluation.md](evaluation.md)) — 0 blocking, 1 warning, 3 notes.
  All acceptance criteria (success 1–6, R1–R12) and standards verified with evidence; evaluator
  independently reproduced the end-to-end proof (635/635 frames via `monitor -tcp`; `-o`
  recording `cmp`-identical to the fixture; `blm` output unchanged; all four flag-conflict paths
  reject; cross-compiles windows/amd64 + linux/arm).
- **Post-evaluation fixes (same session):** (1) cancel-closer goroutine no longer outlives `Run`
  on a non-ctx exit — a `stop` channel closed on return tears it down; the vestigial `closerDone`
  removed (evaluator notes 1–2). (2) Sibling-comparison gap closed: `TestTCPRedialNeverGivesUp`
  added (TCP twin of `TestSerialReconnectNeverGivesUp`). Remaining follow-ups logged, not fixed:
  the warning (cmdTUI's 5-line `-tcp` construct branch tested only indirectly — would need a
  construction-helper refactor, deferred) and the waiting-screen zero-byte hint saying
  "cable/port" on a TCP source (UX wording, natural candidate for the tui-ux-pass Phase E batch).
  T1's live-re-decode oracle accepted as-is (equivalent to the golden by the decoder's own tests).
- **Spec retrospection:** spec.md status updated to implemented; §3.3 ⟦as-built⟧ closer note,
  §10 heading, §11 deferral→implementation record (incl. the typed-nil monitor fix and the
  `-p <port> <file>` rejection). Standards audit: no stale code references in
  `product-knowledge/standards/` (grep clean).
- **Test synchronization:** no stale refs (`byteSource` fully renamed); `fakeBytes` stub matches
  the `liveSource` contract; every new public method covered (T1–T8 + never-gives-up); sibling
  comparison now at parity with `serial_test.go`. Full suite `go test -race -count 1 ./...`
  green; gofmt/vet clean.
- **Trace closed.** PROJECT_STATUS.md (Current Focus §1 + Recent Changes) and ROADMAP.md
  (Horizon 2: TCPProvider ✅, bridge-firmware Stage 1 added) updated. Commit/PR pending user
  go-ahead (`feat:` — pre-1.0 patch bump per versioning standard).
