// SPDX-License-Identifier: BUSL-1.1

package engine

import "testing"

func TestJSRELodashNativeDetect(t *testing.T) {
	src := `/hasOwnProperty|(function).*?(?=\\\()| for .+?(?=\\\])/.test("function hasOwnProperty() { [native code] }")`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("native detect %q", got)
	}
}

func TestJSRELodashQuotedPath(t *testing.T) {
	src := `/(["'])(?:(?!\1)[^\\]|\\.)*?\1/.exec("'hello'")[0]`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'hello'" {
		t.Fatalf("quoted path %q", got)
	}
}

func TestJSRELodashLookaheadDot(t *testing.T) {
	src := `/(?=(?:\.|\[\])(?:\.|\[\]|$))/.test(".")`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("lookahead dot %q", got)
	}
}
