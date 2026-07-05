<!-- SDA: v1.0 -->
# Tasks: TUI UX Pass — Phase A (Trust)

Implementation checklist for [spec-phaseA.md](spec-phaseA.md). Consumer/presentation-only; no `Session`/`Snapshot`/`ecm`/`decoder`/`blm` change.

## A.3 — free-running knock detection (builder first, so the model can call it)
- [x] **T1** `pkg/stream/gridviews.go`: split `sparkExplainer` into a shared tail + normal head + free-run head; `SparkBody` gains `freeRunning bool` (status-line `⚠ free-running counter — not knock` in `ansiBold`; explainer head swap; grid brightness unchanged).
- [x] **T2** `pkg/stream/gridviews_test.go`: `TestSparkBodyFreeRunning` — `true` shows warning + free-run head; `false` output **byte-identical** to the pre-change `SparkBody` (guard the normal path).

## A.1 — surface session errors
- [x] **T3** `cmd/goaldl/tui.go`: `providerDoneMsg{err error}`; model fields `errCh <-chan error`, `fatalErr error`; `waitForSnapshot` reads `errCh` on close; `Update` classifies (nil / context.Canceled / fatal); `View` renders the full-screen panel (live-only serial hints) ahead of the waiting branch; `cmdTUI` wires `errCh` and reprints `fatalErr` to stderr + exit 1.
- [x] **T4** `cmd/goaldl/tui_test.go`: `TestTUIFatalError` — fatal error sets `fatalErr` + panel text + `goaldl ports` hint (live) / no hint (replay); `context.Canceled` → no `fatalErr`, `done` true; `nil` → normal end.

## A.2 — staleness indicator
- [x] **T5** `cmd/goaldl/tui.go`: `tickMsg`, `tick()`, `Init` batches the tick; model fields `lastFrameAt`, `now`; `snapshotMsg`/`tickMsg` update them; pure `stale()`; `heartbeat()` hollow `○` when stale; footer `no data Ns`.
- [x] **T6** `cmd/goaldl/tui_test.go`: `TestTUIStale` — live +6.1 s stale (glyph + footer), +2 s not, replay not, done not, recovery clears.

## A.3 — model-side detection wiring
- [x] **T7** `cmd/goaldl/tui.go`: knock-window fields + `pushKnock`; compute in `accumulate` beside the existing delta; `knockFreeRunning()`; Spark call site passes it; `c`-clear keeps the window+baseline.
- [x] **T8** `cmd/goaldl/tui_test.go`: `TestTUIKnockFreeRunning` — drive fixture → true + warning in `View`; crafted sparse (~1/10) → false; clear keeps window.

## A.4 — help text
- [x] **T9** `cmd/goaldl/main.go`: `printUsage` dashboard description + key line accurate to the 8-tab / session-key dashboard.

## Verify
- [x] **T10** `go test -race -count=1 ./...` green; `go vet` + `gofmt -l pkg cmd` clean; decoder goldens byte-identical (no `-update`); `blm` still 469 over `drive_4800.raw`; forbidden-seam diff (`session.go`/`ecm`/`decoder`/`blm`/`go.mod`) empty, only `gridviews.go` `SparkBody` in `pkg/stream`.

---

# Tasks: TUI UX Pass — Phase C (Session safety + unified outputs)

Implementation checklist for [spec-phaseC.md](spec-phaseC.md). Consumer/presentation-only (`cmd/goaldl` + new `framebuf.go`; reuse of unchanged `RecordSink`/`saveGrids`/`frameCSV`). Forbidden seam stays empty; goldens byte-identical; `blm` 469.

## Slice 1 — C.0 unified outputs

- [x] **C1** `cmd/goaldl/framebuf.go` (new): `bufFrame` (data/elapsedSec/byteOffset/parseOK/promOK/vals) + `frameBuf` ring (`push`/`frames`/`fillPct`), `frameBufCap = 3600`.
- [x] **C2** `cmd/goaldl/framebuf_test.go` (new): `TestFrameBuf` — wrap keeps last cap in oldest→newest order; partial fill count + pct; empty.
- [x] **C3** `cmd/goaldl/csv.go`: `frameCSV.WriteRow(bufFrame)` sharing the row body with `Write` (header/`%.2f` unchanged; ParseOK-only).
- [x] **C4** `cmd/goaldl/tui.go`: refactor `saveGrids` → `saveGrids(dir, base string, sel []gridSel, minSamples int)` (`gridSel{id,suffix,write}`); build the four selectors from the grids.
- [x] **C5** `cmd/goaldl/tui.go`: `outputPicker` modal (state `op`/`items`/`cursor`/`name`/`hint`); key handling (↑↓ move, space toggle, runes/backspace edit name row, enter confirm, esc cancel, ctrl+c quit); render (checklist + name + resolved dest dir, F17).
- [x] **C6** `cmd/goaldl/tui.go`: ring `push` in `snapshotMsg` (every frame; vals in `def.Parameters` order); `buf` field.
- [x] **C7** `cmd/goaldl/tui.go`: `confirmSaveBuffer` (pre-check all targets; write selected grids via `saveGrids` subset + Sensor CSV by replaying `buf.frames()` through `WriteRow`; record `written`; clear `dirtyGrids`); `s` opens the Save Buffer picker.
- [x] **C8** `cmd/goaldl/tui.go`: `confirmLog` (pre-check; RAW→`recSink.Set` + CSV→`newFrameCSV`; `logging=true`); `r` = Log toggle (stop if open, else picker); RAW item hidden when `recSink==nil`; **remove `d`** from key switch + legend.
- [x] **C9** `cmd/goaldl/tui.go`: footer `buf N%` + `LOG`/`REC` chrome (extend `sessionChrome`); `keyLegend` → `s save · c clear · r log · …` (drop `d csv`).
- [x] **C10** `cmd/goaldl/tui_test.go`: `TestSaveBufferCSV` (retroactive, byte-identical to live `frameCSV`, no live CSV open); `TestSaveBufferGridSubset` (F18 single grid + collision keeps partial-safe); `TestLogForward` (RAW+CSV stream, `r` stops, `written` recorded, RAW hidden on replay).

## Slice 2 — C.1–C.4 session safety

- [ ] **C11** `cmd/goaldl/tui.go`: C.1 dirty tracking — `dirtyGrids` set in `accumulate`, cleared on grid-inclusive Save Buffer; `logging` derived (recFile/csvLog open). Ring is NOT dirty (documented).
- [ ] **C12** `cmd/goaldl/tui.go`: C.2 quit guard — `quitArmed`; first `q` while `logging||dirtyGrids` arms + notice (no quit); second `q` quits; other key disarms; `ctrl+c` still immediate.
- [ ] **C13** `cmd/goaldl/tui.go`: C.3 clear undo — `undoGrid`/`undoView`/`undoMins`/`undoMaxs`; `clear()` stashes; `u` restores + re-dirties; one slot; notice `… (u to undo)`.
- [ ] **C14** `cmd/goaldl/tui.go`: C.4 exit summary — `written []outputRecord`; pure `sessionSummary()`; `cmdTUI` prints after teardown (≥1 frame); notice-lifecycle doc comments on `setNotice`/`warn`.
- [ ] **C15** `cmd/goaldl/tui_test.go`: `TestQuitGuard`, `TestClearUndo`, `TestExitSummary`, `TestNoticeClasses`.

## Verify
- [x] **C16** `go test -race -count=1 ./...` green; `go vet` + `gofmt -l pkg cmd` clean; decoder goldens byte-identical (no `-update`); `blm` still 469 over `drive_4800.raw`; forbidden-seam diff (`pkg/stream/session.go`, `pkg/stream/stream.go`, `pkg/ecm`, `pkg/decoder`, `pkg/blm`, `go.mod`, `go.sum`) **empty**.
