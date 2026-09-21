// SPDX-License-Identifier: BUSL-1.1
package engine

import "testing"

func TestSuperMethodUsesDefiningPrototype(t *testing.T) {
	for _, src := range []string{
		`class A {} A.prototype.x=42;class B extends A {constructor(){super();this.x=super.x;}} (new B()).x===42`,
		`class Sub extends Date {} var d=new Sub(0);d instanceof Sub && d instanceof Date && d.getTime()===0`,
		`class Sub extends RegExp {} var r=new Sub('a');r instanceof Sub && r instanceof RegExp && r.test('a')`,
		`class A { m(){return this.n;} } class B extends A { m(){return super.m()+1;} } class C extends B {} var c=new C();c.n=2;c.m()===3`,
		`class A { get value(){return this.n;} } class B extends A { m(){return super.value;} } class C extends B {} var c=new C();c.n=2;c.m()===2`,
		`class A { static m(){return this.n;} } class B extends A { static m(){return super.m()+1;} } class C extends B {} C.n=2;C.m()===3`,
		`class A { get value(){if(this.fail)throw new Error("reject");return this.n;} } class B extends A { m(){return super.value;} } var c=new B();c.fail=true;var rejected=false;try{c.m();}catch(e){rejected=true;}c.fail=false;c.n=2;rejected && c.m()===2`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}

func TestDetachedSuperMethodRetainsHomeDuringGC(t *testing.T) {
	src := `var method=(function(){class A{m(){return this.n;}}class B extends A{m(){return super.m()+1;}}return (new B()).m;})();method.call({n:2})===3`
	got, err := compileAndRun(t, src, true)
	if err != nil || got != "true" {
		t.Fatalf("got %q, %v", got, err)
	}
}
