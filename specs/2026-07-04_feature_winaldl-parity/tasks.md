<!-- SDA: v1.0 -->
# Tasks: WinALDL Parity Phase 1 (MVP)

Breakdown mirrors plan.md Phase 1 / spec.md; agreed with user 2026-07-04 (MVP cut).

- [x] 1.1a `pkg/ecm`: FlagBit/FlagWord/ErrorCode + status types, DecodeFlags/DecodeCodes; Definition gains FlagWords/ErrorCodes/ByteLabels; AltConversion on Parameter (`flags.go`, `ecm.go`)
- [x] 1.1b `pkg/ecm`: 1227747 tables (MW2/MWAF1/MCU2IO flags, MALFFLG1-3 codes, byte labels, knock_count param, Alt conversions CT°C/KPH/MAP kPa/TPS%) (`gm_1227747.go`)
- [x] 1.1c `pkg/ecm`: MapVoltsToKPa → verified (raw+28.06)/2.71; fueltrim↔flag-table consistency test; log-oracle tests (flags, codes, kPa, TPS%) (`fueltrim.go`, `flags_test.go`)
- [x] 1.1d `pkg/stream`: Snapshot gains Flags/Codes; Session caches def, uses def.Parse; tests over drive fixture (`session.go`, `session_test.go`)
- [x] 1.4a `pkg/stream`: BuildRows/SensorTable ALT column; Renderer takes a (calibratable) Definition; def-level Parse added to ecm (`table.go`, `stream_test.go`)
- [x] 1.2 TUI codes view (set prominent, unset dim, not-used hidden unless set) (`statusviews.go` CodesBody, `tui.go`)
- [x] 1.3 TUI flags view (3 groups, checkboxes, SetLabel) (`statusviews.go` FlagsBody, `tui.go`)
- [x] 1.5 TUI raw history grid (labeled rows × last N frames, newest first, width-clamped to ≤14 cols, 64-frame ring) (`statusviews.go` RawHistory, `tui.go`)
- [x] 1.6 TUI heartbeat footer (● green/red + ok/bad counts) + ParseOK gating (decoded views render lastGood; raw view never gated) (`tui.go`)
- [x] 1.4b cmd flags `-tps0/-tps100` on TUI + monitor, degenerate-calibration guard with warning (`tui.go` calibratedDef, `monitor.go`)
- [x] V: full suite green incl. `-race` + vet + gofmt; decoder goldens untouched; blm expectations re-derived for the verified kPa transfer — cross-checked against WinALDL's own table (our 1600/40 avg 117.17 vs WinALDL's 117.5 from a different session); monitor exercised end-to-end over the idle fixture (ALT column renders)
