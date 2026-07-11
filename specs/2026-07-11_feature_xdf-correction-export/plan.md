# Plan: XDF-aware correction export

## Approach in one line

Parse the XDF just enough to get the named table's axes → `blm.New(xdfRPM, xdfMAP)` → the
existing accumulation loop fills it natively → render the correction as a TunerPro
multiply-paste block. No resampling stage; the grid is born on the right axes.

## Why this shape (key finding)

`pkg/blm` was already built generic: `New(rpmLabels, mapLabels []float64)` takes arbitrary axes
(`blm.go:64` — how `NewSpark` gets WinALDL's spark axes today) and `Add` bins by nearest label
(`blm.go:80,104`). `accumulateBLM` (`cmd/goaldl/blm.go:18`) is a pure frames→grid loop
independent of axis choice. So the feature decomposes into *parse axes* + *format output*,
with the entire middle reused untouched. Interpolation/resampling of a finished grid is
explicitly rejected: accumulating raw samples into the target cells is exact where resampling
would smear.

## Components

1. **`pkg/xdf` (new, small)** — read-only XDF parse via stdlib `encoding/xml`:
   - `List(r io.Reader) []TableInfo` (title, dims, axis units) — powers the no-match listing.
   - `FindTable(r io.Reader, title string) (Table, error)` — forgiving title match; `Table`
     carries X/Y axis label values + units + dims. Ambiguity = error listing candidates.
   - Parses axis **labels** only (embedded or computed-from-math per the XDF spec — depth
     decided at spec time against real community files). No Z-data addresses, no bin math.
2. **`cmd/goaldl/blm.go` extension** — `--xdf <file>` + `--table <title>` flags: axes from
   `pkg/xdf` instead of `blm.DefaultRPM/MAP`; plausibility check (RPM-ish × MAP-ish) before
   accumulating; everything else (gating, `-min`, renders) unchanged. Without `--xdf`, behavior
   is byte-identical to today.
3. **Paste-format writer** — tab-delimited correction block (dims = the XDF table's), plus the
   existing human-readable renders and sample-count table. Output destination (`-o` file vs
   stdout section) settled at spec time; orientation/order verified against real TunerPro.

## Layering / seams

- Forbidden seam unchanged: `pkg/decoder`, `pkg/ecm`, `pkg/stream`, `session` untouched;
  goldens byte-identical; `blm` fixture count 469 preserved.
- `pkg/blm` stays a generic grid — ideally zero changes; any addition must be generic
  (à la `Sum()`/`NewSpark()` precedent).
- XDF knowledge lives only in `pkg/xdf` + the command layer. This is deliberate groundwork:
  ADX import (Horizon 3) will want the same XML-definition parsing niche beside it.

## Plan of work

1. **Acquire ground truth**: community $42/1227747 XDF (gearhead-efi et al.); confirm
   redistribution/attribution before committing; derive a trimmed test fixture regardless.
   Inspect real TunerPro paste semantics (clipboard format, orientation, multiply-paste UX)
   on the Windows box / VM.
2. **Spec** (`spec-feature`): XDF element subset to parse (incl. computed-axis math depth),
   exact flag/output UX, paste-format contract, error taxonomy, fixture strategy.
3. **Implement**: `pkg/xdf` (table-driven tests on fixtures) → command wiring → writer
   (golden-style output test) → end-to-end test: drive fixture + fixture XDF → expected block.
4. **Verify** (`verify-feature`): context-isolated evaluator + the real-TunerPro round-trip
   (success criterion 1) as the manual ground-truth step.

## Risks / open questions (for spec)

- **XDF axis encoding variance**: community XDFs encode axis labels embedded, external, or as
  math over addresses. If the $42 XDF's axes need bin-resident data to compute, that collides
  with "no bin reading" — mitigation: support literal/embedded labels first and error clearly
  otherwise ("this XDF computes axes from the bin; supply --axes" or defer). **Must be settled
  by inspecting the real $42 XDF in step 1.**
- **TunerPro clipboard convention** (row-major vs column-major, header row or not, decimal
  precision): unverified assumption until step 1's hands-on check; the writer isolates it.
- **MAP-axis unit mismatch** (kPa vs volts vs %): plausibility check + explicit error; unit
  conversion only if the real XDF demands it (spec decision).
- **License** of the community XDF: fixture must be either cleared or a from-scratch minimal
  XDF written for tests (safe default).
