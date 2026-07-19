<!-- SDA: v1.0 -->
# Evaluation: TCPProvider

Evaluated 2026-07-18 against `evaluation-brief.md`, `requirements.md` (R1–R12, success criteria
1–6), and `spec.md`. All commands were run, not just read; the end-to-end verification in
Section D was performed against a local socket serving the committed drive fixture.

## Verification Log (what was actually run)

- `go build ./...` · `go vet ./...` · `gofmt -l .` — all clean.
- `go test -race ./...` — green across all packages; `go test -race -count=1 -run TestTCP ./pkg/stream/`
  — all 7 TCP tests pass fresh (1.7s).
- `go test -count=1 ./pkg/decoder ./cmd/goaldl` — green; goldens byte-identical (no regeneration).
- Forbidden seam: `git diff --name-only -- pkg/stream/session.go pkg/decoder pkg/ecm pkg/blm go.mod go.sum`
  prints nothing.
- **End-to-end**: a python server on `127.0.0.1:57407` served `pkg/decoder/testdata/drive_4800.raw`
  once (then held silent); `goaldl monitor -tcp 127.0.0.1:57407 -csv out.csv -o rec.raw` decoded and
  reported **"Stopped after 635 frames"** on SIGINT (exit 0); the CSV had exactly 635 data rows;
  `cmp rec.raw drive_4800.raw` — **byte-for-byte identical** (interchangeability proof).
- `blm` over the drive fixture: "635 frames / 469 recorded / 27 of 37 cells trusted" — identical
  output with the change stashed vs applied.
- Flag-error paths exercised live: `monitor -p X -tcp Y`, `monitor -tcp Y file.raw`, TUI `-p X -tcp Y`,
  TUI `-tcp Y file.raw` — all rejected with clear messages, exit 1.
- Cross-compile sanity (platform-support): `GOOS=windows` and `GOOS=linux GOARCH=arm` builds clean.

## Acceptance Criteria

| Criterion | Verdict | Evidence |
|-----------|---------|----------|
| SC1 — `-tcp` drives dashboard/table with same frames as serial | ✅ PASS | `monitor -tcp` end-to-end produced the fixture's known 635 frames; dashboard path covered by model-level tests (`tui_test.go` waiting/reconnect via `fakeBytes` liveSource) per brief's TUI carve-out |
| SC2 — `.raw` via TCP Sink byte-for-byte identical, decodes same | ✅ PASS | `cmp rec.raw drive_4800.raw` identical; T2 (`TestTCPHappyPathAndSinkFidelity`) asserts sink bytes == sent bytes; recorded raw decodes to the same 635 frames by construction (it *is* the fixture) |
| SC3 — drop/restore keeps session alive, reconnecting state shown | ✅ PASS | T3 (`TestTCPReconnectOnDrop`): frames resume after mid-stream close, `Reconnecting()` observed true during the gap, state resets after Run; TUI reconnect indicator tests read through `liveSource` |
| SC4 — unit tests with no hardware/network, all five paths | ✅ PASS | T1–T7 all in-process `127.0.0.1:0`: happy path, reconnect, sink fidelity, cancel promptness (<1s vs 10s deadline), half-open redial |
| SC5 — race green; fmt/vet clean; goldens identical; blm cell count; seam empty | ✅ PASS | All verified above; blm reports 27/37 trusted cells unchanged; seam diff empty |
| SC6 — no new third-party dependency | ✅ PASS | `tcp.go` imports only `context`/`errors`/`io`/`net`/`os`/`sync/atomic`/`time` + internal `decoder`; `go.mod`/`go.sum` untouched |
| R1 — Provider in `pkg/stream`, same event shape | ✅ PASS | `pkg/stream/tcp.go:109-207`: `Run` feeds `decoder.New(cfg)`, emits `FrameEvent{Frame, Index, Elapsed}` with monotonic idx; T1 frames equal direct decode |
| R2 — Config parity | ✅ PASS | `Config decoder.Config` field; both dispatch sites pass the same `cfg` built from `-b`/`-invert` as the serial branch (`monitor.go:44`, `tui.go` construct) |
| R3 — Raw capture tee | ✅ PASS | `Sink io.Writer`, every received byte written before decode (`tcp.go:169-173`); T2 + end-to-end `cmp` |
| R4 — Reconnect on drop | ✅ PASS | `redial` retries forever until ctx cancel (`tcp.go:215-235`); decoder rebuilt after gap (`tcp.go:204`); T3, T6 |
| R5 — Connection diagnostics | ✅ PASS | `Bytes()`/`Reconnecting()`/`ReconnectAttempt()` atomics, method set identical to serial; compile-time asserts in `tcp_flags_test.go:12-15`; waiting screen reads through `liveSource` |
| R6 — Context cancellation | ✅ PASS | `DialContext` + rolling read deadline + per-connection cancel-closer goroutine; T4 measures cancel latency well under a deliberately long 10s deadline |
| R7 — `-tcp` flag both sites, mutual exclusion | ✅ PASS | `tui.go` resolve + construct branch, `monitor.go` guard + branch; all four conflict combinations rejected live and in `TestResolveSourceExclusion` |
| R8 — `Name()` chrome | ✅ PASS | `"tcp:" + Addr`, stable (configured, not resolved); T8 |
| R9 — No facade change | ✅ PASS | Forbidden-seam diff empty; goldens pass unchanged |
| R10 — Testability without hardware | ✅ PASS | In-process listener throughout; injectable `dial`/`sleep` seams mirror the serial idiom |
| R11 — Read timeout / liveness | ✅ PASS | Rolling deadline + `tcpHalfOpenWindows` (4×3s) consecutive-empty-window threshold → redial; T5 proves a silent half-open conn triggers a second accept, bounded |
| R12 — Docs | ✅ PASS | CLAUDE.md command list + pkg/stream description, README quickstart, `docs/mobile-ui.md` "Stage 0 delivered" cross-link, `main.go` usage, `errNoTUISource` help |

## Standards Compliance

| Standard | Verdict | Notes |
|----------|---------|-------|
| architecture/session-api-layering (must) | ✅ | TCPProvider is a `Provider` below the `Session` facade; `liveSource` lives in `cmd/goaldl` and reads only diagnostic scalars; no consumer touches decoder/ecm directly |
| decoder/byte-value-decoding (must) | ✅ | No timing logic anywhere in the provider; bytes fed verbatim to `decoder.Feed`; TCP burstiness is irrelevant by design |
| decoder/raw-data-policy (must) | ✅ | No filtering/smoothing; liveness timeouts gate the *connection* only, documented as such in the constants comment (`tcp.go:67-71`) |
| release/platform-support (must) | ✅ | Pure stdlib `net`, no build tags, no CGO; verified cross-compiling windows/amd64 and linux/arm |
| testing/golden-fixtures (should) | ✅ | Drive fixture is the oracle for T1–T3/T6/T7; decoder goldens untouched. Note: T1's oracle is a live re-decode (`decodeAll`) rather than the committed `.golden` file — acceptable because the decoder is seam-frozen and independently golden-tested, but marginally weaker than the spec's "reuse golden" wording |
| go/tooling (should) | ✅ | fmt/vet/build/`-race` all clean; zero new dependencies; doc comments carry the reasoning at point of use in the repo's style |
| consolidate-over-accrete (core) | ✅ | Grows as a Provider consumer of the existing seam; §3.4 duplication decision honored (serial.go untouched); consumer diagnostics unified via one interface; `replayTCPServer` kept test-only |
| ground-truth-first (core) | ⏳ deferred | Real ESP32+car validation explicitly deferred per spec §11 / brief — not counted against, tracked in trace. Timeout constants are anchored to the measured 1.2s frame cadence |

## Persona Reviews

**Architect.** The change fits the architecture cleanly: a second live provider behind the exact
seam the 2026-07-03 consolidation created, with zero facade/decoder/ecm/blm churn and zero new
dependencies. The §3.4 duplicate-don't-extract decision was honored — `serial.go` is untouched and
`tcp.go` reads top-to-bottom. The consumer-side consolidation (`byteSource`→`liveSource`, one
interface for both providers) is the right altitude; the typed-nil discipline is applied at every
concrete→interface assignment, and the `monitor.go` sink fix (`var sink io.Writer`) actually
repaired a pre-existing latent typed-nil bug in the serial path, logged as an observation. The
cancel-closer channel dance (`swapConn`, `tcp.go:119-151`) is the one piece of genuinely tricky
concurrency; I traced the cancel-during-redial and cancel-during-swap interleavings and found them
sound (double-`Close` on a `net.Conn` is safe), and `-race` is clean — but note the closer
goroutine outlives `Run` when `Run` exits for a non-ctx reason (see Issue 1), and `closerDone` is
a vestigial signal nothing waits on.

**QA.** Coverage of the unhappy paths is strong and honest: dial refused at launch (T6), drop
mid-stream (T3), half-open with no FIN (T5), cancel while blocked in Read with the deadline
deliberately inflated to prove the closer does the work (T4), sink fidelity (T2), concurrent
diagnostics under `-race` (T7). Tests use the real fixture, real sockets on `127.0.0.1:0`, and
polling with generous outer deadlines rather than sleeps — deterministic in my runs (fresh
`-count=1 -race` pass in 1.7s). The end-to-end run reproduced the known 635-frame count and a
byte-identical `-o` recording, and all four flag-conflict paths fail loudly with clear messages.
Gaps: the TUI's `-tcp` *construction* branch (`tui.go:150-153`) has no direct test — the spec's §7
consumer-side matrix asked for an assertion that the branch builds a `TCPProvider` and sets
`m.live`; what exists is compile-time interface satisfaction plus `resolveTUIFlags` tests, which
covers it only indirectly (Issue 2). A Sink-write-error test (spec §8 row 4) is also absent for
TCP, though the code path is three lines and mirrors serial.

**Product Manager.** The user story lands: a desktop user can point `goaldl`/`monitor` at a bridge
address today, with the same recording, CSV, and reconnect UX as the cable — success is measurable
(635 frames, identical `.raw`) and was measured. Scope discipline is good: no TLS, no discovery,
no server mode, no firmware, and the test replay server was kept out of the shipped binary per the
spec decision. Docs meet R12 at all four touchpoints, and `mobile-ui.md` now correctly marks
Stage 0 delivered with the next step named. One UX wrinkle: the waiting screen's zero-byte hint
still says "check the cable/port" even when the source is a TCP bridge — the spec explicitly chose
"copy unchanged," so it's compliant, but the first real bridge user staring at a wrong-IP mistake
will get cable advice (Issue 3). Worth a line in the hardware-stage follow-up.

## Issues Found

1. **Cancel-closer goroutine can outlive `Run` on non-ctx exit.** If `Run` returns because of a
   `Sink` write error (`tcp.go:170-172`), the goroutine at `tcp.go:122-128` stays blocked on
   `<-ctx.Done()` until the caller eventually cancels; `closerDone` (`tcp.go:121`) is closed but
   never waited on, so nothing bounds it. In both real callers (TUI, monitor) the ctx is cancelled
   at session end, so the leak is bounded in practice — but a library consumer reusing a
   long-lived ctx across providers would accumulate one goroutine per errored `Run`.
   Where: `pkg/stream/tcp.go:119-151`. Severity: **note**.
2. **Spec §7 consumer-side test partially delivered.** "Extend a TUI test to assert the `-tcp`
   branch builds a `TCPProvider`, sets `m.live`" — the construct branch in `cmdTUI`
   (`cmd/goaldl/tui.go:150-153`) is not directly exercised; coverage is indirect (compile-time
   `_ liveSource = (*stream.TCPProvider)(nil)` assert, `TestResolveTCPSource`, and the fake-
   liveSource waiting-screen tests). The branch is five straight-line lines mirroring the tested
   serial branch, and `cmdTUI` launches a full Bubble Tea program, so testing it directly would
   need a refactor — a defensible trade, but it is a deviation from the agreed test matrix.
   Where: `cmd/goaldl/tcp_flags_test.go` vs spec §7. Severity: **warning**.
3. **Waiting-screen copy is serial-flavored on a TCP source.** `waitingBody` (`tui.go`, zero-byte
   branch) suggests cable/port causes when `m.live` is a TCPProvider; a wrong bridge IP produces
   the same hint. Spec §4 explicitly declared rendering copy unchanged, so this is compliant —
   flagging as a UX follow-up for the hardware stage. Severity: **note**.
4. **T1 oracle is a re-decode, not the committed golden.** `decodeAll` re-runs the same decoder
   over the fixture rather than comparing against `drive_4800.raw.golden`. Because `pkg/decoder`
   sits in the forbidden seam (verified unchanged) and carries its own golden tests, the oracles
   are equivalent this cycle; they would only diverge if a future change altered the decoder and
   the TCP tests silently followed it. Where: `pkg/stream/tcp_test.go:57-66`. Severity: **note**.

## Overall Verdict

**PASS** — all 6 success criteria and all 12 functional requirements met; all must-standards
compliant; forbidden seam untouched; end-to-end verification reproduced the 635-frame ground truth
and a byte-identical raw recording. No blocking issues. 1 warning (spec §7 consumer-side
construction test delivered only indirectly), 3 notes.
