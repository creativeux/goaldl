package main

import (
	"strings"
	"testing"

	"goaldl/pkg/stream"
)

// Both live providers must satisfy the dashboard's diagnostic surface without
// an adapter — this is the seam the m.live field reads through.
var (
	_ liveSource = (*stream.SerialProvider)(nil)
	_ liveSource = (*stream.TCPProvider)(nil)
)

// TestResolveTCPSource: -tcp alone selects a live bridge source (no capture
// file expected), and the address reaches the resolved config.
func TestResolveTCPSource(t *testing.T) {
	cfg, err := resolveTUIFlags([]string{"-tcp", "192.168.4.1:3333"})
	if err != nil {
		t.Fatalf("resolveTUIFlags: %v", err)
	}
	if cfg.tcpAddr != "192.168.4.1:3333" {
		t.Errorf("tcpAddr = %q, want the given address", cfg.tcpAddr)
	}
	if cfg.portName != "" || cfg.inName != "" {
		t.Errorf("port %q / file %q should be empty with -tcp", cfg.portName, cfg.inName)
	}
}

// TestResolveSourceExclusion: a source is exactly one of -p, -tcp, or a
// capture file — combinations are rejected with a clear message.
func TestResolveSourceExclusion(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"p and tcp", []string{"-p", "/dev/x", "-tcp", "h:1"}, "not both"},
		{"tcp and file", []string{"-tcp", "h:1", "drive.raw"}, "not both"},
		{"p and file", []string{"-p", "/dev/x", "drive.raw"}, "not both"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveTUIFlags(tc.args)
			if err == nil {
				t.Fatalf("resolveTUIFlags(%v) accepted conflicting sources", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}
