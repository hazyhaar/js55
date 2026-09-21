// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

// Instruments T4.1 à T4.6 du plan.

// programs est le corpus d'exécution. Chaque entrée doit produire la MÊME valeur
// de complétion que Node, dans les deux modes de ramasse-miettes.
var programs = []string{
	// Arithmétique et conversions
	`1 + 2`,
	`7 / 2`,
	`7 % 3`,
	`2 ** 10`,
	`-3 * -4`,
	`0.1 + 0.2`,
	`1e21`,
	`1e-7`,
	`123456789012345680000`,
	`1 / 0`,
	`-1 / 0`,
	`0 / 0`,

	// Bits
	`5 & 3`, `5 | 3`, `5 ^ 3`, `~5`, `1 << 10`, `-16 >> 2`, `-16 >>> 28`,

	// Comparaisons et égalité
	`1 < 2`, `2 <= 2`, `3 > 4`, `"a" < "b"`, `"abc" === "abc"`, `1 === "1"`,
	`1 == "1"`, `null == undefined`, `null === undefined`, `NaN === NaN`,
	`0 === -0`,

	// Chaînes
	`"x" + "y"`, `"a" + 1`, `1 + "a"`, `"abc".length`, `"" + null`,
	`"" + undefined`, `"" + true`, `"abc"[1]`,

	// Variables et portée
	`var a = 3; a * 4`,
	`var a = 1; { var a = 2; } a`,
	`var x = 1; x += 2; x *= 3; x`,
	`var i = 0; i++; i++; i`,
	`var i = 0; var j = i++; j + "," + i`,
	`var i = 0; var j = ++i; j + "," + i`,

	// Contrôle
	`var s = 0; for (var i = 0; i < 5; i++) { s += i; } s`,
	`var s = 0, i = 0; while (i < 5) { s += i; i++; } s`,
	`var s = 0, i = 0; do { s += i; i++; } while (i < 5); s`,
	`var s = 0; for (var i = 0; i < 10; i++) { if (i % 2) continue; s += i; } s`,
	`var s = 0; for (var i = 0; i < 10; i++) { if (i > 4) break; s += i; } s`,
	`var x = 5; if (x > 3) { x = "grand"; } else { x = "petit"; } x`,
	`var s = 0; switch (2) { case 1: s = 1; break; case 2: s = 2; break; default: s = 3; } s`,
	`var s = 0; switch (1) { case 1: s = 1; case 2: s += 10; break; } s`,
	`var s = 0; switch (9) { default: s = 7; break; case 1: s = 8; } s`,
	`var s=""; for (var k in {a:1,b:2}) s+=k; s`,
	`var s=0; for (var v of [10,20]) s+=v; s`,
	`null ?? 7`,
	`0 ?? 7`,
	`undefined ?? "x"`,
	`var o = null; o?.a`,
	`var o = {a: 3}; o?.a`,
	`var [a, ...b] = [1, 2, 3]; a + b.length`,
	`var xs = [1, 2]; var ys = [...xs, 3]; ys.length`,
	`function add(a, b) { return a + b; } add(...[10, 32])`,
	`"a" in {a:1}`,
	`"b" in {a:1}`,
	`function f(a = 3) { return a; } f() + f(10)`,
	`var o = {x: 1, m: function(a,b) { return this.x + a + b; }}; o.m(...[2, 3])`,
	`var a = {x:1}; var b = {...a, y:2}; b.x + b.y`,
	`var m = new Map(); m.set(1, 2); m.get(1)`,
	`var s = new Set(); s.add(3); s.has(3)`,
	`JSON.stringify({a:1})`,
	`({valueOf: function() { return 1 }}) + 1`,
	`typeof not_defined_xyz`,
	`var s=0; lbl: for (var i=0;i<5;i++) { if (i===2) break lbl; s+=i; } s`,
	`true ? "oui" : "non"`,
	`false || "défaut"`,
	`0 || "défaut"`,
	`1 && 2`,
	`"" && 2`,

	// Fonctions et fermetures
	`function f(a, b) { return a + b; } f(2, 3)`,
	`function f() {} typeof f`,
	`(function (x) { return x * 2; })(21)`,
	`function mk(n) { return function () { return n * 2; }; } mk(21)()`,
	`function counter() { var n = 0; return function () { n++; return n; }; }
	 var c = counter(); c(); c(); c()`,
	`var add = function (a) { return function (b) { return a + b; }; }; add(3)(4)`,
	`var sq = (x) => x * x; sq(7)`,
	`var f = (a, b) => a - b; f(10, 4)`,
	`function fact(n) { return n <= 1 ? 1 : n * fact(n - 1); } fact(10)`,
	`function fib(n) { return n < 2 ? n : fib(n-1) + fib(n-2); } fib(18)`,

	// Objets et tableaux
	`var o = {a: 1, b: "deux"}; o.a + o.b`,
	`var o = {}; o.x = 5; o.x`,
	`var o = {a: 1}; o["a"]`,
	`var o = {}; o["k" + 1] = 9; o.k1`,
	`var arr = [1,2,3]; arr[1] + arr.length`,
	`var arr = []; arr[0] = "z"; arr[0] + arr.length`,
	`[1,2,3] + ""`,
	`var o = {a: {b: {c: 42}}}; o.a.b.c`,
	`typeof {}`,
	`typeof []`,
	`typeof null`,
	`typeof undefined`,
	`typeof 1`,
	`typeof true`,

	// Gabarits
	"`a${1+1}b`",
	"`${\"x\"}${\"y\"}`",

	// Compositions
	`var t = 0; for (var i = 1; i <= 100; i++) { t += i; } t`,
	`function sum(n) { var t = 0; for (var i = 0; i < n; i++) t += i; return t; } sum(1000)`,
	`var o = {n: 0}; function inc(o) { o.n = o.n + 1; return o; } inc(inc(o)).n`,

	// Décomposition et valeurs par défaut
	`var [a,b]=[1,2]; a+b`,
	`var {x,y}={x:10,y:20}; x+y`,
	`var [c=7]=[]; c`,
	`var [a, b = 2] = [1]; a + b`,
	`var {x = 10, y: z = 20} = {x: 5}; x + z`,
	`var [a, [b, c]] = [1, [2, 3]]; a + b + c`,
	`var {a: {b}} = {a: {b: 42}}; b`,
	`var [, b] = [1, 2]; b`,
	`var a, b; [a, b] = [10, 20]; a + b`,
	`var x, y; ({x, y} = {x: 100, y: 200}); x + y`,
	`function* g() { yield 1; yield 2; } var it = g(); it.next().value + it.next().value`,
	`function* g() { var x = yield 1; yield x; } var it = g(); it.next(); it.next(5).value`,
	`function* i() { yield 2; } function* o() { yield 1; yield* i(); yield 3; } var it = o(); it.next().value + it.next().value + it.next().value`,
	`function* o() { yield* [4, 5]; } var it = o(); it.next().value + it.next().value`,
	`function* g() { yield 10; yield 20; } var s=0; for (var v of g()) s+=v; s`,
	`var o={*g(){ yield 3; yield 4; }}; var it=o.g(); it.next().value+it.next().value`,
	`var o={get x(){ return 7; }}; o.x`,
	`var o={get x(){ return 1; }, set x(v){ this.y=v; }}; o.x=4; o.y`,
	`function tag(qs, a){ return qs[0]+a+qs[1]; } tag` + "`" + `A${9}B` + "`",
	`var s=""; for (var c of "ab") s+=c; s`,
	`async function f(){ return 4; } var x=0; f().then(function(v){ x=v; }); x`,
	`var o={a:1}; Object.freeze(o); o.a`,
	`var o={a:1}; delete o.a; o.a`,
	`"\101"`,

	`throw new TypeError('x')`,
	`(undefined).a`,
	`null.a`,
	`(0)()`,
	`throw new RangeError('x')`,
	`foo`,
	`1n+1`,
	`function f(){return f()} f()`,
	`function f(a){return arguments[0]+arguments.length} f(7,8,9)`,
	`function f(){return arguments.length} f()`,
	`function f(a){return arguments[1]} f(1,2)`,
	`/a(?=b)/.test("ab")`,
	`/a(?=b)/.test("ac")`,
	`/(?<n>a)/.exec("a")[1]`,
	`/(\w)\1/.test("aa")`,
	`/(\w)\1/.test("ab")`,
	`/(?<=a)b/.test("ab")`,
	`"abc".match(/b/)[0]`,
	`"abc".search(/b/)`,
	`"a b c".replace(/ /g, "-")`,
	`"a b c".split(/ /).join(",")`,
	`"a-b-c".split(/-/, 2).join(",")`,
	`"hello".split(/(l)/).join(",")`,
	`"abc".split(/b/).join(",")`,
	`var ntOk; function C(){ ntOk = new.target === C; } new C(); ntOk`,
	`function C(){ return typeof new.target; } C()`,
	`function f({a}){ return a; } f({a:7})`,
	`function f(...xs){ return xs.length; } f(1,2,3)`,
	`var o={}; Object.defineProperty(o,"x",{get:function(){return 4}}); o.x`,
	`var n=1; var o={}; Object.defineProperty(o,"x",{get:function(){return n},set:function(v){n=v}}); o.x=8; o.x`,
	`var o={}; Object.defineProperty(o,"h",{value:1,enumerable:false}); Object.getOwnPropertyDescriptor(o,"h").enumerable`,
	`Object.defineProperties({},{k:{value:9,writable:true}}).k`,
	`var p=new Proxy({a:1},{get:function(t,k){return 7}}); p.a`,
	`var n=0; var p=new Proxy({},{set:function(t,k,v){n=v;return true}}); p.z=3; n`,
	`var p=new Proxy({a:1},{has:function(t,k){return k==="a"}}); ("a" in p)+","+("b" in p)`,
	`var p=new Proxy(function(x){return x},{apply:function(t,th,args){return t(args[0])*2}}); p(5)`,
	`var P=new Proxy(function(){},{construct:function(t,args){return {n:args[0]};}}); var o=new P(4); o.n`,
	`/(?<x>a)(?<y>b)/.exec("ab").groups.y`,
	`"x1y2".replace(/(?<d>\d)/g,"[$<d>]")`,
	`(function(){}).name`,
	`(()=>{}).name`,
	`({m(){}}).m.name`,
	`({a:{get:function(){}}}).a.get.name`,
	`Object.getOwnPropertyNames(function f(a,b){}).join(",")`,
	`Object.keys({b:1,a:2}).join(",")`,
	`var x=0; (async function(){ x=4; })(); x`,
	`var x=0; (async function(){ x=1; await 0; x=2; })(); x`,
	`var f = function(){}; f.name`,
	`let g = () => {}; g.name`,
	`const h = function(){}; h.name`,
	`var f; f = function(){}; f.name`,
	`function p(x = function(){}) { return x.name } p()`,
	`var {a = function(){}} = {}; a.name`,
	`var [b = function(){}] = []; b.name`,
	`({get x(){ throw new TypeError("enc") }})`,
	`var o={}; Object.defineProperty(o,"a",{value:7}); o.a`,
	`var o={}; Object.defineProperty(o,"a",{value:1,enumerable:false}); Object.keys(o).length`,
	`var o={}; Object.defineProperty(o,"a",{value:1,enumerable:true}); Object.keys(o).length`,
	`var o={}; Object.defineProperty(o,"a",{value:1,writable:false}); o.a=2; o.a`,
	`var o={}; Object.defineProperty(o,"a",{value:1,writable:true}); o.a=2; o.a`,
	`var o={}; Object.defineProperty(o,"a",{value:1,configurable:false}); delete o.a; o.a`,
	`var s=0; for (const [k] of [[1],[2]]) s+=k; s`,
	`var s=0; for (const {x} of [{x:1},{x:2}]) s+=x; s`,
	`var s=""; for (let [c] of [["a"],["b"]]) s+=c; s`,
	`var k; for ([k] of [[7],[8]]); k`,
	`var s=0; for (var x of [1,2,3]) { if (x===2) break; s+=x; } s`,
	`function f(){ var s=0; for (var x of arguments) s+=x; return s; } f(1,2,3)`,
}

func compileAndRun(t *testing.T, src string, stress bool) (string, error) {
	t.Helper()
	s, _, err := compileAndRunNamed(t, src, stress)
	return s, err
}

func compileAndRunNamed(t *testing.T, src string, stress bool) (string, string, error) {
	t.Helper()
	prog, err := parser.Parse(src, parser.Options{})
	if err != nil {
		return "", js55ErrorName(err), err
	}
	h := NewHeap()
	chunk, err := Compile(h, prog, "script")
	if err != nil {
		return "", js55ErrorName(err), err
	}
	h.SetStress(stress)
	vm := NewVM(h)
	v, err := vm.Run(chunk)
	if err != nil {
		return "", errorNameFromThrow(vm, err), err
	}
	vm.heap.AddRoot(&v)
	defer vm.heap.RemoveRoot(&v)
	return vm.toDisplayString(v), "", nil
}

func compileAndEncode(t *testing.T, src string, stress bool) (enc, errName, errMsg string, err error) {
	t.Helper()
	prog, err := parser.Parse(src, parser.Options{})
	if err != nil {
		return "", js55ErrorName(err), exceptionMessage(nil, err), err
	}
	h := NewHeap()
	chunk, err := Compile(h, prog, "script")
	if err != nil {
		return "", js55ErrorName(err), exceptionMessage(nil, err), err
	}
	h.SetStress(stress)
	vm := NewVM(h)
	v, err := vm.Run(chunk)
	if err != nil {
		return "", errorNameFromThrow(vm, err), exceptionMessage(vm, err), err
	}
	vm.heap.AddRoot(&v)
	defer vm.heap.RemoveRoot(&v)
	if name, msg, ok := vm.rejectedPromise(v); ok {
		th := &Throw{Value: v, Text: name + ": " + msg}
		return "", name, msg, th
	}
	enc, encErr := vm.encodeValueErr(v)
	if encErr != nil {
		return "", errorNameFromThrow(vm, encErr), exceptionMessage(vm, encErr), encErr
	}
	return enc, "", "", nil
}

func exceptionMessage(vm *VM, err error) string {
	if err == nil {
		return ""
	}
	if th, ok := err.(*Throw); ok {
		if vm != nil && th.Value.IsObject() {
			key := vm.heap.Intern().InternGo("message")
			if mv, ok := vm.heap.GetProperty(th.Value.Handle(), key); ok {
				if s := vm.StringOf(mv); s != nil {
					return s.GoString()
				}
			}
		}
		s := th.Text
		for _, p := range []string{"TypeError: ", "RangeError: ", "ReferenceError: ", "SyntaxError: ", "URIError: ", "EvalError: "} {
			if strings.HasPrefix(s, p) {
				return strings.TrimPrefix(s, p)
			}
		}
		return s
	}
	s := err.Error()
	for _, p := range []string{"TypeError: ", "RangeError: ", "ReferenceError: ", "SyntaxError: ", "URIError: ", "EvalError: "} {
		if strings.HasPrefix(s, p) {
			return strings.TrimPrefix(s, p)
		}
	}
	return s
}

func errorNameFromThrow(vm *VM, err error) string {
	if th, ok := err.(*Throw); ok && vm != nil && th.Value.IsObject() {
		key := vm.heap.Intern().InternGo("name")
		if nv, ok := vm.heap.GetProperty(th.Value.Handle(), key); ok {
			if s := vm.StringOf(nv); s != nil {
				if n := s.GoString(); n != "" {
					return n
				}
			}
		}
	}
	return js55ErrorName(err)
}

// ─── T4.2 : différentiel contre Node ────────────────────────────────────────

type nodeResult struct {
	Value   string `json:"value"`
	Error   string `json:"error"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

func js55ErrorName(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	for _, n := range []string{"TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		if strings.Contains(s, n) {
			return n
		}
	}
	return "Error"
}

func nodeThrowName(ref nodeResult) string {
	if ref.Name != "" {
		return ref.Name
	}
	return ref.Error
}

func t42Diverge(ref nodeResult, mine, gotName, gotMsg string, err error) string {
	wantName := nodeThrowName(ref)
	if wantName != "" {
		if err == nil {
			return "js55 réussit, Node jette " + wantName
		}
		if gotName == "" {
			gotName = js55ErrorName(err)
		}
		if gotName != wantName {
			return "nom " + gotName + " ≠ " + wantName
		}
		if ref.Message != "" && gotMsg != ref.Message {
			return "message " + gotMsg + " ≠ " + ref.Message
		}
		return ""
	}
	if err != nil {
		return err.Error()
	}
	if mine != ref.Value {
		return "valeur " + mine + " ≠ " + ref.Value
	}
	return ""
}

// TestT4_2_DifferentialVsNode compare la valeur de complétion de chaque
// programme à celle de Node, DANS LES DEUX MODES de ramasse-miettes. Le
// JavaScript vient du disque : l'oracle est testdata/eval_oracle.js.
func TestT4_2_DifferentialVsNode(t *testing.T) {
	const script = "testdata/eval_oracle.js"
	if _, err := os.Stat(script); err != nil {
		t.Skipf("oracle absent (%s)", script)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent : le différentiel ne peut pas être mesuré")
	}

	in, err := json.Marshal(map[string]any{"programs": programs})
	if err != nil {
		t.Fatalf("encodage : %v", err)
	}
	cmd := exec.Command("node", script)
	cmd.Stdin = bytes.NewReader(in)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("exécution de l'oracle : %v\n%s", err, errBuf.String())
	}

	var got struct {
		Results []nodeResult `json:"results"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("décodage : %v", err)
	}
	if len(got.Results) != len(programs) {
		t.Fatalf("%d résultats pour %d programmes", len(got.Results), len(programs))
	}

	for _, stress := range []bool{false, true} {
		mode := "normal"
		if stress {
			mode = "stress"
		}
		diverged := 0
		for i, src := range programs {
			ref := got.Results[i]
			mine, gotName, gotMsg, err := compileAndEncode(t, src, stress)
			if why := t42Diverge(ref, mine, gotName, gotMsg, err); why != "" {
				t.Errorf("mode %s, programme %d : %s\n  source : %s\n  js55 : %q err=%v\n  node : %+v",
					mode, i, why, oneLine(src), mine, err, ref)
				diverged++
			}
		}
		t.Logf("mode %-7s : %d programmes, %d divergences", mode, len(programs), diverged)
	}
}

func TestT4_2_WitnessMutationDetectsDivergence(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent")
	}
	src := programs[0]
	enc, name, msg, err := compileAndEncode(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	ok := nodeResult{Value: enc}
	if why := t42Diverge(ok, enc, name, msg, err); why != "" {
		t.Fatalf("égalité vraie divergée : %s", why)
	}
	fake := nodeResult{Value: `{"t":"num","v":"999"}`}
	why := t42Diverge(fake, enc, name, msg, err)
	if why == "" {
		t.Fatal("la boucle t42Diverge n'a pas vu la falsification du champ value")
	}
	t.Logf("mutation témoin : t42Diverge=%s  js55=%s", why, enc)
}

func TestOracleHostileBatch(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent")
	}
	batch := []string{
		`1+2`,
		`(async function(){ throw new TypeError("x") })()`,
		`while(true){}`,
		`({get x(){ throw new TypeError("enc") }})`,
		`3+4`,
	}
	in, err := json.Marshal(map[string]any{"programs": batch})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "testdata/eval_oracle.js")
	cmd.Stdin = bytes.NewReader(in)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("oracle tué : %v\n%s\nout=%q", err, errBuf.String(), out)
	}
	var got struct {
		Results []nodeResult `json:"results"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("JSON : %v raw=%s", err, out)
	}
	if len(got.Results) != 5 {
		t.Fatalf("lot incomplet : %d résultats, 5 attendus raw=%s", len(got.Results), out)
	}
	if got.Results[0].Value == "" {
		t.Fatalf("programme 0 perdu : %+v", got.Results[0])
	}
	if nodeThrowName(got.Results[1]) == "" {
		t.Fatalf("rejet non isolé : %+v", got.Results[1])
	}
	if nodeThrowName(got.Results[2]) == "" {
		t.Fatalf("boucle non isolée : %+v", got.Results[2])
	}
	if nodeThrowName(got.Results[3]) == "" {
		t.Fatalf("encodage non isolé : %+v", got.Results[3])
	}
	if got.Results[4].Value == "" {
		t.Fatalf("programme 4 perdu : %+v", got.Results[4])
	}
}

func TestEncodeObservableStates(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent")
	}
	cases := []string{
		`1`,
		`-0`,
		`0/0`,
		`1/0`,
		`-1/0`,
		`1n`,
		`(function(){})`,
		`throw new TypeError("x")`,
		`var o={}; o.self=o; 1`,
		`(async function(){ throw new TypeError("x") })()`,
		`({get x(){ throw new TypeError("enc") }})`,
		`Symbol("x")`,
	}
	in, err := json.Marshal(map[string]any{"programs": cases})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "testdata/eval_oracle.js")
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("oracle : %v", err)
	}
	var got struct {
		Results []nodeResult `json:"results"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != len(cases) {
		t.Fatalf("%d résultats", len(got.Results))
	}
	for i, src := range cases {
		mine, name, msg, err := compileAndEncode(t, src, false)
		if why := t42Diverge(got.Results[i], mine, name, msg, err); why != "" {
			t.Errorf("état %d %s : %s\n  js55 %q %v\n  node %+v", i, src, why, mine, err, got.Results[i])
		}
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// TestGCModeAgreement exige que le résultat ne dépende pas du mode de
// ramasse-miettes, indépendamment de Node. Un écart est un défaut
// d'enracinement dans l'interpréteur.
func TestGCModeAgreement(t *testing.T) {
	for i, src := range programs {
		a, errA := compileAndRun(t, src, false)
		b, errB := compileAndRun(t, src, true)
		if (errA == nil) != (errB == nil) {
			t.Errorf("programme %d : erreur en mode %v seulement\n  source : %s\n  normal : %v\n  stress : %v",
				i, errA == nil, oneLine(src), errA, errB)
			continue
		}
		if a != b {
			t.Errorf("programme %d : %q en mode normal, %q en mode stress — défaut d'enracinement"+
				"\n  source : %s", i, a, b, oneLine(src))
		}
	}
}

// ─── T4.3 : golden du désassembleur ─────────────────────────────────────────

// goldenPrograms est figé : un changement de génération de code se lit dans le
// diff du golden, il ne se découvre pas en production.
var goldenPrograms = []string{
	`var x = 1 + 2;`,
	`function f(a) { return a * 2; } f(3);`,
	`var s = 0; for (var i = 0; i < 3; i++) { s += i; }`,
	`var o = {a: 1}; o.a = o.a + 1;`,
	`var g = (x) => x + 1;`,
}

func TestT4_3_DisassemblyGolden(t *testing.T) {
	var b strings.Builder
	for _, src := range goldenPrograms {
		prog, err := parser.Parse(src, parser.Options{})
		if err != nil {
			t.Fatalf("%q : %v", src, err)
		}
		h := NewHeap()
		chunk, err := Compile(h, prog, "script")
		if err != nil {
			t.Fatalf("%q : %v", src, err)
		}
		b.WriteString("### " + oneLine(src) + "\n")
		b.WriteString(chunk.Disassemble())
		b.WriteString("\n")
	}
	got := b.String()

	const goldenPath = "testdata/disassembly.golden"
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("écriture du golden : %v", err)
		}
		t.Fatalf("golden absent, il vient d'être écrit dans %s — relancer pour comparer", goldenPath)
	}
	if string(want) != got {
		t.Errorf("la génération de code a changé. Comparer :\n"+
			"  diff <(cat %s) <(go test -run TestT4_3 -v ./js55/engine/)\n"+
			"Si le changement est voulu, supprimer %s et relancer.", goldenPath, goldenPath)
	}
}

// ─── T4.5 : limites ─────────────────────────────────────────────────────────

// TestT4_5_RecursionIsRangeError vérifie qu'une récursion sans fin produit une
// erreur du moteur et non un dépassement de pile du processus.
func TestT4_5_RecursionIsRangeError(t *testing.T) {
	_, err := compileAndRun(t, `function f() { return f(); } f();`, false)
	if err == nil {
		t.Fatal("une récursion sans fin devrait échouer")
	}
	if !strings.Contains(err.Error(), "Maximum call stack") {
		t.Errorf("erreur inattendue : %v", err)
	}
}

// TestT4_5_InfiniteLoopIsBounded vérifie que le quota d'instructions arrête une
// boucle sans fin.
func TestT4_5_InfiniteLoopIsBounded(t *testing.T) {
	prog, err := parser.Parse(`while (true) {}`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHeap()
	chunk, err := Compile(h, prog, "script")
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(h)
	vm.GasLeft = 100000
	if _, err := vm.Run(chunk); err == nil {
		t.Fatal("une boucle sans fin devrait épuiser le quota")
	} else if !strings.Contains(err.Error(), "quota") {
		t.Errorf("erreur inattendue : %v", err)
	}
}

func TestThrowIsCaughtByHost(t *testing.T) {
	_, err := compileAndRun(t, `throw "boum";`, false)
	if err == nil {
		t.Fatal("« throw » devrait remonter une erreur")
	}
	th, ok := err.(*Throw)
	if !ok {
		t.Fatalf("erreur de type %T, *Throw attendu : %v", err, err)
	}
	if th.Text != "boum" {
		t.Errorf("valeur lancée %q, « boum » attendu", th.Text)
	}
}

func TestThrowErrorObjectNamed(t *testing.T) {
	_, err := compileAndRun(t, `throw new TypeError("x");`, false)
	if err == nil {
		t.Fatal("throw attendu")
	}
	th, ok := err.(*Throw)
	if !ok {
		t.Fatalf("type %T, *Throw attendu", err)
	}
	if !strings.HasPrefix(th.Text, "TypeError") {
		t.Errorf("th.Text = %q, préfixe TypeError attendu", th.Text)
	}

	_, err = compileAndRun(t, `var e=new Error("z"); e.name="Test262Error"; throw e;`, false)
	if err == nil {
		t.Fatal("throw attendu")
	}
	th, ok = err.(*Throw)
	if !ok {
		t.Fatalf("type %T, *Throw attendu", err)
	}
	if !strings.Contains(th.Text, "Test262Error") {
		t.Errorf("th.Text = %q, Test262Error attendu", th.Text)
	}

	got, err := compileAndRun(t, `try { foo; } catch (e) { e instanceof ReferenceError }`, false)
	if err != nil {
		t.Fatalf("ReferenceError objet : %v", err)
	}
	if got != "true" {
		t.Errorf("obtenu %q, true attendu", got)
	}
}

// ─── Refus explicites ───────────────────────────────────────────────────────

// TestUnsupportedIsNamed vérifie que ce qui n'est pas compilé est REFUSÉ avec un
// motif, jamais compilé en code faux. C'est la différence entre une portée
// déclarée et un moteur qui ment.
func TestYieldStrictReserved(t *testing.T) {
	_, err := parser.Parse(`for ([x = yield] of [[]]);`, parser.Options{Strict: true})
	if err == nil {
		t.Fatal("SyntaxError attendu pour yield en mode strict")
	}
	if !strings.Contains(err.Error(), "yield") && !strings.Contains(err.Error(), "SyntaxError") {
		t.Errorf("erreur %v, yield/SyntaxError attendu", err)
	}
}

func TestObjectRestTrailingComma(t *testing.T) {
	_, err := parser.Parse(`for ({...rest,} of [{}]);`, parser.Options{})
	if err == nil {
		t.Fatal("SyntaxError attendu pour virgule après le reste objet")
	}
}

func TestForOfInDefault(t *testing.T) {
	got, err := compileAndRun(t, `var x; for ([ x = 'x' in {} ] of [[]]) {} x`, false)
	if err != nil {
		t.Fatalf("in dans un défaut : %v", err)
	}
	if got != "false" {
		t.Errorf("obtenu %q, false attendu", got)
	}
}

func TestSloppyUndeclaredAssign(t *testing.T) {
	got, err := compileAndRun(t, `unresolvable = 1; unresolvable`, false)
	if err != nil {
		t.Fatalf("affectation sloppy : %v", err)
	}
	if got != "1" {
		t.Errorf("obtenu %q, 1 attendu", got)
	}
}

func TestUnsupportedIsNamed(t *testing.T) {
	cases := map[string]string{
		`with (x) {}`: "with",
	}
	for src, want := range cases {
		_, err := compileAndRun(t, src, false)
		if err == nil {
			t.Errorf("%q compile alors qu'il ne devrait pas", src)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q : motif %q attendu dans %v", src, want, err)
		}
	}
}

func TestGenerator_NextYieldReturn(t *testing.T) {
	src := `
		function* g() { yield 1; yield 2; return 3; }
		var it = g();
		var a = it.next();
		var b = it.next();
		var c = it.next();
		a.value + b.value + c.value + (c.done ? 1 : 0)
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("générateur : %v", err)
	}
	if res != "7" {
		t.Errorf("attendu 7, obtenu %s", res)
	}
}

func TestGenerator_Send(t *testing.T) {
	src := `
		function* g() { var x = yield 10; return x; }
		var it = g();
		it.next();
		it.next(4).value
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("envoi : %v", err)
	}
	if res != "4" {
		t.Errorf("attendu 4, obtenu %s", res)
	}
}

func TestGenerator_YieldStarArray(t *testing.T) {
	src := `
		function* o() { yield* [4, 5]; }
		var it = o();
		it.next().value + it.next().value
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("yield* tableau : %v", err)
	}
	if res != "9" {
		t.Errorf("attendu 9, obtenu %s", res)
	}
}

func TestGenerator_YieldStarNested(t *testing.T) {
	src := `
		function* inner() { yield 2; }
		function* outer() { yield 1; yield* inner(); yield 3; }
		var it = outer();
		it.next().value + it.next().value + it.next().value
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("yield* imbriqué : %v", err)
	}
	if res != "6" {
		t.Errorf("attendu 6, obtenu %s", res)
	}
}

func TestGenerator_EmptyDone(t *testing.T) {
	src := `function* g() {} g().next().done`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("vide : %v", err)
	}
	if res != "true" {
		t.Errorf("attendu true, obtenu %s", res)
	}
}

func TestGenerator_ReturnMethod(t *testing.T) {
	src := `
		function* g() { yield 1; yield 2; }
		var it = g();
		it.next();
		it.return(9).value
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("return : %v", err)
	}
	if res != "9" {
		t.Errorf("attendu 9, obtenu %s", res)
	}
}

func TestGenerator_ThrowCaught(t *testing.T) {
	src := `
		function* g() { try { yield 1; } catch (e) { yield e; } }
		var it = g();
		it.next();
		it.throw("x").value
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("throw intercepté : %v", err)
	}
	if res != "x" {
		t.Errorf("attendu x, obtenu %s", res)
	}
}

func TestForOf(t *testing.T) {
	tests := []struct {
		name, source, expected string
	}{
		{"array", `var s=0; for (var x of [1,2,3]) s+=x; s`, "6"},
		{"string", `var s=""; for (var c of "ab") s+=c; s`, "ab"},
		{"break", `var s=0; for (var x of [1,2,3]) { if (x===2) break; s+=x; } s`, "1"},
		{"arguments", `function f(){ var s=0; for (var x of arguments) s+=x; return s; } f(1,2,3)`, "6"},
		{"custom", `var o={}; o[Symbol.iterator]=function(){ var i=0; return {next:function(){ i++; return i<=2?{value:i,done:false}:{done:true}; }}; }; var s=0; for (var x of o) s+=x; s`, "3"},
		{"computed_iterator_call", `var a=[1,2,3]; var s=0; for (var x of a[Symbol.iterator]()) s+=x; s`, "6"},
		{"dstr_arrow_name", `var f; for ([f = () => {}] of [[]]) {} f.name`, "f"},
		{"map_entries", `var m=new Map(); m.set(1,2); var s=0; for (var p of m) s+=p[0]+p[1]; s`, "3"},
		{"typedarray", `var a=new Int8Array([3,2,4,1]); var s=0; for (var x of a) s+=x; s`, "10"},
		{"args_mapped", `function f(a,b){ var out=""; for (var v of arguments){ out+=v; a=b; b=0; } return out; } f(1,2)`, "10"},
		{"args_unmapped_strict", `function f(a,b){ "use strict"; var out=""; for (var v of arguments){ out+=v; a=b; b=0; } return out; } f(1,2)`, "12"},
		{"put_error_closes", `var closed=0; var o={set p(v){ throw 1; }}; var it={next:function(){ return {value:0,done:false}; }, return:function(){ closed=1; return {done:true}; }}; var x={}; x[Symbol.iterator]=function(){ return it; }; try { for (o.p of x) {} } catch(e) {} closed`, "1"},
		{"eval_break", `eval("1; for (var a of [0]) { break; }")`, "undefined"},
		{"bind_call_hasown", `var h=Function.prototype.call.bind(Object.prototype.hasOwnProperty); h({a:1},"a")`, "true"},
		{"eval_same", `eval("1; for (var a of [0]) { break; }")===undefined`, "true"},
		{"string_undef", `String(undefined)`, "undefined"},
		{"string_num", `String(1)`, "1"},
		{"obj_computed", `var a=1; var o={[a]:2, bar:3}; o.bar`, "3"},
		{"obj_computed_key", `var a=1; var o={[a]:2}; o[1]`, "2"},
		{"eval_break_val", `eval("2; for (var b of [0]) { 3; break; }")`, "3"},
		{"eval_outer_cont", `eval("5; outer: do { for (var b of [0]) { 6; continue outer; } } while (false)")`, "6"},
		{"let_fresh", `let s=0; for (let x of [1,2]) s+=x; s`, "3"},
		{"var_redecl", `for (var x of [99]) { var x; } x`, "99"},
		{"astral", `var n=0; for (var c of "ab") n++; n`, "2"},
		{"arr_idx", `var x; for ([...{1:x}] of [[7,8,9]]) {} x`, "8"},
		{"anon_cls", `var cls; for ([cls = class {}] of [[]]) {} cls.name`, "cls"},
		{"destructure", `var s=0; for (const [k] of [[1],[2]]) s+=k; s`, "3"},
		{"empty", `var n=0; for (var x of []) n++; n`, "0"},
		{"generator", `function* g(){ yield 4; yield 5; } var s=0; for (var v of g()) s+=v; s`, "9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := compileAndRun(t, tt.source, false)
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if res != tt.expected {
				t.Errorf("%s: obtenu %q, %s attendu", tt.name, res, tt.expected)
			}
		})
	}
}

func TestForOfIteratorClose(t *testing.T) {
	tests := []struct {
		name, source, expected string
	}{
		{"break_custom_return", `var closed=0; var it={next:function(){ return {value:1,done:false}; }, return:function(){ closed=1; return {done:true}; }}; var o={}; o[Symbol.iterator]=function(){ return it; }; for (var x of o) { break; } closed`, "1"},
		{"break_generator_finally", `var fin=0; function* g(){ try { yield 1; } finally { fin=1; } } for (var x of g()) break; fin`, "1"},
		{"normal_exit_no_return", `var closed=0; var it={i:0, next:function(){ this.i++; return this.i<=2?{value:this.i,done:false}:{done:true}; }, return:function(){ closed=1; return {done:true}; }}; var o={}; o[Symbol.iterator]=function(){ return it; }; for (var x of o) {} closed`, "0"},
		{"break_no_return_method", `var it={next:function(){ return {value:1,done:false}; }}; var o={}; o[Symbol.iterator]=function(){ return it; }; for (var x of o) { break; } 1`, "1"},
		{"break_non_function_return", `var it={next:function(){ return {value:1,done:false}; }, return: 42}; var o={}; o[Symbol.iterator]=function(){ return it; }; try { for (var x of o) { break; } } catch(e) { e instanceof TypeError }`, "true"},
		{"throw_custom_return", `var closed=0; var it={next:function(){ return {value:1,done:false}; }, return:function(){ closed=1; return {done:true}; }}; var o={}; o[Symbol.iterator]=function(){ return it; }; try { for (var x of o) { throw 1; } } catch(e) {} closed`, "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := compileAndRun(t, tt.source, false)
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if res != tt.expected {
				t.Errorf("%s: obtenu %q, %s attendu", tt.name, res, tt.expected)
			}
		})
	}
}

func TestGenerator_ForOf(t *testing.T) {
	src := `function* g() { yield 10; yield 20; } var s=0; for (var v of g()) s+=v; s`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("for-of générateur : %v", err)
	}
	if res != "30" {
		t.Errorf("attendu 30, obtenu %s", res)
	}
}

func TestGenerator_ObjectMethod(t *testing.T) {
	src := `var o={*g(){ yield 3; yield 4; }}; var it=o.g(); it.next().value+it.next().value`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("méthode générateur : %v", err)
	}
	if res != "7" {
		t.Errorf("attendu 7, obtenu %s", res)
	}
}

func TestGenerator_LocalsAcrossYield(t *testing.T) {
	src := `
		function* g() { var a = 3; yield a; a = a + 4; yield a; }
		var it = g();
		it.next().value + it.next().value
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("locaux : %v", err)
	}
	if res != "10" {
		t.Errorf("attendu 10, obtenu %s", res)
	}
}

// TestNewAndThisConstructor vérifie le comportement complet de new et this.
func TestNewAndThisConstructor(t *testing.T) {
	src := `
		function Point(x, y) {
			this.x = x;
			this.y = y;
			this.sum = function() { return this.x + this.y; };
		}
		var p = new Point(10, 32);
		p.sum();
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("Échec exécution constructor new: %v", err)
	}
	if res != "42" {
		t.Errorf("Attendu 42, obtenu %v", res)
	}
}

func TestInstanceof(t *testing.T) {
	res, err := compileAndRun(t, `function C() {} var o = new C(); o instanceof C`, false)
	if err != nil {
		t.Fatalf("instanceof: %v", err)
	}
	if res != "true" {
		t.Errorf("o instanceof C : %s, true attendu", res)
	}
	res, err = compileAndRun(t, `function C() {} 1 instanceof C`, false)
	if err != nil {
		t.Fatalf("1 instanceof C: %v", err)
	}
	if res != "false" {
		t.Errorf("1 instanceof C : %s, false attendu", res)
	}
}

// TestPrototypeChainAndObjectCreate valide la résolution prototypique et Object.create.
func TestPrototypeChainAndObjectCreate(t *testing.T) {
	src := `
		var proto = { bonus: 100, compute: function(x) { return x + this.bonus; } };
		var obj = Object.create(proto);
		obj.bonus = 200;
		var parentProto = Object.getPrototypeOf(obj);
		obj.compute(50) + parentProto.bonus;
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("Échec exécution prototype chain: %v", err)
	}
	if res != "350" {
		t.Errorf("Attendu 350 (250 + 100), obtenu %v", res)
	}
}

// TestClassDeclarationAndMethods valide la déclaration de classes ES6 et leurs méthodes.
func TestClassDeclarationAndMethods(t *testing.T) {
	src := `
		class Calculator {
			constructor(base) {
				this.base = base;
			}
			add(n) {
				return this.base + n;
			}
			static version() {
				return 55;
			}
		}
		var c = new Calculator(1000);
		c.add(234) + Calculator.version();
	`
	res, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("Échec exécution class ES6: %v", err)
	}
	if res != "1289" {
		t.Errorf("Attendu 1289 (1234 + 55), obtenu %v", res)
	}
}

// TestAllDeclaredOpcodesAreImplemented vérifie qu'aucun opcode déclaré ne
// retourne « non implémenté ». C'est exactement le défaut relevé à l'audit sur
// l'ancien jeu : 31 déclarés pour 20 traités.
func TestAllDeclaredOpcodesAreImplemented(t *testing.T) {
	for op := Op(0); int(op) < NumOps; op++ {
		if op.Name() == "op(?)" {
			t.Errorf("opcode %d sans nom : l'énumération a un trou", op)
			continue
		}
		h := NewHeap()
		vm := NewVM(h)
		c := NewChunk("probe")
		c.Consts = append(c.Consts, Const{Kind: ConstNumber, Num: 1})
		vm.frames = []frame{{chunk: c, env: h.NewEnv(4, NoHandle)}}
		// Alimenter la pile pour que les opcodes binaires trouvent des opérandes.
		for i := 0; i < 4; i++ {
			vm.push(Int(1))
		}
		err := vm.step(&vm.frames[0], op, 0)
		if err != nil && strings.Contains(err.Error(), "non implémenté") {
			t.Errorf("opcode %s (%d) déclaré mais non implémenté", op.Name(), op)
		}
	}
}

func TestOpcodeWidthsAreConsistent(t *testing.T) {
	for op := Op(0); int(op) < NumOps; op++ {
		switch op.Width() {
		case 0, 2, 4:
		default:
			t.Errorf("opcode %s : largeur d'opérande %d, seules 0, 2 et 4 sont admises",
				op.Name(), op.Width())
		}
	}
}

// ─── T4.4 : aller-retour du bytecode ────────────────────────────────────────

// TestT4_4_DisassembleCoversAllCode vérifie que le désassembleur parcourt le
// code entier sans se désynchroniser. Une largeur d'opérande fausse se
// manifesterait par un décodage tronqué.
func TestT4_4_DisassembleCoversAllCode(t *testing.T) {
	for _, src := range programs {
		prog, err := parser.Parse(src, parser.Options{})
		if err != nil {
			continue
		}
		h := NewHeap()
		chunk, err := Compile(h, prog, "script")
		if err != nil {
			continue
		}
		d := chunk.Disassemble()
		if strings.Contains(d, "<tronqué>") {
			t.Errorf("désassemblage tronqué pour %s", oneLine(src))
		}
		if strings.Contains(d, "op(?)") {
			t.Errorf("opcode inconnu dans le code émis pour %s", oneLine(src))
		}
	}
}

func FuzzCompileAndRun(f *testing.F) {
	for _, s := range programs {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		// Invariant : aucune entrée ne panique. Une source refusée par le
		// parser, le compilateur ou l'interpréteur produit une erreur, jamais
		// une panique du processus.
		prog, err := parser.Parse(src, parser.Options{})
		if err != nil {
			return
		}
		h := NewHeap()
		chunk, err := Compile(h, prog, "script")
		if err != nil {
			return
		}
		vm := NewVM(h)
		vm.GasLeft = 200000
		vm.MaxDepth = 64
		_, _ = vm.Run(chunk)
	})
}

// ─── Tests Unitaires Try / Catch / Finally ───────────────────────────────────

func TestTryCatchFinally_UnitTests(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "try_catch_simple",
			source:   `var r = 0; try { throw 42; } catch(e) { r = e * 2; } r;`,
			expected: "84",
		},
		{
			name:     "try_catch_cross_function",
			source:   `function fail() { throw "err"; } function main() { try { fail(); return "non"; } catch(e) { return e + "_caught"; } } main();`,
			expected: "err_caught",
		},
		{
			name:     "try_finally_no_exception",
			source:   `var x = 1; try { x = 2; } finally { x = 3; } x;`,
			expected: "3",
		},
		{
			name:     "try_nested_catch_and_rethrow",
			source:   `var res = ""; try { try { throw "A"; } catch(e) { res += e; throw "B"; } } catch(e) { res += e; } res;`,
			expected: "AB",
		},
		{
			name:     "try_catch_anonymous",
			source:   `var r = 0; try { throw 10; } catch { r = 99; } r;`,
			expected: "99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := parser.Parse(tt.source, parser.Options{})
			if err != nil {
				t.Fatalf("Erreur de parsing pour %s: %v", tt.name, err)
			}
			h := NewHeap()
			chunk, err := Compile(h, prog, "test")
			if err != nil {
				t.Fatalf("Erreur de compilation pour %s: %v", tt.name, err)
			}
			vm := NewVM(h)
			val, err := vm.Run(chunk)
			if err != nil {
				t.Fatalf("Erreur d'exécution pour %s: %v", tt.name, err)
			}
			got := vm.toDisplayString(val)
			if got != tt.expected {
				t.Errorf("%s: attendu %q, obtenu %q", tt.name, tt.expected, got)
			}
		})
	}
}

func TestDestructuring(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "array_basic",
			source:   `var [a, b] = [1, 2]; a + b;`,
			expected: "3",
		},
		{
			name:     "array_iter_custom",
			source:   `var it={n:0,next:function(){ this.n++; return {value:this.n,done:this.n>2}; }}; var o={}; o[Symbol.iterator]=function(){ return it; }; var a,b; [a,b]=o; a+b`,
			expected: "3",
		},
		{
			name:     "array_iter_get_throw",
			source:   `var o={}; o[Symbol.iterator]=function(){ throw 7; }; var n=0; try { var [x]=o; } catch(e) { n=e; } n`,
			expected: "7",
		},
		{
			name:     "array_iter_close",
			source:   `var closed=0; var it={next:function(){ return {value:1,done:false}; }, return:function(){ closed=1; return {done:true}; }}; var o={}; o[Symbol.iterator]=function(){ return it; }; var x; [x]=o; closed`,
			expected: "1",
		},
		{
			name:     "array_iter_close_null",
			source:   `var it={next:function(){ return {value:1,done:false}; }, return:function(){ return null; }}; var o={}; o[Symbol.iterator]=function(){ return it; }; var n=""; try { var [x]=o; } catch(e) { n=e instanceof TypeError; } n`,
			expected: "true",
		},
		{
			name:     "object_basic",
			source:   `var {x, y} = {x: 10, y: 20}; x + y;`,
			expected: "30",
		},
		{
			name:     "array_default",
			source:   `var [c = 7] = []; c;`,
			expected: "7",
		},
		{
			name:     "array_default_not_triggered",
			source:   `var [c = 7] = [42]; c;`,
			expected: "42",
		},
		{
			name:     "object_default_shorthand",
			source:   `var {x = 10, y = 20} = {x: 5}; x + y;`,
			expected: "25",
		},
		{
			name:     "object_default_renamed",
			source:   `var {x: a = 10, y: b = 20} = {x: 5}; a + b;`,
			expected: "25",
		},
		{
			name:     "nested_array_in_array",
			source:   `var [a, [b, c]] = [1, [2, 3]]; a + b + c;`,
			expected: "6",
		},
		{
			name:     "nested_object_in_object",
			source:   `var {a: {b}} = {a: {b: 42}}; b;`,
			expected: "42",
		},
		{
			name:     "nested_array_in_object",
			source:   `var {a: [x, y]} = {a: [10, 20]}; x + y;`,
			expected: "30",
		},
		{
			name:     "nested_object_in_array",
			source:   `var [{x, y}] = [{x: 1, y: 2}]; x + y;`,
			expected: "3",
		},
		{
			name:     "array_elision",
			source:   `var [, b, , d] = [1, 2, 3, 4]; b + d;`,
			expected: "6",
		},
		{
			name:     "destructuring_in_function_scope",
			source:   `function test(arr) { var [a, b] = arr; return a * b; } test([6, 7]);`,
			expected: "42",
		},
		{
			name:     "assignment_destructuring_array",
			source:   `var a, b; [a, b] = [10, 20]; a + b;`,
			expected: "30",
		},
		{
			name:     "assignment_destructuring_object",
			source:   `var x, y; ({x, y} = {x: 100, y: 200}); x + y;`,
			expected: "300",
		},
		{
			name:     "array_rest",
			source:   `var [a, ...b] = [1, 2, 3]; a + b.length;`,
			expected: "3",
		},
		{
			name:     "object_rest",
			source:   `var {a, ...b} = {a: 1, b: 2, c: 3}; a + b.b + b.c;`,
			expected: "6",
		},
		{
			name:     "for_of_const_array",
			source:   `var s=0; for (const [k] of [[1],[2]]) s+=k; s`,
			expected: "3",
		},
		{
			name:     "for_of_const_object",
			source:   `var s=0; for (const {x} of [{x:1},{x:2}]) s+=x; s`,
			expected: "3",
		},
		{
			name:     "for_of_let_array",
			source:   `var s=""; for (let [c] of [["a"],["b"]]) s+=c; s`,
			expected: "ab",
		},
		{
			name:     "for_of_assign_array",
			source:   `var k; for ([k] of [[7],[8]]); k`,
			expected: "8",
		},
		{
			name:     "for_of_member_array",
			source:   `var o={}; for ([o.p] of [[7]]); o.p`,
			expected: "7",
		},
		{
			name:     "for_of_member_object",
			source:   `var o={}; for ({x:o.p} of [{x:8}]); o.p`,
			expected: "8",
		},
		{
			name:     "for_of_member_simple",
			source:   `var o={}; for (o.p of [9]); o.p`,
			expected: "9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := compileAndRun(t, tt.source, false)
			if err != nil {
				t.Fatalf("%s: erreur: %v", tt.name, err)
			}
			if res != tt.expected {
				t.Errorf("%s: attendu %q, obtenu %q", tt.name, tt.expected, res)
			}
		})
	}
}

func TestTypedArrayResizableForOf(t *testing.T) {
	got, err := compileAndRun(t, `var rab=new ArrayBuffer(10,{maxByteLength:20}); var w=new Uint8Array(rab); for (var i=0;i<10;i++) w[i]=i; var v=[]; for (var x of new Uint8Array(rab,0,3)) v.push(x); rab.resize(20); v.join(",")`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0,1,2" {
		t.Fatalf("attendu %q, obtenu %q", "0,1,2", got)
	}
}

func TestLexicalScopeHoistedAccess(t *testing.T) {
	src := `
	(function() {
		let x = 42;
		function getX() {
			return x;
		}
		function setX(v) {
			x = v;
		}
		if (getX() !== 42) return "e1";
		setX(100);
		if (getX() !== 100) return "e2";
		const c = "ok";
		function getC() {
			return c;
		}
		if (getC() !== "ok") return "e3";
		return "pass";
	})()
	`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("erreur d'exécution: %v", err)
	}
	if got != "pass" {
		t.Fatalf("attendu \"pass\", obtenu %q", got)
	}
}
