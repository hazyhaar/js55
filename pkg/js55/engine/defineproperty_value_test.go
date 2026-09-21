package engine

import (
	"strings"
	"testing"
)

func TestDefinePropertyValue(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:7}); o.a`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "7" {
		t.Errorf("obtenu %q, 7 attendu", got)
	}
}

func TestDefinePropertyEnumerableFalse(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:1,enumerable:false}); Object.keys(o).length`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0" {
		t.Errorf("obtenu %q, 0 attendu", got)
	}
}

func TestDefinePropertyWritableFalse(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:1,writable:false}); o.a=2; o.a`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1" {
		t.Errorf("obtenu %q, 1 attendu", got)
	}
}

func TestDefinePropertyEnumerableTrue(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:1,enumerable:true}); Object.keys(o).length`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1" {
		t.Errorf("obtenu %q, 1 attendu", got)
	}
}

func TestDefinePropertyWritableTrue(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:1,writable:true}); o.a=2; o.a`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2" {
		t.Errorf("obtenu %q, 2 attendu", got)
	}
}

func TestDefinePropertyConfigurableFalse(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:1,configurable:false}); delete o.a; o.a`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1" {
		t.Errorf("obtenu %q, 1 attendu", got)
	}
}

func TestObjectHasOwnPrototypeAbsent(t *testing.T) {
	got, err := compileAndRun(t, `Object.hasOwn.prototype`, false)
	if err != nil {
		t.Fatalf("erreur : %v", err)
	}
	if got != "undefined" {
		t.Errorf("obtenu %q, undefined attendu", got)
	}
}

func TestToLocaleStringPrototypeAbsent(t *testing.T) {
	got, err := compileAndRun(t, `Object.prototype.toLocaleString.prototype`, false)
	if err != nil {
		t.Fatalf("erreur : %v", err)
	}
	if got != "undefined" {
		t.Errorf("obtenu %q, undefined attendu", got)
	}
}

func TestDefinePropertyDescriptorBits(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:1}); var d=Object.getOwnPropertyDescriptor(o,"a"); ""+d.enumerable+d.writable+d.configurable`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "falsefalsefalse" {
		t.Errorf("obtenu %q, falsefalsefalse attendu", got)
	}
}

func TestDefinePropertyNonObject(t *testing.T) {
	_, err := compileAndRun(t, `Object.defineProperty(null,"a",{value:1})`, false)
	if err == nil {
		t.Fatal("defineProperty(null) devrait jeter")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet %v", err)
	}
	got, err := compileAndRun(t, `var n=false; try { Object.defineProperty(1,"a",{value:1}); } catch(e) { n=true; } n`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("defineProperty non-objet catch %q", got)
	}
}
