// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import "testing"

func TestDateParseISOAndUTC(t *testing.T) {
	g := runGlobal(t, `var g=Date.parse("1970-01-01T00:00:00Z");`)
	if g.ToInt() != 0 {
		t.Fatalf("parse epoch g=%v", g)
	}
	g = runGlobal(t, `var g=Date.UTC(1970,0,2);`)
	if g.ToInt() != 86400000 {
		t.Fatalf("UTC jour g=%v", g)
	}
	g = runGlobal(t, `var g=Date.parse("2020-01-15T00:00:00.000Z")-Date.UTC(2020,0,15);`)
	if g.ToInt() != 0 {
		t.Fatalf("parse ISO 2020 g=%v", g)
	}
	g = runGlobal(t, `var g=isNaN(Date.parse("pas-une-date"))?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("parse rejet")
	}
}

func TestDateParseRejectInvalid(t *testing.T) {
	g := runGlobal(t, `var g=isNaN(Date.parse(""))?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("parse chaîne vide")
	}
	g = runGlobal(t, `var g=isNaN(Date.parse("-000000-03-31T00:45Z"))?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("parse année zéro étendue")
	}
	g = runGlobal(t, `var g=isNaN(Date.parse())?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("parse sans argument")
	}
}

func TestDateTimeClip(t *testing.T) {
	g := runGlobal(t, `var g=isNaN(new Date(8.64e15+1).valueOf())?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("TimeClip au-delà du maximum")
	}
	g = runGlobal(t, `var g=isNaN(new Date(-8.64e15-1).valueOf())?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("TimeClip en deçà du minimum")
	}
	g = runGlobal(t, `var g=new Date(8.64e15).valueOf()-8.64e15;`)
	if g.ToInt() != 0 {
		t.Fatalf("TimeClip borne max g=%v", g)
	}
}

func TestDateNowPresence(t *testing.T) {
	g := runGlobal(t, `var n=Date.now(); var g=(typeof n==="number" && n===n && n>=0)?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("Date.now présence")
	}
}
