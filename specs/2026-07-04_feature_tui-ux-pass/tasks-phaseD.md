<!-- SDA: v1.0 -->
# Tasks: TUI UX Pass — Phase D (Replay & startup ergonomics)

Implementation checklist for [spec-phaseD.md](spec-phaseD.md). Three independently-shippable slices. The only below-facade changes are provider methods on `ReplayProvider`/`SerialProvider` (`pkg/stream/replay.go`, `pkg/stream/serial.go`); everything else is `cmd/goaldl`. **No `Session`/`Snapshot`/`ecm`/`decoder`/`blm`/`go.mod`/`go.sum` change; decode goldens byte-identical; `blm` 469.**

## Slice 1 — D.1 replay position + seek (F6)

- [x] **D1** `pkg/stream/replay.go`: cache the one-time decode (`frames` + `total time.Duration`) so `Run` reuses it; add `Duration() time.Duration` (O(1), callable before `Run`). Empty capture → 0.
- [x] **D2** `pkg/stream/replay.go`: add `seekTo *time.Duration` under the existing mutex + `Seek(target time.Duration)` (clamp `[0,total]`; no-op when `Speed==0`); apply a pending seek at the frame-boundary in `Run` (binary-search the first frame with `Elapsed>=target`, reposition the loop index, re-anchor `anchorData`/`anchorWall`). Backward seek re-emits.
- [x] **D3** `pkg/stream/replay_test.go`: `TestReplaySeek` (forward → next Index≥target; backward `Seek(0)` re-emits from 0; clamp past end; seek-while-paused repositions but holds; `Speed==0` ignores seek) + `TestReplayDuration` (empty 0, single-frame, matches full-Run last Elapsed). Uses the injectable `now`/`sleep`.
- [x] **D4** `cmd/goaldl/tui.go`: `m.replayTotal` set from `Duration()` in `cmdTUI`; `seekBy(d)` (clamp `m.latest.Elapsed+d`); `,`/`.`/`0` keys through `replayGuard`; doc comment that grids/ring are NOT rewound on backward seek (`c` resets).
- [x] **D5** `cmd/goaldl/tui.go`: `replayNav` leads with `t=m:ss / m:ss (N%)` (percent guards `total==0`), then the pause/speed hints + `[,/.] ±10s · [0] restart`.
- [x] **D6** `cmd/goaldl/tui_test.go`: `TestTUISeekKeys` — `,`/`.`/`0` seek to the clamped target (spy replay), `replayNav` carries `t=`/total/`%` + seek hints; live model warns (`pause/speed are replay-only`) + never seeks; unpaced replay warns.

## Slice 2 — D.3 waiting-screen byte diagnostics (F8)

- [x] **D7** `pkg/stream/serial.go`: `nbytes atomic.Int64` + `Bytes() int64`; `p.nbytes.Add(int64(n))` after a non-zero `Read`.
- [x] **D8** `cmd/goaldl/tui.go`: typed `m.serial *stream.SerialProvider` handle (set in `cmdTUI`, nil on replay); `m.bytesSeen`/`m.prevBytes` sampled each `tickMsg` from `m.serial.Bytes()` (rate = delta over the 1 s tick).
- [x] **D9** `cmd/goaldl/tui.go`: extend the `!m.hasFrame` branch of `activeBody` (live only) — `bytesSeen==0` → `no bytes yet — check cable / port / driver`; `>0` → `NNN B/s, no sync — check baud (-b) / polarity (-invert)`. Replay (`m.serial==nil`) unchanged.
- [x] **D10** `cmd/goaldl/tui_test.go`: `TestWaitingDiagnostics` (0 bytes → cable hint; >0 bytes → sync hint + rate; replay → bare text; `hasFrame` → neither) + `TestSerialBytes` at the model level via the same stub (counter the waiting screen reads).

## Slice 3 — D.2 port discovery UX (F7)

- [x] **D11** `cmd/goaldl/tui.go` (or a new `portpicker.go`): a small `portPicker` Bubble Tea model (`ports`/`cursor`/`err`, injectable port-lister) — `View` (2+ list + `↑/↓`/`enter`/`q`; 0 = retry + PL2303 driver hint), `Update` re-polls on a 1 s tick, auto-advances when the count drops to exactly 1, `enter` returns the chosen port, `q`/`ctrl+c` return "".
- [x] **D12** `cmd/goaldl/main.go`: `launchTUI` — bare `goaldl` + port count ≠ 1 (and a TTY) → run the picker first (returns a port or ""); a chosen port re-enters `cmdTUI` with `-p`; `""`/non-TTY → improved `errNoTUISource` (now lists detected ports). Stdlib `os.Stdin.Stat()` char-device guard (no new dep).
- [x] **D13** `cmd/goaldl/tui.go`: `errNoTUISource` stderr text lists detected ports + how to pick.
- [x] **D14** `cmd/goaldl/tui_test.go` (or `portpicker_test.go`): `TestPortPicker` — 2 ports render + cursor move (clamped); tick re-poll dropping to 1 auto-advances/returns the port; 0 ports render retry + driver hint; `q` returns "" (exit-0 intent).

## Verify (per slice + final)
- [x] **D15** `go test -race -count=1 ./...` green; `go vet ./...` + `gofmt -l pkg cmd` clean; decoder goldens byte-identical (no `-update`); `blm` still **469** over `drive_4800.raw`; forbidden-seam diff (`pkg/stream/session.go`, `pkg/stream/stream.go`, `pkg/ecm`, `pkg/decoder`, `pkg/blm`, `go.mod`, `go.sum`) **empty**; `pkg/stream` changes confined to `replay.go`+`serial.go`; `Snapshot` gains no field (grep the diff).
