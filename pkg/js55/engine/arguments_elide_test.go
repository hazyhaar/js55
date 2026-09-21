// SPDX-License-Identifier: BUSL-1.1
package engine

import "testing"

func TestArgumentsObjectPreservedWhenUsed(t *testing.T) {
	for _, src := range []string{
		`function f(a){return arguments[0]+arguments.length} f(7,8,9)===10`,
		`function f(){return arguments.length} f()===0`,
		`function f(a){return arguments[1]} f(1,2)===2`,
		`function f(){ var s=0; for (var x of arguments) s+=x; return s; } f(1,2,3)===6`,
		`class A { constructor(a,b){ this.n=a+b; } } class B extends A { constructor(){ super(...arguments); } } (new B(2,3)).n===5`,
		`Function("return arguments[0]")(9)===9`,
		`function f(x){return (()=>arguments[0])();} f(7)===7`,
		`function f(x=arguments.length){return x;} f()===0`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}

func TestArgumentsObjectElidedWhenUnused(t *testing.T) {
	for _, src := range []string{
		`function f(a){return a+1} f(2)===3`,
		`function get(i){return i*2} var s=0; for(var i=0;i<10;i++)s+=get(i); s===90`,
		`var a=new Float32Array([1,2,3]); function gx(i){return a[i]} gx(1)===2`,
		`function outer(){ function inner(){ return arguments[0]; } return inner(4); } outer()===4`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}

func TestEvalCallStillRunsWhenArgumentsElidedLookupIsGlobal(t *testing.T) {
	got, err := compileAndRun(t, `function f(){ eval("1"); return 2; } f()===2`, false)
	if err != nil || got != "true" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
