<!-- SDA: v1.0 -->
# Spec: TUI UX Pass — Phase B (Layout Resilience), slice 1

**Scope**: the layout defects the user hit directly (2026-07-04) — B.1 (short terminal scrolls the tab bar off the top, no way to scroll the body), B.4 (grid explainers eat vertical space; hide them behind an info accordion), and B.2 (wide views wrap on a narrow terminal). B.3 (collapse trailing empty grid rows) was implemented then **reverted at the user's request** ("show the full table all the time") — see §B.3 REVERTED. Presentation-only for the layering seam (no `Session`/`Snapshot`/`ecm`/`decoder`/`blm` change); B.2 promotes `x/ansi` from indirect to a direct dep (already in the build graph — see §B.2).

**As-shipped**: B.1 + B.4 + B.2. The full RPM×MAP grid always renders every row; a short terminal scrolls (B.1) with the tab bar pinned; a narrow terminal truncates with a `›` cue (B.2).

**User decision (2026-07-04, direct feedback)**: "In a short terminal, the tabs are hidden on top and there's no way to scroll. Also, the helper text for each table is just taking up unnecessary vertical space. It would be better to hide those in an accordion under an 'i' info icon." → explainers **collapsed by default**, toggled by `i`; the body scrolls so the pinned tab bar never disappears.

This corrects the empirical assumption in the heuristic analysis (F4 originally read as "footer clips"): with Bubble Tea's standard alt-screen renderer, an over-tall frame scrolls the **top** off — the tab bar, not the footer. Ground-truth-first: the user's observation wins.

---

## B.1 — Pinned chrome + scrollable body

### Design
`View()` renders a frame of **exactly the terminal height**, with the tab bar/loop line pinned at the top and a two-line footer pinned at the bottom.

- **Fixed chrome = 6 lines**: tab bar, the blank line above the body, the blank line below it, and a **three-row bottom bar** (recording state · status · key legend — see below). `chromeLines` const.
- `bodyBudget() = height − chromeLines` (min 1); unbounded before the first `WindowSizeMsg` (`height == 0`) so the initial frame renders in full.
- `activeBody()` extracts the per-tab content switch (was inline in `View`).
- `clampBody(body)` fits the body to **exactly** `bodyBudget` lines: if it fits, **pad with blank lines**; if it overflows, show a scroll window of `budget − 1` lines from `m.scroll` + a reserved status line `↑/↓ j/k scroll · lines A–B of N`. Either way the body region is exactly the budget, so `chrome + body == height` and the footer is on the last row every render.
- `padHeight()` is a final safety net that pads/clamps the whole frame (and the variable-height error panel) to exactly `m.height`.
- **Scroll state** `m.scroll`: `j`/`down` increment, `k`/`up` decrement via `clampScroll` (→ `[0, maxScroll]`). Switching tabs and toggling the accordion re-home `scroll` to 0; **`WindowSizeMsg` re-clamps** it (a grown terminal can't leave the offset past the new end).

### Three-row bottom bar + resize ghosting (2026-07-04, user reports)
Three rounds of user feedback reshaped the chrome:
- **Resize ghosting + wide footer**: the one-line footer overflowed, and resizing left a **frozen duplicate footer** (an old render's footer stranded on a row the new, differently-sized frame no longer wrote). Fixed by a constant full-height frame: `clampBody`/`padHeight` make every frame exactly `m.height` lines (footer always on the last rows), and `Update` returns `tea.ClearScreen` on `WindowSizeMsg` (also re-clamps `m.scroll`) to wipe stale rows — every screen row is rewritten each render.
- **Chrome reorg** (2nd report): the loop-status line moved off the top into the bottom bar, and the footer PROM mark was replaced by the loop badge.
- **Declutter** (3rd + 4th reports): the per-grid **status line was removed** from all grid tabs (BLM/INT/O2/Spark) — the "CLOSED LOOP RPM… MAP… BLM… cell n/n" readout was overwhelming and duplicative of the bottom bar. A **blank line above the table** was added on every tab to let it breathe. The Spark **free-running warning** (Phase A trust) initially stayed as a status line, but that duplicated the legend's free-run warning; it now lives **only in the legend/explainer** (`sparkLegendFreeRun` collapsed / `sparkExplainerFreeRun` expanded — bold, always visible), so no grid has a status line and the warning appears exactly once.

Final layout:
- **Top**: tab bar (+ source), a blank line, then the body.
- **Bottom bar, 3 rows**: (1) `recDotsLine()` — per-grid recording state `rec: BLM ● INT ● O2 ● SPARK ●` + frozen/disabled suffix (moved from the top); (2) status — `frame/t/`**`loop badge`**`/heartbeat/counts` + REC/CSV/speed chrome + notice (the coloured CLOSED/OPEN LOOP badge replaces the old `PROM ✓`; PROM status still reads from the heartbeat colour and the Raw tab); (3) key legend, or the filename prompt while open.
- Each row is width-truncated independently by `fitWidth`; `loopStatusLine()` split into `styledLoopBadge()` (status) + `recDotsLine()` (bottom row).

**Grid-status removal mechanics**: `gridHeat` now skips the status line when it is empty (was: always printed). The dashboard grid builders all pass `""` (INTBody/O2Body drop their now-unused current-value params; SparkBody's `freeRunning` only selects the free-run legend/explainer variant — the warning is not duplicated as a status line). BLM splits into `BLMBody` (streaming `monitor -blm`, `showStatus=true` — it has no bottom bar, so keeps the live loop/RPM/MAP/BLM readout) and `BLMBodyDash` (dashboard, `showStatus=false`, compact legend or explainer per `showInfo`); `blmBody` gains a `showStatus` param, `BLMBodyExplained` is folded into `BLMBodyDash`. The monitor path is unchanged.

### Keys added
`i` (accordion), `j`/`down` (scroll down), `k`/`up` (scroll up). These were free — `left`/`right`/`h`/`l`/`tab` remain tab navigation; `j`/`k` follow vim down/up, consistent with `h`/`l`.

### Guarantee (test oracle)
On an 80×12 terminal driven over the drive fixture, the Raw tab (tallest body) renders with the tab bar present and the scroll status shown; `j` advances the offset, `k` at the top holds 0, hammering `j` clamps at `maxScroll`, switching tabs resets to 0 (`TestTUIBodyScroll`). `TestTUIFrameHeight` asserts the frame is **exactly** the terminal height at 12/20/30/44 rows, the last line is the key legend and the second-to-last is the status, and a resize re-clamps scroll + returns a clear command.

### Edge cases
- **height 0** (pre-size): no clamp/pad, full render — the model gets a `WindowSizeMsg` immediately on program start, so this is a single transient frame.
- **height < 7**: `bodyBudget` floors at 1 so the frame wants 7 lines; `padHeight` clamps to `m.height`, which can cut the bottom bar — accepted degenerate case (a sub-7-row terminal can't show tabs + blank + 3-row bottom bar + 1 body line).
- **narrow width**: addressed in B.2 (`fitWidth` truncates each line, so a wrapped chrome line can't throw off the height math).

## B.4 — Info accordion

### Design
The grid tabs (BLM/INT/O2/Spark) render a **compact one-line legend** by default and the full multi-line explainer only when `m.showInfo` is set.

- New compact legend constants in `gridviews.go`: `intLegend`, `o2Legend`, `sparkLegend`, `sparkLegendFreeRun` (BLM already had a compact legend in `BLMBody`, shared with `monitor -blm`).
- `INTBody`/`O2Body` gain a trailing `showInfo bool`; internally `legend := compact; if showInfo { legend = explainer }`.
- `SparkBody` gains `showInfo` alongside its `freeRunning`: a 4-way legend pick (free-run explainer / free-run compact / normal explainer / normal compact). The `⚠ free-running counter — not knock` **status-line** warning is independent of `showInfo` — it shows even when collapsed (Phase A's trust guarantee is not gated behind the accordion).
- BLM in the TUI selects `BLMBodyExplained` (showInfo) vs `BLMBody` (compact) — no BLM signature change, keeping `monitor -blm` untouched.
- `i` toggles `m.showInfo` and re-homes scroll (the body height changes).
- Footer legend gains `i info`, shown **only on grid tabs** (`keyLegend()` + `isGridTab()`) — the only tabs with an explainer to toggle.

### Test oracle
Collapsed grid tabs contain the compact legend text and **not** the explainer markers; `i` flips both (`TestTUIInfoAccordion`, `TestGridLegendAccordion`). Collapsed BLM does not show "Block Learn Multiplier"; expanded does. A collapsed free-running Spark still warns.

## B.3 — Collapse trailing empty grid rows — REVERTED 2026-07-04

> **Reverted at user request** the same session it was implemented: "I don't like the collapsing table. We should show the full table all the time." All row-collapse code was backed out (`gridHeat` draws every row again; `collapse`/`rowEmpty`/summary removed; builder signatures restored; `TestGridCollapse` removed). The design below is retained as a record of what was tried and why it was dropped. On a short terminal the full grid now scrolls (B.1) instead of collapsing.

### Design (as-built, then reverted)
`gridHeat` gains a trailing `collapse bool`. When set, it walks up from the last RPM row past rows that are entirely empty (`rowEmpty` over `Grid.Samples()`) and stops, replacing the hidden rows with one dim summary line `(RPM <first>–<last> empty)`. The populated range and any interior empty rows are kept, so the fuel map's positional integrity is preserved.

- **Active-row guard**: the walk never hides the active row (`lastRow != ar`) — the engine's current cell must stay visible even before it accumulates a sample (e.g. a brief high-RPM excursion expands the grid to show it).
- **Display-only**: saved files render through `blm.Grid.Render*` (not `gridHeat`), so every row is always written — verified by `TestGridCollapse`.
- **Dashboard-only**: the dashboard grid builders (`INTBody`/`O2Body`/`SparkBody`/`BLMBodyExplained` and the TUI's collapsed-BLM `BLMBody(…, true)`) pass `collapse=true`. The streaming `monitor -blm` passes `false` (`BLMView.Render`) because it redraws in place by moving the cursor up a fixed line count — a shrinking grid would leave stale rows. This kept the monitor redraw path untouched rather than making it height-adaptive.

### Effect
On the drive fixture the BLM grid drops from 16 RPM rows to the 8 populated ones plus a one-line summary. Combined with the B.4 accordion, a grid tab at 90×20 renders in 17 lines — it fits a short terminal *without* scrolling (scroll remains the safety net for the Raw tab and expanded explainers).

### Test oracle
`TestGridCollapse`: trailing empty rows summarized (`(RPM 2000–6400 empty)`), the populated 1600 row and interior empties kept, the empty 6400 row not drawn; the monitor path (`BLMBody(…, false)`) draws every row with no summary; an empty high-RPM active row stays visible; `Grid.RenderInt` (saved file) still contains every row.

## B.2 — Width awareness (added 2026-07-04)

**User decision**: truncate-with-cue (over horizontal scroll); the sensor table drops ALT first, then RAW.

The height work (B.1) assumed the chrome never soft-wraps; below ~90 cols the footer key legend wrapped, which broke the height clamp (the evaluator's note #1). B.2 makes the frame width-safe in three layers:

1. **Grids** (`gridHeat` gains `width int`): cap the MAP columns to what fits at a **whole-column boundary** and append a ` ›` cue — never a partial number (a truncated "6232"→"62" would misread as a smaller value in a tuning tool). `width<=0` = no limit. **gridHeat self-limits so its own lines are always ≤ width** — the caller's ANSI catch-all (§3) must never have to cut a data cell. When not even one 5-wide column fits (`width < 16`, narrower than the `SENSOR` header), it emits the RPM label + › only (`cols=0`) rather than let `fitWidth` slice a cell mid-digit. Threaded through `blmBody`/`BLMBody`/`BLMBodyExplained`/`INTBody`/`O2Body`/`SparkBody`; the streaming `monitor -blm` passes `0` (its fixed in-place redraw isn't width-clamped).
2. **Sensor table** (`SensorTableExtrema`/`renderTableExtrema` gain `width int`): drop the lowest-value columns in order — **ALT, then RAW** — while the table exceeds width, keeping SENSOR/VALUE/MIN/MAX. Rewritten column-driven so dropping is clean.
3. **Chrome + residual** (`tuiModel.fitWidth`): a final pass truncates every line of the assembled frame (tab bar, loop line, footer, and any body still over-wide) to `m.width` with a `›` cue, ANSI-aware via `github.com/charmbracelet/x/ansi` (`ansi.Truncate`). This is the load-bearing fix — it guarantees no line soft-wraps, so a wrapped chrome line can never push the tab bar off the top. No-op before the first `WindowSizeMsg`.

**Dependency note**: `x/ansi` was promoted from an indirect to a direct dependency (same version `v0.10.1`, already in the build graph via lipgloss/bubbletea — `go.sum` unchanged, zero new code). It's the canonical, correct tool for ANSI-aware width truncation; hand-rolling it would reinvent a battle-tested library (consolidate-over-accrete).

### Test oracle
`TestGridWidthTruncation` (narrow grid caps columns + shows `›`, no partial number, display width ≤ width; width 0 = all columns), `TestSensorTableColumnDrop` (ALT drops first, then RAW; SENSOR/VALUE always survive), `TestTUIWidthFit` (at width 44 every frame line's display width ≤ 44 across Sensors/Spark/Raw/BLM, tab bar still present).

### Effect
At 64 cols: the Spark grid shows 10 of 15 MAP columns + `›` (clean boundary); the sensor table drops ALT + RAW (SENSOR/VALUE/MIN shown, MAX truncated by the residual pass); the tab bar/status/footer truncate with `›` instead of wrapping. The tab bar stays pinned at every width.

## Deferred (not pursued)
- **Sub-6-row terminals** (evaluator note #2): `bodyBudget` floors at 1, so a window shorter than the 5-line chrome + 1 body line still overflows. Not a realistic "short terminal"; could drop the blank separators / loop line under extreme height pressure — low value, deferred.

## Files changed
| File | Change |
|---|---|
*(B.3-only changes were fully reverted; the table below is the as-shipped B.1+B.4+B.2 state.)*

| `pkg/stream/gridviews.go` | compact legend consts; `showInfo bool` on `INTBody`/`O2Body`/`SparkBody`; 4-way spark legend; **B.2**: `width int` on `gridHeat`/`INTBody`/`O2Body`/`SparkBody` — column-boundary truncation + `›` cue |
| `pkg/stream/blmview.go` | **B.2**: `width int` on `BLMBody`/`BLMBodyExplained`/`blmBody`; `BLMView.Render` passes `0` (monitor unaffected) |
| `pkg/stream/table.go` | **B.2**: `width int` on `SensorTableExtrema`; `renderTableExtrema` rewritten column-driven, drops ALT then RAW |
| `cmd/goaldl/tui.go` | `showInfo`/`scroll` fields; `i`/`j`/`k`/`↑`/`↓` keys + scroll-reset on tab change; `View` restructured around `activeBody`/`clampBody`; `bodyBudget`/`maxScroll`/`clampScroll`/`isGridTab`/`keyLegend` helpers; BLM accordion via `BLMBody`/`BLMBodyExplained`; **B.2**: `fitWidth` (ANSI-aware, `x/ansi`), `m.width` threaded into all body builders, applied to the frame + error panel |
| `go.mod` | **B.2**: `x/ansi` indirect → direct (v0.10.1, `go.sum` unchanged) |
| `pkg/stream/gridviews_test.go` | builder call sites updated; `TestGridLegendAccordion`; **B.2**: `TestGridWidthTruncation`, `TestSensorTableColumnDrop` |
| `cmd/goaldl/tui_test.go` | `TestTUIViewPerTab` (accordion); `TestTUIInfoAccordion`; `TestTUIBodyScroll`; **B.2**: `TestTUIWidthFit` |

## Regression
`go test -race ./...` green; goldens byte-identical (no `-update`); `blm` still 469; forbidden-seam diff (`session.go`/`stream.go`/`ecm`/`decoder`/`blm`/`go.mod`/`go.sum`) empty — the only `pkg/stream` change is the `gridviews.go` presentation builders.
