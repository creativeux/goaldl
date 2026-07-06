<!-- SDA: v1.0 -->
# High-Level Plan: TCPProvider

## Approach in one line

Clone `SerialProvider`'s proven read/reconnect loop over a `net.Conn` instead of a serial port,
wire a `-tcp host:port` source into the two provider-construction sites, and verify entirely with
an in-process TCP listener replaying a committed fixture — no hardware, no facade change.

## Where it lives (seam analysis)

The `Provider` interface is the single seam:

```go
// pkg/stream/stream.go:32
type Provider interface {
    Name() string
    Run(ctx context.Context, emit func(FrameEvent)) error
}
```

Everything above it is origin-blind. So the change set is deliberately small and all below-facade:

| Area | Change | Note |
|---|---|---|
| `pkg/stream/tcp.go` (new) | `TCPProvider` struct + `Run` + reconnect + diagnostics | mirrors `serial.go` |
| `pkg/stream/tcp_test.go` (new) | in-process listener tests | mirrors `serial_test.go` fake-source idiom |
| `cmd/goaldl/tui.go` (~L129–143) | `-tcp` flag → construct `TCPProvider` | one more branch beside serial/replay |
| `cmd/goaldl/monitor.go` (~L50–81) | same `-tcp` branch | keep the two sites consistent |
| `cmd/goaldl/main.go` / flag parse | register `-tcp`, enforce source mutual-exclusion | small |
| `CLAUDE.md`, `README.md`, `docs/mobile-ui.md` | document the new source | R12 |

**Untouched (forbidden seam):** `pkg/stream/session.go`, `pkg/decoder`, `pkg/ecm`, `pkg/blm`,
`go.mod`. This is the same discipline every recent TUI phase held to.

## The reconnect model — reuse, don't reinvent

`SerialProvider` already defines the contract we want (`serial.go:88-198`): initial-source-missing
and mid-session-drop both funnel into a `reconnect` loop that retries at ~frame cadence until the
source returns or `ctx` is cancelled, and the decoder is rebuilt after a gap so it resyncs from
the next sync pattern. `TCPProvider` should present the **same behavior and the same diagnostic
surface** so the dashboard's existing waiting/reconnecting chrome works unchanged:

- `Bytes() int64` — total bytes received (waiting screen: "connected but no sync" vs "no bytes").
- `Reconnecting() bool` / `ReconnectAttempt() int` — reconnecting indicator vs fatal panel.

**Design question for the spec (Architect):** SerialProvider and TCPProvider will share nearly
identical read-and-reconnect scaffolding. Two honest options —
  (a) **Duplicate** the loop in `tcp.go` (simple, no churn to hardware-validated serial code, two
      short files that each read top-to-bottom), or
  (b) **Extract** a shared `streamBytes(ctx, source, cfg, sink, emit, reconnect)` helper over a
      tiny `byteSource` interface (`Read`/`Close`) that both providers feed.
The repo's stated philosophy is *consolidate-over-accrete*, which points at (b); but (b) touches
the hardware-validated serial path. **Recommendation: (b) a minimal extraction**, but only if it
lands with the serial tests still green and the serial reconnect semantics provably unchanged;
otherwise fall back to (a). The spec will decide with the code in front of it. Either way the
*external* behavior is fixed by requirements R4/R5.

## Transport-specific details the serial path doesn't have

These are the only genuinely new concerns (everything else is a port of existing logic):

1. **Dial vs open.** `net.Dial("tcp", addr)` (or a `net.Dialer` with `DialContext` so the dial
   itself honors `ctx`). A refused/timed-out dial is the analogue of "port not present" → reconnect
   loop, not a fatal error.
2. **Read deadlines for liveness (R11).** A serial read returns on a timeout with `n==0`; a TCP
   read on a silently half-open socket can block forever. Use `SetReadDeadline` on a rolling
   interval so a dead bridge (powered off without FIN) is detected and routed into reconnect. A
   deadline-exceeded with no bytes is "still waiting," not an error; a genuine read error or
   prolonged silence trips reconnect.
3. **Cancellation unblocking (R6).** A blocked `conn.Read` won't return on `ctx` cancel by itself.
   Either close the conn from a `ctx`-watching goroutine, or rely on the rolling read deadline to
   wake the loop so it can observe `ctx.Err()`. Pick one and test it (R10 context-cancel case).
4. **Framing.** None needed — the bridge forwards the raw UART byte stream and the *decoder* finds
   frame sync (9× `0x00`). TCP is just a byte pipe; the provider does zero protocol parsing beyond
   what `decoder.Feed` already does. This is the crucial simplification and worth stating loudly.

## Test strategy (QA) — the whole point of doing desktop-first

All tests use an **in-process `net.Listener`** on `127.0.0.1:0` (OS-assigned port); no hardware, no
external network, deterministic in CI. Mirrors the `SerialProvider` fake-source idiom.

- **Happy path:** listener streams `pkg/decoder/testdata/drive_4800.raw` bytes; assert the emitted
  `FrameEvent` sequence matches decoding the same fixture directly (reuse the golden expectation).
- **Sink fidelity (R3):** tee to a buffer; assert the buffer equals the bytes the listener sent →
  byte-for-byte interchangeability with a serial `.raw`.
- **Reconnect (R4):** listener accepts, sends a partial stream, closes; a second accept resumes;
  assert the session survives, `Reconnecting()` toggled, and frames resume after the gap.
- **Context cancel (R6):** cancel mid-stream; assert `Run` returns `ctx.Err()` promptly (bounded).
- **Half-open/timeout (R11):** listener accepts then goes silent; assert the read deadline trips
  reconnect within the expected window rather than hanging.
- **Diagnostics (R5):** assert `Bytes()` climbs with received data and is readable concurrently
  with `Run` (race-tested).
- **Optional dev fixture:** a tiny `replay-over-TCP` stub (reads a `.raw`, serves it paced ~160 B/s
  on a socket) — doubles as the manual bench tool for Stage 0/1 bring-up in `docs/mobile-ui.md`.
  Kept out of the production build path (a test helper or a `tools/`-style command, TBD in spec).

## Staged rollout (from docs/mobile-ui.md, for context — not all in this feature)

This feature delivers **Stage 0**: the `TCPProvider` + `-tcp` flag + the in-process test harness,
so desktop `goaldl` can consume *any* TCP byte source. Stages 1–2 (ESP32 firmware bring-up against
the car) and the iOS binding are downstream, gated on the S3 arriving and on this provider being
verified. Desktop ships network transport independently of any of that.

## Risks / watch-items

- **Touching hardware-validated serial code** if the shared-helper extraction (option b) is chosen
  — mitigated by keeping serial's tests green as the gate; fall back to duplication if not clean.
- **Cancellation correctness** — the classic "blocked Read won't cancel" trap; explicitly tested.
- **Read-deadline tuning** — too tight ⇒ false reconnects on a slow-but-alive link; too loose ⇒
  slow dead-link detection. Anchor to frame cadence (~1.2 s/frame), same logic serial uses.

## Open questions to resolve in spec-feature

1. Shared-helper extraction (b) vs duplication (a) — decide with the code open; serial tests are
   the gate.
2. Cancellation mechanism — `ctx`-watching goroutine closing the conn vs rolling read-deadline
   poll. (Leaning: `DialContext` for the dial + a conn-closer goroutine for the read, simplest to
   reason about; confirm against the `Provider` "never block past ctx" contract.)
3. Exact `-tcp` flag ergonomics and the source mutual-exclusion error wording (`-p` vs `-tcp` vs
   replay-file) — align with how the two dispatch sites already validate sources.
4. Where the optional replay-over-TCP stub lives (test-only helper vs a small committed dev
   command) so it can serve double duty as the bench tool without bloating the production binary.
5. Whether `Name()` should surface the resolved remote address after connect, or the configured
   `host:port` (parallels `live:<port>`; configured is simpler and stable across reconnects).

## Handoff

Proceed to **spec-feature** to turn this into a concrete technical spec (types, signatures, the
reconnect-model decision, and the test matrix), then stop — no implementation this cycle.
