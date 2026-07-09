<!-- The automated agent reviewer checks this description for completeness before a
     human reviews. Fill in each section — empty ones get flagged. -->

## Summary

<!-- What does this change do, and why? -->

## Related spec

<!-- Link the specs/<work-unit>/ directory this implements, or "N/A". -->

## Type of change

<!-- Commits on main MUST be Conventional Commits. Which type is this? -->

- [ ] `feat:` — new capability
- [ ] `fix:` — bug fix
- [ ] `feat!:` / `BREAKING CHANGE:` — breaking change
- [ ] `docs:` / `refactor:` / `test:` / `chore:` / `ci:` / `build:` — no release on its own

## How verified

<!-- How did you confirm this works? At minimum the CI gate:
     go fmt ./... && go vet ./... && go build ./... && go test -race ./...
     Note any hardware/replay testing. Golden fixtures are only regenerated for a
     deliberate decoder change (`-update`) — say so if you did, and why. -->

## Checklist

- [ ] `gofmt`-clean, `go vet`, `go build`, and `go test -race ./...` all pass
- [ ] Conventional Commit title
- [ ] For consumer-only work: no changes to `pkg/decoder`, `pkg/ecm`, `pkg/stream/session.go`, or `go.mod`
- [ ] Golden fixtures (`pkg/decoder/testdata/*.golden`) are byte-identical, unless this is a deliberate decoder change (explained above)
- [ ] No plausibility filtering / smoothing added to the decode path (raw-data policy)
