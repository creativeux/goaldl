<!--
SDA: v1.0
GLaDOS-MANAGED DOCUMENT
Last Updated: 2026-07-04
To modify: Edit directly.
-->

# Roadmap

Mission: a working, cross-platform scanner/datalogger/tuning aid for GM 160-baud ALDL ECMs (primary target: GM 1227747), in Go, over an ordinary USB serial adapter. See [MISSION.md](MISSION.md).

## Phase 0: Decode & consolidate — ✅ done (2026-07-03)

- [x] Byte-value decoder (one UART byte per ALDL bit), unit- + hardware-validated (635/635 PROM match on a real drive)
- [x] record / decode / monitor / blm / simulate pipeline
- [x] Consolidation to the working core; test suite rooted in real captures + golden frame dumps
- [x] Core `stream.Session` → `Snapshot` API; TUI dashboard as the default face

## Phase 1: WinALDL parity — diagnose — ✅ shipped (2026-07-04)

**Goal**: at the car, read everything the ECM says without WinALDL.

- [x] Flags / error-codes / knock decoded as `pkg/ecm` data tables (A033.ads bit order, log-verified); `Snapshot` carries Flags/Codes
- [x] TUI: Flags, Codes, scrolling raw-history tabs; heartbeat + bad-sample gating
- [x] Sensor table dual-unit (TPS % via `-tps0`/`-tps100`, MAP kPa); `MapVoltsToKPa` corrected to the WinALDL-verified transfer

## Phase 2: WinALDL parity — tune — ✅ shipped + verified (2026-07-04)

**Goal**: a live tuning session needs no post-hoc `blm` run.

- [x] INT grid (closed-loop gated) and O2 grid (ungated) tabs — `blm.Grid` reuse
- [x] In-TUI Save (`s`, all three grids → timestamped files) / Clear (`c`, active grid or sensor Min/Max)
- [x] Always-on sensor MIN/MAX columns
- [x] Persistent loop-state line on every tab (Open/Closed + per-grid recording dots)
- [x] Consumer-side accumulation only — no `Snapshot`/`Session`/`blm`/`ecm` change; decode path untouched

## Phase 3: WinALDL parity — session UX — ✅ shipped + verified (2026-07-04)

- [x] In-TUI recording toggle (`r`) — `stream.RecordSink` behind the existing `SerialProvider.Sink` seam; fail-soft on write errors (detach + notice, session survives)
- [x] Replay pause / speed keys (space, +/-/=) — runtime `ReplayProvider` controls, re-anchored non-retroactive pacing, 0.25×–16×
- [x] Spark-counts grid (knock-delta accumulator, mod-256 wrap) on WinALDL's spark axes (RPM 400–3600/400 × MAP 30–100/5), tab 5 of 8
- [x] In-TUI CSV logging toggle (`d`, reuses `csv.go`; ParseOK rows — `monitor -csv` parity)
- [x] Filename prompt on all file actions (`s`/`r`/`d`, default `goaldl_<ts>`, exclusive-create, no silent overwrite) — fulfils the deferred editable-filename request; no-op key warnings self-expire after 3 s
- [x] `Snapshot`/`Session`/`ecm`/decoder untouched; `pkg/blm` gains only generic `Sum()`/`NewSpark()`; zero new dependencies

## Phase 4: Beyond parity — ⏳ opportunistic

- [ ] `serve` adapter (HTTP/WebSocket) proving the `Session` API drives a non-terminal front-end (web/mobile) — inherits flags/codes/grids for free from `Snapshot`
- [ ] Dash (big-number) view
- [ ] Config-file persistence (port / ECM / TPS calibration)
- [ ] Additional ECM definitions (data-only, demand-driven)
- [ ] Optional onboard MCU datalogging bridge (bot-thoughts falling-edge method)

**Permanent non-goals**: Narrow/Avg10/StdDev grid modes; Windows-dialog config UX; any plausibility filtering in the decode path (raw-data-raw).
