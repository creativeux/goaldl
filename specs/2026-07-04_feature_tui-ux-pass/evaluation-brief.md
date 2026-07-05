<!-- SDA: v1.0 -->
# Evaluation Brief: TUI UX Pass — Phases A (Trust) + B (Layout)

Self-contained brief for a fresh QA evaluator. You have no implementation context; verify what was built against what was agreed. Be skeptical.

**What shipped in this feature branch (uncommitted working tree)**: Phase A (Trust — findings F1/F2/F3/F15) and Phase B slice 1 (Layout — B.1 pinned/scrollable body + B.4 explainer accordion). **B.3 (collapse empty grid rows) was implemented then reverted at the user's request** — the full grid must render every RPM row; verify the revert is complete. B.2 (width) and Phases C–E are out of scope (not implemented).

---

## Section A: What Was Requested

See [requirements.md](requirements.md) (19 findings F1–F19, ranked S1–S4). In scope for THIS evaluation:

- **F1** (S1): a live-source failure was silent (`session.Run` error discarded); must surface.
- **F2** (S1): `KNOCK_CNT` free-runs on the target vehicle (~+76/frame — verified in `pkg/decoder/testdata/drive_4800.raw` AND `data/20250601_111156_LOG.txt`), so the Spark grid shows counter artifacts as "knock". Must warn (consumer-side), but keep the raw values visible per the raw-data policy.
- **F3** (S1): no staleness signal when live frames stop.
- **F15**: `goaldl help` key text was two phases stale.
- **F4** (S2): short terminal scrolled the tab bar off the top (the user's empirical report corrected the original "footer clips" framing); grid explainers ate vertical space.

Out of scope (do NOT fail the feature for these — they are documented as not-yet-done): F5–F14, F16–F19, B.2 width wrapping, B.3 row-collapse (reverted by design).

## Section B: What Was Agreed To

Acceptance criteria (from [spec-phaseA.md](spec-phaseA.md) and [spec-phaseB.md](spec-phaseB.md); user decisions in [README.md](README.md)):

**Phase A**
1. **A.1** A fatal live error (e.g. port won't open) shows a full-screen error panel with the real error text and, for a live source only, serial hints (`goaldl ports`, `-b 2400`, `-invert`); it is re-printed to stderr on exit. `context.Canceled` (user quit) and a clean `nil` end are NOT errors.
2. **A.2** A live stream quiet for ~6 s (≈5 missed frames) is flagged stale: hollow `○` heartbeat (a glyph change, not just colour) + `no data Ns` footer. Replay, pre-first-frame, and ended streams are never stale; a fresh frame clears it.
3. **A.3** When the knock counter is free-running (drive fixture), the Spark tab shows a warning in its status line; grid values stay at full brightness (not hidden/dimmed). A crafted sparse-knock stream does NOT warn. Clearing the Spark grid preserves the detection window.
4. **A.4** `goaldl help` reflects the 8-tab / session-key dashboard.

**Phase B**
5. **B.1** On a short terminal the rendered frame is ≤ terminal height (the tab bar never scrolls off); an overflowing body scrolls with `j`/`k`/↑/↓ and shows a `lines A–B of N` status. Switching tabs re-homes scroll.
6. **B.4** Grid tabs (BLM/INT/O2/Spark) show a compact one-line legend by default; `i` toggles the full explainer; `i info` appears in the footer only on grid tabs.
7. **B.3 revert (regression check)**: the grid renders EVERY RPM row (400–6400 for the trim grids) with no `(RPM x–y empty)` summary line, in both the dashboard and `monitor -blm`.

**Cross-cutting invariant (all phases)**: presentation/consumer-only. No change to `pkg/stream/session.go`, `pkg/stream/stream.go`, `pkg/ecm`, `pkg/decoder`, `pkg/blm`, or `go.mod`/`go.sum`. Decode path untouched (goldens byte-identical). The `blm` command still records 469 closed-loop samples over the drive fixture. No new dependencies.

## Section C: What Changed

Production files (read them directly):
- `cmd/goaldl/main.go` — A.4 help text.
- `cmd/goaldl/tui.go` — A.1 (`errCh`/`providerDoneMsg{err}`/`fatalErr`/`errorPanel`), A.2 (`tick`/`tickMsg`/`stale`/hollow heartbeat/footer age), A.3 (knock ring window/`pushKnock`/`knockFreeRunning`), B.1 (`activeBody`/`clampBody`/`bodyBudget`/`maxScroll`/`clampScroll`/scroll keys/scroll-reset), B.4 (`showInfo`/`i` key/`keyLegend`/`isGridTab`).
- `pkg/stream/gridviews.go` — A.3 (`SparkBody(..., freeRunning bool)` + split spark explainer), B.4 (`showInfo bool` on `INTBody`/`O2Body`/`SparkBody` + compact legend consts).
- `pkg/stream/blmview.go` — one comment word only (no code change; B.3 threading was reverted).

Test files: `cmd/goaldl/tui_test.go`, `pkg/stream/gridviews_test.go`.
Docs (ignore for correctness): `product-knowledge/PROJECT_STATUS.md`, `product-knowledge/observations/observed-philosophies.md`, `specs/2026-07-04_feature_tui-ux-pass/*`.

`git diff --stat HEAD` for the full list. Read files directly rather than trusting this summary.

## Section D: How to Verify

- **Build/vet/fmt**: `go build ./...` · `go vet ./...` · `gofmt -l pkg cmd` (expect empty).
- **Tests**: `go test -race -count=1 ./...` (expect all green). Feature-specific: `go test ./cmd/goaldl/ -run 'TestTUIFatalError|TestTUIStale|TestTUIKnockFreeRunning|TestTUIInfoAccordion|TestTUIBodyScroll|TestTUIViewPerTab'` and `go test ./pkg/stream/ -run 'TestSparkBody|TestGridLegendAccordion|TestGridExplainers'`.
- **Goldens (decode path untouched)**: `go test ./pkg/decoder -run TestGolden` must pass with NO `-update`.
- **BLM regression**: `go run ./cmd/goaldl blm pkg/decoder/testdata/drive_4800.raw` must print `Recorded 469 into BLM cells`.
- **Forbidden-seam check**: `git diff --stat HEAD -- pkg/stream/session.go pkg/stream/stream.go pkg/ecm pkg/decoder pkg/blm go.mod go.sum` must be EMPTY.
- **B.3-revert check**: `grep -rn "collapse\|rowEmpty\|RPM.*empty)" pkg/stream/gridviews.go pkg/stream/blmview.go` should find no row-collapse logic (the only `collapse` hits are the B.4 explainer-accordion comments). Confirm `gridHeat` loops over all `g.RPM` rows.
- **App is an interactive TUI** (Bubble Tea alt-screen) — not browser-drivable. Verify behavior via the model-level tests (they drive the real `Session`/`ReplayProvider` over the committed `drive_4800.raw` fixture and call `Update`/`View` directly) and by reading `View()`/builder logic. You may write a throwaway `_test.go` harness that drives the model at a chosen `tea.WindowSizeMsg` size and logs `View()` if you want to eyeball layout; delete it after.

## Section E: Standards to Enforce

Read each directly:
- `product-knowledge/standards/decoder/raw-data-policy.md` — **critical**: F2 (knock) and F3 (staleness) must ANNOTATE, never drop/filter/hide data. Spark grid values must stay visible; no frame dropped.
- `product-knowledge/standards/architecture/session-api-layering.md` — Snapshot/Session unchanged; consumers read the existing stream; frame-layout knowledge stays in `pkg/ecm`; `pkg/blm` generic. Presentation builders are allowed on top of the core.
- `product-knowledge/standards/decoder/byte-value-decoding.md` — decode path must be untouched.
- `product-knowledge/standards/testing/golden-fixtures.md` — goldens byte-identical; tests rooted in real captures.
- `product-knowledge/standards/go/tooling.md` — gofmt/vet/`test -race`; no new deps.
- Core philosophies (blocking): `product-knowledge/philosophies/consolidate-over-accrete.md`, `product-knowledge/philosophies/ground-truth-first.md`.

## Section F: Personas to Consult

- `product-knowledge/personas/product-manager.md` — scope discipline (only F1/F2/F3/F4/F15 in scope; B.3 correctly reverted), testable criteria, at-the-car user value.
- `product-knowledge/personas/architect.md` — layering (facade untouched), no new deps, consolidation (helpers extracted not duplicated).
- `product-knowledge/personas/qa.md` — edge cases and oracles: the Canceled-not-error case, staleness boundaries + replay/done exemptions, the sparse-knock false-oracle, scroll clamping, the B.3-revert regression, decode-path/`blm`-469 non-regression.

## Output

Write your evaluation to `specs/2026-07-04_feature_tui-ux-pass/evaluation.md` in the format from the spawn instructions (Acceptance Criteria table, Standards Compliance table, Persona Reviews, Issues Found, Overall Verdict).
