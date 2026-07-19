<!-- SDA: v1.0 -->
# Tasks: TCPProvider implementation (per spec.md)

Implementation session 2026-07-18 — ESP32-S3 (Adafruit QT Py ESP32-S3) arrived and enumerates;
hold released. Stage 0 = this feature (no hardware in the loop); Stage 1 bench validation follows.

- [x] 1. `pkg/stream/tcp.go` — `TCPProvider` (types/signatures per spec §2): Run loop (§3.1),
      redial loop (§3.2, fixed Addr, no rescan), cancellation via `DialContext` + rolling read
      deadline + per-conn cancel-closer goroutine (§3.3), constants (§6).
- [x] 2. `pkg/stream/tcp_test.go` — test matrix T1–T8 (§7) against an in-process
      `127.0.0.1:0` listener + `replayTCPServer` helper (test-only, not a command).
- [x] 3. `cmd/goaldl/tui.go` — rename `byteSource`→`liveSource`, model field `serial`→`live`
      (six read sites); `-tcp` flag in `resolveTUIFlags` + source rules; `-tcp` construct branch
      with `RecordSink`; `errNoTUISource` help gains a `-tcp` line.
- [x] 4. `cmd/goaldl/monitor.go` — `-tcp` flag, source mutual-exclusion guard
      (`-p`/`-tcp`/file), construct branch with `-o` sink support, bridge title.
- [x] 5. `cmd/goaldl` consumer tests — flag resolution (`-tcp` alone / `-p`+`-tcp` error /
      `-tcp`+file error), `-tcp` branch builds a TCPProvider and sets `m.live`, waiting-screen
      byte-rate path driven through a fake `liveSource`.
- [x] 6. Docs — `CLAUDE.md`, `README.md` (`-tcp` source), `docs/mobile-ui.md` (Stage 0
      delivered cross-link).
- [x] 7. Gates — `go fmt` / `go vet` / `go build` / `go test -race ./...`; forbidden seam empty
      in diff (`pkg/stream/session.go`, `pkg/decoder/**`, `pkg/ecm/**`, `pkg/blm/**`, `go.mod`).
