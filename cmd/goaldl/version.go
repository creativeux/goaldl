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

// resolveVersion returns the version, commit, and date, filling gaps from the
// embedded build info when the ldflags aren't set (a local `go build`). A tagged
// `go install module@vX.Y.Z` build carries the real version in build info; a
// dirty working tree gets a "+dirty" suffix.
func resolveVersion() (v, c, d string) {
	v, c, d = version, commit, date
	if info, ok := debug.ReadBuildInfo(); ok {
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
				if s.Value == "true" && !strings.HasSuffix(v, "+dirty") {
					v += "+dirty"
				}
			}
		}
	}
	return v, c, d
}

// shortCommit trims a commit hash to 7 chars.
func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// versionString renders the human-facing version line (`goaldl vX.Y.Z (abc1234,
// date)`), used by `goaldl version` and `--version`.
func versionString() string {
	v, c, d := resolveVersion()
	out := "goaldl " + v
	if c != "" {
		out += " (" + shortCommit(c)
		if d != "" {
			out += ", " + d
		}
		out += ")"
	}
	return out
}

// versionShort is a compact build identifier for the dashboard title bar: a
// clean release tag when built from one (e.g. "v0.1.0"), otherwise "dev" plus
// the short commit ("dev·a1b2c3d") when the VCS stamp is present. A local build's
// Go pseudo-version (long, hash-laden) collapses to the dev·commit form.
func versionShort() string {
	v, c, _ := resolveVersion()
	if v != "dev" && v != "dev+dirty" && !isPseudoVersion(v) {
		return v
	}
	if c != "" {
		return "dev·" + shortCommit(c)
	}
	return "dev"
}

// isPseudoVersion reports whether v is a Go module pseudo-version rather than a
// clean release tag, detected by the 14-digit UTC build timestamp it embeds
// (e.g. v0.1.1-0.20260706055303-abc123). A clean tag never has 14 digits in a row.
func isPseudoVersion(v string) bool {
	run := 0
	for _, r := range v {
		if r >= '0' && r <= '9' {
			if run++; run >= 14 {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

// cmdVersion prints the version line plus the Go toolchain and platform.
func cmdVersion() {
	fmt.Println(versionString())
	fmt.Printf("  %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
