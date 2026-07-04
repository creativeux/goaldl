<!-- SDA: v1.0 -->
# Evaluation Brief: WinALDL Parity — Phase 2 (Tune)

You are verifying **Phase 2** of the WinALDL-parity feature for `goaldl` (a Go ALDL scanner/datalogger for the GM 1227747 ECM). Phase 1 already shipped; only the Phase 2 changes below are under evaluation. Evaluate against what was agreed, not against WinALDL feature-completeness.

---

## Section A: What Was Requested

Phase 2 ("Tune") turns the TUI dashboard into a live tuning instrument. Scope (plan.md Phase 2, steps 2.1–2.4) with user decisions:

- **2.1 INT grid tab** — integrator (short-term fuel trim) as an RPM×MAP Wide-Average grid, **closed-loop gated** (distinct from BLM's closed-loop **and** block-learn-enabled gate).
- **2.2 O2 grid tab** — oxygen-sensor voltage grid, **ungated** (populates on every parsed frame).
- **2.3 In-TUI Save/Clear** — `s` saves **all three grids** (BLM+INT+O2) to timestamped files; `c` clears the **current** grid (or resets extrema on the sensor tab).
- **2.4 Sensor Min/Max** — **always-on MIN and MAX columns** on the sensor tab (not a mode toggle); `c` resets them.
- **Decision 4 (added mid-spec):** a **persistent loop-state line** on every tab (Open/Closed loop + per-grid recording indicators), because loop state governs whether the BLM/INT grids are accumulating at all.

**Tab order decision:** grids grouped — `1 Sensors · 2 BLM · 3 INT · 4 O2 · 5 Flags · 6 Codes · 7 Raw` (keys 1–7).

**Architecture constraint (hard):** Phase 2 must be **presentation + consumer-side accumulation only** — **no changes** to `stream.Snapshot`, `stream.Session`, `pkg/blm`, or `pkg/ecm`, and the **decode path must be untouched** (decoder golden files byte-identical). INT/O2 grids and extrema are accumulated in the TUI model from the existing `Snapshot` stream (`Sensors` + `FuelTrim`).

Full requirements: `specs/2026-07-04_feature_winaldl-parity/requirements.md` (deltas D4, D6, D7, D9, D16). Full spec: `specs/2026-07-04_feature_winaldl-parity/spec-phase2.md`.

---

## Section B: What Was Agreed To (acceptance criteria)

1. **INT grid** accumulates the integrator binned by RPM×MAP, only when **closed loop**; open-loop frames do not add to it. Renders a heatmap with active-cell highlight and status line.
2. **O2 grid** accumulates O2 volts (`Sensors["oxygen_sensor"]/1000`) on **every parseable frame** (ungated). Grid cells render to **2 decimals** (legibility — a 3-decimal cell fills the whole column and collides); the current-reading status line and the **saved** O2 file keep **3 decimals**.
3. **Save (`s`)** from any tab writes three files `goaldl_<YYYYMMDD_HHMMSS>_{BLM,INT,O2}.txt`: BLM & INT contain Samples + Wide Average + **Correction** (avg/128); O2 contains Samples + Wide Average and **no correction table**.
4. **Clear (`c`)** clears only the active grid (BLM/INT/O2 independently); on the sensor tab it resets Min/Max extrema; on Flags/Codes/Raw it is a no-op.
5. **Sensor tab** shows a 6-column table `SENSOR·RAW·VALUE·MIN·MAX·ALT`; MIN/MAX track per-sensor extrema in the primary unit and reset on `c`.
6. **Persistent loop line** appears on **every** tab: badge `CLOSED LOOP` (green) / `OPEN LOOP` (amber) / `LOOP —` (dim before first good frame), plus `BLM ●/○ INT ●/○ O2 ●/○` recording dots reflecting each grid's gate. It is derived from the last **parseable** frame (holds across a following bad frame, no flicker).
7. **Tabs** reorder to Sensors·BLM·INT·O2·Flags·Codes·Raw; keys 1–7 select; tab/arrows cycle all 7.
8. **No regression:** `monitor` sensor table stays 4-column; `monitor -blm` and the `blm` command are unchanged (still record 469 closed-loop samples over the drive fixture). `stream.Snapshot`/`Session`/`pkg/blm`/`pkg/ecm` unchanged. Decoder goldens byte-identical.

Out of scope (do NOT flag as missing): spark-counts grid, recording/CSV toggles, replay pause/speed keys, Dash view, config persistence, multi-ECM, Narrow/Avg10/StdDev grid modes; INT/O2 in the `monitor` command (dashboard-only this phase).

---

## Section C: What Changed

New files:
- `pkg/stream/gridviews.go` — `gridHeat` (shared heatmap renderer), `INTBody`, `O2Body`, `LoopBadge`, `LoopStatus`.
- `pkg/stream/gridviews_test.go` — tests for the above.
- `specs/.../spec-phase2.md` — the spec (reference).

Modified:
- `pkg/stream/blmview.go` — `BLMBody` refactored to delegate to `gridHeat` (behavior intended identical; still used by `monitor -blm`).
- `pkg/stream/table.go` — `Row` gained `Min`/`Max`; added `BuildRowsExtrema`, `renderTableExtrema`, `SensorTableExtrema` (nil extrema → 4-column `SensorTable`).
- `cmd/goaldl/tui.go` — view enum regrouped + keys 1–7; model fields `intGrid/o2Grid/mins/maxs/hasExtrema/notice`; `accumulate`, `save`, `clear`, `saveGrids`, `writeTrimGridFile`, `writeO2File`, `loopStatusLine`; INT/O2/extrema wired into `View`.
- `cmd/goaldl/tui_test.go` — new tests + existing tab/view tests updated for 7 tabs.

Review the diff with: `git diff HEAD -- pkg cmd` and read the new files directly. The branch point is the current `HEAD` (Phase 2 is uncommitted working-tree changes).

---

## Section D: How to Verify

- **Build/vet:** `go build ./...` · `go vet ./...`
- **Format:** `gofmt -l pkg cmd` (must print nothing)
- **Full test suite (race):** `go test -race ./...`
- **Decoder goldens untouched:** `go test ./pkg/decoder -run TestGolden -count=1` (must pass with **no** `-update`)
- **Non-regression sanity (optional, over the committed fixture):**
  - `go run ./cmd/goaldl blm pkg/decoder/testdata/drive_4800.raw` → expect `Recorded 469 into BLM cells`.
  - `go run ./cmd/goaldl monitor pkg/decoder/testdata/drive_4800.raw -blm -speed 0` → BLM grid renders.
- **End-to-end dashboard test:** `go test ./cmd/goaldl -run TestTUIDriveFixtureEndToEnd -v` drives all 635 drive-fixture frames through a real `Session` into the model (BLM==469 cross-check, INT>BLM, O2≥INT, all 7 tabs render).
- The interactive TUI needs a real terminal (Bubble Tea alt-screen) — the model logic is exercised headlessly by the tests above; you are not expected to drive the live TUI. No browser tools apply.

---

## Section E: Standards to Enforce

Read each and check the diff against it:
- `product-knowledge/standards/decoder/byte-value-decoding.md` (must) — decode path.
- `product-knowledge/standards/decoder/raw-data-policy.md` (must) — no plausibility filtering in the transport; gating is a consumer/view decision; PROM/parse are quality signals, not drop-filters.
- `product-knowledge/standards/architecture/session-api-layering.md` (must) — front-ends consume `stream.Session`/`Snapshot`; frame-layout knowledge stays in `pkg/ecm`; `pkg/blm` stays a generic grid; presentation layered on top.
- `product-knowledge/standards/testing/golden-fixtures.md` (should) — tests rooted in real captures; goldens only regenerate on an intended decoder change.
- `product-knowledge/standards/go/tooling.md` (should) — gofmt/vet/build/test-race is the whole gate; minimal deps.
- Philosophies (core, blocking): `product-knowledge/philosophies/consolidate-over-accrete.md`, `product-knowledge/philosophies/ground-truth-first.md`.

---

## Section F: Personas to Consult

Adopt each and critique Phase 2:
- `product-knowledge/personas/product-manager.md` — user value, scope, testable acceptance.
- `product-knowledge/personas/architect.md` — layering, no new deps, consistency, tech debt.
- `product-knowledge/personas/qa.md` — edge cases, gating correctness, regression, coverage.

---

Write your evaluation to `specs/2026-07-04_feature_winaldl-parity/evaluation.md` per the required format. Be skeptical — your job is to find problems, not confirm success. Pay particular attention to: (a) the INT-vs-BLM gating distinction being actually correct in code, (b) whether any of the four forbidden packages (`Snapshot`/`Session`/`pkg/blm`/`pkg/ecm`) were in fact modified, and (c) whether `accumulate` can bin spurious zeros from an unparsed frame.
