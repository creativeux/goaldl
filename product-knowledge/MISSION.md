<!--
GLaDOS-MANAGED DOCUMENT
Last Updated: 2026-07-11
Status: Confirmed by user 2026-07-04; positioning amended by user direction 2026-07-11 (complement to TunerPro RT)
To modify: Edit this file directly. GLaDOS will read the current state before making future updates.
-->
# Product Mission

## Problem
Pre-OBD2 GM vehicles (mid-'80s–early-'90s, e.g. the GM 1227747 A033 TBI ECM) speak a 160-baud one-wire ALDL stream that modern diagnostic tools ignore. The existing options are aging Windows-only freeware (WinALDL), abandoned open-source projects (linuxaldl, rustaldl), or dedicated hardware — none of which give a modern, scriptable, cross-platform way to log, decode, and *tune* from that data.

## Audience
Owners and tuners of pre-OBD2 GM vehicles who want live diagnostics and fuel-trim (BLM) tuning data on modern machines — starting with the project author's own vehicle (macOS, PL2303 USB serial adapter).

## Solution
goaldl: a single Go binary that turns an ordinary USB serial adapter into a validated ALDL scanner and datalogger. Its unique core is the **byte-value decoder** — one UART byte per ALDL bit, immune to host-side timing jitter — validated 635/635 frames against a real drive. On top of that engine: an interactive TUI dashboard (default UX), scripting commands (record/decode/monitor/blm/simulate), and a BLM fuel-trim table builder for tuning — with a Session/Snapshot core API designed to drive future web/mobile front-ends from the same stream.

## Positioning

goaldl is the **car-side instrument that complements desk-side tuning tools** (TunerPro RT in particular), not a replacement for them. The division of labor: goaldl owns capture and analysis at the vehicle — onboard/headless logging, live dashboards on whatever device is at hand, raw-first lossless recording, and computed data prep (correction tables, session reports) — while the established Windows bin editors own calibration editing, emulation, and PROM work. Interop over duplication: goaldl speaks the community's definition and log formats (XDF-aware exports, ADX-derived definitions) so its output pastes straight into the tuner's existing workflow. Bin editing, emulation, and PROM burning are permanent non-goals.
