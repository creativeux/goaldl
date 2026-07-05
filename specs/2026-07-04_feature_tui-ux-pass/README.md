<!-- SDA: v1.0 -->
# Trace: TUI UX Pass (Heuristic Analysis Remediation)

**Workflow**: plan-feature
**Started**: 2026-07-04
**Feature**: tui-ux-pass — full Nielsen-heuristic evaluation of the dashboard TUI, and a five-phase plan (A Trust · B Layout · C Safety · D Replay/Startup · E Learnability) to fix the trust, layout, and data-safety defects it surfaced.

## Active Personas
- Product Manager — severity ranking, phase cut lines, at-the-car user story
- Architect — keeping fixes on the established seams (providers below the Session facade; pure builders; consumer-side accumulation)
- QA — oracles for failure-path findings (crafted captures for staleness/knock/seek; ground-truth log as the free-running-counter oracle)

*(Selected per winaldl-parity precedent — full installed set; user delegated setup.)*

## Active Capabilities
- Go toolchain — build/vet/test -race, replay-driven TUI tests against `pkg/decoder/testdata/drive_4800.raw`
- Scratchpad view-render harness — all 8 tab bodies rendered headless from the drive fixture (grounded the heuristic findings in actual screens; F2/F4 discovered this way)
- Ground-truth cross-check — `data/20250601_111156_LOG.txt` (confirmed KNOCK_CNT free-runs on the real vehicle too)
- Subagents — available for context-isolated evaluation in later verify workflows

## Log
- 2026-07-04: Session start. User requested a thorough UX heuristic analysis of the TUI + improvement plan; analysis delivered in-conversation, then formalized here via plan-feature. Feature name `tui-ux-pass` (proposed in-conversation, user approved). Personas PM + Architect + QA (full installed set, per precedent).
- 2026-07-04: Analysis method: code walkthrough (`tui.go`, `pkg/stream` builders, `replay.go`, `session.go`, `main.go`) + headless render of every tab from the 635-frame drive fixture + ground-truth log cross-checks.
- 2026-07-04: **Key discovery (F2)**: `KNOCK_CNT` free-runs at ~+76/frame while driving in BOTH the drive capture and the WinALDL ground-truth log — the spark grid's large values on this vehicle are counter artifacts, not knock. Consumer-side detection planned (A.3); raw data stays displayed per raw-data-raw. Also logged to PROJECT_STATUS Known Issues.
- 2026-07-04: **Key discovery (F1)**: `cmdTUI` discards `session.Run`'s error — a failed port open renders as `waiting for frames… (stream ended)` with no diagnostic. Highest-severity finding.
- 2026-07-04: Wrote [requirements.md](requirements.md) — 19 findings (F1–F19) ranked S1–S4 with heuristic tags and evidence, 11 success criteria, explicit non-goals (no decode-path filtering; explainers stay; serve/Dash/config/multi-ECM out of scope).
- 2026-07-04: Wrote [plan.md](plan.md) — five phases: **A Trust** (error surfacing, staleness tick, free-running-knock detection, stale help text), **B Layout resilience** (pinned chrome + clamped body, width awareness, collapse empty grid rows, explainer toggle), **C Session safety** (dirty tracking, quit/clear guards, exit summary), **D Replay & startup** (position/seek, port discovery, waiting-screen byte diagnostics), **E Learnability** (`?` overlay, context footer, rec→learn rename, codes latch, PROM-gated extrema, prompt polish). Recommended cut: A→B→C now; D/E can ride with the serve-adapter work.
- 2026-07-04: Architecture decision logged: `Session`/`Snapshot` stay untouched; only provider-level additions (seek D.1, byte counter D.3) below the facade — the same seam as parity Phase 3's RecordSink/pause. Everything else is tui.go model/View + pure builders.
- 2026-07-04: Six open decisions deferred to spec-feature (clear guard style, PROM-gated extrema, port picker depth, scroll vs truncate, column-drop order, knock-warning threshold) — see plan.md §Open decisions.

## Phase A (Trust) — spec-feature (2026-07-04)
- 2026-07-04: Resumed to spec **Phase A** (findings F1 silent errors, F3 staleness, F2 free-running knock, F15 stale help). Switched default model to Opus 4.8 for the spec/implement passes.
- 2026-07-04: **Three UX decisions taken with the user** (AskUserQuestion): (A.3 knock) **warn + keep grid normal** — a status-line warning + explainer head swap, grid values at full brightness (raw-data-raw: annotate, never dim/hide); (A.1 fatal error) **full-screen error panel** replacing the tabs (error text + live-only serial hints: `goaldl ports`, `-b 2400`, `-invert`) + stderr reprint on exit; (A.2 staleness) **~6 s / 5 missed frames** before flagging stale (user chose the more tolerant option over the 3.6 s default — fewer false alarms on a laggy USB/SSH link).
- 2026-07-04: Wrote [spec-phaseA.md](spec-phaseA.md). Key findings confirmed against the code: **the F1 error already reaches `session.Run`'s return** (`SerialProvider.Run`→`Session.Run` pass it straight through; `serial.go` returns real open/read errors, and `n==0` read-timeouts are *not* errors) — it is simply discarded in `cmdTUI`'s goroutine. So A.1 is pure delivery+classification: buffered `errCh`, `providerDoneMsg{err}`, and a three-way split (`nil`=normal end · `context.Canceled`=user quit, ignore · else=fatal panel). This cleanly separates **transport dead** (panel, A.1) from **transport alive but silent** (staleness, A.2). All four items are **consumer/presentation-only**: no `Session`/`Snapshot`/`ecm`/`decoder`/`blm` change, no new dependency; the sole `pkg/stream` change is the `SparkBody(..., freeRunning bool)` signature + a split `sparkExplainer`. Staleness (`stale()`) and free-run (`knockFreeRunning()`) are **pure predicates over model fields** so the tests are wall-clock-free and deterministic. Free-run detection reuses the delta the model already computes; window 40 / min 20 / ≥50 % nonzero cleanly separates this vehicle's ~100 %-nonzero counter from genuine sparse ESC knock.

## Persona Review (spec-phaseA.md)
- **Product Manager**: Directly serves the at-the-car user story — the two S1 findings are exactly "the tool silently lies/hides failure." Scope is tight (4 findings, no creep into B/C); the non-goals section explicitly fences off layout, guards, and the D.3 byte-counter (a common confusion, since A.2 and D.3 both touch the "no frames" state — the spec draws the line correctly: A.2 = *had* a frame then went quiet, D.3 = *never* got one). User decisions are recorded and reflected. Success criteria 1–3, 10 map to A.1–A.4 and are testable. **Approve.**
- **Architect**: Session facade untouched a fourth phase running; the only cross-package change is one pure-builder signature — consistent with the BLMBody/statusviews idiom (inline ANSI, no lipgloss in `pkg/stream`). No new dependency (tick via `tea.Tick`, error via a buffered channel). The `errCh`+`close(snaps)` ordering is correct (buffered send always precedes close; single reader). One noted pre-existing debt, unchanged: the model keeps growing fields — acceptable for the interactive face, and Phase A's additions are cohesive (error, time, knock-window). Free-run detection placed in the consumer (it needs frame history the stateless builder lacks) with the verdict passed down as a bool — right seam. **Approve.**
- **QA**: Every item has a named oracle and an unhappy path. A.1: three-way classification each tested incl. the `context.Canceled`-is-not-an-error case (the subtle one). A.2: pure predicate tested at both boundaries + replay-exempt + done-exempt + the recovery path; heartbeat glyph change asserted (not just color). A.3: the ground-truth fixture as the true-oracle and a crafted sparse-knock sequence as the false-oracle (success criterion 3 is a real assertion, not vacuous), plus the clear-keeps-window invariant. Regression row pins goldens byte-identical + `blm` 469 + the forbidden-seam diff. Two additions folded in as requirements: (a) `TestSparkBodyFreeRunning` must assert the `freeRunning=false` output is **byte-identical to the current** `SparkBody` output (guards the normal path against accidental drift); (b) the stale-recovery case (a new frame clears the markers) must be asserted, not just described. Both already present in §Test plan / A.2 edge cases. **Approve.**

Synthesis: all three approve → proceed to standards gate.

## Standards Gate Report (Phase A pre-implementation)
| Standard | Scope | Severity | Verdict |
|---|---|---|---|
| decoder/byte-value-decoding | decoder | must | ✅ PASSES (decode path untouched; no timing logic added) |
| decoder/raw-data-policy | decoder | must | ✅ PASSES (F2/F3 are consumer-side **annotation**, not filtering — knock grid values render unchanged at full brightness; staleness flags age without dropping or altering any frame; PROM/ParseOK never gate frames out) |
| testing/golden-fixtures | testing | should | ✅ PASSES (goldens byte-identical, no `-update`; new tests rooted in `drive_4800.raw` + the ground-truth log as the knock oracle + crafted sparse-knock counter-oracle) |
| architecture/session-api-layering | architecture | must | ✅ PASSES (Snapshot/Session unchanged; consumers read the existing stream; frame-layout knowledge stays in `pkg/ecm`; `pkg/blm` untouched; the one `pkg/stream` change is a presentation builder, which the standard explicitly classes as on-top-of-core) |
| go/tooling | go | should | ✅ PASSES (zero new dependencies; gofmt/vet/test -race gate unchanged) |
| philosophy: consolidate-over-accrete | core | — | ✅ (error surfacing reuses the value `Run` already returns rather than adding a parallel error path; `sparkExplainer` split shares a common tail; no forked code) |
| philosophy: ground-truth-first | core | — | ✅ (the free-running-counter claim is anchored to BOTH the drive capture and the WinALDL log; the detector's threshold is justified by the measured ~100 %-vs-sparse separation; staleness cadence derived from the real ~1.2 s frame period) |

Gate decision: **PROCEED to implement-feature.**

## Phase A — implement-feature (2026-07-04)
- 2026-07-04: Resumed for implementation (Opus 4.8). Capabilities unchanged: Go toolchain + replay-driven TUI tests against `drive_4800.raw`; no browser/DB/docs needed. Wrote the 10-step breakdown in [tasks.md](tasks.md) and proceeded (mechanical decomposition of the approved spec — no new decisions).
- 2026-07-04: **Phase A implemented** — all 10 tasks + verify complete. Confirmed forbidden seam (`session.go`/`stream.go`/`pkg/ecm`/`pkg/decoder`/`pkg/blm`/`go.mod`/`go.sum`) **diff empty**; the sole `pkg/stream` change is `gridviews.go`'s `SparkBody` signature + the explainer split.

### Files changed (Phase A)
- `pkg/stream/gridviews.go` — `SparkBody(..., freeRunning bool)`; `sparkExplainer` split into `sparkExplainerNormal` / `sparkExplainerFreeRun` sharing `sparkExplainerTail`; free-run status warning (`⚠ free-running counter — not knock`, inline `ansiBold`) and explainer-head swap; grid values unchanged (full brightness — raw-data policy).
- `cmd/goaldl/tui.go` — **A.1**: buffered `errCh`, `providerDoneMsg{err}`, `fatalErr` field, three-way classify in `Update` (nil / `context.Canceled` / fatal), `errorPanel()` (live-only serial hints) short-circuited at the top of `View`, `cmdTUI` wires `errCh` and reprints `fatalErr` to stderr + exit 1. **A.2**: `tickMsg`/`tick()`, `Init` batches the tick, `lastFrameAt`/`now` fields, pure `stale()` (replay/pre-frame/done exempt, ~6s), hollow `○` heartbeat + `no data Ns` footer. **A.3**: `knockWindow` ring + `pushKnock`/`knockFreeRunning()` (40/20/≥50%), fed in `accumulate` beside the existing delta, passed to `SparkBody`; `c`-clear leaves the window+baseline intact.
- `cmd/goaldl/main.go` — **A.4**: `printUsage` dashboard description + key line accurate to the 8-tab / session-key dashboard.
- Tests: `pkg/stream/gridviews_test.go` (`TestSparkBodyFreeRunning` — warning present/absent, explainer head swap, values still shown, `freeRunning=false` keeps the usual explainer); `cmd/goaldl/tui_test.go` (`TestTUIFatalError` — panel + live/replay hints + Canceled-not-error + clean-end; `TestTUIStale` — boundaries + replay/done exempt + glyph + footer + recovery; `TestTUIKnockFreeRunning` — drive-fixture true + Spark warning, crafted-sparse false, clear-keeps-window; new `knockSnapshot` helper).

### Verify (implement-feature)
- `go test -race -count=1 ./...` all green; `go vet ./...` + `gofmt -l pkg cmd` clean.
- Decoder goldens byte-identical (`TestGolden`, no `-update`) — decode path untouched.
- Non-regression: `blm` command still records **469** over `drive_4800.raw` (27/37 cells trusted).
- Forbidden-seam diff **empty** (`git diff --stat` over session/stream/ecm/decoder/blm/go.mod/go.sum).
- Visual checks: `goaldl help` shows the corrected 8-tab key line; the free-running `SparkBody` renders the warning + swapped explainer with the grid artifact values (76, 6232) at full brightness, active-cell highlight intact, all explainer lines ≤75 chars.

### Pattern observations (pattern-observer)
- Logged one **UX philosophy** (pending, High confidence) in `observations/observed-philosophies.md`: *the raw-data-raw policy has a view-layer twin — annotate known-suspect data (stale / artifact / bad sample) loudly rather than hiding, dimming into illegibility, or filtering it.* Detected as a 3+ recurrence (Phase 1 bad-sample gating → Phase A knock-artifact warning → Phase A staleness) reinforced by the user's "warn + keep grid normal" choice. Candidate to promote as a corollary of `standards/decoder/raw-data-policy.md` at `recombobulate`. No new enforceable standard; the work followed existing idioms (Session facade untouched, pure content builders with inline ANSI, consumer-side accumulation).
- 2026-07-04: **Handoff** — Phase A implemented and self-verified; awaiting `verify-feature` (fresh-context evaluator) before closing. Working tree uncommitted.

## Phase B (Layout Resilience), slice 1 — spec + implement (2026-07-04)
- 2026-07-04: **Direct user feedback** (not a workflow command): "In a short terminal, the tabs are hidden on top and there's no way to scroll. Also, the helper text for each table is just taking up unnecessary vertical space — better to hide those in an accordion under an 'i' info icon." This is plan.md Phase B (B.1 pinned/scroll + B.4 explainer accordion). User made the key UX call (explainers **collapsed by default** under `i`), so I specified + implemented in one pass rather than pausing for separate approval. Wrote [spec-phaseB.md](spec-phaseB.md).
- 2026-07-04: **Ground-truth correction**: the heuristic analysis's F4 read as "footer clips off the bottom." The user's empirical report shows the opposite — Bubble Tea's standard alt-screen renderer scrolls the **top** (tab bar) off an over-tall frame. Fixed the assumption; the fix pins chrome and clamps the body to height.
- 2026-07-04: **Implemented** (B.1 + B.4). `View()` now guarantees frame height ≤ terminal height (fixed 5-line chrome + `clampBody` scroll window with an `↑/↓ j/k · lines A–B of N` status); `i` toggles the explainer accordion (grid tabs render a compact one-line legend by default, full explainer on demand); `j`/`k`/`↑`/`↓` scroll; scroll re-homes on tab switch and accordion toggle. Confirmed with a throwaway visual harness at 90×16: every tab renders exactly 16 lines, tab bar always visible, BLM body 24→19 lines collapsed, free-running Spark warning shows even when collapsed, `i info` in the footer only on grid tabs.

### Files changed (Phase B slice 1)
- `pkg/stream/gridviews.go` — compact legend consts (`intLegend`/`o2Legend`/`sparkLegend`/`sparkLegendFreeRun`); `showInfo bool` added to `INTBody`/`O2Body`/`SparkBody`; 4-way spark legend (free-run × showInfo). Spark status-line warning stays independent of `showInfo` (Phase A trust not gated behind the accordion).
- `cmd/goaldl/tui.go` — `showInfo`/`scroll` fields; `i`/`j`/`k`/`↑`/`↓` keys + `prevActive` scroll-reset on tab change; `View` restructured around new `activeBody()`/`clampBody()`; `chromeLines`/`bodyBudget()`/`maxScroll()`/`clampScroll()`/`isGridTab()`/`keyLegend()` helpers; BLM accordion via existing `BLMBody`/`BLMBodyExplained` (no BLM signature change → `monitor -blm` untouched).
- Tests: `pkg/stream/gridviews_test.go` (call sites + `TestGridLegendAccordion`); `cmd/goaldl/tui_test.go` (`TestTUIViewPerTab` reworked for the accordion, `TestTUIInfoAccordion`, `TestTUIBodyScroll` — asserts frame ≤ height, tab bar present, scroll clamp).

### Standards Gate (Phase B slice 1)
| Standard | Verdict |
|---|---|
| decoder/byte-value-decoding · raw-data-policy | ✅ decode path untouched; layout is pure presentation (no frames dropped/altered) |
| architecture/session-api-layering | ✅ Snapshot/Session unchanged; the one `pkg/stream` change is presentation builders (explicitly on-top-of-core per the standard) |
| testing/golden-fixtures | ✅ goldens byte-identical (no `-update`); new tests rooted in the drive fixture + crafted sizes |
| go/tooling | ✅ zero new deps (scrolling/accordion hand-rolled — no `bubbles/viewport`); gofmt/vet/race clean |
| philosophy: consolidate-over-accrete | ✅ `activeBody`/`clampBody` extract rather than duplicate; BLM reuses the existing compact/explained pair; `gridHeat` legend param reused |
| philosophy: ground-truth-first | ✅ the layout assumption was re-derived from the user's real observation (top scrolls off, not bottom) |

Gate: **PROCEED.** No new decisions blocked.

### Verify
- `go test -race -count=1 ./...` green; `go vet` + `gofmt -l pkg cmd` clean; decoder goldens byte-identical; `blm` still 469; forbidden-seam diff empty.

### Deferred (next Phase B steps)
- **B.3 collapse trailing empty grid rows** (RPM 4000–6400 never populate here — 7 dead rows): would let a grid tab fit a short terminal *without* scrolling. Strong complement to this slice; recommended next.
- **B.2 width awareness**: spark grid (83 cols) / sensor table (~100) still wrap below their widths.

- 2026-07-04: **Handoff (Phase B slice 1)** — implemented + self-verified alongside Phase A; both await `verify-feature`. Working tree uncommitted.

### B.3 collapse empty grid rows (added same session, direct request)
- 2026-07-04: User: "now do B.3, collapse the empty rows." Implemented in `gridHeat` (new `collapse bool` + `rowEmpty` + trailing-row summary `(RPM x–y empty)`), with an **active-row guard** (never hide the engine's current cell) and a **dashboard-only** application: the streaming `monitor -blm` (`BLMView.Render`) passes `collapse=false` to keep a stable redraw height (it moves the cursor up a fixed line count; a shrinking grid would leave stale rows), so the monitor path stays byte-for-byte untouched. Saved files are unaffected by construction (they render via `blm.Grid.Render*`, not `gridHeat`). Threaded `collapse` through `BLMBody`/`blmBody` (BLM is the one builder shared with monitor); `INTBody`/`O2Body`/`SparkBody`/`BLMBodyExplained` and the TUI's collapsed-BLM pass `true`.
- 2026-07-04: **Effect confirmed** at 90×20: the BLM grid drops from 16 RPM rows to the 8 populated ones + `(RPM 3600–6400 empty)`; the whole frame is 17 lines — a grid tab now fits a short terminal *without* scrolling (scroll remains the safety net for Raw / expanded explainers). New `TestGridCollapse` (trailing collapse, active-row guard, monitor-draws-all, saved-file-unaffected); `TestSparkBody` dim-check scoped past the collapse summary. Standards gate unchanged (still presentation-only, no new deps, decode path/goldens/`blm` 469 intact, forbidden seam empty). B.2 (width) is the only remaining Phase B item.
- 2026-07-04: **B.3 REVERTED (user decision)**: "I don't like the collapsing table. We should show the full table all the time." Backed out all row-collapse: `gridHeat` restored to draw every RPM row (no `collapse` param, no `rowEmpty`, no summary line); `BLMBody`/`blmBody` back to their pre-B.3 signatures; `INTBody`/`O2Body`/`SparkBody`/`BLMBodyExplained` and the TUI/monitor call sites reverted; `TestGridCollapse` removed and `TestSparkBody`'s dim-check restored. **B.1 (pinned chrome + scroll) and B.4 (info accordion) are retained** — the full 16-row grid scrolls with `j`/`k` on a short terminal, tab bar still pinned. Full `-race` suite green; goldens byte-identical; `blm` 469; forbidden seam empty. Net Phase B: B.1 + B.4 shipped; B.2 (width) + full-table-density (B.3) intentionally not pursued.

## Verify-feature (2026-07-04) — Phases A + B
- 2026-07-04: Assembled [evaluation-brief.md](evaluation-brief.md) (Sections A–F) scoped to what shipped: Phase A (F1/F2/F3/F15) + Phase B B.1/B.4, with the B.3-revert as an explicit regression criterion; B.2/C–E declared out of scope. Spawned a fresh evaluator (general-purpose agent, clean context, filesystem-only handoff).
- 2026-07-04: **Evaluator verdict: PASS** ([evaluation.md](evaluation.md)). All 7 in-scope acceptance criteria (A.1–A.4, B.1, B.4, B.3-revert) ✅ with concrete test/code evidence; all 5 standards + both core philosophies upheld. Independently confirmed: forbidden-seam diff empty; goldens byte-identical (no `-update`); `blm` 469; `-race` green; no new deps; raw-data policy honoured (F2 Spark values render full-brightness, F3 annotates chrome only — no frame dropped); the `errCh`/`close(snaps)` handoff proven deadlock-free (buffered(1), sent before close). **Two out-of-scope notes, both already documented**: (1) the ≤-height clamp counts newline-lines not post-wrap visual rows (that's B.2/width, deferred); (2) degenerate terminals < 6 rows can exceed the fixed 5-line chrome (accepted in spec-phaseB.md §B.1 edge cases). No blocking or warning issues.

### Spec retrospection
- **Spec alignment**: implementation matches spec-phaseA.md and spec-phaseB.md; the evaluator's cited line numbers correspond to the specified design (errCh/providerDoneMsg classification, pure `stale()`, knock ring reuse, `clampBody`/`chromeLines`, accordion). No undocumented divergence. Both evaluator notes were already captured (note 1 in spec-phaseB §Deferred B.2; note 2 in spec-phaseB §B.1 edge cases) — no spec update needed.
- **Standards audit**: `architecture/session-api-layering.md` cites `BLMBody`/`SensorTable`/`Renderer`/`BLMView` as presentation examples — `BLMBody`'s signature was reverted to its original, so that reference is still accurate. `SparkBody`/`INTBody`/`O2Body` gained params but are not cited in any standard. No stale code examples anywhere in `product-knowledge/standards/`.

### Test synchronization
- **Stale references**: none — grep for `TestGridCollapse`/`rowEmpty`/collapse-param call sites is empty; the `decoder` import in `gridviews_test.go` is still used by `gridFrame`.
- **Fakes/doubles**: none new — feature tests drive the real `ReplayProvider` + `Session` + real `ecm` definition over the committed `drive_4800.raw`; staleness/knock use pure predicates over crafted model state (no wall clock).
- **New API coverage**: `SparkBody(…, freeRunning, showInfo)` (TestSparkBody/TestSparkBodyFreeRunning/TestGridLegendAccordion), `INTBody`/`O2Body(…, showInfo)` (TestINTBodyGating/TestGridExplainers/TestGridLegendAccordion); model methods `stale`/`knockFreeRunning`/`errorPanel`/`clampBody`/`maxScroll`/`clampScroll`/`keyLegend`/`activeBody` via TestTUIFatalError/TestTUIStale/TestTUIKnockFreeRunning/TestTUIInfoAccordion/TestTUIBodyScroll/TestTUIViewPerTab.
- **Sibling comparison**: the new grid-builder tests match the coverage shape of their siblings (`TestINTBodyGating`/`TestO2BodyPrecision` — status/highlight/legend); the layout tests are new (no sibling) and are covered directly. No gap.
- **Regression**: `go test -race -count=1 ./...` green; `go vet` + `gofmt -l pkg cmd` clean; goldens byte-identical; `blm` 469; forbidden seam empty.

### Completion (2026-07-04)
- **Phases A (Trust) + B (Layout: B.1+B.4) verified and closed.** Observability updated: PROJECT_STATUS marks both shipped+verified; ROADMAP updated. Remaining tui-ux-pass backlog: Phase B B.2 (width awareness), Phases C (session safety), D (replay/startup), E (learnability) — all still planned in plan.md.
- 2026-07-04: Committed as `f6db587` on `feat/tui-ux-pass-trust-and-layout`; **PR #5** opened: https://github.com/creativeux/goaldl/pull/5

## Phase B — B.2 width awareness (2026-07-04)
- 2026-07-04: User: "Continue with B.2." Design decision taken (AskUserQuestion): **truncate-with-a-›-cue** over horizontal scroll; the sensor table drops **ALT first, then RAW**. Rationale for truncating grids at a *whole-column boundary* (not mid-cell): a truncated number would misread as a smaller value — unacceptable in a tuning tool (data-correctness / ground-truth-first).
- 2026-07-04: **Implemented.** Three layers: (1) `gridHeat` gains `width` — caps MAP columns at a column boundary + ` ›` cue (threaded through all grid builders; monitor passes 0); (2) `SensorTableExtrema`/`renderTableExtrema` gain `width` and drop ALT→RAW (rewritten column-driven); (3) `tuiModel.fitWidth` truncates every frame line to `m.width` (ANSI-aware via `x/ansi.Truncate`) — the load-bearing guarantee that no line soft-wraps, so a wrapped chrome line can't push the tab bar off the top (closes the A+B evaluator's note #1). `x/ansi` promoted indirect→direct (v0.10.1, already in the build graph via lipgloss/bubbletea; `go.sum` unchanged — no new code). Confirmed at 64 cols: Spark shows 10/15 columns + `›`, sensor table drops ALT+RAW, chrome truncates without wrap, tab bar pinned.
- 2026-07-04: **Verify (self):** `go test -race ./...` green; `go vet` + `gofmt` clean; decoder goldens byte-identical; `blm` 469; layering seam (`session.go`/`stream.go`/`ecm`/`decoder`/`blm`) diff empty. New tests: `TestGridWidthTruncation`, `TestSensorTableColumnDrop`, `TestTUIWidthFit`. Phase B is now complete (B.1+B.2+B.4; B.3 reverted). Committed `9446cc6`; PR #5 updated.

### Verify-feature (B.2, 2026-07-04)
- 2026-07-04: Assembled a B.2-scoped [evaluation-brief-b2.md](evaluation-brief-b2.md) (scope: B.2 only; A/B.1/B.4 already verified). Spawned a fresh evaluator (clean context, filesystem-only handoff).
- 2026-07-04: **Evaluator verdict: PASS — no blockers** ([evaluation-b2.md](evaluation-b2.md)). All 4 B.2 criteria + the cross-cutting invariant hold. Independently drove the model over 400 real frames × 8 tabs × 15 widths measuring with `ansi.StringWidth`: **no fitted line ever exceeds `m.width`** (incl. active-cell highlight rows), `gridHeat` **never emits a partial number** in its own output, ANSI escapes survive `fitWidth` intact, sensor drop order ALT→RAW with SENSOR/VALUE/MIN/MAX preserved. Confirmed: goldens pass without `-update`; `blm` 469; layering-seam diff empty; `go.sum` unchanged; `go.mod` only moves `x/ansi` indirect→direct (acceptable — no new code); `BLMView.Render` passes `width=0` (monitor unaffected).
- 2026-07-04: **One non-blocking warning fixed same session.** Below width 16 (narrower than the `SENSOR` header — well outside F4's 40–97 target band, analogous to the deferred sub-6-row case), `gridHeat`'s `fit = (width-11)/5 < 1` so it declined to truncate and the (non-column-aware) `fitWidth` catch-all cut a data cell mid-digit (`117`→`11`) — the partial-number misread the raw-data policy forbids. **Fix:** `gridHeat` now sets `cols=0, truncated=true` when `fit<1`, emitting the RPM label + › only — so gridHeat's lines are always ≤ width and `fitWidth` never has to cut a data cell. New assertion in `TestGridWidthTruncation` drives width 13 and asserts every grid line's display width ≤ 13 (closing the evaluator's "no test below width 16" note). `-race` suite green; goldens byte-identical; `blm` 469.

### Spec retrospection (B.2)
- **Spec alignment**: implementation matches spec-phaseB.md §B.2; the sub-16 `cols=0` guard is a strengthening of the column-boundary rule ("gridHeat self-limits so the caller's catch-all never cuts a cell") — folded into the §B.2 grid-layer description. No other divergence.
- **Standards audit**: `architecture/session-api-layering.md` still cites `SensorTable`/`BLMBody`/`Renderer`/`BLMView` — all present; `BLMBody`/`SensorTableExtrema` gained a `width` param but the standard names them generically as presentation, still accurate. No stale examples.

### Test synchronization (B.2)
- **Stale references**: none — B.2 call-site updates are the `, width` additions; no deleted/renamed symbols. `x/ansi` imported in both test files that measure display width.
- **Fakes/doubles**: none — width tests call the real builders at chosen widths and the real model over `drive_4800.raw`.
- **New API coverage**: `gridHeat` width path (`TestGridWidthTruncation`, incl. width 0 / narrow / sub-16); `SensorTableExtrema` width (`TestSensorTableColumnDrop`); `fitWidth` (`TestTUIWidthFit`). `BLMBody`/`INTBody`/`O2Body`/`SparkBody`/`BLMBodyExplained` width params exercised transitively (0 in existing tests, non-zero in the new ones and the end-to-end).
- **Sibling comparison**: width tests mirror the existing builder-test shape (crafted grid + substring/■width assertions); no gap.
- **Regression**: `go test -race -count=1 ./...` green; `go vet` + `gofmt -l pkg cmd` clean.

### Completion (B.2, 2026-07-04)
- **Phase B fully verified and closed** (B.1+B.4 prior pass, B.2 this pass; B.3 reverted). Only the sub-6-row degenerate *height* case remains deferred. Observability updated (PROJECT_STATUS, ROADMAP). Fix committed to the PR #5 branch.
