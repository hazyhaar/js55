// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"strings"
	"testing"
)

func TestArrayMapFilterReduceFind(t *testing.T) {
	got, err := compileAndRun(t, `[1,2,3].map(function(x){return x*2;}).join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2,4,6" {
		t.Fatalf("map %q", got)
	}
	got, err = compileAndRun(t, `[1,2,3].filter(function(x){return x>1;}).join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2,3" {
		t.Fatalf("filter %q", got)
	}
	got, err = compileAndRun(t, `[1,2,3].reduce(function(a,b){return a+b;},0)`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "6" {
		t.Fatalf("reduce %q", got)
	}
	got, err = compileAndRun(t, `[1,2,3].reduce(function(a,b){return a+b;})`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "6" {
		t.Fatalf("reduce sans init %q", got)
	}
	got, err = compileAndRun(t, `[1,2,3].find(function(x){return x>1;})`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2" {
		t.Fatalf("find %q", got)
	}
	_, err = compileAndRun(t, `[].reduce(function(a,b){return a+b;})`, false)
	if err == nil {
		t.Fatal("reduce tableau vide devrait jeter")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("reduce vide %v", err)
	}
}

func TestArrayMapFilterReduceFromGeneric(t *testing.T) {
	got, err := compileAndRun(t, `Array.prototype.map.call({0:12,1:11,length:2},function(x){return x>10;}).join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true,true" {
		t.Fatalf("map array-like %q", got)
	}
	got, err = compileAndRun(t, `var t={}; var r=[1,2].map(function(x){return this.k+x;}, {k:10}); r.join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "11,12" {
		t.Fatalf("map thisArg %q", got)
	}
	got, err = compileAndRun(t, `Array.prototype.filter.call({0:1,1:2,2:3,length:3},function(x){return x>1;}).join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2,3" {
		t.Fatalf("filter array-like %q", got)
	}
	got, err = compileAndRun(t, `Array.prototype.reduce.call({0:1,1:2,2:3,length:3},function(a,b){return a+b;},0)`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "6" {
		t.Fatalf("reduce array-like %q", got)
	}
	got, err = compileAndRun(t, `Array.from({0:41,1:42,length:2},function(x){return x*2;}).join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "82,84" {
		t.Fatalf("from mapfn %q", got)
	}
	got, err = compileAndRun(t, `var t={k:3}; Array.from([1,2], function(x){return this.k+x;}, t).join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "4,5" {
		t.Fatalf("from thisArg %q", got)
	}
	_, err = compileAndRun(t, `Array.from(null)`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("from null %v", err)
	}
	_, err = compileAndRun(t, `Array.from([], null)`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("from mapfn non callable %v", err)
	}
	got, err = compileAndRun(t, `Array.from.call(null, [7,8]).join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "7,8" {
		t.Fatalf("from this null %q", got)
	}
}

func TestArrayReverse(t *testing.T) {
	got, err := compileAndRun(t, `[1, 2, 3].reverse().join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "3,2,1" {
		t.Fatalf("reverse [1,2,3] = %q, attendu \"3,2,1\"", got)
	}
	got, err = compileAndRun(t, `var a = ["a", "b"]; var b = a.reverse(); a === b && a.join() === "b,a"`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("reverse in-place = %q", got)
	}
}

