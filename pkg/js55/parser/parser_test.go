// SPDX-License-Identifier: BUSL-1.1
package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/ast"
	"github.com/hazyhaar/js55/pkg/js55/lexer"
	"github.com/hazyhaar/js55/pkg/js55/printer"
)

// ─── Empreinte structurelle ─────────────────────────────────────────────────

// dump rend une empreinte de l'arbre où les positions sont omises. Deux arbres
// de même empreinte sont structurellement identiques ; c'est l'oracle de
// l'aller-retour T1.2, qui doit rester insensible à la mise en forme.
func dump(n any) string {
	var b strings.Builder
	dumpValue(&b, reflect.ValueOf(n))
	return b.String()
}

var posType = reflect.TypeOf(lexer.Position{})
var baseType = reflect.TypeOf(ast.Base{})

func dumpValue(b *strings.Builder, v reflect.Value) {
	if !v.IsValid() {
		b.WriteString("nil")
		return
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			b.WriteString("nil")
			return
		}
		dumpValue(b, v.Elem())
	case reflect.Struct:
		t := v.Type()
		if t == posType || t == baseType {
			return // les positions ne participent pas à l'empreinte
		}
		b.WriteString(t.Name() + "{")
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" || f.Type == baseType {
				continue
			}
			b.WriteString(f.Name + ":")
			dumpValue(b, v.Field(i))
			b.WriteString(" ")
		}
		b.WriteString("}")
	case reflect.Slice:
		b.WriteString("[")
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteString(",")
			}
			dumpValue(b, v.Index(i))
		}
		b.WriteString("]")
	case reflect.String:
		fmt.Fprintf(b, "%q", v.String())
	case reflect.Bool:
		fmt.Fprintf(b, "%v", v.Bool())
	default:
		fmt.Fprintf(b, "%v", v.Interface())
	}
}

// ─── Corpus ─────────────────────────────────────────────────────────────────

// validCorpus couvre la grammaire ES2020 admise. Chaque entrée doit analyser,
// puis survivre à l'aller-retour.
var validCorpus = []string{
	// Bases
	`var x = 1;`,
	`let a, b = 2, c;`,
	`const k = [1,2,3];`,
	`x = y = z;`,
	`a += 1; b **= 2; c ??= 3; d ||= 4; e &&= 5;`,
	`x++; --y; z--;`,
	`a ? b : c ? d : e;`,
	`a, b, c;`,
	`void 0; typeof x; delete a.b; !x; -x; ~x;`,

	// Précédence et associativité — le cœur de ce que l'aller-retour attrape
	`a + b * c;`,
	`(a + b) * c;`,
	`a ** b ** c;`,
	`a ** -b;`,
	`(-a) ** b;`,
	`(a ** b) ** c;`,
	`a - b - c;`,
	`a << b >> c >>> d;`,
	`a | b ^ c & d;`,
	`a == b != c;`,
	`a < b instanceof c;`,
	`a in b;`,
	`(a && b) ?? c;`,
	`a ?? (b || c);`,
	`a && b || c;`,

	// Accès, appels, chaînes optionnelles
	`a.b.c;`,
	`a[b][c];`,
	`f(1,2,3);`,
	`f(...args);`,
	`new Foo();`,
	`new Foo.Bar(1);`,
	`new.target;`,
	`a?.b;`,
	`a?.[b];`,
	`a?.();`,
	`a?.b.c?.d;`,

	// Fonctions
	`function f(){}`,
	`function f(a, b = 1, ...rest){ return a; }`,
	`function* g(){ yield 1; yield* h(); }`,
	`async function h(){ await p; }`,
	`(function(){})();`,
	`x => x;`,
	`(x) => x;`,
	`(a, b) => a + b;`,
	`() => {};`,
	`(a, ...b) => b;`,
	`async x => x;`,
	`({a}) => a;`,
	`([a, b]) => a;`,

	// Objets et classes
	`({});`,
	`({a: 1, b, "c": 3, 4: d, [e]: f});`,
	`({m(){}, *g(){}, async a(){}, get x(){}, set x(v){}});`,
	`({...spread});`,
	`class A {}`,
	`class A extends B { constructor(){ super(); } m(){} static s(){} }`,
	`class A { #p = 1; get #q(){} static { } }`,
	`(class {});`,

	// Décomposition
	`var [a, b] = c;`,
	`var {a, b: c, d = 1} = e;`,
	`var [a, ...b] = c;`,
	`var {a, ...b} = c;`,
	`[a, b] = c;`,
	`({a, b} = c);`,
	`var [c = 7] = [];`,
	`var {x = 10, y: z = 20} = {x: 5};`,
	`var [a, [b, c]] = [1, [2, 3]];`,

	// Instructions
	`if (a) b; else c;`,
	`for (;;) break;`,
	`for (var i = 0; i < 10; i++) f(i);`,
	`for (let x of y) f(x);`,
	`for (const k in o) f(k);`,
	`while (a) b;`,
	`do a; while (b);`,
	`try { a; } catch { b; }`,
	`try { a; } catch (e) { b; } finally { c; }`,
	`switch (a) { case 1: b; break; default: c; }`,
	`lbl: for (;;) { continue lbl; }`,
	`throw new Error("x");`,
	`{ let x = 1; }`,
	`;`,
	`debugger;`,

	// Littéraux textuels et expressions régulières
	"`abc`;",
	"`a${b}c${d}e`;",
	"tag`a${b}c`;",
	`/ab+c/gi.test(x);`,
	`a / b / c;`,
	`if (x) /re/.test(y);`,
	`'chaîne'; "autre";`,
	`0x1F; 0b1010; 0o17; 1_000; 123n; 1e10; .5;`,

	// Insertion automatique de points-virgules
	"var a = 1\nvar b = 2",
	"return\n",
	"a\n++b",
}

func TestParseValidCorpus(t *testing.T) {
	for _, src := range validCorpus {
		s := src
		if strings.HasPrefix(s, "return") {
			s = "function f(){" + s + "}"
		}
		if _, err := Parse(s, Options{}); err != nil {
			t.Errorf("analyse de %q : %v", s, err)
		}
	}
}

// TestRoundTrip est l'instrument T1.2. Il attrape les erreurs de précédence et
// d'associativité, que la suite de conformité laisse largement passer.
func TestRoundTrip(t *testing.T) {
	for _, src := range validCorpus {
		s := src
		if strings.HasPrefix(s, "return") {
			s = "function f(){" + s + "}"
		}

		first, err := Parse(s, Options{})
		if err != nil {
			continue // couvert par TestParseValidCorpus
		}
		out := printer.Print(first)

		second, err := Parse(out, Options{})
		if err != nil {
			t.Errorf("réanalyse impossible pour %q\nréimprimé : %q\nerreur : %v", s, out, err)
			continue
		}
		if a, b := dump(first), dump(second); a != b {
			t.Errorf("aller-retour non stable pour %q\nréimprimé : %q\navant : %s\naprès : %s",
				s, out, a, b)
		}
	}
}

func TestModules(t *testing.T) {
	mods := []string{
		`import "m";`,
		`import d from "m";`,
		`import * as ns from "m";`,
		`import {a, b as c} from "m";`,
		`import d, {a} from "m";`,
		`export {a, b as c};`,
		`export * from "m";`,
		`export * as ns from "m";`,
		`export default 1;`,
		`export default function f(){}`,
		`export const x = 1;`,
		`export function f(){}`,
	}
	for _, src := range mods {
		first, err := Parse(src, Options{Module: true})
		if err != nil {
			t.Errorf("analyse de %q : %v", src, err)
			continue
		}
		out := printer.Print(first)
		second, err := Parse(out, Options{Module: true})
		if err != nil {
			t.Errorf("réanalyse impossible pour %q\nréimprimé : %q\nerreur : %v", src, out, err)
			continue
		}
		if a, b := dump(first), dump(second); a != b {
			t.Errorf("aller-retour non stable pour %q\nréimprimé : %q", src, out)
		}
	}
}

// ─── Erreurs de syntaxe ─────────────────────────────────────────────────────

func TestSyntaxErrors(t *testing.T) {
	bad := []string{
		`var`,
		`var 1 = 2;`,
		`function (){}`,
		`-a ** b;`,     // unaire non parenthésé à GAUCHE de ** : c'est le côté interdit
		`a && b ?? c;`, // mélange interdit sans parenthèses
		`a ?? b || c;`,
		`{`,
		`(`,
		`a[`,
		`if (a`,
		`try { }`, // ni catch ni finally
		`switch (a) { case 1: break; default: ; default: ; }`,
		`1 = 2;`,
		`a++++;`,
		`({a b});`,
		`class A { constructor(){} `,
		`for (;;`,
		`export {a};`, // export hors module
	}
	for _, src := range bad {
		prog, err := Parse(src, Options{})
		if err == nil {
			t.Errorf("%q devrait produire une SyntaxError, arbre obtenu : %v", src, prog != nil)
			continue
		}
		if _, ok := err.(*SyntaxError); !ok {
			t.Errorf("%q : erreur de type %T, *SyntaxError attendu", src, err)
		}
	}
}

func TestErrorPositions(t *testing.T) {
	// Plan T1.4 : la position rapportée doit pointer le bon caractère.
	cases := []struct {
		src       string
		line, col int
	}{
		{"var x = 1;\nvar 2 = 3;", 2, 5},
		{"a b", 1, 3},
		{"var x = ;", 1, 9},
	}
	for _, c := range cases {
		_, err := Parse(c.src, Options{})
		se, ok := err.(*SyntaxError)
		if !ok {
			t.Errorf("%q : SyntaxError attendue, obtenu %v", c.src, err)
			continue
		}
		if se.Pos.Line != c.line || se.Pos.Col != c.col {
			t.Errorf("%q : erreur en %d:%d, attendu %d:%d (%s)",
				c.src, se.Pos.Line, se.Pos.Col, c.line, c.col, se.Msg)
		}
	}
}

func TestStrictModeDifferences(t *testing.T) {
	// « with » est licite en script non strict, interdit en mode strict.
	if _, err := Parse(`with (o) { a; }`, Options{}); err != nil {
		t.Errorf("« with » devrait être licite hors mode strict : %v", err)
	}
	if _, err := Parse(`with (o) { a; }`, Options{Strict: true}); err == nil {
		t.Error("« with » devrait être interdit en mode strict")
	}
	// « let » et « static » sont des identifiants hors mode strict.
	if _, err := Parse(`var static = 1;`, Options{}); err != nil {
		t.Errorf("« static » devrait être un identifiant hors mode strict : %v", err)
	}
}

// ─── Invariants durs ────────────────────────────────────────────────────────

func TestParseAuditPanicFiles(t *testing.T) {
	files := []string{
		"/devhoros/pkg/js55/testdata/test262/test/intl402/BigInt/prototype/toLocaleString/throws-same-exceptions-as-NumberFormat.js",
		"/devhoros/pkg/js55/testdata/test262/test/staging/sm/Map/constructor-iterator-close.js",
		"/devhoros/pkg/js55/testdata/test262/test/staging/sm/RegExp/prototype-different-global.js",
		"/devhoros/pkg/js55/testdata/test262/test/staging/sm/regress/regress-596805-2.js",
		"/devhoros/pkg/js55/testdata/test262/test/staging/sm/statements/for-inof-loop-const-declaration.js",
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panique Parse : %v", r)
				}
			}()
			_, _ = Parse(string(src), Options{})
		})
	}
}

func TestCollectVarDeclaredNames(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{`var x;`, []string{"x"}},
		{`let x;`, nil},
		{`const x = 1;`, nil},
		{`{ var x; }`, []string{"x"}},
		{`if (0) var x; else var y;`, []string{"x", "y"}},
		{`while (0) var x;`, []string{"x"}},
		{`do var x; while (0);`, []string{"x"}},
		{`for (;;) var x;`, []string{"x"}},
		{`for (var i = 0;;) ;`, []string{"i"}},
		{`for (let i = 0;;) var x;`, []string{"x"}},
		{`for (var k in o) ;`, []string{"k"}},
		{`for (let k in o) var x;`, []string{"x"}},
		{`for (var k of o) ;`, []string{"k"}},
		{`try { var x; } catch (e) { var y; } finally { var z; }`, []string{"x", "y", "z"}},
		{`L: var x;`, []string{"x"}},
		{`switch (0) { case 1: var x; default: var y; }`, []string{"x", "y"}},
		{`with (o) var x;`, []string{"x"}},
		{`function f() { var x; }`, nil},
		{`var {a, b: c, d = 1} = e;`, []string{"a", "c", "d"}},
		{`var [a, ...b] = c;`, []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			prog, err := Parse(c.src, Options{})
			if err != nil {
				t.Fatalf("analyse : %v", err)
			}
			got := map[string]bool{}
			for _, st := range prog.Body {
				collectVarDeclaredNames(st, got)
			}
			want := map[string]bool{}
			for _, n := range c.want {
				want[n] = true
			}
			if len(got) != len(want) {
				t.Fatalf("noms %v, attendu %v", got, want)
			}
			for n := range want {
				if !got[n] {
					t.Fatalf("nom « %s » absent de %v", n, got)
				}
			}
		})
	}
	collectVarDeclaredNames(nil, map[string]bool{})
	collectVarDeclaredNames((*ast.BlockStmt)(nil), map[string]bool{})
	collectVarDeclaredNames((*ast.TryStmt)(nil), map[string]bool{})
	collectVarDeclaredNames((*ast.WithStmt)(nil), map[string]bool{})
}

func TestForDeclEarlyErrors(t *testing.T) {
	reject := []string{
		`for (let x in o) { var x; }`,
		`for (const x of o) { var x; }`,
		`for (let x of o) var x;`,
		`for (let {a} of o) { var a; }`,
		`for (let [x] of o) { if (0) var x; }`,
		`for (let x of o) { for (;;) { var x; } }`,
		`for (let x of o) { for (var x in z) {} }`,
		`for (let x of o) { try { var x; } catch (e) {} }`,
		`for (let x of o) { try {} catch (e) { var x; } }`,
		`for (let x of o) { try {} finally { var x; } }`,
		`for (let x of o) { switch (0) { default: var x; } }`,
		`for (let x of o) { with (z) { var x; } }`,
		`for (let x of o) { L: { var x; } }`,
		`for (let x of o) { do { var x; } while (0); }`,
		`for (let {x, x} of o) {}`,
		`for (let let of o) {}`,
	}
	accept := []string{
		`for (let x in o) { var y; }`,
		`for (const x of o) { var y; }`,
		`for (let x of o) var y;`,
		`for (let {a} of o) { var b; }`,
		`for (var x of o) { var x; }`,
		`for (let x of o) { function f() { var x; } }`,
		`for (let x of o) { let x; }`,
	}
	for _, src := range reject {
		t.Run("rejet/"+src, func(t *testing.T) {
			_, err := Parse(src, Options{})
			if err == nil {
				t.Fatalf("SyntaxError attendue")
			}
			if _, ok := err.(*SyntaxError); !ok {
				t.Fatalf("type %T, *SyntaxError attendu", err)
			}
			_, err2 := Parse(`for (let ok of o) { var distinct; }`, Options{})
			if err2 != nil {
				t.Fatalf("reprise après rejet : %v", err2)
			}
		})
	}
	for _, src := range accept {
		t.Run("nominal/"+src, func(t *testing.T) {
			if _, err := Parse(src, Options{}); err != nil {
				t.Fatalf("analyse : %v", err)
			}
		})
	}
}

func TestParseForConstInOfString(t *testing.T) {
	srcs := []string{
		`for (const x in "abcdef") {}`,
		`for (const x of "012345") {}`,
		`for (const { length, 0: c } in "abcdef") {}`,
		`for (const { length, 0: c } of "012345") {}`,
		`for (const x in "abcdef") { try { x = 3; } catch (e) {} }`,
		`for (const x of "012345") { try { x = 3; } catch (e) {} }`,
	}
	for _, src := range srcs {
		t.Run(src, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panique : %v", r)
				}
			}()
			_, err := Parse(src, Options{})
			if err != nil {
				t.Fatalf("erreur : %v", err)
			}
		})
	}
}

func TestNeverPanics(t *testing.T) {
	corpus := []string{
		"", "\x00", "\xff\xfe", "/*", "`", "${", "}", `"`, `\`, "#",
		strings.Repeat("(", 5000),
		strings.Repeat("[", 5000),
		strings.Repeat("{", 5000),
		strings.Repeat("-", 5000) + "a",
		strings.Repeat("a=", 5000) + "b",
		strings.Repeat("typeof ", 5000) + "a",
		"function " + strings.Repeat("f(){function ", 400),
	}
	for _, src := range corpus {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panique sur une source de %d octets : %v", len(src), r)
				}
			}()
			_, _ = Parse(src, Options{})
		}()
	}
}

// TestDeepNestingIsSyntaxError vérifie qu'une imbrication extrême produit une
// erreur de syntaxe et non un dépassement de pile du processus.
func TestDeepNestingIsSyntaxError(t *testing.T) {
	src := strings.Repeat("(", 100000) + "1" + strings.Repeat(")", 100000)
	_, err := Parse(src, Options{})
	if err == nil {
		t.Fatal("une imbrication de 100 000 niveaux devrait être refusée")
	}
	if !strings.Contains(err.Error(), "imbrication") {
		t.Errorf("erreur inattendue : %v", err)
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range validCorpus {
		f.Add(s)
	}
	f.Add("")
	f.Fuzz(func(t *testing.T, src string) {
		// Invariant T1.3 : aucune entrée ne panique ni ne boucle.
		prog, err := Parse(src, Options{})
		if err != nil {
			if _, ok := err.(*SyntaxError); !ok {
				t.Fatalf("erreur de type %T, *SyntaxError attendu : %v", err, err)
			}
			return
		}
		// Une source analysée doit se réimprimer et se réanalyser.
		out := printer.Print(prog)
		again, err := Parse(out, Options{})
		if err != nil {
			// Exemption unique et nommée : le printer parenthèse, ce qui
			// augmente la profondeur d'imbrication à la relecture. Une source
			// analysée juste sous la borne peut donc la franchir une fois
			// réimprimée. L'invariant d'aller-retour n'est pas revendiqué au
			// voisinage de la borne ; il l'est partout ailleurs.
			if strings.Contains(err.Error(), "imbrication trop profonde") {
				return
			}
			t.Fatalf("réanalyse impossible\nsource   : %q\nréimprimé : %q\nerreur   : %v", src, out, err)
		}
		if a, b := dump(prog), dump(again); a != b {
			t.Fatalf("aller-retour non stable\nsource   : %q\nréimprimé : %q", src, out)
		}
	})
}
