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
