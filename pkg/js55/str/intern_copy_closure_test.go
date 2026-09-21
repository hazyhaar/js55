package str_test

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

func TestClosureCopiedStringCanonicalIdentity(t *testing.T) {
	a, b := str.NewTable(), str.NewTable()
	canonical := a.InternGo("x")
	copied := *canonical
	if got, ok := a.Lookup(&copied); !ok || got != canonical {
		t.Errorf("Lookup(copy) = %p, %v; canonical = %p", got, ok, canonical)
	}
	if got := a.Intern(&copied); got != canonical {
		t.Errorf("Intern(copy) = %p; canonical = %p", got, canonical)
	}
	foreign := b.Intern(&copied)
	if foreign == canonical || foreign == &copied || !foreign.Equal(canonical) {
		t.Fatal("copy crossed tables without local canonicalization")
	}
	foreignCopy := *foreign
	if a.Intern(&foreignCopy) != canonical || b.Intern(&copied) != foreign {
		t.Fatal("cross-table roundtrip lost canonical identity")
	}
	if n := testing.AllocsPerRun(1000, func() {
		if a.Intern(canonical) != canonical {
			panic("canonical hit")
		}
	}); n != 0 {
		t.Fatalf("canonical hits allocated %v", n)
	}
}
