package engine

import "testing"

func TestArrowCapturesSuperAndThis(t *testing.T) {
	src:=`class A{m(){return this.n+1;}}class B extends A{m(){return ()=>super.m();}}var b=new B();b.n=2;var f=b.m();f.call({n:99})===3 && f()===3`
	for _,stress:=range []bool{false,true} {
		got,err:=compileAndRun(t,src,stress)
		if err!=nil || got!="true" {t.Fatalf("stress=%v got %q, %v",stress,got,err)}
	}
}
