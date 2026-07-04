package stream

import (
	"fmt"
	"strings"

	"goaldl/pkg/ecm"
)

// Pure content builders for the flag-data, error-code, and raw-history views —
// terminal strings with no positioning codes, shared by the TUI (and any other
// terminal presenter), in the same spirit as SensorTable/BLMBody. Set/unset
// emphasis uses inline bold/dim ANSI like BLMBody does.

const (
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"
	ansiReset = "\033[0m"
)

// FlagsBody renders decoded status words as grouped checklists: every defined
// bit in bit order, set bits bold with their set-state label (e.g. "Loop
// status: CLOSED"), clear bits dimmed.
func FlagsBody(flags []ecm.FlagWordStatus) string {
	if len(flags) == 0 {
		return "  (no flag data)"
	}
	var b strings.Builder
	for i, w := range flags {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "  %s%s%s  (0x%02X)\n", ansiBold, w.Word, ansiReset, w.Raw)
		for _, bit := range w.Bits {
			name := bit.Name
			if bit.Set && bit.SetLabel != "" {
				name += ": " + bit.SetLabel
			}
			if bit.Set {
				fmt.Fprintf(&b, "  %s[x] %s%s\n", ansiBold, name, ansiReset)
			} else {
				fmt.Fprintf(&b, "  %s[ ] %s%s\n", ansiDim, name, ansiReset)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// CodesBody renders decoded trouble codes grouped by malfunction byte: a
// summary line first, then each group with set codes bold at the top and clear
// codes dimmed below. Codes unused on this ECM are hidden unless (unexpectedly)
// set.
func CodesBody(codes []ecm.CodeStatus) string {
	if len(codes) == 0 {
		return "  (no error-code data)"
	}
	nSet := 0
	for _, c := range codes {
		if c.Set {
			nSet++
		}
	}
	var b strings.Builder
	if nSet == 0 {
		fmt.Fprintf(&b, "  %sno codes set%s\n", ansiDim, ansiReset)
	} else {
		fmt.Fprintf(&b, "  %s%d CODE(S) SET%s\n", ansiBold, nSet, ansiReset)
	}

	// Group by word, preserving first-encounter order (codes are code-sorted).
	var words []string
	byWord := map[string][]ecm.CodeStatus{}
	for _, c := range codes {
		if _, ok := byWord[c.Word]; !ok {
			words = append(words, c.Word)
		}
		byWord[c.Word] = append(byWord[c.Word], c)
	}
	for _, w := range words {
		fmt.Fprintf(&b, "\n  %s\n", w)
		// Set codes first, then clear ones; unused hidden unless set.
		for _, c := range byWord[w] {
			if c.Set {
				fmt.Fprintf(&b, "  %s[X] %d — %s%s\n", ansiBold, c.Code, c.Description, ansiReset)
			}
		}
		for _, c := range byWord[w] {
			if !c.Set && !c.Unused {
				fmt.Fprintf(&b, "  %s[ ] %d — %s%s\n", ansiDim, c.Code, c.Description, ansiReset)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// maxHistoryCols caps the raw-history grid at WinALDL's ~14 visible samples.
const maxHistoryCols = 14

// RawHistory renders the scrolling raw-byte grid: one labeled row per frame
// byte, one column per past frame (newest first, header 0 -1 -2 …), decimal
// values — the WinALDL RAW Data view. history is newest-first; width is the
// terminal width used to clamp the column count (0 = no clamp beyond the
// 14-sample cap).
func RawHistory(labels []string, history [][]byte, width int) string {
	if len(history) == 0 {
		return "  (no frames yet)"
	}
	labelW := len("SAMPLE")
	for _, l := range labels {
		labelW = max(labelW, len(l))
	}
	const cellW = 5 // " %4d"
	cols := min(len(history), maxHistoryCols)
	if width > 0 {
		cols = min(cols, max(1, (width-labelW-2)/cellW))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "  %-*s", labelW, "SAMPLE")
	for i := 0; i < cols; i++ {
		fmt.Fprintf(&b, " %4d", -i)
	}
	b.WriteByte('\n')
	nBytes := len(labels)
	if nBytes == 0 && len(history[0]) > 0 {
		nBytes = len(history[0])
	}
	for i := 0; i < nBytes; i++ {
		label := fmt.Sprintf("byte %d", i)
		if i < len(labels) {
			label = labels[i]
		}
		fmt.Fprintf(&b, "  %-*s", labelW, label)
		for c := 0; c < cols; c++ {
			if i < len(history[c]) {
				fmt.Fprintf(&b, " %4d", history[c][i])
			} else {
				fmt.Fprintf(&b, " %4s", "·")
			}
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
