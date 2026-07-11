# Tasks: XDF-aware correction export

Branch: `feat/xdf-correction-export` (off main `866e81b`, post-PR #40).

- [x] 1. `pkg/xdf`: types (`Axis`/`Table`/`File`), `Parse` entry with format sniff (legacy text vs XML 2.0)
- [x] 2. `pkg/xdf`: legacy text parser (`%%TABLE%%` blocks, keyed lines by numeric ID, line-numbered errors)
- [x] 3. `pkg/xdf`: XML 2.0 parser (`XDFTABLE`/`XDFAXIS`/`LABEL`, dims from z `EMBEDDEDDATA` w/ x/y `indexcount` fallback)
- [x] 4. `pkg/xdf`: `Find` — trimmed case-insensitive exact → substring; ambiguity/not-found/no-tables errors; axis validation (count, monotonic, embedded/non-identity rejection)
- [x] 5. `pkg/xdf/testdata`: from-scratch fixtures (mini-legacy, mini-xml, transposed table, ambiguity pair, category pseudo-table, embedded-axis + broken variants) + unit tests
- [x] 6. `pkg/xdf`: skip-guarded `TestRealXDF` against `data/xdf/42.xdf` (exact Main VE Table axes)
- [x] 7. `cmd/goaldl/blm.go`: `-xdf`/`-table`/`-paste` flags; discovery listing (`-xdf` alone, exit 0); flag-dependency usage errors; `accumulateBLMInto` refactor (+ out-of-range count)
- [x] 8. `cmd/goaldl`: axis-role classification (units → range fallback), report lines, `writePasteBlock` (headerless TSV, CRLF, %.3f, table-layout orientation incl. transpose)
- [x] 9. `cmd/goaldl` tests: VE-axes parity vs drive fixture (1600×40 avg 117.17, 469 samples), paste-block content/orientation/format, discovery, transposed, error paths; no-`-xdf` path untouched (diff evidence)
- [x] 10. Docs: CLAUDE.md + README command examples (the drive→paste workflow)
- [x] 11. Gate: gofmt/vet/build/`test -race` green; goldens byte-identical; forbidden seam diff empty (`pkg/decoder`, `pkg/ecm`, `pkg/stream`, `go.mod`); `pkg/blm` zero-change
