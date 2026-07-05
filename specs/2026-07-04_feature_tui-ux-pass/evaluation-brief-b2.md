<!-- SDA: v1.0 -->
# Evaluation Brief: TUI UX Pass — Phase B.2 (Width Awareness)

Self-contained brief for a fresh QA evaluator. You have no implementation context; verify what was built against what was agreed. Be skeptical — your job is to find problems.

**Scope**: ONLY the B.2 width-awareness slice. Phase A and B.1/B.4 were already verified in a prior pass (`evaluation.md`, PASS) — do not re-litigate them except where B.2 could have regressed them. B.3 (row collapse) was reverted earlier; not in scope. Phases C/D/E and the sub-6-row degenerate-height case are out of scope (documented as deferred).

The project is a Go TUI ALDL scanner at `/Users/aaronstone/Development/aldl/goaldl`. Work from there.

---

## Section A: What Was Requested

Finding **F4 (width dimension)** from [requirements.md](requirements.md): wide views wrap and garble on a narrow terminal (the Spark grid is ~84 cols, the 6-column sensor table ~97, the footer key legend ~90). Below those widths the terminal soft-wraps, which *also* breaks B.1's height clamp — a wrapped chrome line pushes the pinned tab bar off the top (this was the A+B evaluator's out-of-scope note #1, now addressed).

**User decision (recorded in [README.md](README.md), "Phase B — B.2")**: truncate-with-a-`›`-cue (NOT horizontal scroll); the sensor table drops **ALT first, then RAW**. Grids must truncate at a **whole-column boundary**, never mid-number (a truncated "6232"→"62" would misread as a smaller value — unacceptable in a tuning tool).

## Section B: What Was Agreed To (acceptance criteria)

From [spec-phaseB.md](spec-phaseB.md) §B.2:

1. **Grid column truncation.** When a grid is wider than the terminal, `gridHeat` caps the MAP columns to what fits at a whole-column boundary and appends a ` ›` cue. No partial numbers ever appear. `width<=0` renders every column with no cue. The active-cell highlight and all existing grid behaviour are preserved for the visible columns.
2. **Sensor-table column drop.** When the table exceeds width, it drops ALT first, then RAW, keeping SENSOR/VALUE/MIN/MAX. `width<=0` keeps all six columns. SENSOR and VALUE always survive.
3. **Chrome + residual truncation (`fitWidth`).** Every line of the assembled frame (tab bar, loop line, footer, and any still-over-wide body) is truncated to `m.width` with a `›` cue, ANSI-aware (styling escapes pass through and are reset at the cut, not corrupted). Result: NO line's display width exceeds `m.width`, so nothing soft-wraps and the tab bar is never pushed off the top. No-op before the first `WindowSizeMsg` (`m.width == 0`).
4. **No regression to B.1/B.4/A or the monitor.** The height clamp/scroll, the info accordion, the trust features, and `monitor -blm` (which must pass `width=0` — its in-place redraw needs a stable line count) all still work.

**Cross-cutting invariant.** Layering seam untouched: no change to `pkg/stream/session.go`, `pkg/stream/stream.go`, `pkg/ecm`, `pkg/decoder`, `pkg/blm`. Decode path untouched (goldens byte-identical). `blm` command still records 469. The ONLY dependency change is `x/ansi` promoted from indirect to direct in `go.mod` (same version `v0.10.1`, already in the build graph via lipgloss/bubbletea; `go.sum` must be unchanged — verify this) — assess whether that is acceptable under the go/tooling standard (no NEW code entering the build).

## Section C: What Changed (B.2 only — since commit f6db587)

Read these directly:
- `pkg/stream/gridviews.go` — `gridHeat` gains `width int` (column-boundary cap + `›`); `INTBody`/`O2Body`/`SparkBody` gain trailing `width int`.
- `pkg/stream/blmview.go` — `BLMBody`/`BLMBodyExplained`/`blmBody` gain `width int`; `BLMView.Render` passes `0`.
- `pkg/stream/table.go` — `SensorTableExtrema` gains `width int`; `renderTableExtrema` rewritten column-driven, drops ALT→RAW.
- `cmd/goaldl/tui.go` — `fitWidth` helper (imports `github.com/charmbracelet/x/ansi`); `m.width` threaded into all body builders in `activeBody`; `fitWidth` applied to the frame in `View` and to the error panel.
- `go.mod` — `x/ansi` indirect→direct.
- Tests: `pkg/stream/gridviews_test.go` (`TestGridWidthTruncation`, `TestSensorTableColumnDrop`, call-site updates), `cmd/goaldl/tui_test.go` (`TestTUIWidthFit`).

`git diff f6db587 HEAD --stat` shows the full list. Read files directly.

## Section D: How to Verify

- `go build ./...` · `go vet ./...` · `gofmt -l pkg cmd` (expect empty).
- `go test -race -count=1 ./...` (all green). B.2-specific: `go test ./pkg/stream/ -run 'TestGridWidthTruncation|TestSensorTableColumnDrop|TestSensorTableExtrema|TestSparkBody|TestGridExplainers|TestGridLegendAccordion'` and `go test ./cmd/goaldl/ -run 'TestTUIWidthFit|TestTUIBodyScroll|TestTUIViewPerTab'`.
- Goldens (decode path): `go test ./pkg/decoder -run TestGolden` must pass with NO `-update`.
- BLM regression: `go run ./cmd/goaldl blm pkg/decoder/testdata/drive_4800.raw` → `Recorded 469 into BLM cells`.
- Layering seam: `git diff --stat f6db587 HEAD -- pkg/stream/session.go pkg/stream/stream.go pkg/ecm pkg/decoder pkg/blm` must be EMPTY.
- Dependency: `git diff f6db587 HEAD -- go.sum` must be EMPTY; `go.mod` should show only `x/ansi` moving indirect→direct.
- **Interactive TUI** (Bubble Tea alt-screen) — not browser-drivable. Verify via the model-level tests and by reading `View`/`fitWidth`/`gridHeat`/`renderTableExtrema`. You MAY write a throwaway `zz_eval_test.go` in `cmd/goaldl` that drives the model at a narrow `tea.WindowSizeMsg` (e.g. 64×40 and 44×24) over the drive fixture and `t.Logf`s `View()` (strip ANSI or measure with `ansi.StringWidth`) — DELETE it before finishing, leave no new files.

**Scrutinise especially:**
- Does the grid ever emit a **partial number**? Drive a wide grid (Spark, 15 cols) at a width that would cut mid-cell if naive, and confirm the cut is at a column boundary with `›`, showing only whole numbers.
- After `fitWidth`, is any line's **display width** (`ansi.StringWidth`, not byte/rune length) > `m.width`? Check across tabs and widths, including the active-cell reverse-highlight line (its escapes must not corrupt or inflate the visible width).
- Does `fitWidth` corrupt ANSI (leave styling bleeding past the cut, or break an escape sequence mid-bytes)?
- Sensor table at a width that drops only ALT vs one that drops ALT+RAW — is the order correct and are SENSOR/VALUE always present?
- `monitor -blm` path: does `BLMView.Render` pass `width=0` (no truncation, stable redraw)? A non-zero width here would be a regression.

## Section E: Standards to Enforce

Read each directly under `product-knowledge/standards/`:
- `decoder/raw-data-policy.md` — truncation is a *display* choice; it must not drop/alter frames or values. The grid `›` hides columns but shows only whole, correct numbers for what's visible. Confirm no data is misrepresented (this is why mid-cell truncation was rejected).
- `architecture/session-api-layering.md` — Snapshot/Session/ecm/decoder/blm untouched; width handling is pure presentation in `pkg/stream` builders + the TUI.
- `decoder/byte-value-decoding.md` — decode path untouched (goldens).
- `testing/golden-fixtures.md` — goldens byte-identical; new tests rooted in real fixtures / crafted widths.
- `go/tooling.md` — gofmt/vet/`-race`; assess the `x/ansi` indirect→direct promotion (no new code in the graph, `go.sum` unchanged) against "no new deps."
- Philosophies (blocking): `philosophies/consolidate-over-accrete.md` (using an already-present library vs hand-rolling), `philosophies/ground-truth-first.md` (whole-number truncation preserves data correctness).

## Section F: Personas to Consult

`product-knowledge/personas/product-manager.md`, `architect.md`, `qa.md`. Include: scope discipline (B.2 only), the data-correctness rationale for column-boundary truncation, the dependency-promotion judgment, and edge cases (width 0, very narrow width forcing residual truncation, active-cell highlight in a truncated row, monitor unaffected).

## Output

Write your evaluation to `specs/2026-07-04_feature_tui-ux-pass/evaluation-b2.md` (Acceptance Criteria table, Standards Compliance table, Persona Reviews, Issues Found, Overall Verdict). Return a concise verdict + any blockers as your final message. Leave no throwaway files behind.
