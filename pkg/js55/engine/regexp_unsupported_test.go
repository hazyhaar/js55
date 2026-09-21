// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"strings"
	"testing"
)

func TestRegExpLookaroundNamedGroupMatch(t *testing.T) {
	cases := []struct {
		src, want string
	}{
		{`new RegExp("a(?=b)").test("ab")`, "true"},
		{`new RegExp("a(?=b)").test("ac")`, "false"},
		{`new RegExp("a(?!b)").test("ac")`, "true"},
		{`new RegExp("(?<=a)b").test("ab")`, "true"},
		{`new RegExp("(?<!a)b").test("xb")`, "true"},
		{`new RegExp("(?<n>a)").exec("a")[1]`, "a"},
		{`new RegExp("(\\w)\\1").test("aa")`, "true"},
		{`new RegExp("(\\w)\\1").test("ab")`, "false"},
		{`/a(?=b)/.test("ab")`, "true"},
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

func TestRegExpInvalidStillSyntaxError(t *testing.T) {
	_, err := compileAndRun(t, `new RegExp("(")`, false)
	if err == nil || !strings.Contains(err.Error(), "SyntaxError") {
		t.Fatalf("SyntaxError attendu, obtenu %v", err)
	}
}

func TestRegExpNonCapturingAndClassStillCompile(t *testing.T) {
	got, err := compileAndRun(t, `new RegExp("a(?:b)").test("ab")`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("non-capturant %q", got)
	}
	got, err = compileAndRun(t, `new RegExp("[a(?=b)]").test("=")`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("classe %q", got)
	}
}
