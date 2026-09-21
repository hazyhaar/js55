package engine

import "testing"

func TestConstructorAndNativeArgumentsSurviveGC(t *testing.T) {
	for _, src := range []string{
		`class Matrix { constructor(){this.elements=[1,0,0,1];} } var m=new Matrix();m.elements[0]===1 && m.elements[3]===1`,
		`var a=new Float32Array([1,2,3]);a.set([4,5],1);a[0]===1 && a[1]===4 && a[2]===5`,
		`class A { constructor(){this.x=[1];} } class B extends A {constructor(){super();this.y=[2];}} var b=new B();b.x[0]===1 && b.y[0]===2`,
	} {
		got, err := compileAndRun(t, src, true)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}
