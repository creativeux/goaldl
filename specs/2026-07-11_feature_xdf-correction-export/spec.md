# Spec: XDF-aware correction export

**Status**: Draft for persona review — 2026-07-11
**Inputs**: [requirements.md](requirements.md) · [plan.md](plan.md) · ground truth `data/xdf/42.xdf`
(official tunerpro.net, legacy text v1.1) · XML 2.0 structure confirmed against a real community
XDF (`XDFFORMAT`/`XDFTABLE`/`XDFAXIS`/`LABEL`, BMW-XDFs repo example).
**User decisions**: multipliers only (no .bin read) · `-table` explicit selection · **both XDF
formats, sniffed** (2026-07-11).

## 1. Ground truth (drives everything below)

`data/xdf/42.xdf` → `Main VE Table`: `Rows=8` (Y, RPM: `YLabels=400,800,…,3200`, `YUnits="RPM"`),
`Cols=9` (X, MAP: `XLabels=20,30,…,100`, `XUnits="kPa"`), identity axis equations, literal
embedded labels — no bin-resident axis data. `PopByCol=1`. Note: the X/MAP labels equal
`blm.DefaultMAP` exactly; the Y/RPM labels are the first 8 of `blm.DefaultRPM`.

XML 2.0 equivalent (from the confirmed example): `<XDFTABLE><title>…</title>` +
`<XDFAXIS id="x|y">` with `<indexcount>` and `<LABEL index value>` children (+ optional
`<units>`), `<XDFAXIS id="z">` carrying `EMBEDDEDDATA rowcount/colcount`. XML axes **may be
bin-embedded** (labels absent or all `0.00` + an `EMBEDDEDDATA` address on x/y) — must be
detected and rejected with a specific error (collides with no-bin-read).

## 2. Package: `pkg/xdf` (new)

Read-only, dependency-free (stdlib only: `encoding/xml`, `bufio`, `strings`, `strconv`).
Parses *only* what the feature needs: table titles, dimensions, X/Y axis labels + units.
No Z data, no addresses, no equations beyond noting non-identity (see 2.4).

```go
type Axis struct {
    Labels []float64 // literal label values, in file order
    Units  string    // as declared ("kPa", "RPM", may be "")
}
type Table struct {
    Title string
    X, Y  Axis // X = columns, Y = rows (both formats' convention)
    Rows, Cols int
}
type File struct { /* Format ("legacy"|"xml"), Title/Desc, tables */ }

func Parse(r io.Reader) (*File, error)          // sniffs format, parses all tables
func (f *File) Tables() []Table                  // in file order (category pseudo-tables filtered: no axes)
func (f *File) Find(title string) (*Table, error)// forgiving match, see 2.3
```

### 2.1 Format sniff
First non-whitespace bytes: `<` (after optional `<!-- … -->` comment) → XML; literal `XDF`
magic on line 1 → legacy text; anything else → `ErrUnknownFormat` ("not a TunerPro XDF").

### 2.2 Parsers
- **Legacy** (line-based; ground truth `42.xdf`): scan `%%TABLE%%…%%END%%` blocks; within a
  block read keyed lines — `040005 Title`, `040300 Rows` / `040305 Cols` (hex `0x…`),
  `040320 XUnits` / `040325 YUnits`, `040350 XLabels` / `040360 YLabels` (comma-separated
  floats), `040354 XEq` / `040364 YEq` (identity check only). CRLF tolerated. Keys matched by
  numeric ID (stable across writers), names informational. Parse errors carry the line number
  (Architect review).
- **XML** (`encoding/xml`): `XDFTABLE` elements; `title`; `XDFAXIS id="x"/"y"` → `LABEL`
  `value` attrs ordered by `index`, `indexcount`, `units` child if present; dims from z-axis
  `EMBEDDEDDATA` `mmedrowcount`/`mmedcolcount` (fallback: y/x `indexcount`). `MATH equation`
  captured for the identity check.

### 2.3 Title matching (`Find`)
1. Case-insensitive exact match on whitespace-trimmed titles → unique hit wins; **multiple
   exact hits (duplicate titles in the XDF) → `ErrAmbiguous`** (QA review).
2. Else case-insensitive substring match → exactly one hit wins; multiple hits →
   `ErrAmbiguous` carrying the candidate titles; zero hits → `ErrNotFound` carrying all
   table titles (the command prints them). A file with zero real tables (only category
   pseudo-tables or none) → distinct "XDF contains no tables" error (QA review).
Category separator rows in the legacy file (e.g. title `"     Fuel"`, no labels) are excluded
from matching and listing.

### 2.4 Axis validation (parse-time, per table on `Find`; listing is lenient)
- Label count must equal the corresponding dimension (X↔Cols, Y↔Rows).
- Labels must parse as floats and be strictly monotonic (ascending or descending).
- **Embedded-axis rejection**: labels missing, or all equal (the XML `0.00` pattern) →
  `ErrEmbeddedAxis`: "this XDF computes its axis from the bin; goaldl does not read bins".
- Non-identity axis equation (`XEq`/`MATH` ≠ `X`) → same rejection (labels would need scaling
  we can't verify without the bin).

## 3. Command UX (`goaldl blm`, `cmd/goaldl/blm.go`)

New flags (Go single-dash style, matching `-b`/`-o`/`-min`):

```
goaldl blm <capture.raw> -xdf 42.xdf -table "main ve" [-min 4] [-o corr.csv] [-paste ve.txt]
goaldl blm <capture.raw> -xdf 42.xdf              # discovery: list tables, exit 0
```

- `-xdf` alone (no `-table`): print the table list (title + dims + units) and exit 0 —
  the discovery path. `-table`/`-paste` without `-xdf`: usage error.
- With `-xdf -table`:
  1. `xdf.Parse` + `Find` (errors per §2.3/2.4, exit 1).
  2. **Axis role mapping**: identify the RPM axis vs the MAP axis by units first
     (case-insensitive contains "rpm" / "kpa"), falling back to range plausibility
     (RPM-like: max > 300 and max ≤ 8000 with min ≥ 0; MAP-like: all labels within 10–110).
     Ground truth: X=kPa, Y=RPM, but a transposed table must work — the export block is
     emitted in **the table's own layout** (rows=Y, cols=X) regardless of which is RPM.
     If either axis can't be classified, or both classify the same: specific error naming
     the units/ranges seen — never bin into a guessed orientation.
  3. `grid := blm.New(rpmLabels, mapLabels)` — the existing nearest-label `Add` semantics
     apply unchanged (out-of-range samples absorb into edge cells, e.g. >3200 RPM into the
     3200 row on the VE axes; documented in output, see below).
  4. Accumulate via the existing `accumulateBLM` loop (gating unchanged) — refactored only to
     take a `*blm.Grid` parameter (`accumulateBLMInto(grid, frames)`; `accumulateBLM` stays
     as the default-grid wrapper so existing tests/paths are untouched).
  5. Stdout: today's report (frames/skips/trusted-cells + Samples/Average/Correction renders)
     on the XDF axes, plus one line naming the XDF file, table title, and axis orientation
     (e.g. `Axes from "Main VE Table" (42.xdf): rows RPM 400–3200, cols kPa 20–100`), plus an
     edge-absorption note when any sample exceeded the axis range.
  6. **Paste block**: printed to stdout last under a `--- TunerPro paste block (…) ---`
     marker, and written verbatim to the `-paste` file when given.
- Without `-xdf`: byte-identical behavior to today (criterion 3).
- `-o` semantics unchanged (correction CSV, now on the active grid's axes).

### 3.1 Paste block format (the TunerPro contract)
Tab-separated values only — no headers, no axis labels: `Rows` lines × `Cols` values,
`%.3f`, CRLF line endings, in the table's own label order (row 1 = first Y label, col 1 =
first X label). Untrusted cells (< `-min` samples) emit `1.000`.
**Assumption to verify hands-on in TunerPro RT** (verify-feature manual step): TunerPro's
table clipboard format is a headerless TSV block in display order, and paste-with-multiply
accepts it cell-for-cell. The writer is one isolated function (`writePasteBlock`) so a
convention correction touches one place. `PopByCol=1` refers to Z bin storage order, not
clipboard order — irrelevant to us (we never read Z), noted to preempt confusion.

## 4. Edge cases / error taxonomy

| Case | Behavior |
|---|---|
| XDF unreadable / unknown format | exit 1, sniff error message |
| `-table` no match | exit 1, "no table matching X; available:" + list |
| `-table` ambiguous | exit 1, "matches N tables:" + candidates |
| Embedded/computed axis, non-identity equation | exit 1, `ErrEmbeddedAxis` text (§2.4) |
| Label count ≠ dims, non-monotonic, non-numeric | exit 1, per-table parse error naming the axis |
| Axis roles unclassifiable / both same | exit 1, error naming units+ranges seen |
| 1D table (Rows or Cols < 2) | exit 1, "not a 2D RPM×MAP table" |
| Samples beyond axis range | absorbed into edge cells + stdout note (existing Grid semantics) |
| No closed-loop samples | existing friendly message (unchanged) |
| `-paste` file exists | overwrite (consistent with `-o`; TUI-style exclusive-create is a TUI concern) |

## 5. Testing

- **Fixtures written from scratch** (license-safe; `pkg/xdf/testdata/`): `mini-legacy.xdf` and
  `mini-xml.xdf` mirroring the ground-truth Main VE Table structure (8×9, same labels/units,
  a category pseudo-table, a second table to exercise ambiguity), plus broken variants
  (embedded-axis XML with all-zero labels, label-count mismatch, non-monotonic, unknown format).
  The real `data/xdf/42.xdf` stays gitignored; a `TestRealXDF` guarded by
  `os.Stat` skip (runs locally when present, skips in CI) parses it and asserts the Main VE
  Table's exact axes — keeps the ground truth live without committing it.
- **`pkg/xdf` unit tests**: sniff, both parsers (table-driven), Find matching/ambiguity,
  validation errors.
- **Command-level** (`cmd/goaldl`): drive fixture + `mini-*.xdf` end-to-end — asserts (a) the
  VE-axes grid's 1600×40 cell average equals the known 117.17 (same labels as the default grid
  ⇒ same binning for interior cells); (b) total recorded samples 469; (c) paste block golden
  (exact bytes incl. CRLF); (d) discovery listing; (e) transposed-axes fixture produces the
  transposed block; (f) no `-xdf` output byte-identical to today (capture both, compare).
- **Unchanged**: decoder goldens, `pkg/blm` tests (zero code change expected there).

## 6. Layering & standards

- Forbidden seam: `pkg/decoder`, `pkg/ecm`, `pkg/stream`, `Session`/`Snapshot` untouched.
- `pkg/blm`: **zero change** (`New` already takes arbitrary axes).
- `cmd/goaldl/blm.go`: flags + orientation mapping + paste writer + report lines;
  `accumulateBLMInto` refactor as §3.
- New `pkg/xdf`: no dependency on any goaldl package (pure definition parsing) — the future
  ADX importer's sibling.
- Raw-data-raw: untouched decode path; correction math consumer-side as today.
- Conventional commit: `feat: XDF-aware correction export for TunerPro paste` (pre-1.0 ⇒ patch).

## 7. Verify-feature plan (ahead of time)

1. Automated: full suite `-race`, goldens byte-identical, seam diff empty, criteria §5.
2. **Manual ground truth**: real TunerPro RT + `data/xdf/42.xdf` (or V5.9.3) + a $42 bin —
   multiply-paste the exported block onto Main VE Table; spot-check ≥3 cells against
   hand-multiplied values. Owner: user (Windows box/VM).
3. **Open task carried**: obtain `$42-1227747-V5.9.3.xdf` (gearhead-efi login) → confirm the
   XML parser against it (add to the skip-guarded real-file test).

## Open questions resolved in this spec
- Format target → both, sniffed (user 2026-07-11).
- Axis encoding → literal labels in ground truth; embedded-axis XDFs rejected explicitly.
- Discovery UX → `-xdf` without `-table` lists tables.
- Paste destination → stdout block + optional `-paste` file; `-o` CSV unchanged.
