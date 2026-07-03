package decoder

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update regenerates golden files instead of comparing against them:
//
//	go test ./pkg/decoder -run TestGolden -update
//
// Do this deliberately after an intended decoder change, then eyeball the
// golden diff before committing.
var update = flag.Bool("update", false, "regenerate golden files")

// realCaptures are byte-for-byte raw UART recordings from the actual GM
// 1227747 via PL2303 at 4800 baud (see project history). They are the root
// of this suite: any change to the decoder is measured against real data,
// not just synthetic round trips.
//
// Expected stats are exact because offline decoding of a fixed byte stream is
// fully deterministic — a single differing number means the decoder's
// behavior changed and the diff must be understood before it's accepted.
var realCaptures = []struct {
	name          string
	file          string
	promID        int
	wantSyncs     int64
	wantFrames    int64
	wantAborted   int64
	wantNoisy     int64
	wantPromMatch int
}{
	{
		name: "idle", file: "idle_4800.raw", promID: 6291,
		wantSyncs: 50, wantFrames: 47, wantAborted: 2, wantNoisy: 0, wantPromMatch: 47,
	},
	{
		name: "drive", file: "drive_4800.raw", promID: 6291,
		wantSyncs: 694, wantFrames: 635, wantAborted: 58, wantNoisy: 2, wantPromMatch: 635,
	},
}

func loadCapture(t *testing.T, file string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", file, err)
	}
	return raw
}

// TestDecodeRealCapture is the core regression guard: decode each real capture
// and assert its stats and 100% PROM-ID match exactly.
func TestDecodeRealCapture(t *testing.T) {
	cfg := DefaultConfig()
	for _, c := range realCaptures {
		t.Run(c.name, func(t *testing.T) {
			d := New(cfg)
			frames := d.Decode(loadCapture(t, c.file))
			s := d.Stats

			if s.SyncsFound != c.wantSyncs {
				t.Errorf("SyncsFound = %d, want %d", s.SyncsFound, c.wantSyncs)
			}
			if s.FramesEmitted != c.wantFrames {
				t.Errorf("FramesEmitted = %d, want %d", s.FramesEmitted, c.wantFrames)
			}
			if s.FramesAborted != c.wantAborted {
				t.Errorf("FramesAborted = %d, want %d", s.FramesAborted, c.wantAborted)
			}
			if s.NoisyBytes != c.wantNoisy {
				t.Errorf("NoisyBytes = %d, want %d", s.NoisyBytes, c.wantNoisy)
			}
			if int64(len(frames)) != c.wantFrames {
				t.Fatalf("len(frames) = %d, want %d", len(frames), c.wantFrames)
			}

			promMatch := 0
			for _, f := range frames {
				if len(f.Data) != cfg.FrameSize {
					t.Fatalf("frame at offset %d has %d bytes, want %d", f.ByteOffset, len(f.Data), cfg.FrameSize)
				}
				if int(f.Data[1])<<8|int(f.Data[2]) == c.promID {
					promMatch++
				}
			}
			if promMatch != c.wantPromMatch {
				t.Errorf("PROM ID %d matched %d/%d frames, want %d", c.promID, promMatch, len(frames), c.wantPromMatch)
			}
		})
	}
}

// TestGolden pins the exact decoded frame output for each real capture. Unlike
// the stats test, this catches a decoder change that keeps the counts the same
// but alters byte values. Regenerate intentionally with -update.
func TestGolden(t *testing.T) {
	cfg := DefaultConfig()
	for _, c := range realCaptures {
		t.Run(c.name, func(t *testing.T) {
			frames := New(cfg).Decode(loadCapture(t, c.file))
			got := renderFrames(frames)
			goldenPath := filepath.Join("testdata", c.name+".golden")

			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
					t.Fatalf("writing golden: %v", err)
				}
				t.Logf("updated %s (%d frames)", goldenPath, len(frames))
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("reading golden (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("decoded output differs from %s.golden — run -update if intended", c.name)
			}
		})
	}
}

// renderFrames serializes frames as "offset: hexbytes", one per line, for
// deterministic golden comparison.
func renderFrames(frames []Frame) string {
	var b strings.Builder
	for _, f := range frames {
		fmt.Fprintf(&b, "%d:", f.ByteOffset)
		for _, x := range f.Data {
			fmt.Fprintf(&b, " %02X", x)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
