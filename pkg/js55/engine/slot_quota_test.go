// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func TestPropertySlotQuotaRestoredOnCollect(t *testing.T) {
	h := NewHeap()
	var used int64
	h.QuotaTracker = func(delta int64) error {
		next := used + delta
		if next < 0 {
			t.Fatalf("quota counter negative: used=%d delta=%d", used, delta)
		}
		used = next
		return nil
	}
	vm := NewVM(h)
	vm.GasLeft = 100_000_000
	prog, err := parser.Parse(`for(var i=0;i<8000;i++){var o={a:1,b:2,c:3}; o.a+o.b+o.c}`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := Compile(h, prog, "slots.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = vm.Run(ch); err != nil {
		t.Fatal(err)
	}
	before := used
	if before < 1000 {
		t.Fatalf("expected property charges, used=%d", before)
	}
	h.Collect()
	if used < 0 {
		t.Fatal("quota counter negative after collect")
	}
	if used > before/5 && used > 256<<10 {
		t.Fatalf("slot quota not restored: before=%d after=%d", before, used)
	}
	prog, err = parser.Parse(`var ok={x:9}; ok.x===9`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err = Compile(h, prog, "recover.js")
	if err != nil {
		t.Fatal(err)
	}
	got, err := vm.Run(ch)
	if err != nil || vm.ToStringValue(got).GoString() != "true" {
		t.Fatalf("recovery: %v %v", got, err)
	}
}
