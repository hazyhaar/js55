// SPDX-License-Identifier: BUSL-1.1
package str

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

func TestIndependentUTF16ImmutabilityAndHash(t *testing.T) {
	s := FromUTF16([]uint16{0x4e2d, 0xd800})
	h := s.Hash()
	view := s.UTF16()
	view[0] = 0x1234
	tab := NewTable()
	got := tab.Intern(s)
	if s.At(0) != 0x4e2d || s.Hash() != h || got != tab.Intern(FromUTF16([]uint16{0x4e2d, 0xd800})) {
		t.Fatal("public UTF16 export mutated immutable data or poisoned intern hash")
	}
	a := FromUTF16([]uint16{0x4e2d}).Repeat(40)
	rope := a.Concat(FromGo("suffix"))
	exported := rope.UTF16()
	exported[0] = 0x42
	if rope.At(0) != 0x4e2d {
		t.Fatal("flatten export aliases rope")
	}
}

func TestIndependentASCIIContract(t *testing.T) {
	if FromLatin1([]byte{0xe9}).EqualASCII(string([]byte{0xe9})) {
		t.Fatal("EqualASCII accepted non-ASCII literal")
	}
	for _, text := range []string{"size", "length", "", "prototype"} {
		if !FromGo(text).EqualASCII(text) {
			t.Fatal(text)
		}
	}
}

func TestIndependentQuotaBeforeGrowthAndRecovery(t *testing.T) {
	tab := NewTable()
	for i := 0; i < 4; i++ {
		tab.InternGo(fmt.Sprint(i))
	}
	before := len(tab.byHash)
	rejected := false
	tab.SetAllocTracker(func(int64) { panic("quota") })
	func() {
		defer func() { rejected = recover() != nil }()
		tab.InternGo("new-key")
	}()
	if !rejected || len(tab.byHash) != before || tab.Len() != 4 {
		t.Fatalf("quota rejection mutated table: rejected=%v capacity=%d/%d count=%d", rejected, len(tab.byHash), before, tab.Len())
	}
	tab.SetAllocTracker(nil)
	s := tab.InternGo("new-key")
	if tab.InternGo("new-key") != s || tab.Len() != 5 {
		t.Fatal("recovery failed")
	}
	for i := 0; i < 4; i++ {
		if _, ok := tab.Lookup(FromGo(fmt.Sprint(i))); !ok {
			t.Fatal("growth lost key")
		}
	}
}

func TestIndependentGoAliasesDoNotBypassQuota(t *testing.T) {
	tab := NewTable()
	want := tab.InternGo("\ufffd")
	tab.SetAllocTracker(func(int64) { panic("quota") })
	for _, text := range []string{"\xff", "\xfe", "\x80"} {
		var got *String
		rejected := false
		func() {
			defer func() { rejected = recover() != nil }()
			allocs := testing.AllocsPerRun(100, func() { got = tab.InternGo(text) })
			if allocs != 0 {
				t.Errorf("equal decoded Go alias allocated %v", allocs)
			}
		}()
		if rejected || got != want {
			t.Fatal("canonical hit should not allocate or bill alias cache")
		}
	}
	tab.SetAllocTracker(nil)
	if tab.InternGo("next").GoString() != "next" {
		t.Fatal("resume failed")
	}
}

func TestIndependentUnicodeHitsCrossTablesAndGrowth(t *testing.T) {
	a, b := NewTable(), NewTable()
	texts := []string{"size", "é", "中", "😀", "\xff", strings.Repeat("中😀", 100)}
	for _, text := range texts {
		sa := a.InternGo(text)
		sb := b.Intern(sa) // B has never seen this value: must clone.
		if sa == sb || !sa.Equal(sb) {
			t.Fatal("cross-table alias or altered units")
		}
		if got := testing.AllocsPerRun(100, func() {
			if b.InternGo(text) != sb || a.Intern(sb) != sa {
				panic("identity")
			}
		}); got != 0 {
			t.Fatalf("Unicode/cross-table hit %q: %v allocations", text, got)
		}
	}
	for i := 0; i < 1000; i++ {
		text := fmt.Sprintf("collision-growth-%d", i)
		a.InternGo(text)
	}
	for i := 0; i < 1000; i++ {
		text := fmt.Sprintf("collision-growth-%d", i)
		got, ok := a.Lookup(FromGo(text))
		if !ok || got.GoString() != text {
			t.Fatal("growth/probe chain lost key", i)
		}
	}
	x := a.Intern(FromUTF16([]uint16{0xd800}))
	if b.Intern(x) == x {
		t.Fatal("surrogate pointer crossed tables")
	}
}

func TestIndependentRealHashCollision(t *testing.T) {
	x, y := FromGo("21268"), FromGo("7a9b2")
	if x.Hash() != y.Hash() || x.Equal(y) {
		t.Fatal("invalid FNV-1a collision vector")
	}
	a, b := NewTable(), NewTable()
	ax, ay := a.Intern(x), a.Intern(y)
	by, bx := b.Intern(ay), b.Intern(ax)
	if ax == ay || bx == by || ax == bx || ay == by {
		t.Fatal("collision or cross-table alias")
	}
	for i := 0; i < 100; i++ {
		a.InternGo(fmt.Sprint(i))
		b.InternGo(fmt.Sprint(i))
	}
	if a.InternGo("21268") != ax || a.Intern(y) != ay || b.Intern(x) != bx || b.InternGo("7a9b2") != by {
		t.Fatal("collision probe identity lost after growth")
	}
}

func TestIndependentRetainedStorageIsBilled(t *testing.T) {
	tab := NewTable()
	var billed int64
	tab.SetAllocTracker(func(n int64) { billed += n })
	for i := 0; i < 50; i++ {
		tab.InternGo(fmt.Sprintf("quota-%d", i))
	}
	lowerBound := int64(tab.Len())*int64(unsafe.Sizeof(String{})) + int64(len(tab.byHash))*int64(unsafe.Sizeof((*String)(nil)))
	if billed < lowerBound {
		t.Fatalf("retained storage underbilled: %d < %d", billed, lowerBound)
	}
	before := billed
	for i := 0; i < 50; i++ {
		tab.InternGo(fmt.Sprintf("quota-%d", i))
	}
	if billed != before {
		t.Fatal("hits charged as new storage")
	}
}
