// SPDX-License-Identifier: BUSL-1.1
package str

import (
	"strings"
	"testing"
)

func TestInternCrossTableNoAlias(t *testing.T) {
	a := NewTable()
	b := NewTable()
	sa := a.InternGo("size")
	sb := b.InternGo("size")
	if sa == sb {
		t.Fatal("InternGo a produit le même pointeur sur deux tables")
	}
	if a.Intern(sa) != sa {
		t.Fatal("hit interné table A a perdu l'identité")
	}
	if b.Intern(sa) == sa {
		t.Fatal("la table B a renvoyé le pointeur interné de A")
	}
	if !b.Intern(sa).Equal(sa) {
		t.Fatal("clone inter-table de contenu distinct")
	}
}

func TestInternCollisionsAndSurrogates(t *testing.T) {
	tab := NewTable()
	ba := tab.Intern(FromUTF16([]uint16{0x42, 0x41}))
	wide := tab.Intern(FromUTF16([]uint16{0x4142}))
	if ba == wide {
		t.Fatal("« BA » et U+4142 internés ensemble")
	}
	loneHigh := tab.Intern(FromUTF16([]uint16{0xD83D}))
	loneLow := tab.Intern(FromUTF16([]uint16{0xDE00}))
	repl := tab.Intern(FromUTF16([]uint16{0xFFFD}))
	pair := tab.Intern(FromUTF16([]uint16{0xD83D, 0xDE00}))
	if loneHigh == repl || loneLow == repl || loneHigh == loneLow {
		t.Fatal("demi-codets isolés conflattés avec U+FFFD")
	}
	if pair == loneHigh || pair.Len() != 2 {
		t.Fatal("paire de demi-codets internée comme un isolé")
	}
}

func TestInternNonLatin1AndEqualEncodings(t *testing.T) {
	saved := ForceUTF16()
	defer SetForceUTF16(saved)
	tab := NewTable()
	SetForceUTF16(false)
	narrow := tab.Intern(FromLatin1([]byte{'A', 'B'}))
	SetForceUTF16(true)
	wide := tab.Intern(FromUTF16([]uint16{0x41, 0x42}))
	SetForceUTF16(false)
	if narrow != wide {
		t.Fatal("Latin-1 et UTF-16 de même contenu internés à des pointeurs distincts")
	}
	han := tab.Intern(FromUTF16([]uint16{0x4E2D}))
	if han.Len() != 1 || han.At(0) != 0x4E2D {
		t.Fatal("unité hors Latin-1 altérée à l'internement")
	}
	left := FromGo(strings.Repeat("a", 20))
	right := FromGo(strings.Repeat("a", 20))
	rope := left.Concat(right)
	if rope.Kind() != Rope {
		t.Fatalf("corde attendue, forme %v", rope.Kind())
	}
	flat := FromGo(strings.Repeat("a", 40))
	if tab.Intern(rope) != tab.Intern(flat) {
		t.Fatal("corde et forme plate de même contenu internées à des pointeurs distincts")
	}
}

func TestInternGoInvalidUTF8MatchesFromGo(t *testing.T) {
	tab := NewTable()
	invalid := string([]byte{0xff, 0x80, 0xfe})
	from := FromGo(invalid)
	got := tab.InternGo(invalid)
	if !got.Equal(from) {
		t.Fatalf("InternGo %v, FromGo %v", unitsOf(got), unitsOf(from))
	}
	validRepl := tab.InternGo("\uFFFD\uFFFD\uFFFD")
	if validRepl != got {
		t.Fatal("valid replacement text and invalid UTF-8 must canonicalize to identical pointers")
	}
}

func TestInternConstructorAndExportSliceMutation(t *testing.T) {
	in := []uint16{0x41, 0x42}
	s := FromUTF16(in)
	in[0] = 0x5A
	if s.At(0) != 0x41 {
		t.Fatal("FromUTF16 n'a pas recopié l'entrée")
	}
	tab := NewTable()
	interned := tab.Intern(s)
	exported := interned.UTF16()
	if len(exported) < 1 {
		t.Fatal("UTF16 interné vide")
	}
	exported[0] = 0x5A
	if interned.At(0) != 0x41 {
		t.Fatal("mutation de la tranche UTF16 exportée a altéré la chaîne internée")
	}
	raw := []byte{'A', 'B'}
	lat := FromLatin1(raw)
	raw[0] = 'Z'
	internedLat := tab.Intern(lat)
	if internedLat.At(0) != 'A' {
		t.Fatal("FromLatin1 n'a pas recopié l'entrée avant internement")
	}
	view := internedLat.UTF16()
	view[0] = 0x5A
	if internedLat.At(0) != 'A' {
		t.Fatal("mutation UTF16 d'une internée Latin-1 a altéré le contenu")
	}
}

func TestInternQuotaRejectThenRecover(t *testing.T) {
	tab := NewTable()
	allow := false
	billed := int64(0)
	tab.SetAllocTracker(func(n int64) {
		if !allow {
			panic("quota")
		}
		billed += n
	})
	rejected := false
	func() {
		defer func() {
			if recover() != nil {
				rejected = true
			}
		}()
		_ = tab.InternGo("quota-key")
	}()
	if !rejected {
		t.Fatal("rejet de quota attendu")
	}
	if tab.Len() != 0 {
		t.Fatalf("table mutée après rejet, len=%d", tab.Len())
	}
	allow = true
	got := tab.InternGo("quota-key")
	if got == nil || !got.Equal(FromGo("quota-key")) {
		t.Fatal("reprise d'internement après quota échouée")
	}
	if tab.Len() != 1 {
		t.Fatalf("len=%d après reprise, 1 attendue", tab.Len())
	}
	if billed <= 0 {
		t.Fatal("aucune facturation SetAllocTracker après reprise")
	}
	if tab.InternGo("quota-key") != got {
		t.Fatal("hit InternGo après reprise a perdu l'identité")
	}
}
