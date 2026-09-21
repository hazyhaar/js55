// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func TestPromiseAllRaceSettled(t *testing.T) {
	if runGlobal(t, `var g=0; Promise.all([Promise.resolve(1),Promise.resolve(2)]).then(function(a){g=a[0]+a[1]});`).ToInt() != 3 {
		t.Fatal("all nominal")
	}
	if runGlobal(t, `var g=0; Promise.all([Promise.reject(9),Promise.resolve(1)]).catch(function(e){g=e});`).ToInt() != 9 {
		t.Fatal("all rejet hostile")
	}
	if runGlobal(t, `var g=0; Promise.race([Promise.resolve(4),Promise.resolve(8)]).then(function(v){g=v});`).ToInt() != 4 {
		t.Fatal("race")
	}
}

func TestPromiseAllSettledHostile(t *testing.T) {
	g := runGlobal(t, `var g=0; Promise.allSettled([Promise.reject(1),Promise.resolve(2)]).then(function(a){ if(a[0].status==="rejected" && a[1].status==="fulfilled") g=1; });`)
	if g.ToInt() != 1 {
		t.Fatalf("allSettled g=%v", g)
	}
}

func runGlobal(t *testing.T, src string) Value {
	t.Helper()
	prog, err := parser.Parse(src, parser.Options{})
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
	g, ok := vm.GetGlobal("g")
	if !ok {
		t.Fatal("g absente")
	}
	return g
}
