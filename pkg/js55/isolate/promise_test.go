// SPDX-License-Identifier: Apache-2.0 OR MIT

package isolate

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestPromiseThenAfterEval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		iso, err := New(Config{})
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if _, err := iso.Eval(ctx, `var g = 0; Promise.resolve(42).then(function(v) { g = v; });`); err != nil {
			t.Fatalf("Eval : %v", err)
		}
		v, err := iso.Eval(ctx, `g`)
		if err != nil {
			t.Fatalf("lecture g : %v", err)
		}
		if v.ToInt() != 42 {
			t.Errorf("g=%d, 42 attendu après RunMicrotasks", v.ToInt())
		}
	})
}

func TestPromiseNewExecutor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		iso, err := New(Config{})
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if _, err := iso.Eval(ctx, `var g = 0; new Promise(function(resolve) { resolve(9); }).then(function(v) { g = v; });`); err != nil {
			t.Fatalf("Eval : %v", err)
		}
		v, err := iso.Eval(ctx, `g`)
		if err != nil {
			t.Fatalf("lecture g : %v", err)
		}
		if v.ToInt() != 9 {
			t.Errorf("g=%d, 9 attendu", v.ToInt())
		}
	})
}
