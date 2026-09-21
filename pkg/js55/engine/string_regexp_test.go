// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import "testing"

func TestStringReplaceRegexpGlobal(t *testing.T) {
	got, err := compileAndRun(t, `"abc".replace(/b/g, "X")`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "aXc" {
		t.Fatalf("replace %q", got)
	}
}

func TestStringReplaceEscapeSpecials(t *testing.T) {
	src := `var St=/[\\^$.*+?()[\]{}|]/g; "function hasOwnProperty() { [native code] }".replace(St, "\\$&")`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("escaped %q", got)
}

func TestStringReplaceNativeDetect(t *testing.T) {
	src := `Function.prototype.toString.call(Object.prototype.hasOwnProperty).replace(/hasOwnProperty|(function).*?(?:)| for .+?(?:)/g, "$1.*?")`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stripped %q", got)
	src2 := `Function.prototype.toString.call(Object.prototype.hasOwnProperty).replace(/hasOwnProperty|(function).*?(?=\\\()| for .+?(?=\\\])/g, "$1.*?")`
	got2, err := compileAndRun(t, src2, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("published %q", got2)
}

func TestLodashIsNativeSnippet(t *testing.T) {
	src := `
		var St=/[\\^$.*+?()[\]{}|]/g;
		var bl=Object.prototype.hasOwnProperty;
		var dl=Function.prototype.toString;
		var jl=dl.call(Object);
		var a=dl.call(bl).replace(St,"\\$&");
		var b=a.replace(/hasOwnProperty|(function).*?(?:)| for .+?(?:)/g,"$1.*?");
		var kl=RegExp("^"+b+"$");
		kl.test(jl) + "," + a + "," + b
	`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%q", got)
}

func TestStringSplitRegexp(t *testing.T) {
	cases := []struct{ src, want string }{
		{`"a b c".split(/ /).join(",")`, "a,b,c"},
		{`"a-b-c".split(/-/, 2).join(",")`, "a,b"},
		{`"hello".split(/(l)/).join(",")`, "he,l,,l,o"},
		{`"abc".split(/b/).join(",")`, "a,c"},
		{`"abc".split(/a/).join(",")`, ",bc"},
		{`"abc".split(/c/).join(",")`, "ab,"},
	}
	for _, tc := range cases {
		got, err := compileAndRun(t, tc.src, false)
		if err != nil {
			t.Fatalf("%s : %v", tc.src, err)
		}
		if got != tc.want {
			t.Fatalf("%s : %q, attendu %q", tc.src, got, tc.want)
		}
	}
}

func TestStringMatchSimple(t *testing.T) {
	got, err := compileAndRun(t, `"abc".match(/b/)[0]`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "b" {
		t.Fatalf("match %q", got)
	}
}
