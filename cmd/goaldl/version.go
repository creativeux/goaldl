package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Build metadata. These are overwritten at release time via
//
//	-ldflags "-X main.version=... -X main.commit=... -X main.date=..."
//
// (GoReleaser sets all three). For a plain `go build` or `go install` they stay
// at their defaults and versionString falls back to the VCS data the Go
// toolchain stamps into the binary's build info.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// versionString renders the human-facing version line. When the ldflags aren't
// set (local `go build`), it recovers the commit and dirty state from the
// embedded build info so a from-source binary still self-identifies.
func versionString() string {
	v, c, d := version, commit, date
	if info, ok := debug.ReadBuildInfo(); ok {
		// A tagged `go install module@vX.Y.Z` build carries the real version
		// here; prefer it over the "dev" default but never over an explicit
		// ldflags value.
		if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if c == "" {
					c = s.Value
				}
			case "vcs.time":
				if d == "" {
					d = s.Value
				}
			case "vcs.modified":
				// Go may already suffix Main.Version with "+dirty"; only add it
				// ourselves (e.g. for an ldflags-set version) when it's absent.
				if s.Value == "true" && !strings.HasSuffix(v, "+dirty") {
					v += "+dirty"
				}
			}
		}
	}
	out := "goaldl " + v
	if c != "" {
		if len(c) > 7 {
			c = c[:7]
		}
		out += " (" + c
		if d != "" {
			out += ", " + d
		}
		out += ")"
	}
	return out
}

// cmdVersion prints the version line plus the Go toolchain and platform.
func cmdVersion() {
	fmt.Println(versionString())
	fmt.Printf("  %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
