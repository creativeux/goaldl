# Requirements: XDF-aware correction export

## Goal

Close the drive → TunerPro loop. Today `goaldl blm` emits a correction grid on goaldl's own
default RPM×MAP axes, which a tuner must mentally re-map onto their VE table before typing
changes into TunerPro. Instead: read the community XDF definition for the bin being tuned,
build the fuel-trim correction grid **directly on the named XDF table's own axes**, and emit it
in a format TunerPro accepts as a multiply-paste — so a logged drive becomes applied VE-table
corrections in one paste, with zero manual re-binning.

**User story**: I drive with goaldl recording (onboard or laptop). At the desk I run
`goaldl blm session.raw --xdf 747.xdf --table "Main VE Table"`, review the correction table +
its confidence readout, copy the export, and multiply-paste it onto the VE table in TunerPro RT.

## In scope

- Read-only XDF parse: enumerate tables; extract the named table's X/Y axis labels (and units
  where declared) — enough to build axes and label the export. Nothing else from the XDF.
- `--table` selection by title (case-insensitive, forgiving match); on no/ambiguous match,
  list available tables and exit non-zero.
- Correction grid accumulated natively on the XDF table's axes (existing `blm.Grid` semantics:
  Wide Average / 128, nearest-label binning, `-min` confidence threshold, untrusted cells = 1.000).
- Export in TunerPro's paste format (tab-delimited value block matching the table's dimensions,
  row/column orientation matching TunerPro's clipboard convention — exact convention verified
  against real TunerPro during spec/verify).
- Confidence sidecar in the human-readable output: per-cell sample counts, count of trusted
  vs total cells, cells held at 1.000.
- Axis-mismatch honesty: if the XDF table's axes aren't RPM×MAP-shaped (units/ranges
  implausible for BLM data), say so and stop — never silently bin garbage.

## Out of scope (this feature)

- **Any .bin reading or writing** (multipliers only — user decision 2026-07-11; TunerPro's
  multiply-paste applies them). No current→proposed absolute values.
- XDF table auto-detection heuristics.
- ADX import, session report, cross-session diff (separate Horizon 1/3 features).
- XDF editing/writing of any kind; checksums.
- TUI integration (CLI `blm` command only; the TUI can grow an export later).

## Success criteria

1. **Round-trip against real TunerPro**: with a community $42/1227747 XDF loaded in TunerPro RT,
   the exported block pastes onto the named VE table via multiply-paste without manual reshaping,
   and the resulting values equal hand-multiplying our correction factors (spot-check ≥3 cells).
2. **Axes come from the XDF**: changing the axis labels in the XDF changes the export's axes
   (proven by a test fixture XDF variant) — no goaldl-default axes anywhere in the output.
3. **Ground truth preserved**: on `drive_4800.raw` with goaldl-default-equivalent axes, cell
   values match today's `blm` output (e.g. the verified 1600×40 → 117.17 cell); total recorded
   samples stays 469. Decoder/`pkg/ecm` untouched; goldens byte-identical.
4. **Confidence semantics**: cells below `-min` samples export exactly 1.000 and are reported;
   trusted-cell count matches `PopulatedCells(min)`.
5. **Failure honesty**: missing/unparseable XDF, unknown table title, ambiguous match, and
   non-RPM×MAP axes each produce a specific, actionable error (with the table list where
   relevant) — never a silently wrong grid.
6. **No new heavyweight deps**: XDF parsing via stdlib `encoding/xml` (per TECH_STACK.md).

## Constraints & standards in play

- Raw-data-raw policy: correction math stays consumer-side; decode path untouched.
- Layering: XDF knowledge must not leak into `pkg/decoder`/`pkg/stream`; natural home is a new
  `pkg/xdf` (parse) consumed by `cmd/goaldl` alongside `pkg/blm` (which stays a generic grid).
- Conventional commits; feature lands via PR under the agent-review flow.
- Prerequisite asset: a community $42/1227747 XDF (and a trimmed fixture derivative for tests —
  license/attribution checked before committing any community file).
