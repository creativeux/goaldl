<!-- SDA: v1.0 -->
# Evaluation: TUI UX Pass — Phase D (Replay & startup ergonomics)

Branch under test: `feat/tui-phase-d` (working tree, uncommitted). Evaluated against `main`.

## Verification gate (Section D)

| Check | Result |
|-------|--------|
| `gofmt -l pkg cmd` | ✅ no output |
| `go vet ./...` | ✅ clean |
| `go build ./...` | ✅ clean |
| `go test -race -count=1 ./...` | ✅ all packages ok |
| `go test ./pkg/decoder -run TestGolden` (no -update) | ✅ ok — goldens byte-identical |
| `go run ./cmd/goaldl blm .../drive_4800.raw` | ✅ "Recorded 469 into BLM cells" |
| `git diff --stat main -- session.go stream.go pkg/ecm pkg/decoder pkg/blm go.mod go.sum` | ✅ empty |
| `git diff --name-only main -- pkg/stream/` | ✅ only replay.go, serial.go, stream_test.go |
| Snapshot struct field additions | ✅ none (stream.go untouched; Elapsed is pre-existing, promoted from embedded FrameEvent) |
| Named tests (TestReplaySeek/Duration, TestTUISeekKeys, TestWaitingDiagnostics, TestSerialBytes, TestTUIReplayKeys, TestPortPicker) | ✅ all pass under -race |

The hard "no change below the facade except provider methods" constraint holds: only `ReplayProvider.Duration/Seek/PendingSeek` and `SerialProvider.Bytes()` were added; `Session`/`Snapshot`/`ecm`/`decoder`/`blm`/`go.mod`/`go.sum` are byte-identical to `main`. No new dependency — the TTY check uses stdlib `os.Stdin.Stat()`; `x/term` remains only an indirect bubbletea dep and is not imported.

## Acceptance Criteria

| # | Criterion | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | `Duration()` = last frame's Elapsed, 0 for empty, callable before Run, agrees with a full Run | ✅ PASS | `replay.go:109` reads cached `total` set by the shared `ensureDecoded` (`decodeOnce`); `frameElapsed(last)`. `TestReplayDuration` asserts it equals a full Run's last Elapsed and empty→0. Decode is shared with Run via `sync.Once`, so the two can never disagree. |
| 2 | `Seek` clamps to [0,Duration], no-op when Speed==0, applies at frame boundary; forward jumps, backward re-emits, paused shows+holds | ✅ PASS | `replay.go:120` clamps and guards `p.Speed<=0`; `Run` applies `takeSeek` at the top of the inner loop, `seekIndex` (binary search) repositions, re-anchors. `TestReplaySeek` covers forward/backward/clamp/paused/unpaced sub-cases. |
| 3 | TUI `,`/`.`/`0` keys routed through `replayGuard`; warn (no seek) on live or unpaced | ✅ PASS | `tui.go` key cases call `seekBy`/`replayGuard`; both check `m.replay==nil` and `CurrentSpeed()==0`. My throwaway test confirmed a live `seekBy` returns a non-nil warn cmd and does not panic. |
| 4 | Position readout `m:ss / m:ss (N%)`, no `t=`, leads header status left of `Signal:` | ✅ PASS | `replayPosition()` renders `"2:05 / 14:03 (14%)"` (verified). `View()` prepends it before `Signal:`; measured posIdx 47 < signalIdx 68. No `t=` present. |
| 5 | Backward seek does NOT rewind grids/extrema/frame ring | ✅ PASS | `seekBy` only calls `m.replay.Seek`; no grid mutation. Comment documents the whole-session-aggregate intent; `[c]` remains the reset. |
| 6 | `+`/`-` step a fixed ladder (0.25·0.5·1·2·4·8·16), off-ladder start can reach 1× | ✅ PASS | `speedSteps` ladder; `adjustSpeed(up bool)` steps to nearest stop in direction. Verified: from `-speed 5`, down → 4,2,1 (reaches 1×); up clamps at 16. |
| 7 | `SerialProvider.Bytes()` atomic, safe concurrent read, increments after each non-zero Read | ✅ PASS | `serial.go:22,30,57` — `atomic.Int64`, `Add(n)` after the `n==0` continue. `TestSerialBytes` + `-race` clean. |
| 8 | Waiting screen: 0 bytes→cable/port/driver; >0→baud/polarity with B/s; replay bare; gone after first frame | ✅ PASS | `waitingBody()`: nil serial→bare; `bytesSeen==0`→"check cable / port / driver"; else "159 B/s … check baud (-b) / polarity (-invert)". Only rendered while `!m.hasFrame`. Verified all three branches. |
| 9 | `launchTUI`: 1 port auto-connects; 0/2+ on TTY opens picker, choose connects, decline exits 0; non-interactive falls to `errNoTUISource` (now lists ports); explicit `-p`/file never intercepted | ✅ PASS | `main.go:56` — len==1 auto-connect; else `stdinIsInteractive()` gate → `runPortPicker`; `""`→`os.Exit(0)`. Non-TTY falls through; `errNoTUISource` handler lists detected ports. Picker only entered when `len(args)==0`, so `-p`/file bypasses it. Piped non-TTY run did not hang. |
| 10 | `portPicker`: 1s re-poll, auto-connect on drop-to-1, clamped ↑/↓, enter returns highlight, q/ctrl+c decline (""), 0-ports retry+driver hint, scan error surfaced+continues, stdlib TTY check | ✅ PASS | `portpicker.go` implements all. `TestPortPicker` covers clamp both ends, Enter select, drop-to-one auto-connect, 0-ports hint, scan-error surfaced without ending, q decline. `stdinIsInteractive` uses `os.Stdin.Stat()` char-device. |
| 11 | Regression: goldens identical, Session/Snapshot unchanged, blm=469, suite green under -race | ✅ PASS | See verification gate — all hold. |

**Non-goals confirmed absent:** no grid rewind on backward seek, no scrub/mouse, no ±60s tier (only `seekStep=10s`), no port/config persistence, no live reconnection, no Snapshot/Session/ecm/decoder/blm/go.mod change.

## Standards Compliance

| Standard | Verdict | Notes |
|----------|---------|-------|
| architecture/session-api-layering | ✅ | Only below-facade provider methods added; `Session`/`Snapshot` byte-identical. TUI still consumes the `Snapshot` stream; `byteSource`/`ReplayProvider` handles are the TUI reaching to a provider it owns, not a Session API change. |
| decoder/raw-data-policy | ✅ | `Bytes()` is a diagnostic counter, not a filter; seek re-emits frames unmodified; decode path untouched (goldens identical). |
| go/tooling | ✅ | gofmt+vet+build+`test -race` all green; no new dependency (stdlib TTY check). |
| testing/golden-fixtures | ✅ | `TestGolden` passes with no `-update`; blm still 469 over the drive fixture. |

## Persona Reviews

**Architect.** Clean layering. The decode is done exactly once via `sync.Once` and shared between `Duration()` and `Run()`, which is both correct and the reason the two can never disagree — a nice invariant. The seek re-anchor (set `anchorData`/`anchorWall` to the target frame, then pace the next gap from there) is the right design and avoids rush/stall after a jump. The typed-nil-in-interface trap is correctly avoided: `serial` is a concrete `*SerialProvider` local, and `m.serial` (a `byteSource` interface) is assigned only inside `if serial != nil`, so a replay source leaves it a true nil interface — confirmed by test. Provider methods stay below the facade; no `Session`/`Snapshot` leakage. One minor altitude note: `Duration()` is documented "O(1)" but its first call is an O(n) decode (amortized/cached, and unavoidable) — the wording is slightly generous but the behavior is right.

**Product Manager.** All three findings (F6 position/seek, F7 multi-port dead-end, F8 waiting dead-zone) are addressed with exactly the agreed scope and nothing more. The waiting-screen diagnostic is genuinely the differentiated value: it turns the unanswerable "cable or baud?" question into a concrete B/s reading against the known-good ~159 B/s idle. The port picker converging to the bare-`goaldl` happy path when the extra device disappears is a thoughtful touch. Scope discipline is excellent — every listed non-goal is absent.

**QA.** Coverage is thorough and adversarial: `TestReplaySeek` exercises forward/backward/clamp-past-end/paused/unpaced; `TestPortPicker` hits cursor clamps at both ends, drop-to-one auto-connect, 0-ports hint, scan-error-continues, and clean decline; waiting diagnostics test all three branches; `-race` is clean across the suite. Edge cases I probed independently all held (off-ladder speed start reaching 1×, live-seek warning without panic, nil-interface guard, non-TTY no-hang). The one soft spot is that `byteRate` is a raw per-tick delta labeled "B/s" that depends on the tick staying at 1s — accurate today, silently mislabeled if the heartbeat interval ever changes (note-level).

## Issues Found

1. **`byteRate` label couples to the 1s tick interval.** `tui.go` tickMsg handler sets `byteRate = b - bytesSeen` and `waitingBody` prints it as `%d B/s`. This is only true because the tick is `time.Second`. If the heartbeat cadence changes, the "B/s" label silently desyncs. Cosmetic diagnostic only. Severity: **note**.
2. **`Duration()` documented "O(1)" but first call is O(n).** `replay.go:109` — the first call decodes the whole capture (necessary, cached, and shared with `Run` so there's no double-decode). The doc comment does say "O(1) after the first call," so this is a wording nuance, not a defect. Severity: **note**.

No blocking or warning-level issues found.

## Overall Verdict

`PASS — all acceptance criteria met, no blocking issues.`
