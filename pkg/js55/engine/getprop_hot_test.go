package engine

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
	"github.com/hazyhaar/js55/pkg/js55/str"
)

func TestGetPropInternedKeyZeroAlloc(t *testing.T) {
	h := NewHeap()
	vm := NewVM(h)
	key := h.Intern().InternGo("x")
	obj := ObjectValue(h.NewObject())
	h.SetProperty(obj.Handle(), key, Int(7))
	got := testing.AllocsPerRun(1000, func() {
		v := vm.getProp(obj, key)
		if v.ToInt() != 7 {
			panic(v)
		}
	})
	if got != 0 {
		t.Fatalf("getProp interned key allocated %v", got)
	}
}

func TestGetPropStringLengthZeroAlloc(t *testing.T) {
	h := NewHeap()
	vm := NewVM(h)
	sv := vm.NewStringValue(str.FromGo("hello"))
	length := h.Intern().InternGo("length")
	got := testing.AllocsPerRun(1000, func() {
		v := vm.getProp(sv, length)
		if v.ToInt() != 5 {
			panic(v)
		}
	})
	if got != 0 {
		t.Fatalf("getProp string length allocated %v", got)
	}
}

func TestGetPropMapSizeZeroAlloc(t *testing.T) {
	prog, err := parser.Parse(`var m = new Map(); m.set("a", 1);`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHeap()
	chunk, err := Compile(h, prog, "script")
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(h)
	if _, err := vm.Run(chunk); err != nil {
		t.Fatal(err)
	}
	m, ok := vm.GetGlobal("m")
	if !ok || !m.IsObject() {
		t.Fatal("globale m absente")
	}
	size := h.Intern().InternGo("size")
	got := testing.AllocsPerRun(1000, func() {
		v := vm.getProp(m, size)
		if v.ToInt() != 1 {
			panic(v)
		}
	})
	if got != 0 {
		t.Fatalf("getProp map size allocated %v", got)
	}
}
