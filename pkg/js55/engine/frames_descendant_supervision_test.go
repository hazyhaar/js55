// SPDX-License-Identifier: BUSL-1.1
package engine_test

import (
	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/parser"
	"testing"
)

func TestSupervisionDescendantFallback(t *testing.T) {
	p, err := parser.Parse(`function outer() { return function child() { return 7; }; }`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := engine.Compile(engine.NewHeap(), p, "descendant-supervision")
	if err != nil {
		t.Fatal(err)
	}
	if c.SafeEnv {
		t.Fatal("script root marked reusable")
	}
	for _, k := range c.Consts {
		if k.Kind == engine.ConstFunction && k.Fn.Name == "outer" {
			if k.Fn.SafeEnv {
				t.Fatal("outer remains SafeEnv despite descendant closure retaining its .env")
			}
			return
		}
	}
	t.Fatal("outer function absent")
}
