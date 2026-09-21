// SPDX-License-Identifier: BUSL-1.1
package engine_test

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/parser"
)

// The quota hook observes the real JS call path, not a synthetic pool operation.
func TestSupervisionEnvironmentGrowthQuota(t *testing.T) {
	h := engine.NewHeap()
	vm := engine.NewVM(h)
	p, err := parser.Parse(`function small(a) { return a; }
	function large(a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p) { return a; }
	0;`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := engine.Compile(h, p, "quota-supervision")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = vm.Run(c); err != nil {
		t.Fatal(err)
	}
	small, _ := vm.GetGlobal("small")
	large, _ := vm.GetGlobal("large")
	if v, e := vm.CallFunction(small, engine.Undefined, []engine.Value{engine.Int(7)}); e != nil || v.ToInt() != 7 {
		t.Fatalf("warmup: %v %v", v, e)
	}
	var charged int64
	h.QuotaTracker = func(n int64) error { charged += n; return nil }
	v, err := vm.CallFunction(large, engine.Undefined, []engine.Value{engine.Int(9)})
	if err != nil || v.ToInt() != 9 {
		t.Fatalf("growth: %v %v", v, err)
	}
	firstCharge := charged
	// Verify nominal recovery before reporting the accounting violation.
	v, err = vm.CallFunction(small, engine.Undefined, []engine.Value{engine.Int(11)})
	if err != nil || v.ToInt() != 11 {
		t.Fatalf("recovery: %v %v", v, err)
	}
	if firstCharge < 15*8 {
		t.Fatalf("environment grew from 1 to 16 slots but charged only %d bytes, need at least 120", firstCharge)
	}
}
