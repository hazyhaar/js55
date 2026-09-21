// SPDX-License-Identifier: BUSL-1.1
package str

import (
	"unicode/utf16"
	"unsafe"
)

// Internement des clés de propriété (plan §J2.4).
//
// Ce n'est pas une optimisation : c'est un prérequis du modèle à formes du
// jalon 3. Une forme (hidden class) compare ses clés par IDENTITÉ DE POINTEUR,
// jamais par contenu ; sans internement, cette comparaison est fausse. Le
// calcul d'empreinte à l'internement sert ensuite les caches en ligne du
// jalon 6.

const internMinSlots = 8

// Table interne les chaînes. Elle n'est pas sûre en concurrence : chaque isolat
// possède la sienne, ce qui est aussi la raison pour laquelle deux isolats ne
// partagent jamais une chaîne internée.
type Table struct {
	byHash  []*String
	count   int
	OnAlloc func(bytes int64)
}

// NewTable crée une table vide.
func NewTable() *Table {
	return &Table{}
}

// SetAllocTracker configure le rappel de facturation mémoire pour les nouvelles chaînes internées.
func (t *Table) SetAllocTracker(fn func(bytes int64)) {
	t.OnAlloc = fn
}

func sameUnits(a, b *String) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.length != b.length {
		return false
	}
	a.flatten()
	b.flatten()
	n := a.length
	for i := 0; i < n; i++ {
		if a.At(i) != b.At(i) {
			return false
		}
	}
	return true
}

func cloneString(s *String) *String {
	s.flatten()
	c := &String{kind: s.kind, length: s.length, hashState: hashComputedBit | uint64(s.Hash()), narrow: s.narrow}
	switch s.kind {
	case Latin1:
		c.latin1 = append([]byte{}, s.latin1...)
	case UTF16:
		c.utf16 = append([]uint16{}, s.utf16...)
	default:
		unsupported(s.kind, "cloneString")
	}
	return c
}

func (t *Table) lookup(s *String) *String {
	if s == nil || len(t.byHash) == 0 {
		return nil
	}
	h := s.Hash()
	mask := uint32(len(t.byHash) - 1)
	i := h & mask
	for {
		got := t.byHash[i]
		if got == nil {
			return nil
		}
		if sameUnits(got, s) {
			return got
		}
		i = (i + 1) & mask
	}
}

func (t *Table) place(s *String) {
	mask := uint32(len(t.byHash) - 1)
	i := s.Hash() & mask
	for t.byHash[i] != nil {
		i = (i + 1) & mask
	}
	t.byHash[i] = s
}

func (t *Table) grow() {
	old := t.byHash
	n := len(old) * 2
	if n == 0 {
		n = internMinSlots
	}
	t.byHash = make([]*String, n)
	for _, s := range old {
		if s != nil {
			t.place(s)
		}
	}
}

// Intern rend l'unique instance de contenu égal à s. Deux chaînes égales, quelle
// que soit leur FORME, rendent le même pointeur : c'est ce qui rend la
// comparaison par identité correcte. Un hit sur une instance déjà internée dans
// CETTE table ne matérialise aucune clé. Une table distincte clone : aucun
// alias de pointeur inter-isolats.
func (t *Table) Intern(s *String) *String {
	if s == nil {
		s = Empty
	}
	// String is publicly copyable. Only the table entry, not a marker copied
	// along with the struct, establishes canonical pointer identity.
	if got := t.lookup(s); got != nil {
		return got
	}
	t.reserve(s.Len())
	return t.insert(cloneString(s))
}

// reserve bills retained string storage and the complete new probe array before
// allocation or mutation. UTF-16 size is a conservative upper bound for Latin-1.
// Growth is charged in full (the old array may remain live until the Go GC).
func (t *Table) reserve(units int) {
	bytes := int64(unsafe.Sizeof(String{})) + int64(units)*2
	if (t.count+1)*2 > len(t.byHash) {
		n := len(t.byHash) * 2
		if n == 0 {
			n = internMinSlots
		}
		bytes += int64(n) * int64(unsafe.Sizeof((*String)(nil)))
	}
	if t.OnAlloc != nil {
		t.OnAlloc(bytes)
	}
}

func (t *Table) insert(ins *String) *String {
	if (t.count+1)*2 > len(t.byHash) {
		t.grow()
	}
	t.place(ins)
	t.count++
	return ins
}

// InternGo probes the same table using decoded UTF-16 units, without allocating
// a String or retaining Go aliases. Invalid UTF-8 follows Go's range decoding,
// exactly as FromGo does. Equal encodings therefore hit the same entry.
func (t *Table) InternGo(g string) *String {
	h, units := goHash(g)
	if len(t.byHash) != 0 {
		mask := uint32(len(t.byHash) - 1)
		for i := h & mask; ; i = (i + 1) & mask {
			got := t.byHash[i]
			if got == nil {
				break
			}
			if got.Len() == units && got.Hash() == h && equalGo(got, g) {
				return got
			}
		}
	}
	t.reserve(units)
	s := FromGo(g)
	if s == Empty {
		s = cloneString(s)
	}
	s.Hash()
	return t.insert(s)
}

func goHash(g string) (uint32, int) {
	h, n := uint32(2166136261), 0
	add := func(c uint16) {
		h = (h ^ uint32(c&0xff)) * 16777619
		h = (h ^ uint32(c>>8)) * 16777619
		n++
	}
	for _, r := range g {
		if r <= 0xffff {
			add(uint16(r))
		} else {
			a, b := utf16.EncodeRune(r)
			add(uint16(a))
			add(uint16(b))
		}
	}
	return h, n
}

func equalGo(s *String, g string) bool {
	i := 0
	for _, r := range g {
		if r <= 0xffff {
			if s.At(i) != uint16(r) {
				return false
			}
			i++
		} else {
			a, b := utf16.EncodeRune(r)
			if s.At(i) != uint16(a) || s.At(i+1) != uint16(b) {
				return false
			}
			i += 2
		}
	}
	return i == s.Len()
}

// Len rend le nombre de chaînes internées.
func (t *Table) Len() int { return t.count }

// Lookup rend l'instance internée de contenu égal à s, si elle existe.
func (t *Table) Lookup(s *String) (*String, bool) {
	got := t.lookup(s)
	return got, got != nil
}

// Find rend l'instance déjà internée, sans insertion ni allocation.
func (t *Table) Find(s *String) *String {
	if s == nil {
		return nil
	}
	return t.lookup(s)
}
