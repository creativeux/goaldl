# Observed Standards

*This file is populated automatically by the `pattern-observer` module during normal workflow execution.*
*Items here are reviewed and promoted (or discarded) during `{{CMD}}recombobulate`.*

---

<!-- Add observations below this line -->

- **Typed-nil interface guard** (observed 2026-07-18, tcp-provider implementation): the codebase
  guards against Go's typed-nil-in-interface trap at every concrete→interface assignment
  (`tui.go`'s `m.live` guarded assignments, and now `monitor.go`'s sink, where `var sink *os.File`
  assigned into a `Sink io.Writer` field was a real latent bug — a live `monitor -p` *without*
  `-o` handed the provider a non-nil interface wrapping a nil `*os.File`, so the first sink write
  would error the stream). Standard candidate: "declare the variable as the interface type at the
  assignment site (`var sink io.Writer`), never as the concrete pointer."
- **Test servers must be shut down before `wg.Wait()`** (observed 2026-07-18): an accept-loop
  goroutine + `defer ln.Close()` + explicit `wg.Wait()` deadlocks (Close runs after Wait).
  Pattern used in `tcp_test.go`: close the listener explicitly before waiting.
