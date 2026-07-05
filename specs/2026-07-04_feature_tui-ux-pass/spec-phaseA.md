<!-- SDA: v1.0 -->
# Spec: TUI UX Pass — Phase A (Trust)

**Scope**: plan.md Phase A — findings F1 (silent session errors), F3 (no staleness signal), F2 (free-running knock counter shown as data), F15 (stale help text). All four are **consumer/presentation-only**: no `Session`/`Snapshot`/`pkg/ecm`/`pkg/decoder`/`pkg/blm` change, no new dependency. The work lands in `cmd/goaldl/tui.go`, `cmd/goaldl/main.go`, and one pure-builder signature in `pkg/stream/gridviews.go`.

**User decisions (2026-07-04)**:
- **A.3 knock**: *warn + keep grid normal* — a warning line in the Spark status area, grid values render at normal brightness, the explainer's "goal is 0 everywhere" line is replaced by a free-run note. (Raw-data-raw: annotate, never hide or dim.)
- **A.1 fatal error**: *full-screen error panel* — replace the tab view with an error panel (error text + serial hints), and re-print to stderr on exit.
- **A.2 staleness**: *~6 s (5 missed frames)* before flagging data stale on a live source.

---

## A.1 — Surface session errors (F1)

### Current behavior
`cmdTUI` runs the session in a goroutine and **discards** `session.Run`'s return (`tui.go:147-155`). `SerialProvider.Run` returns the real error on a failed open / read (`serial.go:25-45`); `Session.Run` passes it straight through (`session.go:50-54`). So the error already exists at the goroutine — it is dropped. Result: a failed port open renders as `waiting for frames… (stream ended)` with no diagnosis.

### Design

**Error transport (cmdTUI).** Add a buffered `errCh := make(chan error, 1)`. The goroutine sends the run error, then closes `snaps`:

```go
go func() {
    runErr := session.Run(ctx, func(s stream.Snapshot) {
        select {
        case snaps <- s:
        case <-ctx.Done():
        }
    })
    errCh <- runErr   // buffered(1); always sent before the channel closes
    close(snaps)
}()
```

**Delivery (waitForSnapshot).** When `snaps` is closed, read the (already-sent) error and carry it on `providerDoneMsg`:

```go
type providerDoneMsg struct{ err error }

func (m tuiModel) waitForSnapshot() tea.Cmd {
    return func() tea.Msg {
        s, ok := <-m.snaps
        if !ok {
            return providerDoneMsg{err: <-m.errCh}
        }
        return snapshotMsg(s)
    }
}
```
The model gains an `errCh <-chan error` field wired from `cmdTUI`.

**Classification (Update).** On `providerDoneMsg`:

```go
case providerDoneMsg:
    m.done = true
    if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
        m.fatalErr = msg.err
    }
```
- `nil` → normal end of stream (replay finished / port closed cleanly) → existing `(stream ended)` footer, unchanged.
- `context.Canceled` → the user quit (`m.cancel()` was called) → **not** an error; ignore. (In practice `q` triggers `tea.Quit` immediately so this rarely renders, but the guard is correct if the stream is cancelled another way.)
- any other error → `m.fatalErr` set → full-screen error panel.

**Render (View).** When `m.fatalErr != nil`, short-circuit to the error panel **before** the `!hasFrame` waiting branch (a port that never opened has no frames):

```
  ⚠ Cannot read from live:/dev/cu.usbserial-10

  serial: open /dev/cu.usbserial-10: no such file or directory

  • Check the cable and run:  goaldl ports
  • Wrong baud rate?  add  -b 2400
  • Non-inverting cable?  add  -invert

  q quit
```
- Header line names the source (`m.source`).
- Middle line is the raw error text (`m.fatalErr`).
- The three hints are **serial-specific** and shown only for a live source (`m.replay == nil`). For a replay source (fatal replay errors are essentially unreachable — the file is read before `Run`, `tui.go:128-135` — but handled for completeness) the hints are omitted; just error text + `q quit`.
- Rendered with the existing `beatBad`/`dimStyle` styles (no new styles unless the panel needs one).

**Exit (cmdTUI).** After `tea.NewProgram(...).Run()` returns, if the final model carries a `fatalErr`, re-print it to stderr and exit non-zero so it survives the alt-screen teardown and is scriptable:

```go
if fm, ok := final.(tuiModel); ok {
    fm.closeOutputs()
    if fm.fatalErr != nil {
        fmt.Fprintf(os.Stderr, "goaldl: %v\n", fm.fatalErr)
        os.Exit(1)
    }
}
```
(The existing `err` from `.Run()` — a Bubble Tea internal failure — keeps its current handling.)

### Edge cases
- **Quit before any frame**: `q` → `cancel()` + `tea.Quit`; goroutine later sends `context.Canceled`, but the program has exited — nothing to render. Safe.
- **Error after good frames** (cable yanked mid-drive): `fatalErr` set, panel replaces the tabs. The accumulated grids are lost from view, but Phase C (quit/exit summary) and the fact that `closeOutputs` still flushes any open recording/CSV mitigate this; within Phase A scope the panel correctly reports *why* the stream died. The stderr reprint fires on exit.
- **`n == 0` read timeouts** (`serial.go:46-48`) are not errors — the provider loops. Only a real read error terminates `Run`. So a live cable that is connected but silent does **not** trip the panel; it trips staleness (A.2) instead. This is the correct split: panel = transport dead, staleness = transport alive but no frames.

---

## A.2 — Staleness indicator (F3)

### Design
A self-rescheduling 1 s tick lets the model notice that frames have stopped without a new `snapshotMsg`.

**Time tracking (model fields).**
- `lastFrameAt time.Time` — wall time the most recent `snapshotMsg` arrived.
- `now time.Time` — advanced by the tick and by each snapshot; the reference the View compares against.

**Tick command.**
```go
type tickMsg time.Time
func (m tuiModel) tick() tea.Cmd {
    return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}
```
`Init` returns `tea.Batch(m.waitForSnapshot(), m.tick())`. On `tickMsg`: set `m.now = time.Time(msg)` and return `m.tick()` (reschedule). On `snapshotMsg`: set both `m.lastFrameAt` and `m.now` to `time.Now()`.

**Staleness predicate (pure, testable).**
```go
const staleAfter = 6 * time.Second // ~5 frames at the ~1.2s cadence

// stale reports whether the live stream has gone quiet, and for how long.
// Only meaningful for a live source that has delivered at least one frame and
// has not ended; replay pacing and end-of-stream are not "stale".
func (m tuiModel) stale() (bool, time.Duration) {
    if m.replay != nil || !m.hasFrame || m.done {
        return false, 0
    }
    age := m.now.Sub(m.lastFrameAt)
    return age >= staleAfter, age
}
```
Tests call `stale()` directly against a model with crafted `now`/`lastFrameAt` — no wall-clock dependency in the assertion.

**Render.**
- **Heartbeat** (`heartbeat()`): when `stale()` is true, render a **hollow** `○` in `beatBad` (red) instead of the filled `●`. This also satisfies F16 (glyph, not just color) for the stale case. Non-stale behavior unchanged (green ● good / red ● bad).
- **Footer**: when stale, append `dimStyle`-rendered `no data 8s` (rounded whole seconds) to the status segment, near the ok/bad counts.

### Edge cases
- **Replay**: `m.replay != nil` → never stale (paced playback and the paused state are expected quiet). End-of-replay is `m.done` → `(stream ended)`, not stale.
- **Before first frame**: `!m.hasFrame` → not stale; the waiting screen (A.1-adjacent) owns that state.
- **Recovery**: a new frame resets `lastFrameAt`; the next render clears the stale markers automatically.
- **Determinism**: the only wall-clock reads are in the production tick and in `snapshotMsg` (`time.Now()`); the predicate is pure over model fields, so `TestTUIStale` is deterministic.

---

## A.3 — Free-running knock-counter detection (F2)

### Rationale
On the target vehicle `KNOCK_CNT` advances ~+76 every frame (verified in `drive_4800.raw` **and** the WinALDL ground-truth log `data/20250601_111156_LOG.txt`) — it is not a knock signal, so the Spark grid's large per-cell sums are counter artifacts. Genuine ESC knock produces a nonzero delta on only a handful of frames across a whole session. The two regimes are cleanly separable by the fraction of recent parsed frames carrying a nonzero delta.

Raw-data-raw governs: the values are **not** filtered, hidden, or dimmed — the grid renders exactly as today. We add a **consumer-side annotation** so the operator isn't misled.

### Design (consumer-side, in the model)
The model already differences the cumulative counter in `accumulate` (`tui.go:442-448`). Reuse that delta to feed a fixed-size sliding window over **parsed** frames:

- `knockWindow [knockWindowSize]bool` (or an equivalent ring) — did frame *i* have a nonzero knock delta?
- `knockWindowCount int` — frames seen (caps at `knockWindowSize`).
- `knockNonzero int` — count of `true` entries currently in the window.

```go
const (
    knockWindowSize = 40   // recent parsed frames considered
    knockWindowMin  = 20   // need this many before judging
    knockFreeFrac   = 0.5  // ≥50% nonzero deltas ⇒ free-running
)

// in accumulate, right where the knock delta is computed (baseline frame excluded):
if m.hasKnockBase {
    delta := math.Mod(knock-m.knockPrev+256, 256)
    m.pushKnock(delta > 0)          // slide the window
    if delta > 0 { m.sparkGrid.Add(ft.RPM, ft.MapKPa, delta) }
}
```

**Predicate (pure).**
```go
func (m tuiModel) knockFreeRunning() bool {
    if m.knockWindowCount < knockWindowMin {
        return false
    }
    return float64(m.knockNonzero)/float64(m.knockWindowCount) >= knockFreeFrac
}
```

**Builder signature.** `SparkBody` gains a `freeRunning bool` parameter (kept pure — the model computes it):
```go
func SparkBody(g *blm.Grid, ev FrameEvent, knockCnt float64, freeRunning bool) string
```
When `freeRunning`:
- The status line appends `  ⚠ free-running counter — not knock` (in `ansiBold`, consistent with the builder's inline-ANSI idiom; no lipgloss inside pure builders).
- The explainer swaps its first sentence for a free-run note. Concretely, `sparkExplainer` splits into two constants sharing the tail: the normal head ("knock events … the goal is 0 everywhere") vs a free-run head ("This ECM's KNOCK_CNT is advancing every frame — it is a free-running counter on this vehicle, not a knock count; the cell totals below are not meaningful. On an ECM with a working ESC, a cell total > 0 means detonation was counted there."). Grid brightness is unchanged in both.

The TUI call site passes `m.knockFreeRunning()`:
```go
case m.active == viewSpark:
    body = stream.SparkBody(m.sparkGrid, m.lastGood.FrameEvent,
        m.lastGood.Sensors["knock_count"], m.knockFreeRunning())
```

### Edge cases
- **Sparse genuine knock** (success criterion 3): a crafted capture with nonzero deltas on < 50 % of frames → `knockFreeRunning()` stays false → no warning, normal explainer. Deterministic model-level test.
- **Warm-up window**: fewer than `knockWindowMin` parsed frames → false (no premature warning on the first second of data).
- **`c` clears the Spark grid** (`clear()` → `blm.NewSpark()`): the grid resets, but the knock **window and baseline are intentionally preserved** — the free-run property is a fact about the counter, not the grid, and clearing must not manufacture a phantom baseline (mirrors the existing "clear keeps the knock baseline" rule at `tui.go:475-477`). Documented in code.
- **Counter reset mid-session** (ECM power-cycle) produces one wrapped delta — already an accepted artifact (gridviews.go SparkBody comment); it perturbs the window by at most one frame, far below the threshold's margin.

---

## A.4 — Fix stale help text (F15)

`main.go printUsage` currently prints (line 72):
```
  keys: 1-3 / tab switch views · q quit
```
Two phases out of date (8 tabs, session keys). Replace the dashboard block's key line with an accurate summary:
```
  keys: 1-8 select tab · tab/←→ cycle · s save · c clear · r rec · d csv · space/± replay · q quit
```
And update the one-line dashboard description above it from "tab between sensors / BLM grid / raw" to "sensors · fuel-trim grids · flags · codes · raw". No other `printUsage` change in Phase A (a `?` overlay mention is deferred to Phase E, which adds it).

---

## Non-goals (Phase A)
- No layout/clamping work (Phase B) — the error panel and stale footer assume the current concatenated `View()`; they do not fix the 80×24 footer clipping.
- No quit/clear guards or exit summary (Phase C). A.1's stderr reprint is not the Phase C exit summary.
- No provider-level change: A.1 reads an error that already exists; A.2/A.3 are model state. `SerialProvider`/`ReplayProvider`/`Session`/`Snapshot` untouched. (The byte-counter diagnostic on the waiting screen is **D.3**, not A.2 — A.2 only covers the *have-had-a-frame-then-went-quiet* case.)
- No decode-path change; goldens stay byte-identical.

## Files changed (planned)
| File | Change |
|---|---|
| `cmd/goaldl/tui.go` | `providerDoneMsg{err}`; `errCh`/`fatalErr`/`lastFrameAt`/`now`/knock-window fields; `tick()`+`tickMsg`; `stale()`; `knockFreeRunning()`/`pushKnock`; error-panel + stale rendering in `View`/`heartbeat`; Spark call passes `freeRunning`; `Init` batches the tick; `cmdTUI` wires `errCh` and reprints `fatalErr` on exit |
| `cmd/goaldl/main.go` | `printUsage` dashboard key line + description (A.4) |
| `pkg/stream/gridviews.go` | `SparkBody(..., freeRunning bool)`; split `sparkExplainer` into normal/free-run heads sharing a tail |
| `cmd/goaldl/tui_test.go` | `TestTUIFatalError` (classification + panel + Canceled-ignored), `TestTUIStale` (pure predicate + heartbeat glyph + footer), `TestTUIKnockFreeRunning` (fixture true / crafted-sparse false / clear-keeps-window) |
| `pkg/stream/gridviews_test.go` | `TestSparkBodyFreeRunning` (status warning present/absent; explainer head swap; grid brightness unchanged) |

## Test plan
Named oracles, model-level, matching the existing `.Update(msg)` idiom (`tui_test.go`):

1. **A.1 `TestTUIFatalError`**: feed `providerDoneMsg{err: errors.New("serial: open …")}` → `fatalErr` set, `View()` contains the error text + `goaldl ports` hint (live model) and omits it (replay model); feed `providerDoneMsg{err: context.Canceled}` → `fatalErr` nil, `done` true (existing `(stream ended)` path); feed `providerDoneMsg{err: nil}` → unchanged normal end.
2. **A.2 `TestTUIStale`**: construct a live model (`replay == nil`, `hasFrame == true`) with `lastFrameAt` fixed and `now` = +6.1 s → `stale()` true, `heartbeat()` renders the hollow `○`, `View()` footer contains `no data`; `now` = +2 s → false; a replay model (`replay != nil`) at +30 s → false; `done` model → false.
3. **A.3 `TestTUIKnockFreeRunning`**: drive the model over `drive_4800.raw` via a real `Session`/`ReplayProvider` (as `TestTUIDriveFixtureEndToEnd` does, `tui_test.go:805`) → `knockFreeRunning()` true and `SparkBody` output contains the warning; a crafted frame sequence with a nonzero knock delta every ~10th frame → false, no warning; after `knockWindowMin` reached then `c` on the Spark tab → window retained (still true), grid `Sum()==0`.
4. **A.3 `TestSparkBodyFreeRunning`** (builder): `SparkBody(g, ev, cnt, true)` contains the `⚠ free-running` status token and the free-run explainer head, not "goal is 0 everywhere"; `SparkBody(g, ev, cnt, false)` is the current output verbatim (guards against accidental normal-path change).
5. **A.4**: covered by `go vet`/manual — `printUsage` is not unit-tested today; no test added (consistent with the existing untested `printUsage`). Verified by running `goaldl help`.
6. **Regression**: `go test -race -count=1 ./...` green; `go vet` + `gofmt -l pkg cmd` clean; decoder goldens byte-identical (no `-update`); `blm` still 469 over the drive fixture; forbidden-seam diff (`pkg/stream/session.go`, `pkg/ecm`, `pkg/decoder`, `pkg/blm`, `go.mod`) — only `gridviews.go`'s `SparkBody` signature/explainer changes in `pkg/stream`, all else empty.
