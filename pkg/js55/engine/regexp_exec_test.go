// SPDX-License-Identifier: BUSL-1.1

package engine

import "testing"

func TestRegExpExecLastIndexUTF16Supplementary(t *testing.T) {
	g := runGlobal(t, `var r=/./ug; r.exec('\uD834\uDF06'); var g=r.lastIndex;`)
	if g.ToInt() != 2 {
		t.Fatalf("lastIndex unités UTF-16 g=%v", g)
	}
}

func TestRegExpTestLastIndexUTF16Supplementary(t *testing.T) {
	g := runGlobal(t, `var r=/./ug; r.test('\uD834\uDF06'); var g=r.lastIndex;`)
	if g.ToInt() != 2 {
		t.Fatalf("test lastIndex unités UTF-16 g=%v", g)
	}
}

func TestRegExpExecLastIndexUTF16BMP(t *testing.T) {
	g := runGlobal(t, `var r=/./g; r.exec('é'); var g=r.lastIndex;`)
	if g.ToInt() != 1 {
		t.Fatalf("lastIndex BMP g=%v", g)
	}
}

func TestRegExpExecIndexUTF16(t *testing.T) {
	g := runGlobal(t, `var m=/b/.exec('éb'); var g=m.index;`)
	if g.ToInt() != 1 {
		t.Fatalf("index UTF-16 g=%v", g)
	}
}

func TestRegExpExecLastIndexNegativeToLength(t *testing.T) {
	g := runGlobal(t, `var r=/(?:ab|cd)\d?/g; r.lastIndex=-1; var m=r.exec("aacd22 "); var g=(m[0]==="cd2" && r.lastIndex===5)?1:0;`)
	if g.ToInt() != 1 {
		t.Fatalf("ToLength négatif g=%v", g)
	}
}

func TestRegExpExecUnicodeSurrogateNoMatch(t *testing.T) {
	g := runGlobal(t, `var g=/\udf06/u.exec('\uD834\uDF06')===null?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("paire substitut sous u : correspondance attendue nulle")
	}
}

func TestRegExpExecLastIndexBeyondLengthResets(t *testing.T) {
	g := runGlobal(t, `var r=/a/g; r.lastIndex=99; var m=r.exec('a'); var g=(m===null && r.lastIndex===0)?1:0;`)
	if g.ToInt() != 1 {
		t.Fatalf("lastIndex hors borne g=%v", g)
	}
}

func TestRegExpExecIncompatibleThisTypeError(t *testing.T) {
	g := runGlobal(t, `var g=0; try { Object.prototype.exec=RegExp.prototype.exec; (1).exec("x"); } catch(e) { if (e instanceof TypeError) g=1; }`)
	if g.ToInt() != 1 {
		t.Fatal("TypeError attendu sur this incompatible")
	}
}

func TestRegExpExecLastIndexStartUTF16(t *testing.T) {
	g := runGlobal(t, `var r=/b/g; r.lastIndex=1; var m=r.exec('éb'); var g=(m!==null && m[0]==='b' && r.lastIndex===2)?1:0;`)
	if g.ToInt() != 1 {
		t.Fatalf("reprise lastIndex UTF-16 g=%v", g)
	}
}
