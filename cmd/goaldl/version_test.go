package main

import "testing"

func TestIsPseudoVersion(t *testing.T) {
	pseudo := []string{
		"v0.1.1-0.20260706055303-ae5fc7186caa",
		"v0.0.0-20260706055303-abcdef123456",
	}
	for _, v := range pseudo {
		if !isPseudoVersion(v) {
			t.Errorf("%q should be detected as a pseudo-version", v)
		}
	}
	clean := []string{"v0.1.0", "v1.2.3", "v1.0.0-rc1", "dev", "v0.1.0+dirty", ""}
	for _, v := range clean {
		if isPseudoVersion(v) {
			t.Errorf("%q should NOT be a pseudo-version", v)
		}
	}
}

// TestVersionShort drives the header version formatter by overriding the build
// vars. The test binary carries no VCS stamp, so resolveVersion keeps whatever
// commit we set and leaves a "dev" version as "dev".
func TestVersionShort(t *testing.T) {
	ov, oc := version, commit
	defer func() { version, commit = ov, oc }()

	cases := []struct {
		version, commit, want string
	}{
		{"v1.2.3", "deadbeefcafe", "v1.2.3"},                                    // clean release tag
		{"dev", "deadbeefcafe", "dev·deadbee"},                                  // local build → dev·commit
		{"v0.1.1-0.20260706055303-ae5fc7186caa", "ae5fc7186caa", "dev·ae5fc71"}, // pseudo → dev·commit
		{"dev", "", "dev"}, // no VCS info at all
	}
	for _, c := range cases {
		version, commit = c.version, c.commit
		if got := versionShort(); got != c.want {
			t.Errorf("versionShort(version=%q commit=%q) = %q, want %q", c.version, c.commit, got, c.want)
		}
	}
}
