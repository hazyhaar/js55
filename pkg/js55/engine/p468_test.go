// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"strings"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func TestRegExpLiteralTestExec(t *testing.T) {
	got, err := compileAndRun(t, `/foo/.test("foobar")`, false)
	if err != nil {
		t.Fatalf("test : %v", err)
	}
	if got != "true" {
		t.Errorf("/foo/.test : %q, true attendu", got)
	}
	got, err = compileAndRun(t, `/foo/.test("bar")`, false)
	if err != nil {
		t.Fatalf("test négatif : %v", err)
	}
	if got != "false" {
		t.Errorf("/foo/.test bar : %q, false attendu", got)
	}
	got, err = compileAndRun(t, `/a(b)c/.exec("xabcy")[1]`, false)
	if err != nil {
		t.Fatalf("exec : %v", err)
	}
	if got != "b" {
		t.Errorf("groupe capturé : %q, b attendu", got)
	}
	got, err = compileAndRun(t, `/FOO/i.test("foo")`, false)
	if err != nil {
		t.Fatalf("flag i : %v", err)
	}
	if got != "true" {
		t.Errorf("flag i : %q, true attendu", got)
	}
}

func TestDeleteOwnProperty(t *testing.T) {
	got, err := compileAndRun(t, `var o={a:1}; delete o.a; o.a`, false)
	if err != nil {
		t.Fatalf("delete : %v", err)
	}
	if got != "undefined" {
		t.Errorf("delete o.a : %q, undefined attendu", got)
	}
}

func TestOctalEscape(t *testing.T) {
	got, err := compileAndRun(t, `"\101"`, false)
	if err != nil {
		t.Fatalf("octal : %v", err)
	}
	if got != "A" {
		t.Errorf("\\101 : %q, A attendu", got)
	}
}

func TestMicrotaskDrainAfterRun(t *testing.T) {
	prog, err := parser.Parse(`var x=0; Promise.resolve(4).then(function(v){ x=v; });`, parser.Options{})
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
	got, ok := vm.GetGlobal("x")
	if !ok {
		t.Fatal("x absent")
	}
	if vm.toDisplayString(got) != "4" {
		t.Errorf("x après drain : %q, 4 attendu", vm.toDisplayString(got))
	}
}

func TestFunctionCallBind(t *testing.T) {
	got, err := compileAndRun(t, `Function.prototype.call.bind(function(a){ return this.x + a; })({x:10}, 3)`, false)
	if err != nil {
		t.Fatalf("call.bind : %v", err)
	}
	if got != "13" {
		t.Errorf("call.bind : %q, 13 attendu", got)
	}
	got, err = compileAndRun(t, `({a:1}).hasOwnProperty("a") && !({a:1}).hasOwnProperty("b")`, false)
	if err != nil {
		t.Fatalf("hasOwnProperty : %v", err)
	}
	if got != "true" {
		t.Errorf("hasOwnProperty : %q, true attendu", got)
	}
}

func TestReflectApplyOwnKeys(t *testing.T) {
	got, err := compileAndRun(t, `Reflect.apply(function(a,b){ return a+b; }, null, [2, 3])`, false)
	if err != nil {
		t.Fatalf("apply : %v", err)
	}
	if got != "5" {
		t.Errorf("apply : %q, 5 attendu", got)
	}
	got, err = compileAndRun(t, `Reflect.ownKeys({a:1,b:2}).length`, false)
	if err != nil {
		t.Fatalf("ownKeys : %v", err)
	}
	if got != "2" {
		t.Errorf("ownKeys length : %q, 2 attendu", got)
	}
	got, err = compileAndRun(t, `Reflect.getPrototypeOf({}) === Object.prototype`, false)
	if err != nil {
		t.Fatalf("getPrototypeOf : %v", err)
	}
	if got != "true" {
		t.Errorf("getPrototypeOf : %q, true attendu", got)
	}
}

func TestReflectGetHas(t *testing.T) {
	got, err := compileAndRun(t, `Reflect.get({a: 7}, "a")`, false)
	if err != nil {
		t.Fatalf("Reflect.get : %v", err)
	}
	if got != "7" {
		t.Errorf("Reflect.get : %q, 7 attendu", got)
	}
	got, err = compileAndRun(t, `Reflect.has({a: 1}, "a") && !Reflect.has({a: 1}, "b")`, false)
	if err != nil {
		t.Fatalf("Reflect.has : %v", err)
	}
	if got != "true" {
		t.Errorf("Reflect.has : %q, true attendu", got)
	}
}

func TestProxyGetTrap(t *testing.T) {
	src := `var p = new Proxy({a: 1}, {get: function(t, k) { return 42; }}); p.a`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatalf("Proxy get : %v", err)
	}
	if got != "42" {
		t.Errorf("trap get : %q, 42 attendu", got)
	}
}

func TestBigIntLiteral(t *testing.T) {
	got, err := compileAndRun(t, `1n`, false)
	if err != nil {
		t.Fatalf("1n : %v", err)
	}
	if got != "1" {
		t.Errorf("1n : %q, 1 attendu", got)
	}
	got, err = compileAndRun(t, `typeof 1n`, false)
	if err != nil {
		t.Fatalf("typeof 1n : %v", err)
	}
	if got != "bigint" {
		t.Errorf("typeof 1n : %q, bigint attendu", got)
	}
	got, err = compileAndRun(t, `1n === 1n`, false)
	if err != nil {
		t.Fatalf("1n === 1n : %v", err)
	}
	if got != "true" {
		t.Errorf("1n === 1n : %q, true attendu", got)
	}
}

func TestPromiseResolveThen(t *testing.T) {
	src := `var g = 0; Promise.resolve(42).then(function(v) { g = v; });`
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
		t.Fatalf("Run : %v", err)
	}
	g, ok := vm.GetGlobal("g")
	if !ok {
		t.Fatal("globale g absente")
	}
	if g.ToInt() != 42 {
		t.Errorf("g=%v, 42 attendu après vidage des microtâches", g)
	}
}

func TestPromiseExecutorAndCatch(t *testing.T) {
	src := `var g = 0; new Promise(function(resolve, reject) { reject(7); }).catch(function(v) { g = v; });`
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
		t.Fatalf("Run : %v", err)
	}
	g, ok := vm.GetGlobal("g")
	if !ok {
		t.Fatal("globale g absente")
	}
	if g.ToInt() != 7 {
		t.Errorf("g=%v, 7 attendu", g)
	}
}

func TestFunctionCtor(t *testing.T) {
	got, err := compileAndRun(t, `Function("return 1")()`, false)
	if err != nil {
		t.Fatalf("Function() : %v", err)
	}
	if got != "1" {
		t.Errorf("Function return 1 : %q", got)
	}
	got, err = compileAndRun(t, `new Function("a","b","return a+b")(2,3)`, false)
	if err != nil {
		t.Fatalf("new Function : %v", err)
	}
	if got != "5" {
		t.Errorf("new Function : %q", got)
	}
	got, err = compileAndRun(t, `Function("return arguments[0]")(9)`, false)
	if err != nil {
		t.Fatalf("Function arguments : %v", err)
	}
	if got != "9" {
		t.Errorf("Function arguments : %q", got)
	}
}

func TestJSONStringifyCycle(t *testing.T) {
	_, err := compileAndRun(t, `var o={}; o.a=o; JSON.stringify(o)`, false)
	if err == nil {
		t.Fatal("cycle JSON devrait jeter")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Errorf("cycle : %v", err)
	}
	got, err := compileAndRun(t, `JSON.stringify({a:1,b:undefined})`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"a":1}` {
		t.Errorf("omit undefined : %q", got)
	}
}

func TestReflectApplyArrayLike(t *testing.T) {
	got, err := compileAndRun(t, `Reflect.apply(function(a,b){ return a+b; }, null, {0:2,1:3,length:2})`, false)
	if err != nil {
		t.Fatalf("apply array-like : %v", err)
	}
	if got != "5" {
		t.Errorf("apply array-like : %q", got)
	}
}
