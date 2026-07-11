package xdf

import (
	"fmt"
	"strconv"
	"strings"
)

// Legacy text XDF (v1.x). The file is line-oriented: a header, then repeated
// %%TABLE%% / %%VALUE%% / %%FLAG%% blocks closed by %%END%%. Inside a block
// each line is
//
//	\t040005 KeyName          ="value"
//
// where the 6-digit numeric ID is the stable field identifier (the human
// name drifts across writers, the ID doesn't — so we key on the ID). Only
// %%TABLE%% blocks and the handful of axis/shape fields matter here; every
// other line is skipped unread. Ground truth for the shapes below:
// data/xdf/42.xdf (tunerpro.net's official $42 definition).
const (
	legacyKeyTitle   = "040005"
	legacyKeyRows    = "040300"
	legacyKeyCols    = "040305"
	legacyKeyXUnits  = "040320"
	legacyKeyYUnits  = "040325"
	legacyKeyXLabels = "040350"
	legacyKeyXEq     = "040354"
	legacyKeyYLabels = "040360"
	legacyKeyYEq     = "040364"
)

func parseLegacy(text string) (*File, error) {
	f := &File{Format: "legacy"}
	lines := strings.Split(text, "\n")

	inTable := false
	var cur Table
	var curLine int // line number where the current %%TABLE%% opened (1-based)

	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		switch strings.TrimSpace(line) {
		case "%%TABLE%%":
			inTable = true
			cur = Table{}
			curLine = i + 1
			continue
		case "%%END%%":
			if inTable {
				// Keep only real tables: a block with no labels on either
				// axis is a category separator (e.g. "     Fuel"), not data.
				if len(cur.X.Labels) > 0 || len(cur.Y.Labels) > 0 {
					f.Tables = append(f.Tables, cur)
				}
			}
			inTable = false
			continue
		}
		if !inTable {
			continue
		}

		id, value, ok := legacyField(line)
		if !ok {
			continue
		}
		var err error
		switch id {
		case legacyKeyTitle:
			cur.Title = value
		case legacyKeyRows:
			cur.Rows, err = legacyInt(value)
		case legacyKeyCols:
			cur.Cols, err = legacyInt(value)
		case legacyKeyXUnits:
			cur.X.Units = value
		case legacyKeyYUnits:
			cur.Y.Units = value
		case legacyKeyXLabels:
			legacyLabels(&cur.X, value, i+1)
		case legacyKeyYLabels:
			legacyLabels(&cur.Y, value, i+1)
		case legacyKeyXEq:
			cur.X.Eq = legacyEq(value)
		case legacyKeyYEq:
			cur.Y.Eq = legacyEq(value)
		}
		if err != nil {
			return nil, fmt.Errorf("line %d (table %%%%TABLE%%%% at line %d): %w", i+1, curLine, err)
		}
	}
	return f, nil
}

// legacyLabels fills one axis's label list. A bad label doesn't abort the
// file — the official 42.xdf writes "XLabels =(null)" on its 1D tables, and
// a defect in one table must not hide every other table from discovery. The
// literal "(null)" means "no labels"; anything else unparseable is recorded
// on the axis and surfaces only if that table is selected.
func legacyLabels(a *Axis, value string, lineNo int) {
	if strings.TrimSpace(value) == "(null)" {
		a.Labels = nil
		return
	}
	labels, err := legacyFloats(value)
	if err != nil {
		a.Labels, a.LabelErr = nil, fmt.Sprintf("line %d: %v", lineNo, err)
		return
	}
	a.Labels = labels
}

// legacyField splits `\t040005 Name  ="value"` into the numeric ID and the
// decoded value. Lines that don't look like fields report ok=false and are
// skipped (the format is full of fields we don't care about; being strict
// about lines we never read would make the parser fragile for no gain).
func legacyField(line string) (id, value string, ok bool) {
	s := strings.TrimSpace(line)
	if len(s) < 6 || !isDigits(s[:6]) {
		return "", "", false
	}
	id = s[:6]
	eq := strings.IndexByte(s, '=')
	if eq < 0 {
		return "", "", false
	}
	value = strings.TrimSpace(s[eq+1:])
	value = strings.TrimSuffix(strings.TrimPrefix(value, `"`), `"`)
	return id, value, true
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// legacyInt parses the format's integers, which are written in hex with an
// 0x prefix (Rows =0x8) but tolerated as plain decimal too.
func legacyInt(v string) (int, error) {
	n, err := strconv.ParseInt(v, 0, 32)
	if err != nil {
		return 0, fmt.Errorf("bad integer %q: %w", v, err)
	}
	return int(n), nil
}

// legacyFloats parses a comma-separated label list (XLabels =20,30,…,100).
func legacyFloats(v string) ([]float64, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, fmt.Errorf("bad axis label %q: %w", p, err)
		}
		out = append(out, f)
	}
	return out, nil
}

// legacyEq extracts the equation from the format's `X,TH|0|0|0|0|` encoding:
// the expression is everything before the first comma, the rest is editor
// state we don't interpret.
func legacyEq(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		return v[:i]
	}
	return v
}
