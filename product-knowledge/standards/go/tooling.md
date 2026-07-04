<!--
GLaDOS-MANAGED STANDARD
Last Updated: 2026-07-04
-->
# Standard Go tooling is the whole gate

**Rule**: Code MUST pass `gofmt` (zero diffs), `go vet ./...`, `go build ./...`, and `go test -race ./...` — the exact CI gate (`.github/workflows/ci.yml`). No additional linters, formatters, or build tooling are introduced without a documented decision.

```bash
go fmt ./... && go vet ./... && go build ./... && go test -race ./...
```

Conventions observed alongside:
- Package names are short, lower-case, domain-named (`decoder`, `ecm`, `blm`, `stream`).
- Doc comments carry the physical/protocol reasoning at point of use — long package comments (see `pkg/decoder/decoder.go`) are the norm for protocol-model code, not an exception.
- Dependencies stay minimal: serial I/O + Bubble Tea/lipgloss for the TUI; prefer the standard library otherwise.

**Why**: A small, validated codebase (~3.8k lines) after a deliberate consolidation; the toolchain gate is intentionally the boring Go default so contributions and CI stay reproducible with nothing but the Go toolchain installed.
