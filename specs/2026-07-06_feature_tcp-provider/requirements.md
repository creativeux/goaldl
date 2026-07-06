<!-- SDA: v1.0 -->
# Requirements: TCPProvider

## Goal

Let desktop `goaldl` consume the live ALDL byte stream over a **TCP socket** instead of a local
serial port, so an ESP32-S3 wireless bridge (WiFi AP → TCP, or wired Ethernet → TCP) is a
drop-in replacement for the PL2303 cable. The same provider is the transport a future iOS/mobile
front-end binds to (via gomobile), so the desktop app is both the first consumer and the
reference test rig for the bridge.

Context and transport rationale: [`docs/mobile-ui.md`](../../docs/mobile-ui.md). Bridge hardware
plan lives in the same doc.

## Background (why this is nearly free)

The engine already isolates the exact seam a network source slots into. Everything above the
`Provider` interface (`pkg/stream/stream.go:32`) — `Session`, `Snapshot`, the TUI, `blm`, every
view builder — consumes decoded `FrameEvent`s and is blind to their origin. `SerialProvider`
(`pkg/stream/serial.go`) already does *exactly* the work a TCP source needs: read raw bytes into
a buffer, `decoder.Feed` each byte, `emit` a `FrameEvent` per frame, optionally tee raw bytes to
a `Sink`, and reconnect on a dropped source. The byte-value decoding model makes the transport
irrelevant to correctness: byte values are timing-independent, so TCP's bursty delivery is
harmless, and a `.raw` recorded over TCP is byte-for-byte identical to one recorded over serial.

## Functional Requirements

- **R1 — Provider implementation.** A `TCPProvider` type in `pkg/stream` satisfying
  `Provider` (`Name() string`, `Run(ctx, emit) error`). Dials a `host:port`, reads bytes, feeds
  `decoder.New(cfg)`, emits one `FrameEvent` per decoded frame with a monotonic index and elapsed
  time — same event shape SerialProvider produces.
- **R2 — Config parity.** Carries the same `decoder.Config` as the other providers so baud model,
  polarity, and pulse thresholds are identical to a serial session (the bridge forwards the same
  UART byte values, so the decoder config is unchanged).
- **R3 — Raw capture tee.** Optional `Sink io.Writer` teeing every received byte, so
  `record`/the TUI's Log feature can capture a bridge session to a `.raw`. This is what makes the
  byte-for-byte interchangeability provable.
- **R4 — Reconnect on drop.** A dropped connection (bridge reboot, WiFi blip, car bump) must not
  end the session: retry the dial until it succeeds or the context is cancelled, mirroring
  `SerialProvider`'s reconnect contract so the dashboard keeps its accumulated grids across an
  outage. Decoder is resynced from scratch after a gap.
- **R5 — Connection diagnostics.** Expose enough state for the TUI waiting/reconnecting screens to
  distinguish "never connected" (wrong host/port, bridge down) from "connected, bytes flowing but
  no frame sync" (baud/polarity/wiring) from "was connected, now reconnecting" — the network
  analogues of `SerialProvider.Bytes()` / `Reconnecting()` / `ReconnectAttempt()`.
- **R6 — Context cancellation.** `Run` returns promptly on `ctx` cancel and never blocks past it
  (honor the `Provider` contract); socket reads must be unblocked by cancellation, not left
  dangling on a half-open connection.
- **R7 — `-tcp host:port` flag.** A new source selector wired into both dispatch sites that build
  a provider today: the TUI (`cmd/goaldl/tui.go:129-143`) and `monitor` (`cmd/goaldl/monitor.go:50-81`).
  Mutually exclusive with `-p`/replay-file; clear error if more than one source is given.
- **R8 — `Name()` for chrome.** `Name()` returns a `tcp:host:port` style label so the existing
  status chrome and provider-error messages read sensibly (parallels `live:<port>`).
- **R9 — No facade change.** `Session`, `Snapshot`, `pkg/decoder`, `pkg/ecm`, `pkg/blm` are
  untouched. Decoder golden fixtures remain byte-identical. This is a below-facade transport
  addition only.
- **R10 — Testability without hardware.** The provider must be unit-testable with an in-process
  `net` listener (stdlib), no real network or ECM: a fake bridge accepts a connection and writes
  a committed fixture's bytes; the test asserts the emitted frames match the fixture's known
  frames, exercises the reconnect path (server drops mid-stream), and confirms the `Sink` tee
  reproduces the input bytes. Follow the existing injectable-seam test idiom (`SerialProvider`'s
  `open`/`sleep`/`listPorts`).
- **R11 — Read timeout / liveness.** Reads should use a bounded deadline so a silently half-open
  socket (bridge powered off without a clean close) is detected and routed into the reconnect
  loop, rather than the session hanging forever on a dead connection.
- **R12 — Docs.** Note the new `-tcp` source in `CLAUDE.md`'s command list and README usage, and
  cross-link `docs/mobile-ui.md` (which already anticipates a `TCPProvider`).

## Non-Goals (explicitly out of scope)

- **No BLE, USB-gadget, or MFi transport** — those are separate future providers/transports over
  the same seam (see `docs/mobile-ui.md`). This feature is TCP only.
- **No ESP32 firmware** — the bridge firmware is a separate hardware track. This feature is
  desktop-side Go only. (A tiny replay-over-TCP stub server *for testing* is in scope as a test
  fixture / optional dev tool, not production firmware.)
- **No iOS/gomobile binding** — the byte-push binding is a later feature; this provider is its
  foundation but does not implement it.
- **No TLS/auth/encryption** — the bridge is a point-to-point link on a private car-local network;
  a plaintext socket matches the WiFi-OBD2-dongle model. Security hardening is a non-goal now.
- **No service discovery (mDNS/Bonjour)** — `host:port` is given explicitly. Auto-discovery is a
  possible later convenience, not required.
- **No multi-client / server mode** — `goaldl` is a TCP *client* dialing the bridge. The bridge is
  the server. `goaldl` does not listen.
- **No change to the decode path or raw-data policy** — the provider is a faithful transport;
  every structurally-aligned frame is emitted warts-and-all, exactly as SerialProvider does.

## Success Criteria

1. `goaldl -tcp <host:port>` (and `monitor -tcp`) drives the full dashboard/table from a TCP byte
   source, producing the same frames a serial source would from the same bytes.
2. A `.raw` recorded via the TCP Sink over a session is **byte-for-byte identical** to the source
   bytes (mechanized interchangeability proof), and decodes to the same frames as the equivalent
   serial capture.
3. Dropping and restoring the connection mid-session keeps the session alive and resumes frames,
   with the dashboard showing a reconnecting state (not a fatal error) — same UX as a bumped cable.
4. Unit tests pass with **no hardware and no external network** (in-process listener + committed
   fixture), covering: happy-path frame emission, reconnect-on-drop, Sink tee fidelity,
   context-cancel promptness, and the half-open/timeout path.
5. `go test -race ./...` green; gofmt/vet clean; decoder goldens byte-identical; `blm` over the
   drive fixture still reports its known cell count. Forbidden seam (`session.go`, `decoder`,
   `ecm`, `blm`, `go.mod`) diff empty.
6. No new third-party dependency — `net`, `context`, `time` from stdlib only.
