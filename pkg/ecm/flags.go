package ecm

import "sort"

// Flag and error-code definitions are ECM data, not code — like Parameter
// conversions, they mirror the BITS section of an A033.ads-style definition
// (btByteNumber/btBitNumber/strItemTitle). The decoders below are generic;
// adding a flag word or a trouble code never touches them.
//
// Bit numbering is plain LSB-first (bit 0 = 0x01 … bit 7 = 0x80), verified
// three ways for the 1227747: A033.ads btBitNumber, the WinALDL log's
// per-bit column order, and live rows from data/20250601_111156_LOG.txt
// (e.g. MWAF1=64 sets exactly the bit-6 Rich flag).

// FlagBit is one labeled bit of a status word. SetLabel optionally names the
// set state (e.g. Loop status → "CLOSED") for display.
type FlagBit struct {
	Bit      int
	Name     string
	SetLabel string
}

// FlagWord is one status byte of the frame with its defined bits (bits an ECM
// leaves undefined are simply absent).
type FlagWord struct {
	Name   string
	Offset int // byte position in the frame
	Bits   []FlagBit
}

// ErrorCode maps one malfunction-flag bit to its GM trouble code. Unused marks
// codes the ECM never sets in this application (hidden by consumers unless,
// unexpectedly, set).
type ErrorCode struct {
	Code        int
	Description string
	Offset      int // byte position in the frame
	Bit         int
	Unused      bool
}

// FlagBitStatus is one decoded flag bit — plain data, serializes cleanly.
type FlagBitStatus struct {
	Name     string
	Set      bool
	SetLabel string
}

// FlagWordStatus is one decoded status word: its raw byte plus every defined
// bit's state, in bit order.
type FlagWordStatus struct {
	Word string
	Raw  byte
	Bits []FlagBitStatus
}

// CodeStatus is one decoded trouble code. Word names the malfunction byte it
// came from (for grouped display).
type CodeStatus struct {
	Word        string
	Code        int
	Description string
	Set         bool
	Unused      bool
}

// DecodeFlags decodes every defined flag word from a frame. Returns nil when
// the definition is absent or the frame is shorter than the frame size —
// consumers treat nil as "no flag data yet".
func DecodeFlags(def *Definition, frame []byte) []FlagWordStatus {
	if def == nil || len(def.FlagWords) == 0 || len(frame) < def.FrameSize {
		return nil
	}
	out := make([]FlagWordStatus, 0, len(def.FlagWords))
	for _, w := range def.FlagWords {
		raw := frame[w.Offset]
		ws := FlagWordStatus{Word: w.Name, Raw: raw, Bits: make([]FlagBitStatus, 0, len(w.Bits))}
		for _, b := range w.Bits {
			ws.Bits = append(ws.Bits, FlagBitStatus{
				Name:     b.Name,
				Set:      raw>>b.Bit&1 == 1,
				SetLabel: b.SetLabel,
			})
		}
		out = append(out, ws)
	}
	return out
}

// codeWordName resolves the flag-word name for a code's byte offset so grouped
// display can label MALFFLG groups; falls back to the raw offset being unnamed.
func codeWordName(def *Definition, offset int) string {
	if def.ByteLabels != nil && offset < len(def.ByteLabels) {
		return def.ByteLabels[offset]
	}
	return ""
}

// DecodeCodes decodes every defined trouble code from a frame, sorted by code
// number. Returns nil when the definition is absent or the frame is shorter
// than the frame size.
func DecodeCodes(def *Definition, frame []byte) []CodeStatus {
	if def == nil || len(def.ErrorCodes) == 0 || len(frame) < def.FrameSize {
		return nil
	}
	out := make([]CodeStatus, 0, len(def.ErrorCodes))
	for _, c := range def.ErrorCodes {
		out = append(out, CodeStatus{
			Word:        codeWordName(def, c.Offset),
			Code:        c.Code,
			Description: c.Description,
			Set:         frame[c.Offset]>>c.Bit&1 == 1,
			Unused:      c.Unused,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}
