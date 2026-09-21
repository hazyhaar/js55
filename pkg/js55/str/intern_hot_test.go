package str

import (
	"testing"
	"unicode/utf16"
)

func TestInternHotHitZeroAlloc(t *testing.T) {
	tab := NewTable()
	s := tab.InternGo("size")
	got := testing.AllocsPerRun(1000, func() {
		if tab.Intern(s) != s {
			panic("interned hit lost identity")
		}
	})
	if got != 0 {
		t.Fatalf("Intern interned hit allocated %v", got)
	}
}

func TestInternGoHotHitZeroAlloc(t *testing.T) {
	tab := NewTable()
	first := tab.InternGo("size")
	got := testing.AllocsPerRun(1000, func() {
		if tab.InternGo("size") != first {
			panic("InternGo hit lost identity")
		}
	})
	if got != 0 {
		t.Fatalf("InternGo known hit allocated %v", got)
	}
}

func TestInternGoLengthHotHitZeroAlloc(t *testing.T) {
	tab := NewTable()
	first := tab.InternGo("length")
	got := testing.AllocsPerRun(1000, func() {
		if tab.InternGo("length") != first {
			panic("InternGo length hit lost identity")
		}
	})
	if got != 0 {
		t.Fatalf("InternGo length hit allocated %v", got)
	}
}

func unitsOf(s *String) []uint16 {
	out := make([]uint16, s.Len())
	for i := 0; i < s.Len(); i++ {
		out[i] = s.At(i)
	}
	return out
}

func TestInternGoMatchesFromGoInvalidUTF8(t *testing.T) {
	invalid := string([]byte{0xff, 0xfe, 0x80})
	from := FromGo(invalid)
	tab := NewTable()
	got := tab.InternGo(invalid)
	if !got.Equal(from) {
		t.Fatalf("InternGo invalid UTF-8 units %v, FromGo %v", unitsOf(got), unitsOf(from))
	}
	if got.Len() != len(utf16.Encode([]rune(invalid))) {
		t.Fatalf("InternGo len %d, rune-decode len %d", got.Len(), len(utf16.Encode([]rune(invalid))))
	}
}
