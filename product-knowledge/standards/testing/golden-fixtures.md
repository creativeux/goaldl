<!--
GLaDOS-MANAGED STANDARD
Last Updated: 2026-07-04
-->
# Root the test suite in real captures

**Rule**: Decoder correctness MUST be proven against the committed real-hardware fixtures (`pkg/decoder/testdata/idle_4800.raw`, `drive_4800.raw`) and their `.golden` frame dumps — exact-stats regression plus golden comparison. Synthetic round-trip tests (via `pkg/decoder/encode.go`) complement but never replace them.

```bash
go test ./pkg/decoder                      # golden + stats regression
go test ./pkg/decoder -run TestGolden -update   # regenerate goldens after an INTENDED change
```

**Why**: The ground truth for this project is the real vehicle (WinALDL log `data/20250601_111156_LOG.txt`); synthetic data encodes the same assumptions as the decoder and can't catch a shared misconception — that failure mode cost months (see CLAUDE.md history). Any golden regeneration must be an intentional decoder change, and the diff reviewed before committing.

**Convention**: tests live beside their package (`*_test.go`); stream/TUI logic is unit-tested against the drive fixture (`session_test.go`, `tui_test.go`) rather than mocks where practical.
