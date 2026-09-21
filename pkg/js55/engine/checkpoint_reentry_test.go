package engine

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func TestCheckpointRejectsReentryThenResumes(t *testing.T) {
	h:=NewHeap()
	vm:=NewVM(h)
	run:=func(src string) Value {
		t.Helper()
		p,err:=parser.Parse(src,parser.Options{})
		if err!=nil {t.Fatal(err)}
		c,err:=Compile(h,p,"checkpoint")
		if err!=nil {t.Fatal(err)}
		v,err:=vm.Run(c)
		if err!=nil {t.Fatal(err)}
		return v
	}
	f:=run(`var calls=0;function f(){calls++;return calls;}f`)
	h.AddRoot(&f)
	defer h.RemoveRoot(&f)
	rejected:=false
	vm.CheckpointEvery=1
	vm.OnCheckpoint=func()error {
		hook:=vm.OnCheckpoint
		vm.OnCheckpoint=nil
		_,err:=vm.CallFunction(f,Undefined,nil)
		vm.OnCheckpoint=hook
		if err==nil {t.Error("reentry accepted during checkpoint")}
		rejected=err!=nil
		return nil
	}
	if got:=run("1+1");got.ToInt()!=2 {t.Fatalf("evaluation changed: %v",got)}
	vm.OnCheckpoint=nil
	vm.CheckpointEvery=0
	if !rejected || run("calls").ToInt()!=0 {t.Fatal("checkpoint callback mutated the suspended execution")}
	v,err:=vm.CallFunction(f,Undefined,nil)
	if err!=nil || v.ToInt()!=1 {t.Fatalf("resume: %v %v",v,err)}
}
