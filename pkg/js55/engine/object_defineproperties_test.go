// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"strings"
	"testing"
)

func TestObjectDefinePropertiesAndFromEntries(t *testing.T) {
	got, err := compileAndRun(t, `var o=Object.defineProperties({},{a:{value:1,enumerable:true},b:{value:2,enumerable:true}}); o.a+o.b`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "3" {
		t.Fatalf("defineProperties %q", got)
	}
	got, err = compileAndRun(t, `Object.fromEntries([["x",4],["y",5]]).x+Object.fromEntries([["x",4],["y",5]]).y`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "9" {
		t.Fatalf("fromEntries %q", got)
	}
	got, err = compileAndRun(t, `Array.isArray(Array.from([1,2])) && Array.from([1,2]).length===2`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("Array.from %q", got)
	}
}

func TestObjectGetOwnPropertyDescriptors(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:7,enumerable:true,writable:true,configurable:true}); var d=Object.getOwnPropertyDescriptors(o); d.a.value`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "7" {
		t.Fatalf("descriptors value %q", got)
	}
	got, err = compileAndRun(t, `var o={}; Object.defineProperty(o,"h",{value:3,enumerable:false}); Object.getOwnPropertyDescriptors(o).h.value+Object.keys(o).length`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "3" {
		t.Fatalf("descripteur non énumérable %q", got)
	}
	_, err = compileAndRun(t, `Object.getOwnPropertyDescriptors(null)`, false)
	if err == nil {
		t.Fatal("getOwnPropertyDescriptors(null) devrait jeter")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet %v", err)
	}
}

func TestObjectIs(t *testing.T) {
	got, err := compileAndRun(t, `Object.is(NaN,NaN) && Object.is(1,1) && !Object.is(1,2)`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("Object.is %q", got)
	}
}

func TestObjectIsFrozen(t *testing.T) {
	got, err := compileAndRun(t, `var o={a:1}; Object.isFrozen(o)===false && Object.isFrozen(Object.freeze(o))`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("isFrozen %q", got)
	}
}
