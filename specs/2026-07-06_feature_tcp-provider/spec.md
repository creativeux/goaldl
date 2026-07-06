<!-- SDA: v1.0 -->
# Technical Spec: TCPProvider

**Status:** Spec complete — **implementation deferred until the ordered ESP32-S3 arrives.**
**Trace:** [README.md](README.md) · **Requirements:** [requirements.md](requirements.md) · **Plan:** [plan.md](plan.md)
**Transport rationale:** [`docs/mobile-ui.md`](../../docs/mobile-ui.md)

## 1. Overview

Add `stream.TCPProvider`, a `Provider` that dials a `host:port` and streams decoded ALDL frames
from a TCP byte source (the ESP32-S3 bridge, or an in-process test/dev stub). It is a below-facade
transport twin of `SerialProvider`: same read→`decoder.Feed`→`emit` loop, same optional raw `Sink`
tee, same reconnect-across-outage contract, same diagnostic surface. `Session`, `Snapshot`,
`pkg/decoder`, `pkg/ecm`, `pkg/blm` are untouched.

The one non-obvious integration point (discovered during speccing): the TUI reads live-source
diagnostics off the **concrete** `*SerialProvider` at six sites. To let a TCP source drive the same
waiting/reconnecting chrome without forking it, we introduce a tiny consumer-side interface (§4).

## 2. Types & signatures (`pkg/stream/tcp.go`, new)

```go
// TCPProvider streams frames decoded live from a TCP byte source — an ESP32
// bridge forwarding the raw ALDL UART stream over WiFi or wired Ethernet. It is
// the network twin of SerialProvider: the bridge hands us the same 0xFE/0x00
// byte values a local adapter would, so the decoder Config and the whole
// downstream pipeline are identical; only the byte transport differs.
//
// If Sink is non-nil, every received byte is also written to it, so a bridge
// session records to a .raw that is byte-for-byte identical to a serial capture.
//
// A dropped connection never ends the session. Both the initial dial and a
// mid-session read failure (bridge reboot, WiFi blip, cable bump) drop into a
// redial loop that retries until the bridge comes back or the context is
// cancelled — so the dashboard keeps its accumulated grids across an outage and
// resumes when the bridge returns. The consumer distinguishes "connecting" from
// "reconnecting after data" via Reconnecting()/ReconnectAttempt() and whether it
// has yet seen a frame.
type TCPProvider struct {
    Addr   string          // "host:port" (dialed with net; IPv6 literals use [::1]:port form)
    Config decoder.Config  // same baud model/polarity/thresholds as a serial session
    Sink   io.Writer       // optional: raw capture tee (R3)

    // Timeouts (zero ⇒ package defaults, see §6). Injectable so tests run fast.
    DialTimeout time.Duration // per-dial-attempt ceiling
    ReadTimeout time.Duration // rolling read deadline for half-open/liveness detection

    nbytes        atomic.Int64 // total raw bytes received (waiting-screen diagnostics)
    reconnecting  atomic.Bool  // true while Run is redialing a dropped connection
    reconnAttempt atomic.Int64 // current redial attempt (0 when connected)

    // dial/sleep are injectable for tests; nil uses the real ones.
    dial  func(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error)
    sleep func(ctx context.Context, d time.Duration) error
}

func (p *TCPProvider) Name() string            { return "tcp:" + p.Addr } // parallels "live:<port>"
func (p *TCPProvider) Bytes() int64            { return p.nbytes.Load() }
func (p *TCPProvider) Reconnecting() bool       { return p.reconnecting.Load() }
func (p *TCPProvider) ReconnectAttempt() int    { return int(p.reconnAttempt.Load()) }

func (p *TCPProvider) Run(ctx context.Context, emit func(FrameEvent)) error
```

These method names/signatures deliberately match `SerialProvider` exactly so both satisfy the
consumer interface in §4 with no adapter. `Name()` returns the **configured** `Addr` (stable across
reconnects), not the resolved remote — matching how `live:<port>` uses the configured name (closes
plan open-question #5).

## 3. Run loop & reconnect model

### 3.1 Loop (mirrors `serial.go:88-148`)

1. `conn = dial(ctx, Addr, DialTimeout)`. On error → redial loop (§3.2) — a refused/timed-out dial
   is the analogue of "port not present," never fatal.
2. `d := decoder.New(p.Config)`; `buf := make([]byte, 512)`; `start := time.Now()`; `idx := 0`.
3. Loop: check `ctx.Err()`; `conn.SetReadDeadline(time.Now().Add(ReadTimeout))`; `n, err := conn.Read(buf)`.
   - **Timeout with `n==0`** (`errors.Is(err, os.ErrDeadlineExceeded)`): no data this window — `continue`
     (the serial `n==0` analogue). A *run* of consecutive empty windows past a threshold (§6) means
     a silently half-open socket → redial.
   - **Other read error / EOF**: bridge dropped → `conn.Close()`, redial (§3.2), `d = decoder.New(...)`
     to resync from the next sync pattern, `continue`.
   - **`n>0`**: `nbytes.Add(n)`; if `Sink != nil` write `buf[:n]` (a Sink write error ends `Run` with
     that error, same as serial — the caller's `RecordSink` is fail-soft above this layer); feed each
     byte to `d.Feed`, `emit` a `FrameEvent{Frame, Index: idx, Elapsed: time.Since(start)}` per frame.

### 3.2 Redial (mirrors `serial.go:158-185`, minus port rescan)

`reconnecting.Store(true)` for the duration; `reconnAttempt` climbs for the chrome indicator; retry
`dial` at `reconnectInterval` (reuse the existing 1 s constant — ~frame cadence) until success or
`ctx` cancel. **Difference from serial:** no `singlePort()`/re-enumeration rescan — a TCP endpoint
doesn't rename itself; we always redial the same `Addr`. This is *why* the loop is duplicated rather
than shared (see §3.4).

### 3.3 Cancellation (R6) — the one genuinely new correctness concern

A blocked `conn.Read` does not return on `ctx` cancel by itself. Two mechanisms, both used:
- **Dial** honors `ctx` via `net.Dialer{Timeout: DialTimeout}.DialContext(ctx, "tcp", Addr)`.
- **Read** is bounded by the rolling `ReadTimeout` deadline, so the loop wakes at most every
  `ReadTimeout` to observe `ctx.Err()` and return promptly. **Additionally**, a `ctx`-watching
  goroutine closes `conn` on cancel to unblock an in-flight `Read` immediately (so cancel latency is
  ~0, not up to `ReadTimeout`). The goroutine is started per-connection and torn down when the
  connection closes; it must not race the redial swapping `conn` (guard the current conn, or scope
  one closer goroutine per dial). Tested explicitly (§7, T4).

### 3.4 Reconnect-model decision (resolves plan open-question #1)

**Decision: duplicate the read/reconnect loop in `tcp.go`; do NOT extract a shared helper over the
provider internals.** Rationale:
- The two reconnect loops genuinely differ: serial rescans for a re-enumerated `/dev` name
  (`singlePort`/`listPorts`) and calls `ResetInputBuffer`; TCP redials a fixed `Addr` with dial/read
  deadlines and a cancel-closer goroutine. A shared helper would need to abstract over both, adding
  parameters and indirection to the **hardware-validated** serial path.
- The `go/tooling` standard explicitly values "two short files that each read top-to-bottom," and
  `consolidate-over-accrete`'s operative corollary is "new capability grows as a **consumer of the
  existing core** (the Provider seam)" — which TCPProvider does. Duplicating a ~40-line loop behind
  a stable interface is not the harmful accretion that philosophy targets (that was 6 forked
  *decoders*); forking the validated serial reconnect logic to serve TCP's different needs would be.
- `ground-truth-first`: the serial reconnect path is validated against real re-enumeration behavior
  on macOS; leave it byte-for-byte unchanged.

Consolidation instead happens on the **consumer** side (§4), where the two providers genuinely share
one need (feeding the same chrome) with one shape.

## 4. Consumer diagnostic interface (`cmd/goaldl/tui.go`)

**Problem:** the model stores `serial *stream.SerialProvider` and reads `m.serial.Bytes()` /
`.Reconnecting()` / `.ReconnectAttempt()` at six sites (tui.go:675, 1444, 1481, 1486, 1580, 2011-12)
to drive the byte-rate waiting screen and the reconnecting indicator. A TCP source must feed the
same chrome.

**Solution (small, in `cmd/goaldl`):** define a consumer-side interface both providers already
satisfy structurally, and store that instead of the concrete serial pointer:

```go
// liveSource is the diagnostic surface the dashboard reads from a live provider
// (serial or TCP) to drive the waiting/reconnecting chrome. Replay has none.
type liveSource interface {
    Bytes() int64
    Reconnecting() bool
    ReconnectAttempt() int
}
```

- Rename the model field `serial *stream.SerialProvider` → `live liveSource` (nil for replay). The
  six read sites become `m.live.…`; the existing `m.serial != nil` guards become `m.live != nil`.
  The nil-interface-from-typed-nil trap the current code warns about (tui.go:194-196) is handled the
  same way: only assign `m.live` when the concrete provider is non-nil.
- Construction: the `-p` branch sets `live = serial`; the `-tcp` branch sets `live = tcpProv`; replay
  leaves it nil.
- This is the *only* consumer-logic change. Rendering, gating, reconnect copy — all unchanged; they
  now just read through the interface. No `pkg/stream` change is required for this (the interface
  lives in `cmd/goaldl`, matched structurally).

Waiting-screen semantics carry over unchanged and correctly: `Bytes()==0` while `Reconnecting()`
⇒ "connecting to bridge / no bytes yet" (bad `Addr`, bridge down); `Bytes()>0` but no frame ⇒
"connected, bytes flowing, no sync" (baud/polarity/wiring); `Reconnecting()` after a frame ⇒ the
reconnecting indicator. Same three-way split the serial waiting screen already renders (R5).

## 5. Flag wiring (`-tcp host:port`)

Two dispatch sites, kept consistent (R7):

### 5.1 `monitor` (`cmd/goaldl/monitor.go:26-83`)
- Add `tcpAddr := fs.String("tcp", "", "Live: TCP host:port of an ALDL bridge (mutually exclusive with -p)")`.
- Source resolution precedence & mutual exclusion (new guard before the existing `if *portName != ""`):
  reject if more than one of {`-p`, `-tcp`, replay-file arg} is set, with a clear message; reject if
  `-tcp` is combined with `-o` only if we decide recording is unsupported — **it is supported**, so
  `-o` teems the TCP Sink exactly like serial (wire `sink` into `TCPProvider{Sink: sink}`).
- New branch: `provider = &stream.TCPProvider{Addr: *tcpAddr, Config: cfg, Sink: sink}`;
  `title = "goaldl monitor — bridge " + *tcpAddr`.

### 5.2 TUI (`cmd/goaldl/tui.go:52-96` resolve + `129-145` construct)
- `resolveTUIFlags`: add `tcpAddr` flag; add `tcpAddr string` to `tuiFlags`. Source rule: a source is
  one of port / tcp / capture-file. The current `if *portName == ""` "then expect a file arg" logic
  becomes "if neither `-p` nor `-tcp`, expect a file arg" (so `-tcp` doesn't fall through to
  `errNoTUISource`). Trailing-flag re-parse (the `fs.Parse(fs.Args()[1:])` dance) only applies to the
  file branch, unchanged.
- Construction: add a `-tcp` branch beside serial/replay. Like serial, a live TCP source gets a
  `RecordSink` so the `r`/Log key can start/stop capture mid-session:
  `recSink = &stream.RecordSink{}; tcpProv = &stream.TCPProvider{Addr: cfg.tcpAddr, Config: cfg.cfg, Sink: recSink}; provider = tcpProv; live = tcpProv`.
- `errNoTUISource` help text gains a `-tcp` example line.

Flag name: **`-tcp`** taking a `host:port` string (not separate `-host`/`-port`) — one token, matches
the `net.Dial` address form, and reads well in help. IPv6 literals use the standard `[::1]:3333` form.

## 6. Constants & defaults (§ mirrors serial where possible)

```go
const (
    reconnectInterval    = time.Second      // REUSE the existing serial constant (same cadence)
    defaultTCPDialTimeout = 5 * time.Second  // per dial attempt
    defaultTCPReadTimeout = 3 * time.Second  // rolling read deadline; ~2.5× frame cadence (1.2s/frame)
    tcpHalfOpenWindows    = 4                // consecutive empty read windows ⇒ treat as dropped, redial
)
```

`ReadTimeout` rationale (ground-truth-anchored): a healthy bridge delivers a frame's worth of bytes
roughly every 1.2 s, so a 3 s window without a single byte is anomalous but not trigger-happy; after
`tcpHalfOpenWindows` such windows (~12 s) with zero bytes we assume a half-open socket and redial.
These are `should`-tunable, not protocol constants — documented as such (raw-data policy: transport
liveness heuristics are consumer-side, never a filter on frame content).

## 7. Test matrix (`pkg/stream/tcp_test.go`, new) — R10, no hardware/network

All tests use an in-process `net.Listener` on `127.0.0.1:0` (OS-assigned port); deterministic in CI,
`-race` clean. Mirrors the `SerialProvider` injectable-seam idiom (`dial`/`sleep` stubs where timing
must be controlled). Fixtures: `pkg/decoder/testdata/drive_4800.raw` + its golden frame expectation.

| ID | Case | Asserts | Req |
|----|------|---------|-----|
| T1 | Happy path | listener streams the fixture; emitted `FrameEvent`s equal decoding the fixture directly (reuse golden) | R1,R2 |
| T2 | Sink fidelity | tee to a buffer; buffer bytes == bytes the listener sent (byte-for-byte `.raw` interchangeability) | R3 |
| T3 | Reconnect-on-drop | listener sends partial stream, closes; second accept resumes; session survives, `Reconnecting()` toggled true→false, `ReconnectAttempt()`≥1, frames resume after gap | R4,R5 |
| T4 | Context cancel | cancel mid-stream; `Run` returns `ctx.Err()` within a bounded deadline (cancel-closer unblocks the read) | R6 |
| T5 | Half-open/timeout | listener accepts then goes silent (no close); read deadline trips → redial within ~`tcpHalfOpenWindows·ReadTimeout` rather than hanging | R11 |
| T6 | Dial-refused start | no listener at `Addr`; provider stays in the redial loop (not fatal); once a listener appears, connects and emits | R4 |
| T7 | Diagnostics race | `Bytes()` climbs with received data; read concurrently with `Run` under `-race` | R5 |
| T8 | Name | `Name()` == `"tcp:"+Addr`, stable across a reconnect | R8 |

Consumer-side (`cmd/goaldl`): extend a TUI test to assert the `-tcp` branch builds a `TCPProvider`,
sets `m.live`, leaves `m.replay`/`m.serial-equivalent` handling intact, and that the waiting-screen
byte-rate path reads through `liveSource` (drive it with a fake `liveSource`). Flag-parse tests:
`-p`+`-tcp` together → error; `-tcp`+file → error; `-tcp` alone → TCP source; help text lists `-tcp`.

**Optional dev fixture (plan open-question #4):** a `replayTCPServer(data []byte, paced bool)` test
helper in `tcp_test.go` doubles as the manual bench tool for `docs/mobile-ui.md` Stage 0/1. **Decision:
keep it a test helper, not a committed command** — it stays out of the production binary (no bloat,
consolidate-over-accrete) and is trivially promoted later if a standalone bench tool is wanted.

## 8. Edge cases & error handling

| Situation | Behavior |
|---|---|
| Bad `Addr` / bridge down at launch | Redial loop, waiting screen (R5); never fatal |
| Bridge reboots mid-drive | Redial, grids preserved, resync decoder, reconnecting indicator |
| Half-open socket (bridge powered off, no FIN) | Read-deadline windows → redial (T5) |
| `Sink` write error (disk full) | `Run` returns the error; in the TUI the `RecordSink` fail-soft layer above catches it and detaches recording without killing the session (existing behavior, unchanged) |
| `ctx` cancel (quit) | Prompt return of `ctx.Err()`; conn closed by cancel-closer (T4) |
| Bytes flowing but never a frame | Not the provider's concern — emits nothing; the waiting screen's "bytes, no sync" branch explains it (baud/polarity) |
| Malformed/garbage bytes | Faithfully fed to the decoder; raw-data policy — no filtering; decoder simply won't sync, exactly as with a bad serial line |
| IPv6 `Addr` | `[host]:port` form; `net.Dial` handles it |

## 9. Standards & layering compliance (self-check; formal gate in README)

- **architecture/session-api-layering** (must): TCPProvider is a `Provider`; consumers still go
  through `Session`/`Snapshot`. The `liveSource` interface lives in `cmd/goaldl` and reads only
  diagnostic scalars, not pipeline internals. ✅
- **decoder/raw-data-policy** (must): provider is a faithful byte transport; every aligned frame
  emitted warts-and-all; liveness timeouts gate the *connection*, never frame content. ✅
- **decoder/byte-value-decoding** (must): decode path untouched; TCP delivers the same UART byte
  values; timing-independence is the whole reason this works. ✅
- **release/platform-support** (must): `net` is pure-Go stdlib, `CGO_ENABLED=0`, no build tags, no
  OS-conditional code — cross-compiles on every tier; OS-specific seams (`pkg/serial`, VT) untouched.
  TinyGo door unaffected (this lives in `pkg/stream`, which Tier 3 already excludes). ✅
- **testing/golden-fixtures** (should): decoder goldens byte-identical; new tests reuse the drive
  fixture as oracle. ✅
- **go/tooling** (should): no new dependency (`net`/`context`/`time`/`io`/`sync/atomic` stdlib);
  gofmt/vet/build/`-race` gate. ✅
- **consolidate-over-accrete** (core): grows as a Provider consumer of the existing seam; consumer
  diagnostics unified via one interface; provider loop duplicated only where the paths truly diverge,
  with reasoning. ✅
- **ground-truth-first** (core): timeout constants anchored to the measured ~1.2 s frame cadence;
  hardware-validated serial reconnect path left unchanged; **real end-to-end validation against the
  ESP32-S3 + car is a required post-implementation step, not satisfied by the in-process tests**
  (synthetic sources share the decoder's assumptions — the standard's core caveat). ⏳ deferred with
  the implementation.

## 10. Files (when implemented — NOT this cycle)

| File | Change |
|---|---|
| `pkg/stream/tcp.go` | new — `TCPProvider` + Run + redial + diagnostics |
| `pkg/stream/tcp_test.go` | new — T1–T8 in-process listener tests + `replayTCPServer` helper |
| `cmd/goaldl/tui.go` | `-tcp` flag/field; `liveSource` interface; `m.serial`→`m.live`; `-tcp` construct branch; help text |
| `cmd/goaldl/monitor.go` | `-tcp` flag; source mutual-exclusion; `-tcp` construct branch; title |
| `cmd/goaldl/*_test.go` | consumer-side flag + `liveSource` tests |
| `CLAUDE.md`, `README.md` | document the `-tcp` source |
| `docs/mobile-ui.md` | cross-link "Stage 0 delivered by the TCPProvider spec" |

**Forbidden seam (must stay empty in the diff):** `pkg/stream/session.go`, `pkg/decoder/**`,
`pkg/ecm/**`, `pkg/blm/**`, `go.mod`, `go.sum`.

## 11. Deferral

Per user direction: **spec only.** Implementation waits on the ESP32-S3 (ordered) so that Stage 1/2
bring-up (firmware over the wire, then at the car) can validate the provider against a real bridge —
honoring ground-truth-first rather than shipping a transport proven only on a loopback socket. When
ready, run `implement-feature` against this spec.
