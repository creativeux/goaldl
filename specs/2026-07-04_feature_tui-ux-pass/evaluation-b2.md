<!-- SDA: v1.0 -->
# Evaluation: TUI UX Pass — Phase B.2 (Width Awareness)

QA evaluation of the B.2 width-awareness slice (since commit f6db587). Skeptical, adversarial verification against the brief's acceptance criteria and standards.

## Verification Run (Section D)

| Command | Result |
| --- | --- |
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `gofmt -l pkg cmd` | empty (clean) |
| `go test -race -count=1 ./...` | all green (cmd/goaldl, blm, decoder, ecm, stream) |
| B.2 stream tests | `TestGridWidthTruncation`, `TestSensorTableColumnDrop`, `TestSensorTableExtrema`, `TestSparkBody`, `TestGridExplainers`, `TestGridLegendAccordion` all PASS |
| B.2 tui tests | `TestTUIWidthFit`, `TestTUIBodyScroll`, `TestTUIViewPerTab` all PASS |
| `go test ./pkg/decoder -run TestGolden` (no `-update`) | PASS |
| `blm` 469 check | `Recorded 469 into BLM cells` ✓ |
| Layering-seam diff (session/stream/ecm/decoder/blm) | EMPTY ✓ |
| `go.sum` diff | EMPTY ✓ |
| `go.mod` diff | only `x/ansi v0.10.1` indirect→direct ✓ |

Independent adversarial check: a throwaway harness drove the model over `drive_4800.raw` (400 frames) across widths {200,97,84,63,56,51,50,44,30,24,16,15,14,12,6} on all 8 tabs, measuring every line with `ansi.StringWidth` (not byte length) and rune-splitting grid cells. Deleted after use; repo left clean.

### Acceptance Criteria

| Criterion | Verdict | Evidence |
| --- | --- | --- |
| 1. Grid column truncation (whole-column boundary + ` ›`, no partial numbers, width≤0 = all columns, highlight preserved) | ✅ PASS (with edge caveat, see Issue 1) | `gridviews.go:35-39` caps `cols` via `fit := (width-labelW-2)/cellW` only when `fit>=1 && fit<cols`; cue at `:47-49,:68-70`. My sweep: `activeBody()` grid rows emit only whole integers/decimals or `·` at every width (rune-aware check, INVARIANT 2 green). `TestGridWidthTruncation` confirms width 0 = all 15 spark columns no cue, width 50 drops rightmost + shows ›. Active-cell highlight applied inside the kept-column loop (`:63-65`) so a dropped column emits no dangling escape; escape balance 1 open/1 close verified. |
| 2. Sensor-table column drop (ALT then RAW; SENSOR/VALUE/MIN/MAX kept; width≤0 keeps six; SENSOR+VALUE always) | ✅ PASS | `table.go:214-256` — column-driven; drop order `[]int{5,1}` = ALT then RAW (`:251`); SENSOR(0)/VALUE(2) never in drop set. `TestSensorTableColumnDrop`: width 60 drops ALT keeps RAW/MIN; width 40 drops RAW too, keeps SENSOR/VALUE; width 0 keeps all six. |
| 3. Chrome + residual `fitWidth` (every line ≤ m.width, ANSI-aware, no-op at width 0) | ✅ PASS (with Issue 1 caveat) | `tui.go:883-892` uses `ansi.Truncate(ln, m.width, "›")` per line — display-aware, resets styling at the cut. `:884` no-op when `m.width<=0`. My sweep INVARIANT 1: **no** fitted line's `ansi.StringWidth` exceeded `m.width` on any tab at any tested width (incl. active-cell reverse-highlight rows). Applied in `View` (`:874`) and to the error panel (`:837`). |
| 4. No regression to B.1/B.4/A or monitor (`BLMView.Render` passes width=0) | ✅ PASS | `blmview.go:47` — `BLMBody(v.Grid, ev, v.minCount, 0)`; comment `:46` documents the fixed-line-count rationale. Height clamp (`clampBody`), scroll, goldens, `blm` 469 all intact. `TestTUIBodyScroll`/`TestTUIViewPerTab` green. |
| Cross-cutting: layering seam untouched, decode goldens identical, `blm` 469, only `x/ansi` indirect→direct, `go.sum` unchanged | ✅ PASS | Diffs empty as tabulated above. |

### Standards Compliance

| Standard | Verdict | Notes |
| --- | --- | --- |
| `decoder/raw-data-policy.md` | ✅ PASS (edge caveat) | Truncation is display-only; no frame/value dropped or altered. Visible grid cells are whole, correct numbers at every realistic width (≥16). The `›` honestly signals hidden columns. Caveat: at width 12–15 the `fitWidth` catch-all can render a partial data digit (Issue 1) — the exact `6232→62` misread the policy warns against, but only at a sub-usable terminal width. |
| `architecture/session-api-layering.md` | ✅ PASS | Width is pure presentation: threaded through `pkg/stream` builders and the TUI only. `Snapshot`/`Session`/`ecm`/`decoder`/`blm` diff EMPTY. |
| `decoder/byte-value-decoding.md` | ✅ PASS | Decode path untouched; `TestGolden` passes without `-update`. |
| `testing/golden-fixtures.md` | ✅ PASS | Goldens byte-identical; new tests rooted in the real spark grid and a crafted frame + crafted widths. |
| `go/tooling.md` | ✅ PASS | gofmt/vet/`-race` clean. `x/ansi` promotion adds no code to the build graph (already pulled by lipgloss/bubbletea; `go.sum` unchanged, same `v0.10.1`) — acceptable under "no new deps." |
| `philosophies/consolidate-over-accrete.md` | ✅ PASS | Reuses the already-present `x/ansi` `Truncate`/`StringWidth` for display-width math rather than hand-rolling an ANSI-aware truncator. |
| `philosophies/ground-truth-first.md` | ✅ PASS (edge caveat) | Whole-number column-boundary truncation preserves data correctness for all usable widths; see Issue 1 for the degenerate-width gap. |

### Persona Reviews

**Product Manager.** Scope discipline is clean: the change is confined to the B.2 width slice, no re-litigation of A/B.1/B.4, no scope creep into C/D/E. The user's recorded decision — truncate-with-`›` (not horizontal scroll), drop ALT then RAW — is implemented exactly. F4's real target (wrapping at ~84/97/90 cols) is solved: at every width in that band the grid caps at a whole-column boundary and the tab bar stays pinned. The one gap (Issue 1) is at terminal widths below 16 columns, which is narrower than a single sensor label and not a real operating condition — I would accept it as a documented deferral alongside the already-deferred sub-6-row height case, not hold the slice for it.

**Architect.** The layering seam is respected to the letter: an empty diff across the core packages, width handled purely in the `pkg/stream` builders plus the TUI's `fitWidth`. The two-tier design (semantic truncation in `gridHeat`/`renderTableExtrema`, a catch-all `fitWidth` over the assembled frame) is sound and the dependency promotion is the right call — using the ANSI library already in the graph beats a bespoke escape-aware cutter. My one architectural note: the two tiers have a seam of their own. `gridHeat` guarantees its output is ≤ width *only when at least one column fits* (`fit>=1`, i.e. width≥16); below that it silently emits a full-width line and defers to `fitWidth`, which is not column-aware and will cut mid-number. The contract "gridHeat leaves grid rows already ≤ width so fitWidth is a no-op on them" holds for width≥16 and breaks below it. A one-line guard (render label-only, `cols=0`, when `fit<1`) would make the invariant total.

**QA.** Tests are honest and rooted in real fixtures/crafted widths, and they measure display width with `ansi.StringWidth` rather than byte length — the correct discipline for ANSI content. I independently confirmed both headline invariants over 400 real frames × 8 tabs × 15 widths: no fitted line exceeds `m.width`, and `gridHeat`'s own body never emits a fragment. The gap I found is a coverage hole: the existing suite tests down to 44×24 but nothing below 16 columns, and that is exactly where the `fitWidth` fallback mis-cuts a populated first-MAP-column cell (`117`→`11` at width 13, `117`→`1` at width 12 — reproduced directly). It is a genuine partial-number leak, but only reachable at unusably narrow widths, so warning-level, not a blocker.

### Issues Found

1. **Partial data number at width 12–15 (below one grid column).** `pkg/stream/gridviews.go:35-39`. When `width < 16`, `fit = (width - 9 - 2)/5 < 1`, so `gridHeat` takes the no-truncation path and returns the **full** (~53-wide for BLM, ~84 for Spark) grid line. The TUI's `fitWidth` (`cmd/goaldl/tui.go:883-892`) then cuts that line to `m.width` with `ansi.Truncate`, landing mid-digit on any populated low-MAP-column cell. Reproduced directly: a BLM grid with the first MAP column = `117` renders `"    400   11›"` at width 13 (reads as **11**) and `"    400   1›"` at width 12 (reads as **1**). This is precisely the `6232→62` misrepresentation the raw-data policy and criterion 1 forbid — relocated from `gridHeat` into the `fitWidth` catch-all. At width ≥16 the truncation is clean and no partial ever appears (verified). **Severity: warning** — width < 16 is a degenerate, unusable terminal (narrower than the `SENSOR` header), analogous to the already-deferred sub-6-row height case, and F4's target band is 40–97. **Clean fix:** in `gridHeat`, when `width>0` and `fit<1`, set `cols=0, truncated=true` so only the label + `›` is emitted and `fitWidth` never sees a data cell.

2. **No test coverage below width 16.** `cmd/goaldl/tui_test.go:980` (`TestTUIWidthFit`) exercises 44×24 only; `pkg/stream/gridviews_test.go` uses width 50. The sub-column-width regime where Issue 1 lives is untested. **Severity: note** — add a case at width ≤15 once Issue 1's guard lands, asserting no fitted grid row contains a truncated digit.

### Overall Verdict

**PASS.** All four acceptance criteria and the cross-cutting invariant are met; every Section D verification command passes (build, vet, gofmt, `-race`, goldens without `-update`, `blm` 469, empty layering-seam and `go.sum` diffs, `x/ansi`-only `go.mod` change). Truncation is column-boundary-clean and display-width-correct at every usable width, ANSI escapes survive intact, the sensor drop order is correct, and `monitor`'s `BLMView.Render` still passes `width=0`. The dependency promotion is acceptable (no new code in the graph). 

**No blockers.** One warning-level defect (Issue 1: partial data digit at width 12–15, an unusably narrow terminal outside F4's scope) and one note (Issue 2: missing sub-16-width test) — both recommended for follow-up, neither gating this slice.
