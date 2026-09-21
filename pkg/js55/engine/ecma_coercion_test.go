// SPDX-License-Identifier: BUSL-1.1

package engine_test

import (
	"context"
	"math"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

// TestECMACoercion_MathAndParsing vérifie tous les cas de coercion et de parsing soulevés lors de l'audit.
func TestECMACoercion_MathAndParsing(t *testing.T) {
	iso, err := isolate.New(isolate.Config{
		MaxMemoryBytes: 8 * 1024 * 1024,
		GasLimit:       10_000_000,
	})
	if err != nil {
		t.Fatalf("failed to create isolate: %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		script   string
		expected float64
		checkNaN bool
	}{
		{name: "Math.abs string coercion", script: "Math.abs('-5')", expected: 5},
		{name: "Math.abs boolean true", script: "Math.abs(true)", expected: 1},
		{name: "Math.abs null coercion", script: "Math.abs(null)", expected: 0},
		{name: "Math.abs undefined is NaN", script: "Math.abs(undefined)", checkNaN: true},
		{name: "Math.max string coercion", script: "Math.max('3', '7')", expected: 7},
		{name: "Math.min string coercion", script: "Math.min('10', '2')", expected: 2},
		{name: "Math.max empty args is -Infinity", script: "Math.max()", expected: math.Inf(-1)},
		{name: "Math.min empty args is +Infinity", script: "Math.min()", expected: math.Inf(1)},
		{name: "Math.round -0.5 is 0 in JS", script: "Math.round(-0.5)", expected: 0},
		{name: "Math.round 0.5 is 1", script: "Math.round(0.5)", expected: 1},
		{name: "parseInt prefix string", script: "parseInt('42abc')", expected: 42},
		{name: "parseInt with hex prefix", script: "parseInt('0x1F')", expected: 31},
		{name: "parseInt with base 16", script: "parseInt('ff', 16)", expected: 255},
		{name: "parseFloat prefix string", script: "parseFloat('3.14abc')", expected: 3.14},
		{name: "parseFloat exponential", script: "parseFloat('1.5e2abc')", expected: 150},
		{name: "isNaN on string number is false", script: "isNaN('123')", expected: 0}, // false -> 0
		{name: "isNaN on string text is true", script: "isNaN('hello')", expected: 1},  // true -> 1
		{name: "isFinite on 100 is true", script: "isFinite(100)", expected: 1},
		{name: "isFinite on Infinity is false", script: "isFinite(Infinity)", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := iso.EvalContext(ctx, tt.script)
			if err != nil {
				t.Fatalf("Erreur exécution '%s': %v", tt.script, err)
			}
			f := res.ToFloat()
			if tt.checkNaN {
				if !math.IsNaN(f) {
					t.Fatalf("Attendu NaN pour '%s', obtenu %f", tt.script, f)
				}
				return
			}
			if math.IsInf(tt.expected, 0) {
				if f != tt.expected {
					t.Fatalf("Attendu %f pour '%s', obtenu %f", tt.expected, tt.script, f)
				}
				return
			}
			if res.IsBool() {
				expectedBool := tt.expected != 0
				if res.ToBool() != expectedBool {
					t.Fatalf("Attendu bool %v pour '%s', obtenu %v", expectedBool, tt.script, res.ToBool())
				}
				return
			}
			if math.Abs(f-tt.expected) > 1e-6 {
				t.Fatalf("Attendu %f pour '%s', obtenu %f", tt.expected, tt.script, f)
			}
		})
	}
}

func TestECMACoercion_PrimitiveConversion(t *testing.T) {
	iso, err := isolate.New(isolate.Config{
		MaxMemoryBytes: 8 * 1024 * 1024,
		GasLimit:       10_000_000,
	})
	if err != nil {
		t.Fatalf("failed to create isolate: %v", err)
	}
	ctx := context.Background()

	// 1. String({toString(){return 'ok';}})
	t.Run("String with custom toString returning primitive string", func(t *testing.T) {
		res, err := iso.EvalContext(ctx, "String({toString(){return 'ok';}})")
		if err != nil {
			t.Fatalf("Erreur inattendue: %v", err)
		}
		s := iso.VM().StringOf(res)
		if s == nil || s.GoString() != "ok" {
			t.Fatalf("Attendu 'ok', obtenu %v", s)
		}
	})

	// 2. Objet numérique valueOf renvoyant string
	t.Run("Numeric coercion with valueOf returning string", func(t *testing.T) {
		res, err := iso.EvalContext(ctx, "const obj = { valueOf() { return '42'; } }; +obj")
		if err != nil {
			t.Fatalf("Erreur inattendue: %v", err)
		}
		if res.ToFloat() != 42 {
			t.Fatalf("Attendu 42, obtenu %f", res.ToFloat())
		}

		res2, err2 := iso.EvalContext(ctx, "const obj2 = { valueOf() { return '5'; } }; 3 * obj2")
		if err2 != nil {
			t.Fatalf("Erreur inattendue: %v", err2)
		}
		if res2.ToFloat() != 15 {
			t.Fatalf("Attendu 15, obtenu %f", res2.ToFloat())
		}
	})

	// 3. toString et valueOf renvoyant tous deux des objets -> TypeError puis reprise valide
	t.Run("Both toString and valueOf returning objects causes TypeError then recovery", func(t *testing.T) {
		script := `
			let caught = false;
			try {
				const bad = { toString() { return {}; }, valueOf() { return {}; } };
				String(bad);
			} catch (e) {
				if (e instanceof TypeError || (e && e.toString().indexOf("TypeError") !== -1)) {
					caught = true;
				}
			}
			caught;
		`
		res, err := iso.EvalContext(ctx, script)
		if err != nil {
			t.Fatalf("Erreur inattendue: %v", err)
		}
		if !res.ToBool() {
			t.Fatalf("Attendu caught=true suite à TypeError")
		}

		// Reprise valide sur la même instance après l'erreur
		resRecover, errRecover := iso.EvalContext(ctx, "1 + 1")
		if errRecover != nil {
			t.Fatalf("Erreur lors de la reprise: %v", errRecover)
		}
		if resRecover.ToFloat() != 2 {
			t.Fatalf("Attendu 2 lors de la reprise, obtenu %f", resRecover.ToFloat())
		}
	})

	// 4. Tester distinction wrappers/primitifs
	t.Run("Distinction wrappers vs primitives in toPrimitive", func(t *testing.T) {
		// Wrapper StringObject ou NumberObject retourné par toString et valueOf ne doit PAS être accepté comme primitif
		scriptWrapper := `
			let caughtWrapper = false;
			try {
				const badWrapper = {
					toString() { return new String("wrapped"); },
					valueOf() { return new Number(99); }
				};
				"" + badWrapper;
			} catch (e) {
				caughtWrapper = true;
			}
			caughtWrapper;
		`
		resW, errW := iso.EvalContext(ctx, scriptWrapper)
		if errW != nil {
			t.Fatalf("Erreur inattendue: %v", errW)
		}
		if !resW.ToBool() {
			t.Fatalf("Attendu caughtWrapper=true car les wrappers ne sont pas des primitifs")
		}

		// Primitif string retourné par toString est accepté
		resP, errP := iso.EvalContext(ctx, "const good = { toString() { return 'primitive'; }, valueOf() { return {}; } }; '' + good")
		if errP != nil {
			t.Fatalf("Erreur inattendue: %v", errP)
		}
		sP := iso.VM().StringOf(resP)
		if sP == nil || sP.GoString() != "primitive" {
			t.Fatalf("Attendu 'primitive', obtenu %v", sP)
		}
	})

	// 5. Si Symbol devient reconnu primitif, ToNumber(Symbol) doit lever TypeError sans recursion infinie
	t.Run("ToNumber on Symbol throws TypeError without infinite recursion", func(t *testing.T) {
		scriptSym := `
			let caughtSym = false;
			try {
				const s = Symbol("test");
				+s;
			} catch (e) {
				if (e instanceof TypeError || (e && e.toString().indexOf("TypeError") !== -1)) {
					caughtSym = true;
				}
			}
			caughtSym;
		`
		resSym, errSym := iso.EvalContext(ctx, scriptSym)
		if errSym != nil {
			t.Fatalf("Erreur inattendue lors de +Symbol: %v", errSym)
		}
		if !resSym.ToBool() {
			t.Fatalf("Attendu caughtSym=true pour +Symbol")
		}

		// Objet dont valueOf retourne un Symbol -> doit aussi lever TypeError sans récursion infinie
		scriptObjSym := `
			let caughtObjSym = false;
			try {
				const obj = { valueOf() { return Symbol("symVal"); } };
				+obj;
			} catch (e) {
				caughtObjSym = true;
			}
			caughtObjSym;
		`
		resObjSym, errObjSym := iso.EvalContext(ctx, scriptObjSym)
		if errObjSym != nil {
			t.Fatalf("Erreur inattendue lors de +objWithSymbol: %v", errObjSym)
		}
		if !resObjSym.ToBool() {
			t.Fatalf("Attendu caughtObjSym=true pour objet avec valueOf Symbol")
		}
	})
}
