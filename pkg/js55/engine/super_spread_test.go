// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import "testing"

func TestSuperSpreadArguments(t *testing.T) {
	src := `class A { constructor(a,b){ this.n=a+b; } } class B extends A { constructor(){ super(...arguments); } } (new B(2,3)).n`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "5" {
		t.Fatalf("super spread %q", got)
	}
}

func TestSuperUsesActiveConstructor(t *testing.T) {
	for _, src := range []string{
		`class Base { constructor(){this.ok=true;} } class Texture extends Base { constructor(){super();} } class DataTexture extends Texture { constructor(){super();} } (new DataTexture()).ok`,
		`class Base { constructor(){this.n=arguments.length;} } class Texture extends Base {} class DataTexture extends Texture {} (new DataTexture(1,2,3,4,5)).n===5`,
		`class Base { constructor(fail){if(fail)throw new Error("reject");this.ok=true;} } class Texture extends Base { constructor(fail){super(fail);} } class DataTexture extends Texture {} var rejected=false;try{new DataTexture(true);}catch(e){rejected=true;} rejected && (new DataTexture(false)).ok`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: got %q, %v", src, got, err)
		}
	}
}
