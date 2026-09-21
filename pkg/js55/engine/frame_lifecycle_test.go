package engine

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func frameEval(t *testing.T, vm *VM, src string) (Value, error) {
	t.Helper()
	p, err := parser.Parse(src, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Compile(vm.heap, p, "frame-lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	return vm.Run(c)
}

func frameSetup(t *testing.T, src string) (*VM, Value) {
	t.Helper()
	vm := NewVM(NewHeap())
	v, err := frameEval(t, vm, src)
	if err != nil {
		t.Fatal(err)
	}
	return vm, v
}

func TestFrameQuotaPanicThenRetry(t *testing.T) {
	vm, small := frameSetup(t, `function small(a){return a;} function large(a,b,c,d,e,f,g,h){return a;} small;`)
	large, _ := vm.GetGlobal("large")
	if _, err := vm.CallFunction(small, Undefined, []Value{Int(3)}); err != nil {
		t.Fatal(err)
	}
	if len(vm.safeEnvs) != 1 {
		t.Fatalf("warm reserve: %d", len(vm.safeEnvs))
	}
	e := vm.safeEnvs[0]
	oldCap := cap(vm.heap.MustGet(e).elements)
	depth, stack, this, news := len(vm.frames), vm.heap.StackLen(), len(vm.thisStack), len(vm.newStack)
	denied := errors.New("test quota denied")
	vm.heap.QuotaTracker = func(n int64) error {
		if n > 0 {
			return denied
		}
		return nil
	}
	var caught any
	func() {
		defer func() { caught = recover() }()
		_, _ = vm.CallFunction(large, Undefined, []Value{Int(5)})
	}()
	unchanged := len(vm.safeEnvs) == 1 && vm.safeEnvs[0] == e && cap(vm.heap.MustGet(e).elements) == oldCap
	clean := len(vm.frames) == depth && vm.heap.StackLen() == stack && len(vm.thisStack) == this && len(vm.newStack) == news
	vm.heap.QuotaTracker = nil
	v, err := vm.CallFunction(large, Undefined, []Value{Int(11)})
	if err != nil || v.ToInt() != 11 {
		t.Fatalf("retry: %v %v", v, err)
	}
	vm.heap.Collect()
	v, err = vm.CallFunction(small, Undefined, []Value{Int(13)})
	if err != nil || v.ToInt() != 13 {
		t.Fatalf("post-GC recovery: %v %v", v, err)
	}
	if caught != denied || !unchanged || !clean {
		t.Fatalf("denied=%v reserve unchanged=%v stacks clean=%v", caught, unchanged, clean)
	}
}

func TestFrameGasAndDepthCleanup(t *testing.T) {
	for _, mode := range []string{"gas", "depth", "checkpoint-panic"} {
		t.Run(mode, func(t *testing.T) {
			vm, f := frameSetup(t, `function f(n){if(n<0){while(true){}} if(n>0)return f(n-1); return 42;} f;`)
			vm.MaxDepth = 12
			depth, stack, this, news := len(vm.frames), vm.heap.StackLen(), len(vm.thisStack), len(vm.newStack)
			arg := Int(100)
			if mode == "gas" {
				vm.GasLeft = 100
				arg = Int(-1)
			}
			if mode == "checkpoint-panic" {
				vm.CheckpointEvery = 1
				vm.OnCheckpoint = func() error { panic("checkpoint") }
			}
			var err error
			var caught any
			func() { defer func() { caught = recover() }(); _, err = vm.CallFunction(f, Undefined, []Value{arg}) }()
			clean := len(vm.frames) == depth && vm.heap.StackLen() == stack && len(vm.thisStack) == this && len(vm.newStack) == news && !vm.Yielding()
			vm.OnCheckpoint = nil
			vm.CheckpointEvery = 0
			vm.GasLeft = 10000
			v, e := vm.CallFunction(f, Undefined, []Value{Int(0)})
			if e != nil || v.ToInt() != 42 {
				t.Fatalf("same function recovery: %v %v", v, e)
			}
			if err == nil && caught == nil {
				t.Fatal("rejection not observed")
			}
			if !clean {
				t.Fatal("call boundary retained execution state")
			}
		})
	}
}

func TestFrameNativeReentryFailureThenOuterContinues(t *testing.T) {
	vm, _ := frameSetup(t, `function inner(x){if(x===-2)return panicInner();if(x<0)throw 17;return x+1;} function outer(x){var keep=x; return host()+keep+this.n;} 0;`)
	inner, _ := vm.GetGlobal("inner")
	vm.SetGlobal("panicInner", ObjectValue(vm.heap.NewFunction(&Chunk{Native: func(*VM, []Value) (Value, error) { panic("inner panic") }}, NoHandle)))
	vm.SetGlobal("host", ObjectValue(vm.heap.NewFunction(&Chunk{Name: "host", Native: func(vm *VM, args []Value) (Value, error) {
		d, s, ts, ns := len(vm.frames), vm.heap.StackLen(), len(vm.thisStack), len(vm.newStack)
		_, err := vm.CallFunction(inner, Undefined, []Value{Int(-1)})
		if err == nil {
			t.Error("missing inner rejection")
		}
		if len(vm.frames) != d || vm.heap.StackLen() != s || len(vm.thisStack) != ts || len(vm.newStack) != ns {
			t.Error("inner failure changed outer boundary")
		}
		var caught any
		func() {
			defer func() { caught = recover() }()
			_, _ = vm.CallFunction(inner, Undefined, []Value{Int(-2)})
		}()
		if caught != "inner panic" {
			t.Errorf("inner panic not propagated: %v", caught)
		}
		if len(vm.frames) != d || vm.heap.StackLen() != s || len(vm.thisStack) != ts || len(vm.newStack) != ns {
			t.Error("inner panic changed outer boundary")
		}
		vm.heap.Collect()
		return vm.CallFunction(inner, Undefined, []Value{Int(9)})
	}}, NoHandle)))
	vm.heap.SetStress(true)
	v, err := frameEval(t, vm, `var receiver={n:5,go:outer};receiver.go(7);`)
	if err != nil || v.ToInt() != 22 {
		t.Fatalf("outer recovery: %v %v", v, err)
	}
	v, err = vm.CallFunction(inner, Undefined, []Value{Int(40)})
	if err != nil || v.ToInt() != 41 {
		t.Fatalf("subsequent inner: %v %v", v, err)
	}
}

func TestFrameBehaviorNodeOracle(t *testing.T) {
	corpus := []string{
		`function set(x,y,z){this.x=x;this.y=y;this.z=z;return this.x+this.y+this.z;} var a={m:set}; a.m(1,2,3)+a.m(4,5,6);`,
		`function f(x){if(x<0)throw 19;return x+2;} var r=0;try{f(-1);}catch(e){r=e;} r+f(3);`,
		`function C(x){this.x=x;this.ok=new.target===C;} var a=new C(7);var b=new C(9);a.x+b.x+(a.ok&&b.ok?1:0);`,
		`function f(x){return function(){return x;};} var a=f(7),b=f(11); function churn(x){return x+1;} for(var i=0;i<40;i++)churn(i);a()+b();`,
		`function f(a){a=a+1;return arguments;}var a=f(7),b=f(20);Object.getOwnPropertyDescriptor(a,"0").value+Object.getOwnPropertyDescriptor(b,"0").value;`,
		`function* g(x){yield x;return x+1;}var a=g(5);var x=a.next().value;function f(x){return x*2;}f(99);x+a.next().value;`,
		`function f(){return ()=>7;} var c=f();function x(a){return a+1;}x(99);c();`,
		`function f(){try{eval("throw 7")}catch(e){return this.x+e;}}var a={x:5,f:f};a.f();`,
		`function* g(){yield 1;return 2;}function f(){var a=g();a.next();a.next();return this.x;}var a={x:9,f:f};a.f();`,
	}
	for _, src := range corpus {
		out, err := exec.Command("node", "-e", "console.log(eval("+quoteFrameJS(src)+"))").CombinedOutput()
		if err != nil {
			t.Fatalf("Node oracle: %s %v", out, err)
		}
		for _, stress := range []bool{false, true} {
			vm := NewVM(NewHeap())
			vm.heap.SetStress(stress)
			v, e := frameEval(t, vm, src)
			if e != nil || vm.toDisplayString(v) != strings.TrimSpace(string(out)) {
				t.Fatalf("stress=%v source=%s got=%v err=%v Node=%s", stress, src, v, e, out)
			}
			vm.heap.Collect()
			v, e = frameEval(t, vm, "21+21")
			if e != nil || v.ToInt() != 42 {
				t.Fatalf("post-GC: %v %v", v, e)
			}
		}
	}
}

func quoteFrameJS(s string) string { return "`" + s + "`" }

func TestFrameGeneratorSuspendCollectResume(t *testing.T) {
	vm, _ := frameSetup(t, `function* gen(x){var y=yield x;return x+y;}function numeric(x){return x+1;}var it=gen(5);var first=it.next().value;`)
	vm.heap.SetStress(true)
	if _, e := frameEval(t, vm, `numeric(3);numeric(7);`); e != nil {
		t.Fatal(e)
	}
	vm.heap.Collect()
	v, e := frameEval(t, vm, `first+it.next(7).value;`)
	if e != nil || v.ToInt() != 17 {
		t.Fatalf("generator after collect: %v %v", v, e)
	}
	v, e = frameEval(t, vm, `var bad=gen(9);bad.next();var caught=0;try{bad.throw(11);}catch(e){caught=e;}var again=gen(2);again.next();caught+again.next(3).value;`)
	if e != nil || v.ToInt() != 16 {
		t.Fatalf("generator rejection then recovery: %v %v", v, e)
	}
}

func TestFrameClosureCollectAndAsyncResume(t *testing.T) {
	vm, _ := frameSetup(t, `function hold(x){return function(){return x;};}var a=hold(7),b=hold(11);function numeric(x){return x+1;} 0;`)
	vm.heap.SetStress(true)
	for i := 0; i < 3; i++ {
		if _, e := frameEval(t, vm, `numeric(99);`); e != nil {
			t.Fatal(e)
		}
		vm.heap.Collect()
		v, e := frameEval(t, vm, `a()+b();`)
		if e != nil || v.ToInt() != 18 {
			t.Fatalf("captured values: %v %v", v, e)
		}
	}
	// The baseline's native Promise continuations are not rooted across full
	// collections (see the preserved baseline-known-issues.log). Exercise the
	// async fallback lifecycle without claiming to fix that independent bug.
	vm.heap.SetStress(false)
	_, e := frameEval(t, vm, `var settled=0;var release;var wait=new Promise(function(r){release=r;});async function af(x){await wait;return x+3;}af(8).then(function(v){settled=v;});numeric(70);`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = frameEval(t, vm, `release(1);`); e != nil {
		t.Fatal(e)
	}
	v, e := frameEval(t, vm, `settled;`)
	if e != nil || v.ToInt() != 11 {
		t.Fatalf("async resume: %v %v", v, e)
	}
}
