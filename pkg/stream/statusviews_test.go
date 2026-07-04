package stream

import (
	"strings"
	"testing"

	"goaldl/pkg/ecm"
)

func def1227747(t *testing.T) *ecm.Definition {
	t.Helper()
	def, ok := ecm.NewRegistry().GetDefinition("1227747")
	if !ok {
		t.Fatal("no 1227747 definition")
	}
	return def
}

// TestFlagsBody renders a closed-loop frame: set bits bold with their set
// label, clear bits dimmed, all three words present.
func TestFlagsBody(t *testing.T) {
	def := def1227747(t)
	f := make([]byte, 20)
	f[14] = 0x82 // closed loop + block learn enable

	out := FlagsBody(ecm.DecodeFlags(def, f))
	for _, want := range []string{"MW2", "MWAF1", "MCU2IO",
		"[x] Loop status: CLOSED", "[x] Block learn enable", "[ ] Idle flag"} {
		if !strings.Contains(out, want) {
			t.Errorf("flags body missing %q:\n%s", want, out)
		}
	}
	if FlagsBody(nil) != "  (no flag data)" {
		t.Error("nil flags should render the empty-state message")
	}
}

// TestCodesBody: set codes render prominent with a summary count; unused codes
// stay hidden unless set; a clean frame says so.
func TestCodesBody(t *testing.T) {
	def := def1227747(t)

	clean := CodesBody(ecm.DecodeCodes(def, make([]byte, 20)))
	if !strings.Contains(clean, "no codes set") {
		t.Errorf("clean frame should say no codes set:\n%s", clean)
	}
	if strings.Contains(clean, "41") {
		t.Error("unused code 41 should be hidden when clear")
	}
	if !strings.Contains(clean, "24 — VSS") {
		t.Error("defined codes should list dimmed when clear")
	}

	f := make([]byte, 20)
	f[13] = 1 << 6 // code 44 O2 lean
	set := CodesBody(ecm.DecodeCodes(def, f))
	if !strings.Contains(set, "1 CODE(S) SET") {
		t.Errorf("summary line missing:\n%s", set)
	}
	if !strings.Contains(set, "[X] 44 — O2 lean") {
		t.Errorf("set code not rendered prominently:\n%s", set)
	}

	// An unused code that is unexpectedly set must surface.
	f2 := make([]byte, 20)
	f2[12] = 1 << 1 // code 41, Unused
	if out := CodesBody(ecm.DecodeCodes(def, f2)); !strings.Contains(out, "[X] 41") {
		t.Errorf("unexpectedly-set unused code must be shown:\n%s", out)
	}
}

// TestRawHistory: newest-first columns under 0/-1/… headers, labeled rows,
// width clamping down to one column, and the empty state.
func TestRawHistory(t *testing.T) {
	def := def1227747(t)
	newest := make([]byte, 20)
	older := make([]byte, 20)
	newest[18], older[18] = 118, 120 // BLM row distinguishes the columns

	out := RawHistory(def.ByteLabels, [][]byte{newest, older}, 120)
	lines := strings.Split(out, "\n")
	if len(lines) != 21 { // header + 20 byte rows
		t.Fatalf("got %d lines, want 21:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "SAMPLE") || !strings.Contains(lines[0], "-1") {
		t.Errorf("header = %q, want SAMPLE with 0/-1 columns", lines[0])
	}
	var blmLine string
	for _, l := range lines {
		if strings.Contains(l, "BLM") && !strings.Contains(l, "O2_CNT") {
			blmLine = l
		}
	}
	// Newest (118) must precede older (120) on the BLM row.
	if i, j := strings.Index(blmLine, "118"), strings.Index(blmLine, "120"); i < 0 || j < 0 || i > j {
		t.Errorf("BLM row = %q, want 118 before 120 (newest first)", blmLine)
	}

	// A too-narrow terminal clamps to one column.
	narrow := RawHistory(def.ByteLabels, [][]byte{newest, older}, 10)
	if strings.Contains(strings.Split(narrow, "\n")[0], "-1") {
		t.Error("narrow render should clamp to a single column")
	}

	if RawHistory(def.ByteLabels, nil, 80) != "  (no frames yet)" {
		t.Error("empty history should render the empty-state message")
	}
}
