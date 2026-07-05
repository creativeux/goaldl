package main

import "testing"

// mkBuf pushes n frames whose byteOffset encodes push order, so tests can assert
// both count and oldest→newest ordering after a wrap.
func mkBuf(n int) *frameBuf {
	b := newFrameBuf()
	for i := 0; i < n; i++ {
		b.push(bufFrame{byteOffset: int64(i), parseOK: true})
	}
	return b
}

func TestFrameBufEmpty(t *testing.T) {
	b := newFrameBuf()
	if len(b.frames()) != 0 || b.fillPct() != 0 {
		t.Fatalf("empty: frames=%d pct=%d, want 0/0", len(b.frames()), b.fillPct())
	}
}

func TestFrameBufPartial(t *testing.T) {
	b := mkBuf(100)
	f := b.frames()
	if len(f) != 100 {
		t.Fatalf("frames=%d, want 100", len(f))
	}
	if f[0].byteOffset != 0 || f[99].byteOffset != 99 {
		t.Errorf("order: first=%d last=%d, want 0/99", f[0].byteOffset, f[99].byteOffset)
	}
	if want := 100 * 100 / frameBufCap; b.fillPct() != want {
		t.Errorf("pct=%d, want %d", b.fillPct(), want)
	}
}

func TestFrameBufWrap(t *testing.T) {
	b := mkBuf(frameBufCap + 50) // 50 past full
	f := b.frames()
	if len(f) != frameBufCap {
		t.Fatalf("frames=%d, want %d (capped)", len(f), frameBufCap)
	}
	if b.fillPct() != 100 {
		t.Errorf("pct=%d, want 100 after wrap", b.fillPct())
	}
	// Oldest 50 dropped: window is [50 .. cap+49], oldest-first.
	if f[0].byteOffset != 50 {
		t.Errorf("oldest=%d, want 50 (older dropped)", f[0].byteOffset)
	}
	if f[frameBufCap-1].byteOffset != int64(frameBufCap+49) {
		t.Errorf("newest=%d, want %d", f[frameBufCap-1].byteOffset, frameBufCap+49)
	}
	// Strictly increasing (no scramble across the wrap seam).
	for i := 1; i < len(f); i++ {
		if f[i].byteOffset != f[i-1].byteOffset+1 {
			t.Fatalf("out of order at %d: %d after %d", i, f[i].byteOffset, f[i-1].byteOffset)
		}
	}
}
