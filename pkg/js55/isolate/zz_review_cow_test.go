// SPDX-License-Identifier: BUSL-1.1
package isolate

import (
	"context"
	"sync"
	"testing"
)

// Sonde de revue : chaque script d'attaque tourne dans un isolat A ; un isolat
// B neuf vérifie ensuite l'invariant. Tout écart = fuite du royaume racine.
func TestReview_CrossIsolateLeaks(t *testing.T) {
	ctx := context.Background()
	cases := []struct{ name, attack, check string }{
		{"reflect_setPrototypeOf_ArrayProto", `Reflect.setPrototypeOf(Array.prototype, null)`, `typeof [].hasOwnProperty === "function"`},
		{"object_setPrototypeOf_ArrayProto", `Object.setPrototypeOf(Array.prototype, null)`, `typeof [].hasOwnProperty === "function"`},
		{"proto_setter_ArrayProto", `Array.prototype.__proto__ = null`, `typeof [].hasOwnProperty === "function"`},
		{"defineProperty_existing_accessor", `Object.defineProperty(Object.prototype, "__proto__", {get: function(){ return 42; }})`, `({}).__proto__ === Object.prototype`},
		{"freeze_ObjectProto", `Object.freeze(Object.prototype)`, `!Object.isFrozen(Object.prototype) && (Object.prototype.zz = 1, Object.prototype.zz === 1)`},
		{"preventExtensions_reflect", `Reflect.preventExtensions(Array.prototype)`, `Reflect.isExtensible(Array.prototype)`},
		{"delete_toString", `delete Object.prototype.toString`, `typeof ({}).toString === "function"`},
		{"define_accessor_on_root", `Object.defineProperty(Array.prototype, "length2", {get: function(){return 7}, configurable:true})`, `typeof [].length2 === "undefined"`},
		{"math_pi", `Math.PI = 3`, `Math.PI > 3.14`},
		{"reverse_root_array", `try { Array.prototype.reverse.call(Array.prototype) } catch(e) {}`, `typeof [].push === "function"`},
		{"global_override", `Object = function(){}`, `typeof Object.keys === "function"`},
		{"symbol_for", `Symbol.for("zz_review")`, `true`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := New(Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if _, err := a.EvalContext(ctx, c.attack); err != nil {
				t.Logf("attaque rejetée (%v) : ok si volontaire", err)
			}
			b, err := New(Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			v, err := b.EvalContext(ctx, c.check)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if b.VM().ToStringValue(v).GoString() != "true" {
				t.Fatalf("FUITE : invariant faux dans l'isolat B après %q", c.attack)
			}
		})
	}
}

// Sonde de courses : N isolats mutent en parallèle les intrinsèques.
func TestReview_ParallelMutation(t *testing.T) {
	ctx := context.Background()
	script := `
		Object.prototype.p = 1; Array.prototype.q = 2; Function.prototype.r = 3;
		Reflect.setPrototypeOf(Array.prototype, null);
		Object.defineProperty(Object.prototype, "__proto__", {get: function(){ return 1; }});
		var a = []; for (var i = 0; i < 200; i++) a.push({x: i, y: "s" + i});
		JSON.stringify(a).length > 0 && [1,2,3].map(function(x){return x*2}).join(",") === "2,4,6"
	`
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				iso, err := New(Config{})
				if err != nil {
					t.Error(err)
					return
				}
				v, err := iso.EvalContext(ctx, script)
				if err != nil {
					t.Error(err)
				} else if iso.VM().ToStringValue(v).GoString() != "true" {
					t.Error("résultat inattendu")
				}
				_ = iso.Reset()
				_ = iso.Close()
			}
		}()
	}
	wg.Wait()
}

// Sonde Reset : l'état d'un cycle ne survit pas au suivant.
func TestReview_ResetCarryover(t *testing.T) {
	ctx := context.Background()
	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer iso.Close()
	if _, err := iso.EvalContext(ctx, `Object.prototype.leak = 1; var g = 5; Object.freeze(Array.prototype);`); err != nil {
		t.Fatal(err)
	}
	if err := iso.Reset(); err != nil {
		t.Fatal(err)
	}
	v, err := iso.EvalContext(ctx, `typeof ({}).leak === "undefined" && typeof g === "undefined" && !Object.isFrozen(Array.prototype)`)
	if err != nil {
		t.Fatal(err)
	}
	if iso.VM().ToStringValue(v).GoString() != "true" {
		t.Fatal("état du cycle précédent visible après Reset")
	}
}
