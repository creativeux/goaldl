<!-- SDA: v1.0 -->
# Evaluation: TUI UX Pass — Phases A (Trust) + B (Layout)

QA evaluation against `evaluation-brief.md`. Verdict: **PASS**.

Environment: `go build`/`go vet` clean, `gofmt -l pkg cmd` empty, `go test -race -count=1 ./...` all green (cmd/goaldl, pkg/blm, pkg/decoder, pkg/ecm, pkg/stream). Golden test passes with NO `-update`. `blm` prints `Recorded 469 into BLM cells`. Forbidden-seam diff EMPTY. No new deps (go.mod/go.sum untouched).

### Acceptance Criteria
| Criterion | Verdict | Evidence |
|-----------|---------|----------|
| **A.1** Fatal live error → full-screen panel with real error text + serial hints (live only); reprinted to stderr on exit; `context.Canceled` & clean `nil` are NOT errors | ✅ PASS | `Update` providerDoneMsg sets `fatalErr` only when `msg.err != nil && !errors.Is(msg.err, context.Canceled)` (tui.go:481). `errorPanel()` shows hints only when `m.replay == nil` (tui.go:1074). Reprint at tui.go:182-187. `TestTUIFatalError` covers live-with-hints, replay-without-hints, Canceled-not-error, nil-clean-end. |
| **A.2** Live stream quiet ~6s flagged stale: hollow `○` (glyph change) + `no data Ns` footer; replay/pre-frame/ended never stale; fresh frame clears | ✅ PASS | `staleAfter = 6s` (tui.go:262). `stale()` returns false for `replay != nil \|\| !hasFrame \|\| done` (tui.go:356-362). `heartbeat()` returns `"○"` when stale — a shape change, not just colour (tui.go:1057). Footer `no data %.0fs` at tui.go:855-857. `TestTUIStale` covers all boundaries + recovery. |
| **A.3** Free-running knock warns on Spark status; values stay full-brightness; sparse-knock does NOT warn; clearing Spark preserves detection window | ✅ PASS | Detector reuses the SAME `delta` computed in `accumulate` (`m.pushKnock(delta > 0)`, tui.go:540) that also feeds the grid — no separate path. `knockFreeRunning()` gated by `knockWindowMin=20` (tui.go:1103). `clear()` for viewSpark only replaces `sparkGrid`, leaving `knockWindow`/`knockNonzero`/baseline intact (tui.go:572-573). Raw-data policy honoured: `gridHeat` values unchanged when freeRunning; `TestSparkBodyFreeRunning` asserts the cell value (`"    2"`) still renders. Sparse test is non-vacuous: 3 nonzero of 39 deltas = 0.077 < 0.5 threshold, with count ≥ min so detection is active (`TestTUIKnockFreeRunning`). |
| **A.4** `goaldl help` reflects 8-tab/session-key dashboard | ✅ PASS | main.go printUsage now reads `sensors · fuel-trim grids · flags · codes · raw` and `keys: 1-8 select tab · tab/←→ cycle · s save · c clear · r rec · d csv · space/± replay · q quit`. Verified via `go run ./cmd/goaldl help`. |
| **B.1** Short terminal: frame ≤ terminal height (tab bar never scrolls off); overflow scrolls j/k/↑/↓ with `lines A–B of N`; tab switch re-homes scroll | ✅ PASS | `chromeLines=5`, `bodyBudget = height-5`, `clampBody` shows `budget-1` body lines + 1 status = exactly budget, so total frame = height (tui.go:919-985). `clampScroll`/`maxScroll` clamp to `[0,max]`; `k` at top stays 0. Tab change sets `m.scroll = 0` (tui.go:433-435). `TestTUIBodyScroll` at 80×12 asserts `lines <= height`, tab bar present, scroll status present, j advances, k held at top, 500× j clamped to maxScroll, tab switch re-homes. |
| **B.4** Grid tabs show compact one-line legend by default; `i` toggles full explainer; `i info` in footer only on grid tabs | ✅ PASS | `showInfo` default false → compact `intLegend`/`o2Legend`/`sparkLegend`; `i` toggles + re-homes scroll (tui.go:403-406). `keyLegend()` inserts `i info · ` only when `isGridTab(m.active)` (tui.go:909-914). `TestTUIInfoAccordion`, `TestGridLegendAccordion`, `TestGridExplainers`. |
| **B.3 revert** Every RPM row renders; no `(RPM x–y empty)` summary; both dashboard and `monitor -blm` | ✅ PASS | `gridHeat` iterates `for r, rpm := range g.RPM` with no collapse param (gridviews.go:35). No `collapse`/`rowEmpty` code — only hit is a B.4 accordion comment (gridviews.go:119). Both dashboard (`BLMBody`/`INTBody`/…) and `monitor -blm` (`blmBody`) route through the same `gridHeat`. |

### Standards Compliance
| Standard | Verdict | Notes |
|----------|---------|-------|
| raw-data-policy (annotate, never drop/hide/filter) | ✅ | F2: Spark grid values shown at full brightness when free-running; only status line + explainer text annotate (gridviews.go:184-202, comment 179-183). F3: staleness annotates chrome only, never drops a frame — every snapshot still updates raw history/counters (tui.go:444-452). Bad frames still feed raw view + badCount (unchanged gating). |
| session-api-layering | ✅ | `session.go`/`stream.go` untouched (empty forbidden-seam diff). All new logic is consumer-side (tuiModel) or presentation builders in pkg/stream on top of the existing `Snapshot` stream. No frame-layout knowledge leaked out of pkg/ecm; `FuelTrimSample`/`Parse` used as-is. |
| byte-value-decoding / decode path untouched | ✅ | pkg/decoder untouched; golden test passes WITHOUT `-update`. |
| golden-fixtures | ✅ | Goldens byte-identical; new tests rooted in the real `drive_4800.raw` (end-to-end + knock-free-running). |
| go/tooling | ✅ | gofmt clean, vet clean, `-race` green, no new deps. |
| consolidate-over-accrete / ground-truth-first | ✅ | Helpers extracted, not duplicated: `gridHeat` shared by all grid builders; `blmBody` shared by BLMBody/BLMBodyExplained; free-run detection is one ring reused by grid + warning. Free-running threshold validated against the real fixture AND the WinALDL log. |

### Persona Reviews

**Product Manager.** Scope is disciplined: only F1/F2/F3/F4/F15 landed; B.3 is fully reverted per the user's request (verified, not just claimed); B.2/C–E are correctly absent. Each criterion is observable and tested. The at-the-car value is real and additive: a dead cable now shows a diagnosis panel with the exact next commands instead of a silent hang (F1); a stalled connection reads even without colour via the hollow glyph (F3); the free-running-knock warning stops an operator from chasing phantom detonation on this vehicle while still showing the raw counts (F2). Help text no longer lies about "1-3 / tab".

**Architect.** The facade (Session/Snapshot) is untouched — the forbidden-seam diff is empty and no new dependency entered go.mod. Everything rides on the existing stream: staleness and knock detection are pure functions over model fields (testable without a wall clock), and the presentation builders (`gridHeat`, split spark explainer) live in pkg/stream where they belong. Duplication was avoided rather than accrued. The one seam I scrutinised — the `errCh`/`close(snaps)` handoff to `waitForSnapshot` — is correct and deadlock-free (errCh is buffered(1) and always sent before the close, so the reader never blocks or misses it).

**QA.** The oracles are honest. `context.Canceled` is explicitly excluded from fatal (not just "no panel appears"). Staleness boundaries test replay/pre-frame/ended exemptions AND recovery, and assert the glyph shape (`○`), not merely a colour. The sparse-knock case is provably below threshold (3/39 ≈ 0.08 vs 0.5) with the window past its minimum, so it is not a vacuous pass. Scroll clamping is hammered 500× and pinned to `maxScroll()`; `k` at the top is verified to hold. The end-to-end drive-fixture test cross-checks the spark total against an independent delta recomputation and re-asserts the `blm`-469 non-regression through the model path. Decode-path immutability is guarded by the un-updated goldens.

### Issues Found
1. **Frame-fits guarantee is in newline-lines, not post-wrap visual rows** — `cmd/goaldl/tui.go:938` (test) / `clampBody` tui.go:964. The ≤-height clamp counts `\n`-delimited lines; a long footer/header that the terminal soft-wraps at narrow widths is not accounted for. This is explicitly B.2 (width) territory, documented out of scope, so not a defect against B.1 — **note** only.
2. **Degenerate tiny heights (<6 rows)** — `bodyBudget` floors at 1, so the fixed 5-line chrome + 1 body = 6 lines can exceed a terminal shorter than 6 rows. Not a realistic "short terminal" and not covered by the criterion (which targets a normally-short window). **Note** only.

No blocking or warning issues.

### Overall Verdict
**PASS** — all in-scope acceptance criteria (A.1–A.4, B.1, B.4, B.3-revert) met with concrete test/code evidence; all standards upheld (raw-data policy annotate-not-hide confirmed, forbidden seam empty, goldens/`blm`-469 non-regressions intact); no new dependencies. The two issues found are out-of-scope notes (B.2 width), not defects.
