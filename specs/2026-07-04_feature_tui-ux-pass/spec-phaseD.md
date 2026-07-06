<!-- SDA: v1.0 -->
# Spec: TUI UX Pass — Phase D (Replay & startup ergonomics)

**Scope**: plan.md Phase D. Three loosely-coupled improvements, all keyed to the operator's first minutes with the tool — either bringing a live cable up (D.2, D.3) or navigating a captured drive (D.1):

1. **D.1 Replay position + seek (F6)** — a `t=34s / 812s (4%)` position readout plus `,`/`.` ±10 s seek and `0` restart.
2. **D.2 Port discovery UX (F7)** — with 0 or 2+ ports, enter the TUI in a **port-picker/waiting state** instead of exiting to stderr.
3. **D.3 Waiting-screen byte diagnostics (F8)** — before the first frame syncs on a live source, distinguish *no bytes at all* from *bytes but no frame sync*, with a rate.

**Architectural stance** — unlike Phases A–C (strictly `cmd/goaldl` consumer work), Phase D makes the epic's **only remaining below-facade additions**: two provider methods on `ReplayProvider` (`Seek`, duration) and one atomic byte counter on `SerialProvider` (`Bytes()`). This is the **same sanctioned seam** that took `RecordSink` and pause/speed in parity Phase 3 — additions on the **providers**, which sit *below* the `Session` facade. **`Session`, `Snapshot`, `pkg/ecm`, `pkg/decoder`, `pkg/blm` stay untouched; `Snapshot` gains no field; the decode path is byte-identical; `blm` still records 469 over the drive fixture.** The forbidden-seam diff is narrower than A–C by exactly these named provider files (`pkg/stream/replay.go`, `pkg/stream/serial.go`) — enumerated in the test plan.

**User decisions carried in (2026-07-05)**:
- **D.2 = in-TUI picker** (not error-text-only): with 0/2+ ports, the dashboard opens in a waiting/picker state, re-polls `AvailablePorts` on the existing staleness tick, arrow-selects, Enter connects.
- **D.1 backward seek does NOT rewind consumer grids/extrema** — grids accumulate a *session*, not a *timeline*; a re-played frame re-accumulates. Documented; `c` still clears. (Same rule the plan states for D.1.)
- **D.1 seek granularity** = `,`/`.` ±10 s, `0` restart. No ±60 s tier.

**Divergences from this spec that shipped (recorded during verify-feature retrospection, 2026-07-05; all PASS'd the fresh evaluator):**
- **Position readout moved to the header, `t=` dropped (user follow-on).** The spec put `t=m:ss / m:ss (N%)` on the replay playback row. As shipped, `replayPosition()` renders `m:ss / m:ss (N%)` (no `t=` prefix) in the **title-bar status, left of `Signal:`**; the playback row (`replayNav`) keeps only the controls (`[space] pause · [±] speed (N×) · [,/.] ±10s · [0] restart`). The `(Replay)` prefix on that row was also dropped (the row only renders in replay, so it's redundant).
- **`+`/`-` step a fixed speed ladder, not ×2/÷2 (user follow-on).** `adjustSpeed(up bool)` moves to the nearest stop on `speedSteps = {0.25,0.5,1,2,4,8,16}` so an off-ladder start (e.g. `-speed 5`) can reach 1× — a ×0.5 factor never would. Same 0.25×–16× clamp.
- **`m.serial` is a 1-method `byteSource` interface, not the concrete `*SerialProvider` (testability).** There is no serial-port fake in the tree, so the field is `interface{ Bytes() int64 }`; the assignment in `cmdTUI` is guarded (`if serial != nil`) so a nil `*SerialProvider` never becomes a non-nil interface on a replay source.
- **`ReplayProvider.PendingSeek()` added** as a read accessor (mirrors `Paused()`/`CurrentSpeed()`) so the TUI's seek-key tests can confirm the clamped target without decoding.

---

## D.1 — Replay position + seek (F6)

### Current behavior
`ReplayProvider` decodes `p.Data` up front into `frames` and paces emission by each frame's `ByteOffset/160` seconds (`replay.go:104,129`). Runtime controls exist (`SetPaused`/`SetSpeed`/`CurrentSpeed`/`Paused`) via a mutex + wall/data anchor pair (`anchorWall`/`anchorData`), re-anchored on every control change so a change is never retroactive. The TUI shows `(Replay) [space] pause · [±] speed (N×)` in `replayNav` (`tui.go:1503`) and a `⏸` in `sessionChrome` (`tui.go:1659`) — but **no position, no total, no seek**. `Snapshot.Elapsed` already carries the data-timeline position per frame (the TUI stores it as `m.latest.Elapsed`), so the *current* position is already in hand consumer-side; only **total** and **seek** are missing.

### Design — provider additions (below the facade)

Add to `ReplayProvider`, mirroring the existing pause/speed mutex discipline:

```go
// Duration returns the total data-timeline length of the capture — the last
// frame's Elapsed. Decoding is done once up front (Run already does it); cache
// it so Duration is O(1) and callable before Run starts.
func (p *ReplayProvider) Duration() time.Duration

// Seek requests a jump to data-timeline position target (clamped to
// [0, Duration]). It is applied at the next frame boundary in Run — the pacing
// loop reads the pending seek, repositions the frame index, and re-anchors
// (anchorData = the new position, anchorWall = now) so playback continues from
// there at the current speed with no catch-up rush. Safe to call concurrently
// with Run (guarded by the same mutex as pause/speed). No-op when Speed==0
// (unpaced) — consistent with SetPaused/SetSpeed being inert there.
func (p *ReplayProvider) Seek(target time.Duration)
```

Implementation notes:
- **Duration**: decode `p.Data` once (lazily, guarded) and cache both `frames` and `frames[len-1].Elapsed`. `Run` reuses the cached `frames` instead of re-decoding, so `Duration()` and `Run()` never disagree and the up-front decode isn't paid twice. Empty capture → `Duration()==0`.
- **Seek**: store a `seekTo *time.Duration` (nil = none) under the mutex. In `Run`'s pacing loop, after the `controls()` read, check for a pending seek: if set, find the first frame index whose `Elapsed >= target` (binary search over the monotonic `ByteOffset`), set the loop index there, set `anchorData = frames[i].Elapsed`, `anchorWall = now()`, clear `seekTo`, and continue. Because emission is a plain index walk, a **backward** seek simply resumes from an earlier index and **re-emits** those frames — which is exactly why consumer grids must be tolerant of re-accumulation (documented below).
- The unpaced (`Speed==0`, test) path ignores seeks — it emits as fast as possible with no control loop, so seek has no meaning there; documented on `Seek`.

**Why provider-level, not consumer-level**: the frame timeline lives in the provider (the consumer only ever sees the *current* `Snapshot`). Re-pacing after a jump needs the same anchor mechanics pause/speed already own. Reconstructing this above the facade would mean re-decoding in the consumer and forking the pacing logic — the coupling the layering standard exists to prevent. Seek belongs beside pause/speed. **No `Session`/`Snapshot` change** — seek is a provider control, exactly like `SetPaused`.

### Design — consumer (TUI)

- **Model handle**: `m.replay` already holds the `*ReplayProvider` (nil when live). Store `m.replayTotal time.Duration` once at construction (`replay.Duration()`), so the footer doesn't call into the provider every render.
- **Position readout**: extend `replayNav` (`tui.go:1503`) to lead with position:
  `(Replay)  t=0:34 / 13:32 (4%)   [space] pause · [±] speed (N×) · [,/.] ±10s · [0] restart`
  Position = `m.latest.Elapsed` (the last snapshot's data position), total = `m.replayTotal`, percent = `latest/total` (0 % when total is 0). Uses the existing `formatElapsed` (m:ss). The percent updates every frame as snapshots arrive — no extra timer.
- **Keys** (added to the `KeyMsg` switch, gated through the existing `replayGuard` so they're inert/warned on a live source or an unpaced replay, exactly like `space`/`±`):
  - `,` → `m.seekBy(-10 * time.Second)`
  - `.` → `m.seekBy(+10 * time.Second)`
  - `0` → `m.replay.Seek(0)` (restart)
  - `seekBy(d)` computes `target = clamp(m.latest.Elapsed + d, 0, m.replayTotal)` and calls `m.replay.Seek(target)`. No notice (the position readout reflects the jump live), matching how `adjustSpeed` stays silent.
- **Grid state on seek — unchanged (user decision).** A backward seek re-emits earlier frames; the TUI's grids/extrema (`accumulate`) simply keep accumulating over them. This is consistent with grids being **whole-session aggregates, not a timeline** (the rule already documented for `c`-clear and for the ring buffer). A doc comment on `seekBy` states this explicitly and points at `c` (clear) as the reset. The frame **ring buffer** (`m.buf`) likewise just keeps pushing — a Save Buffer after seeking dumps the most recent `frameBufCap` frames as always, including any re-played ones (documented, not silently surprising).

### Edge cases
- **Seek past either end**: clamped to `[0, Duration]`. `.` at the end is a no-op (already at/after last frame); `,` at the top clamps to 0.
- **Seek while paused**: the loop's pause branch still runs; `Seek` sets `seekTo`, and on the next control-loop iteration the seek applies even though playback stays paused (position readout jumps, playback remains frozen until `space`). Verified in test.
- **Unpaced replay (`-speed 0`)**: `replayGuard` already warns `unpaced replay (-speed 0)` for `space`/`±`; the seek keys route through it identically, so they warn rather than silently no-op.
- **Live source**: `m.replay == nil` → `replayGuard` warns `pause/speed are replay-only`; seek keys never reach the provider. (The `,`/`.`/`0` keys are otherwise unbound on live, so no collision.)
- **Empty/1-frame capture**: `Duration()==0` → percent shows `0%`, seek clamps to 0 (no-op). No divide-by-zero (percent guards `total==0`).

---

## D.2 — Port discovery UX (F7)

### Current behavior
`launchTUI` (`main.go:59`) auto-connects **only** when exactly one port is present; with 0 or 2+ ports it falls through to `cmdTUI` with no `-p`, which hits `errNoTUISource` and **exits to stderr** (`tui.go:108`). The stderr text lists example invocations but not the *actual detected ports* — the operator must separately run `goaldl ports`. F7: a multi-port machine (common: a PL2303 plus a built-in Bluetooth serial) dead-ends on the first run.

### Design — in-TUI port picker (user decision)

Rather than only improving the stderr text, **enter the dashboard in a picker/waiting state** when a live session is wanted but the port is ambiguous, and connect from inside the TUI. This keeps the adoption path on-screen (the operator never leaves the tool to discover a port name that "drifts", per CLAUDE.md).

**Entry condition** (in `launchTUI`, `main.go`): when `len(args)==0` (bare `goaldl`, the auto-connect path) **and** the port count is **not exactly 1**, launch `cmdTUI` in **picker mode** rather than exiting. A bare `goaldl` with exactly one port keeps today's zero-friction auto-connect; an explicit `-p name` or a capture-file arg is unchanged (never enters the picker). So the picker is strictly the *bare-invocation, ambiguous-port* case — it never overrides an explicit choice.

**Picker mode** — a pre-session TUI state that owns the screen *before* any `Session` starts:

```go
// portPicker is the pre-session state shown when a bare `goaldl` finds 0 or 2+
// ports. It re-polls on the staleness tick and, on Enter, tears itself down and
// starts a live Session on the chosen port. Nil once a session is running.
type portPicker struct {
    ports  []string // last AvailablePorts() result
    cursor int
    err    error    // AvailablePorts error, if any
}
```

- **Rendering** (a distinct `View` branch ahead of the waiting/error branches, like the A.1 fatal panel): a titled panel —
  - 2+ ports: `Select a port  (↑/↓ move · enter connect · q quit)` then the list, cursor row highlighted, each row `● /dev/cu.usbserial-10`.
  - 0 ports: `No serial ports found — plug in the adapter (retrying…)` plus the "macOS needs the PL2303 driver" hint from the hardware notes, and (dimmed) `q quit`. Re-polls live, so plugging in the cable makes a row appear without restart.
- **Polling**: reuse the **existing 1 s `tick()`** (already batched in `Init` for staleness). In picker mode, each `tickMsg` re-runs `serial.AvailablePorts()` and updates `ports`/`cursor` (clamp cursor into range; a vanished selected port snaps the cursor to a valid row). No new timer.
- **Auto-advance**: if a poll drops the count to **exactly 1**, the picker auto-selects it and connects (converges to the bare-`goaldl` happy path once the extra device disappears). Configurable? No — one port means unambiguous, connect.
- **Keys** (only while `m.picker != nil`, i.e. `portPicker`): `↑`/`↓` move the cursor; `enter` connects to `ports[cursor]`; `q`/`ctrl+c` quit (exit 0 — the operator chose not to connect, not an error). Other keys ignored.
- **Connecting**: on `enter`, the model needs to *start a Session it didn't have at launch*. Two implementation options — **spec picks (a)** for the smaller blast radius:
  - **(a) Re-exec the normal path**: the picker returns the chosen port to `cmdTUI`, which builds the `SerialProvider`/`RecordSink`/`Session` exactly as the `cfg.portName != ""` branch does today, then runs the *real* dashboard model. Concretely: `cmdTUI` runs a **tiny first Bubble Tea program** for the picker that quits returning the chosen port (or ""), then, if a port was chosen, falls into the existing live-setup block. This keeps session construction in one place (no half-built model), at the cost of two sequential `tea.NewProgram` runs (the picker is trivial and short-lived, so the alt-screen flthis is invisible in practice — verified by eye).
  - (b) reject: building the `Session`+goroutine lazily inside the running model duplicates the `cmdTUI` wiring (channels, cancel, recSink) into the `Update` path — more state, more error surface, for a one-time transition.

**Naming collision note**: Phase C already introduced an `outputPicker` field named `m.picker` for the Save Buffer/Log checklist modal. The port picker is a **different, pre-session** modal — name its field **`m.portPick`** (type `*portPicker`) to avoid confusion with the output `m.picker`. The two never coexist (port picker runs before the session; output picker only during it). Under option (a) the port picker isn't even a field on the *dashboard* model — it's its own tiny model — so the collision is avoided entirely; `portPick` naming applies only if a future refactor inlines it.

**stderr fallback stays for the non-bare case.** An explicit `goaldl -p badname` still errors normally (that's a live session that *failed*, A.1's fatal panel territory, not port ambiguity). And `errNoTUISource`'s message is still improved to **list detected ports** (cheap, helps the `goaldl` piped/non-TTY case where the picker can't run): `No source. Detected ports: /dev/cu.usbserial-10, /dev/cu.Bluetooth — pass one with -p, or run 'goaldl' on a terminal to pick.`

### Edge cases
- **Non-interactive stdin** (piped, CI): Bubble Tea needs a TTY. Guard with a **stdlib-only** TTY check — `fi, _ := os.Stdin.Stat(); isTTY := fi.Mode()&os.ModeCharDevice != 0` — and when not a TTY, fall back to the improved `errNoTUISource` stderr text; never hang a headless invocation on a keypress. (Stdlib on purpose: `go/tooling.md` favours minimal deps, and a new `x/term` direct dependency would break Phase D's "`go.sum` unchanged" claim — the char-device check is sufficient here.)
- **`AvailablePorts` error**: show it in the picker (`err` row: `port scan failed: …`), keep retrying on the tick — a transient enumeration error shouldn't kill the picker.
- **Port count races** (device unplugged between poll and Enter): the `SerialProvider` open will fail → the *live* session surfaces it via the A.1 fatal-error panel (existing path). The picker doesn't need to pre-validate.
- **Zero ports forever**: `q` exits cleanly (0). The panel keeps saying "retrying…" so the state is never mistaken for a hang.

---

## D.3 — Waiting-screen byte diagnostics (F8)

### Current behavior
Live, before the first frame syncs, `activeBody` renders a bare `waiting for frames…` (`tui.go:1442`, gated on `!m.hasFrame`). Nothing indicates whether **any bytes** are arriving — the classic "is it the cable, the port, or the baud/polarity?" question is unanswerable from the screen. `SerialProvider.Run` reads into a 512-byte buffer and feeds the decoder (`serial.go:42-59`), but the raw byte count never crosses the provider→session seam.

### Design — provider byte counter (below the facade)

Add an atomic read counter to `SerialProvider`, mirroring `RecordSink.Bytes()` precedent exactly:

```go
type SerialProvider struct {
    Port   string
    Baud   int
    Config decoder.Config
    Sink   io.Writer
    nbytes atomic.Int64 // total raw bytes read (diagnostics; see Bytes)
}

// Bytes returns the total raw bytes read from the port so far. Used by the
// waiting screen to distinguish "no bytes at all" (cable/port) from "bytes but
// no frame sync" (baud/polarity). Safe for concurrent read while Run streams.
func (p *SerialProvider) Bytes() int64 { return p.nbytes.Load() }
```

In `Run`, after a successful non-zero `Read`, `p.nbytes.Add(int64(n))` (one line, before the sink tee). Atomic → no lock, no contention with the read loop. **No `Snapshot` field, no `Session` change** — the TUI already holds the concrete `*SerialProvider`? **No** — today it holds `provider stream.Provider` and only keeps the *typed* `replay`/`recSink` handles. Add a typed `serial *stream.SerialProvider` handle in `cmdTUI` (nil on replay), passed to the model as `m.serial`, exactly as `m.replay`/`m.recSink` are. This is a **consumer-held provider handle**, not a facade change.

### Design — consumer (waiting screen)

Extend the `!m.hasFrame` branch of `activeBody` (live only — replay always has frames or ends) to show a diagnostic line driven by `m.serial.Bytes()` sampled on the existing tick:

- Track `m.firstByteAt`/rate in the model, or compute simply: keep `m.bytesSeen int64` updated each `tickMsg` from `m.serial.Bytes()`, plus the previous sample and the 1 s interval → a `B/s` rate (the tick is 1 s, so `rate = bytesSeen - prevBytesSeen`).
- Render:
  - `bytesSeen == 0`: `waiting for frames…  no bytes yet — check cable / port / driver` (amber). Points at the physical layer.
  - `bytesSeen > 0` but still `!hasFrame`: `waiting for frame sync…  159 B/s, no sync — check baud (-b) / polarity (-invert)` (amber). Points at decode config. 159 B/s is the known-good idle rate (CLAUDE.md), so a number near it with no sync strongly implies polarity/baud, and the hint says so.
- Replay: unchanged (`m.serial == nil` → no diagnostic line; replay reaching `!hasFrame` only at the very start is momentary).
- Once `hasFrame` flips true, the branch is gone (normal dashboard renders) — the diagnostic is purely a pre-sync aid.

This composes with A.2 staleness (post-sync) and A.1 fatal errors (open failure): A.1 covers *open failed*, D.3 covers *open OK but no frames yet*, A.2 covers *had frames, went quiet*. Three distinct, non-overlapping failure windows.

### Edge cases
- **Bytes arriving but garbage** (wrong polarity): `Read` returns bytes → counter climbs → D.3 shows the rate and the `-invert`/`-b` hint. Exactly the intended signal.
- **Slow trickle**: rate may read 0 B/s on a given tick even with a nonzero total; the total (`bytesSeen`) drives the cable-vs-config split, the rate is advisory. So "some bytes ever, 0 this second" still shows the *sync* hint, not the *cable* hint.
- **Sink write error** already returns from `Run` (`serial.go:51`) → A.1 fatal panel; the byte counter is irrelevant there.

---

## Non-goals (Phase D)
- **`?` help overlay, context-sensitive footer legend, rec→learn terminology, codes/flags session latch, PROM-gated extrema** — Phase E.
- **Grid rewind on seek** — explicitly out (user decision); grids are session aggregates, `c` resets.
- **Scrub-to-arbitrary-position / mouse timeline** — seek is keyboard ±10 s + restart only.
- **±60 s coarse seek tier** — not this slice (user chose ±10 s only).
- **Persisting the chosen port / a port config file** — no config layer yet (same stance as Phase C's per-invocation format choice).
- **Live re-connection / port hot-swap mid-session** — was a Phase D non-goal (the picker is pre-session only; a live disconnect surfaced via A.2 staleness + A.1). **Superseded 2026-07-05** by a follow-up feature (branch `fix/serial-reconnect`): `SerialProvider` now retries indefinitely on a missing/dropped port (initial open included), so the session and its grids survive an outage; the dashboard shows an inline `⟳ reconnecting…` for a brief blip and a full "waiting for a port" screen for startup-with-no-port / a prolonged outage, resuming intact when the cable returns.
- **Changing `Snapshot`/`Session`/`pkg/ecm`/`pkg/decoder`/`pkg/blm`** — the only core-adjacent changes are provider methods (`ReplayProvider.Seek`/`Duration`, `SerialProvider.Bytes`), below the facade. Decode path byte-identical; `blm` still 469 over the drive fixture.

## Files changed (planned)
| File | Change |
|---|---|
| `pkg/stream/replay.go` | `Duration()` (cached one-time decode, shared with `Run`); `Seek(target)` + `seekTo` field applied at the frame boundary with re-anchor; both mutex-guarded like pause/speed; no-op when `Speed==0` |
| `pkg/stream/serial.go` | `nbytes atomic.Int64`; `Bytes()`; one `Add` after a non-zero `Read` |
| `pkg/stream/replay_test.go` | seek forward/backward/clamp/while-paused; `Duration`; re-emission after backward seek |
| `pkg/stream/serial_test.go` (or existing) | `Bytes()` accrues over reads (fake port) — if a serial fake exists; else covered at the model level via a stub provider |
| `cmd/goaldl/main.go` | `launchTUI`: bare-invocation + not-exactly-1-port → picker mode; `errNoTUISource` message lists detected ports |
| `cmd/goaldl/tui.go` | D.1 `m.replayTotal`, `seekBy`, `,`/`.`/`0` keys through `replayGuard`, position in `replayNav`; D.2 `portPicker` model + `View`/`Update`/keys + `cmdTUI` two-stage launch + `os.Stdin.Stat()` char-device headless guard; D.3 `m.serial` handle, `m.bytesSeen`/rate on the tick, diagnostic waiting-screen text |
| `cmd/goaldl/tui_test.go` | tests below |

## Test plan
Model-level `.Update(msg)` idiom over the real `ReplayProvider`+`Session` on `drive_4800.raw` (as `TestTUIDriveFixtureEndToEnd`); provider-level unit tests for seek/duration/bytes; crafted stubs for the picker and byte-diagnostic oracles. Filesystem-free.

1. **`TestReplaySeek`** (provider unit): decode the drive fixture; `Duration()` == last frame Elapsed; `Seek(+t)` then run → first emitted Index is at/after the target; backward `Seek(0)` → next frames re-emit from index 0 (proves re-emission); `Seek(2×Duration)` clamps to the end; `Seek` while `SetPaused(true)` repositions but doesn't advance until resumed; `Speed==0` path ignores `Seek`. Uses the injectable `now`/`sleep` already in `ReplayProvider`.
2. **`TestReplayDuration`**: empty data → `Duration()==0`; single-frame → its Elapsed; matches the last `Elapsed` seen by a full `Run`.
3. **`TestTUISeekKeys`** (model): drive the fixture a few frames, `m.replayTotal` set from `Duration()`; `,`/`.`/`0` call `Seek` with the clamped target (assert via a spy `ReplayProvider` or the provider's resulting next index); `replayNav` string contains `t=` / total / `%` and the seek-key hints; on a **live** model the same keys warn (`pause/speed are replay-only`) and never seek; unpaced replay warns `unpaced replay`.
4. **`TestSerialBytes`** — `Bytes()` starts 0, accrues the sum of `Read` lengths, is stable after `Run` returns. No serial fake exists in the tree today, so this is **not optional**: cover it at the **model level** via a stub provider exposing `Bytes()` (the same stub `TestWaitingDiagnostics` uses), asserting the counter the waiting screen reads. (If a serial fake is later added, promote to a provider-unit test.)
5. **`TestWaitingDiagnostics`** (model): with `m.serial` a stub reporting **0** bytes and `!hasFrame` → waiting text contains `no bytes` + cable hint; stub reporting **>0** bytes still `!hasFrame` → text contains `no sync` + `-b`/`-invert` hint and the `B/s` rate; replay (`m.serial==nil`) → bare `waiting for frames…` unchanged; once `hasFrame`, neither string renders.
6. **`TestPortPicker`** (model): a `portPicker` with 2 ports renders the list + `enter connect`; `↑`/`↓` move the cursor (clamped); a `tickMsg` re-polls (inject the port lister) and, when the list drops to 1, auto-advances/returns the chosen port; 0 ports renders the retry + driver hint; `q` returns quit with exit-0 intent (no `fatalErr`). The `AvailablePorts` call is injected (a func field or package var) so the test controls the sequence.
7. **Regression**: `go test -race -count=1 ./...` green; `go vet` + `gofmt -l pkg cmd` clean; decoder goldens **byte-identical** (no `-update`); `blm` command still **469** over `drive_4800.raw`; **forbidden-seam diff empty** for `pkg/stream/session.go`, `pkg/stream/stream.go`, `pkg/ecm`, `pkg/decoder`, `pkg/blm`, `go.mod`, `go.sum` — Phase D's `pkg/stream` changes are confined to **`replay.go`** and **`serial.go`** (provider files below the facade), plus `cmd/goaldl`. `Snapshot` has no new field (grep the diff).

## Suggested implementation slices
1. **Slice 1 — D.1 replay position + seek** (self-contained provider + footer + keys; the highest-value analysis win, no startup-flow risk).
2. **Slice 2 — D.3 byte diagnostics** (tiny provider counter + waiting-screen text; independent).
3. **Slice 3 — D.2 port picker** (the largest, touches `launchTUI` + a pre-session model; ship last so the two-stage launch lands on a stable base).

Each slice is independently shippable, testable, and verifiable; verify-feature can gate them together or in turn. D.1 and D.3 are pure additions; D.2 changes the bare-`goaldl` startup path and warrants the most eyes.
