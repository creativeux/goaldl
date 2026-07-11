# Feature: XDF-aware correction export — Trace Log

**Started**: 2026-07-11
**Roadmap**: Horizon 1 lead feature ([ROADMAP.md](../../product-knowledge/ROADMAP.md)); positioning trace `specs/2026-07-11_plan-product/`.

## Active Personas
- Product Manager (`product-knowledge/personas/product-manager.md`)
- Architect (`product-knowledge/personas/architect.md`)
- QA (`product-knowledge/personas/qa.md`)

## Active Capabilities
- Web research (WebSearch/WebFetch) — XDF format spec, community $42 XDF, TunerPro paste semantics.
- Subagent spawn — context-isolated evaluation available for verify-feature.
- No browser-UI or DB tools relevant (CLI feature, file in / file out).
- Ground-truth assets in-repo: real captures (`pkg/decoder/testdata/`), WinALDL log (`data/`).
  **Gap**: no XDF file in-repo yet — acquiring a community $42/1227747 XDF is a prerequisite task.

## Session log

- 2026-07-11: Session start. User decisions at kickoff:
  1. **Feature name**: `xdf-correction-export`.
  2. **Personas**: PM + Architect + QA (the standard trio).
  3. **Scope — multipliers only**: emit the correction grid as multiply-by factors on the XDF
     table's axes; **no .bin parsing** (user applies via TunerPro's multiply-paste). Keeps the
     feature cleanly inside the "no bin editing" permanent non-goal. Bin-reading for
     current→proposed absolute values is a possible later feature; not designed for now.
  4. **Table selection — user names it**: explicit `--table "<title>"` flag with
     case-insensitive/fuzzy title match; on no/ambiguous match, list the XDF's tables and exit.
     No auto-detect heuristics.
- 2026-07-11: Codebase grounding: `pkg/blm.New(rpmLabels, mapLabels)` already accepts arbitrary
  axes and `Grid.Add` bins by nearest label (`blm.go:64,80,104`); `accumulateBLM`
  (`cmd/goaldl/blm.go:18`) is the pure sample loop. **Key architectural finding**: XDF-awareness
  = construct the grid *on the XDF table's axes* and accumulate natively — no post-hoc
  resampling/interpolation stage exists or is needed.
- 2026-07-11: `requirements.md` + `plan.md` written. Status updated. Next: spec-feature.
- 2026-07-11: **Ground-truth XDF acquired** — official `42.xdf` from tunerpro.net's bin-definitions
  page ("$42", author Robert Saar, 2009) → `data/xdf/42.xdf` (gitignored pending license check).
  Findings that resolve plan risks:
  - **Axis encoding RESOLVED (best case)**: `Main VE Table` = 8 rows × 9 cols, **literal embedded
    labels** — `XLabels=20,30,…,100` kPa (X=MAP), `YLabels=400,800,…,3200` RPM (Y), identity
    axis equations (`XEq/YEq = X`). No bin-resident axis data → no collision with no-bin-read.
    `PopByCol=1` (column-major Z population) noted for the paste-orientation question.
  - **Format SURPRISE**: this official file is the **legacy text XDF v1.1** (`%%HEADER%%/%%TABLE%%`
    key=value blocks, CRLF), *not* XML. The XML XDF 2.0 format arrived with TunerPro v5; the
    community's `$42-1227747-V5.9.3.xdf` (gearhead-efi thread 304 attachments, 344.8 KB) is the
    one modern users load and is presumably XML — **unconfirmed**: gearhead-efi is behind a
    Sucuri JS challenge (curl blocked) and the Chrome extension wasn't connected this session.
    → Spec must decide the format target (likely: XML 2.0 primary since that's what current
    users have + the ADX synergy; legacy text as evidence/fallback). Acquiring V5.9.3 manually
    (user's forum login) is an open task.
  - TECH_STACK note "stdlib `encoding/xml`" holds for XML 2.0; the legacy format would need a
    small line-based parser instead (both dependency-free).
- 2026-07-11: **spec-feature session.** User decision: **both XDF formats, sniffed** (legacy
  grounded in `data/xdf/42.xdf`; XML 2.0 grounded in a real community example — `XDFTABLE` /
  `XDFAXIS x|y` / `LABEL index,value` structure confirmed from the BMW-XDFs GitHub repo — and
  to be re-confirmed against `$42-1227747-V5.9.3.xdf` when downloaded). `spec.md` written:
  read-only `pkg/xdf` (stdlib-only, no goaldl deps), `-xdf`/`-table`/`-paste` flags on `blm`,
  grid born on the XDF table's axes via `blm.New` (zero `pkg/blm` change), headerless TSV
  paste block (CRLF, `%.3f`, isolated writer pending the hands-on TunerPro convention check),
  full error taxonomy, from-scratch fixtures + skip-guarded real-XDF test.

## Persona review — spec (2026-07-11)

**Product Manager**: APPROVE. User story concrete (`blm session.raw -xdf 42.xdf -table "main ve"`
→ paste → multiply); scope tightly guarded (multipliers only, no bin read, no TUI, no
auto-detect); success criteria testable; the one manual criterion (TunerPro round trip) has a
named owner (user, Windows box). Discovery mode (`-xdf` alone lists tables) is good first-run
UX. *Note*: implementation should include the user-facing docs (README/CLAUDE.md command
examples) — the feature's value is the workflow, not the flag.

**Architect**: APPROVE. `pkg/xdf` is dependency-free and imports no goaldl package (correct
niche — future ADX importer's sibling); `pkg/blm` untouched (New already generic); forbidden
seam untouched; RPM/MAP role classification correctly lives in the command layer, keeping
`pkg/xdf` semantically neutral about what tables mean; `accumulateBLMInto` refactor is the
minimal seam. Paste-convention risk is isolated in one writer function. *Fixed in spec*:
legacy parse errors carry line numbers.

**QA**: APPROVE. Error taxonomy is a table with exit behavior per case; unhappy paths include
the embedded-axis XML pattern found in the real-world example (all-zero labels); byte-identical
no-`-xdf` regression is an explicit test; paste block golden asserts exact bytes incl. CRLF.
*Fixed in spec*: duplicate exact titles → ErrAmbiguous made explicit; zero-table XDF gets a
distinct error. *Note*: the skip-guarded real-XDF test is silent in CI by design (license);
compensated by structure-mirroring fixtures + the V5.9.3 open task.

**Synthesis**: 3/3 approve, no blockers; two spec fixes applied inline, two notes carried to
implementation.

## Standards Gate Report — pre-implementation (2026-07-11)

| Standard | Scope | Severity | Verdict |
|---|---|---|---|
| decoder/byte-value-decoding | decoder | must | ✅ PASSES — decode path untouched |
| decoder/raw-data-policy | decoder | must | ✅ PASSES — correction math stays consumer-side; no filtering added anywhere |
| architecture/session-api-layering | architecture | must | ✅ PASSES — `pkg/xdf` standalone; Session/Snapshot/ecm/blm seams untouched |
| testing/golden-fixtures | testing | should | ✅ PASSES (with note) — paste-block golden + real-capture fixtures reused; real XDF can't be committed (license) so fixtures are structure-mirroring + a skip-guarded local real-file test |
| go/tooling | go | should | ✅ PASSES — stdlib only, zero new deps, standard gate |
| release/versioning | release | must | ✅ PASSES — `feat:` conventional commit (pre-1.0 patch bump) |
| release/platform-support | release | should | ✅ PASSES — pure-Go file parsing, no platform code |
| review/agent-pr-review | review | must | ✅ PASSES — lands via PR under the agent-review flow |

**Philosophy cross-check (core)**:
- *Ground truth first*: ✅ with one ⚠️ **by-design WARNING** (same pattern as TCPProvider):
  the TunerPro clipboard/paste convention is an **unverified assumption, named and tracked**
  (spec §3.1) — resolved only by the hands-on TunerPro RT check in verify-feature. Axis
  encoding is grounded in the real official XDF; XML structure in a real community file;
  fixtures mirror both. Second tracked gap: V5.9.3 XML confirmation (manual download).
- *Consolidate over accrete*: ✅ — capability grows as a consumer of the existing grid engine;
  no parallel path (no resampler, no second accumulation loop; one wrapper refactor).

**Gate decision: PROCEED** (no violations; one by-design warning carried to verify).

## Implementation — 2026-07-11 (branch `feat/xdf-correction-export`, off main `866e81b`)

All 11 tasks complete ([tasks.md](tasks.md)). Files:

- **New `pkg/xdf/`**: `xdf.go` (types, sniff, `Find`, validation), `legacy.go` (text v1.x parser),
  `xml.go` (XML parser), `xdf_test.go` + `testdata/mini-legacy.xdf` / `mini-xml.xdf`
  (from-scratch fixtures, license-safe).
- **`cmd/goaldl/blm.go`**: `-xdf`/`-table`/`-paste` flags, discovery listing, `classifyAxes`
  (units → range fallback; refuses to guess), `accumulateBLMInto` refactor (+out-of-range count;
  `accumulateBLM` wrapper keeps the old call sites/tests untouched), `pasteBlock` (headerless
  TSV, CRLF, %.3f, table-layout orientation incl. transpose).
- **`cmd/goaldl/blm_xdf_test.go`**: VE-axes parity vs the drive fixture (469 samples, 1600×40 ≈
  117.17 — same numbers as the default-axes regression), paste-block format + transposed
  orientation, classification refusals, discovery listing, wrapper-parity.
- **Docs**: `docs/blm-tuning.md` new "Straight into TunerPro" section; `CLAUDE.md` command +
  architecture entries.

**Spec deviations (2, both parser-hardening found by the real file):**
1. Legacy label defects are per-axis (`Axis.LabelErr`), not parse-fatal — the official 42.xdf
   writes `XLabels =(null)` on its 1D tables, and one bad table must not hide the other 49.
   `(null)` literally means "no labels". (Spec assumed label errors were block-fatal.)
2. Same leniency on the XML side for non-numeric (text) labels — the table lists in discovery,
   the defect surfaces on selection.

**Verified against real artifacts**: `TestRealXDF` parses the official 42.xdf (50 tables,
exact VE axes); end-to-end smoke run on `drive_4800.raw` + `42.xdf` produced the discovery
listing and an 8×9 VE-axes paste block.

**Gate**: gofmt/vet/build/`test -race` all green; forbidden seam diff **empty**
(`pkg/decoder`, `pkg/ecm`, `pkg/stream`, `go.mod` and `pkg/blm` — zero change, as specced);
decoder goldens untouched by construction.

**Pattern observed** (for pattern-observer): *license-blocked ground truth* — when the real
artifact can't be committed (redistribution unverified), pair a from-scratch structure-mirroring
fixture (runs everywhere) with a skip-guarded real-file test (runs where the artifact exists).
Second use of the shape after `data/xdf/` itself; candidate for a testing standard if it recurs.

**Remaining before close**: verify-feature (fresh evaluator) + the two carried manual steps —
hands-on TunerPro multiply-paste check (user, Windows box) and the V5.9.3 XML confirmation
(user download).

## PR + XML confirmation — 2026-07-11 (later)

- **PR #41 opened**; CI + agent review both pass ("No blocking issues found").
- **XML-confirmation task CLOSED**: user downloaded `$42-1227747-V5.9.3.xdf` and
  `$42-1227747-V4T.xdf` (gearhead-efi thread 304; forum-page PDF saved as
  `data/1227747 ECM Information $42.pdf`, gitignored). Both parse as XML (57/48 tables);
  end-to-end export against the drive capture reproduces the same correction values as the
  official legacy file, and V5.9.3's genuinely transposed "Main Fuel Table Corrected" (9×8,
  X=RPM) exports correctly in its own layout — the orientation contract proven against a real
  in-the-wild table. `TestRealXDF` is now table-driven over all three real files (`07376c8`).
- Finding worth keeping: the three real definitions use three different VE-table titles
  ("Main VE Table" / "Fuel VE 1 - Main Fuel Table" / "VE as % (FL1)") — community naming
  drift that validates the explicit `-table` decision over auto-detect.
- **Still carried to verify**: only the hands-on TunerPro multiply-paste check remains.
