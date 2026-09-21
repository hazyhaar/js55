// SPDX-License-Identifier: BUSL-1.1

package engine

import "testing"

func TestDateUTCGettersCoherence(t *testing.T) {
	g := runGlobal(t, `var d=new Date(Date.UTC(2020,0,15,12,30,45,123)); var g=(d.getFullYear()===d.getUTCFullYear() && d.getMonth()===d.getUTCMonth() && d.getDate()===d.getUTCDate() && d.getDay()===d.getUTCDay() && d.getHours()===d.getUTCHours() && d.getMinutes()===d.getUTCMinutes() && d.getSeconds()===d.getUTCSeconds() && d.getMilliseconds()===d.getUTCMilliseconds())?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("getUTC cohérence locale")
	}
}

func TestDateUTCGettersComponents(t *testing.T) {
	g := runGlobal(t, `var d=new Date(Date.UTC(2020,0,15,12,30,45,123)); var g=d.getUTCFullYear();`)
	if g.ToInt() != 2020 {
		t.Fatalf("getUTCFullYear g=%v", g)
	}
	g = runGlobal(t, `var d=new Date(Date.UTC(2020,0,15,12,30,45,123)); var g=d.getUTCMonth();`)
	if g.ToInt() != 0 {
		t.Fatalf("getUTCMonth g=%v", g)
	}
	g = runGlobal(t, `var d=new Date(Date.UTC(2020,0,15,12,30,45,123)); var g=d.getUTCDate();`)
	if g.ToInt() != 15 {
		t.Fatalf("getUTCDate g=%v", g)
	}
	g = runGlobal(t, `var d=new Date(Date.UTC(2020,0,15,12,30,45,123)); var g=d.getUTCHours();`)
	if g.ToInt() != 12 {
		t.Fatalf("getUTCHours g=%v", g)
	}
	g = runGlobal(t, `var d=new Date(Date.UTC(2020,0,15,12,30,45,123)); var g=d.getUTCMinutes();`)
	if g.ToInt() != 30 {
		t.Fatalf("getUTCMinutes g=%v", g)
	}
	g = runGlobal(t, `var d=new Date(Date.UTC(2020,0,15,12,30,45,123)); var g=d.getUTCSeconds();`)
	if g.ToInt() != 45 {
		t.Fatalf("getUTCSeconds g=%v", g)
	}
	g = runGlobal(t, `var d=new Date(Date.UTC(2020,0,15,12,30,45,123)); var g=d.getUTCMilliseconds();`)
	if g.ToInt() != 123 {
		t.Fatalf("getUTCMilliseconds g=%v", g)
	}
	g = runGlobal(t, `var d=new Date(Date.UTC(2020,0,15)); var g=d.getUTCDay();`)
	if g.ToInt() != 3 {
		t.Fatalf("getUTCDay mercredi g=%v", g)
	}
}

func TestDateUTCGettersNaN(t *testing.T) {
	g := runGlobal(t, `var d=new Date(NaN); var g=(isNaN(d.getUTCFullYear()) && isNaN(d.getUTCMonth()) && isNaN(d.getUTCDate()) && isNaN(d.getUTCHours()))?1:0;`)
	if g.ToInt() != 1 {
		t.Fatal("getUTC NaN")
	}
}
