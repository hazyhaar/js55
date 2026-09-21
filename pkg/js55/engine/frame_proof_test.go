package engine

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestFrameFreshEnvRejectedHeaderThenRetry(t *testing.T) {
	vm, f := frameSetup(t, `function f(a,b){return a+b;} f;`)
	var charged int64
	denied := errors.New("header quota")
	vm.heap.QuotaTracker = func(n int64) error {
		if n == 96 {
			return denied
		}
		charged += n
		return nil
	}
	var caught any
	func() {
		defer func() { caught = recover() }()
		_, _ = vm.CallFunction(f, Undefined, []Value{Int(2), Int(3)})
	}()
	remaining := charged
	vm.heap.QuotaTracker = nil
	v, e := vm.CallFunction(f, Undefined, []Value{Int(3), Int(4)})
	if e != nil || v.ToInt() != 7 {
		t.Fatalf("fresh retry: %v %v", v, e)
	}
	if caught != denied || remaining != 0 {
		t.Fatalf("header denied=%v uninstalled slots charged=%d", caught, remaining)
	}
}

func TestFrameCompilerProofAndUntrustedChunks(t *testing.T) {
	for _, tc := range []struct {
		src  string
		safe bool
	}{
		{`function f(x){return x*2+1;} f;`, true},
		{`function f(x){this.x=x;return this;} f;`, true},
		{`function f(a,i){return a[i]+Math.sqrt(i);} f;`, true},
		{`function f(){return function(){return 1;};} f;`, false},
		{`function f(){return ()=>1;} f;`, false},
		{`function f(){return class X{};} f;`, false},
		{`function f(a){"use strict";return arguments;} f;`, false},
		{`function f(a){return arguments;} f;`, false},
		{`function f(){eval("1");return 2;} f;`, false},
		{`function* f(){yield 1;} f;`, false},
		{`async function f(){return 1;} f;`, false},
		{`function f(a){for(let x of a){if(x)return x;}return 0;} f;`, false},
	} {
		vm, f := frameSetup(t, tc.src)
		c := vm.heap.MustGet(f.Handle()).fn
		if c.SafeEnv != tc.safe || c.safeEnvVerified != tc.safe {
			t.Fatalf("%s proof=%v verified=%v", tc.src, c.SafeEnv, c.safeEnvVerified)
		}
	}
	vm, f := frameSetup(t, `function f(a){return a+1;} f;`)
	compiled := vm.heap.MustGet(f.Handle()).fn
	manual := NewChunk("decoded")
	// Decode public metadata into a fresh unit. The private compiler proof
	// cannot be supplied by JSON (or any external Chunk struct literal).
	if err := json.Unmarshal([]byte(`{"SafeEnv":true,"safeEnvVerified":true}`), manual); err != nil {
		t.Fatal(err)
	}
	if manual.SafeEnv || manual.safeEnvVerified {
		t.Fatal("decoded opt-in trusted")
	}
	manual.Code = compiled.Code
	manual.Consts = compiled.Consts
	manual.Locals = compiled.Locals
	manual.Params = compiled.Params
	manual.SafeEnv = true // even a forged public hint must retain the generic path
	v := ObjectValue(vm.heap.NewFunction(manual, NoHandle))
	vm.heap.AddRoot(&v)
	defer vm.heap.RemoveRoot(&v)
	for i := 0; i < 2; i++ {
		got, err := vm.CallFunction(v, Undefined, []Value{Int(int32(i))})
		if err != nil || got.ToInt() != int32(i+1) {
			t.Fatalf("manual fallback: %v %v", got, err)
		}
		if len(vm.safeEnvs) != 0 {
			t.Fatal("untrusted unit populated reusable pool")
		}
	}
}

func TestFrameReserveClearsCapacityAndHasBound(t *testing.T) {
	vm, f := frameSetup(t, `function f(a,b,c,d,e,f,g,h){return h;}function small(a){return a;}function rec(n){if(n>0)return rec(n-1);return 42;} f;`)
	obj := ObjectValue(vm.heap.NewObject())
	vm.heap.AddRoot(&obj)
	defer vm.heap.RemoveRoot(&obj)
	vm.heap.SetStress(true)
	args := []Value{obj, obj, obj, obj, obj, obj, obj, obj}
	v, e := vm.CallFunction(f, Undefined, args)
	if e != nil || v != obj {
		t.Fatalf("object return: %v %v", v, e)
	}
	small, _ := vm.GetGlobal("small")
	if _, e = vm.CallFunction(small, Undefined, []Value{Int(3)}); e != nil {
		t.Fatal(e)
	}
	vm.heap.Collect()
	for _, h := range vm.safeEnvs {
		o := vm.heap.MustGet(h)
		if o.proto != NoHandle || o.env != NoHandle || o.homeObject != NoHandle || o.fn != nil {
			t.Fatal("retained environment reference")
		}
		for _, v := range o.elements[:cap(o.elements)] {
			if !v.IsUndefined() {
				t.Fatal("retained value above active length")
			}
		}
	}
	vm.heap.SetStress(false)
	rec, _ := vm.GetGlobal("rec")
	v, e = vm.CallFunction(rec, Undefined, []Value{Int(90)})
	if e != nil || v.ToInt() != 42 {
		t.Fatalf("deep call: %v %v", v, e)
	}
	if len(vm.safeEnvs) > maxIdleFrameEnvs {
		t.Fatalf("unbounded idle pool: %d", len(vm.safeEnvs))
	}
	seen := map[Handle]bool{}
	for _, h := range vm.safeEnvs {
		if seen[h] {
			t.Fatal("double release")
		}
		seen[h] = true
	}
}

func TestFrameRunBoundariesOnThrowGasAndPanic(t *testing.T) {
	for _, src := range []string{`function fail(){throw 17;}fail();`, `function fail(){while(true){}}fail();`, `function fail(){return panicHost();}fail();`} {
		vm := NewVM(NewHeap())
		vm.SetGlobal("panicHost", ObjectValue(vm.heap.NewFunction(&Chunk{Native: func(*VM, []Value) (Value, error) { panic("native panic") }}, NoHandle)))
		vm.GasLeft = 100
		b := vm.frameBoundary()
		var err error
		var caught any
		func() { defer func() { caught = recover() }(); _, err = frameEval(t, vm, src) }()
		clean := len(vm.frames) == b.frames && vm.heap.StackLen() == b.stack && len(vm.thisStack) == b.this && len(vm.newStack) == b.news
		vm.GasLeft = 10000
		v, e := frameEval(t, vm, `function success(x){return x+1;} success(41);`)
		if e != nil || v.ToInt() != 42 {
			t.Fatalf("Run recovery: %v %v", v, e)
		}
		if err == nil && caught == nil {
			t.Fatal("rejection absent")
		}
		if !clean {
			t.Fatal("Run did not close its execution boundary")
		}
	}
}
