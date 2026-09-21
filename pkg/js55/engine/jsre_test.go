// SPDX-License-Identifier: BUSL-1.1

package engine

import "testing"

func TestJSRELookaroundNamedBackref(t *testing.T) {
	cases := []struct {
		pat, flags, input string
		want              bool
	}{
		{`a(?=b)`, "", "ab", true},
		{`a(?=b)`, "", "ac", false},
		{`a(?!b)`, "", "ac", true},
		{`a(?!b)`, "", "ab", false},
		{`(?<=a)b`, "", "ab", true},
		{`(?<=a)b`, "", "xb", false},
		{`(?<!a)b`, "", "xb", true},
		{`(?<!a)b`, "", "ab", false},
		{`(?<n>a)`, "", "a", true},
		{`(\w)\1`, "", "aa", true},
		{`(\w)\1`, "", "ab", false},
		{`a(?:b)c`, "", "abc", true},
	}
	for _, tc := range cases {
		p, err := compileJSRE(tc.pat, tc.flags)
		if err != nil {
			t.Fatalf("compile %q : %v", tc.pat, err)
		}
		got := p.FindStringSubmatchIndex(tc.input) != nil
		if got != tc.want {
			t.Errorf("%q on %q : got %v want %v", tc.pat, tc.input, got, tc.want)
		}
	}
}

func TestJSRENamedGroupCapture(t *testing.T) {
	p, err := compileJSRE(`(?<n>a)`, "")
	if err != nil {
		t.Fatal(err)
	}
	loc := p.FindStringSubmatchIndex("xa")
	if loc == nil || len(loc) < 4 {
		t.Fatalf("captures %v", loc)
	}
	if loc[2] < 0 || "xa"[loc[2]:loc[3]] != "a" {
		t.Fatalf("group %v", loc)
	}
}
