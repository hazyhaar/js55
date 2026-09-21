// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"strings"
	"testing"
)

func TestStringBuiltins_CharAtCodePointAt_HorsBMP(t *testing.T) {
	// charAt
	got, err := compileAndRun(t, `"a😀b".charAt(0)`, false)
	if err != nil || got != "a" {
		t.Fatalf("charAt(0) = %q, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".charAt(1)`, false)
	if err != nil || len(got) == 0 {
		t.Fatalf("charAt(1) = %q, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".charAt(-1)`, false)
	if err != nil || got != "" {
		t.Fatalf("charAt(-1) = %q, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".charAt(10)`, false)
	if err != nil || got != "" {
		t.Fatalf("charAt(10) = %q, err = %v", got, err)
	}

	// charCodeAt
	got, err = compileAndRun(t, `"a😀b".charCodeAt(0)`, false)
	if err != nil || got != "97" {
		t.Fatalf("charCodeAt(0) = %q, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".charCodeAt(1)`, false)
	if err != nil || got != "55357" { // 0xD83D
		t.Fatalf("charCodeAt(1) = %q, attendu 55357, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".charCodeAt(2)`, false)
	if err != nil || got != "56832" { // 0xDE00
		t.Fatalf("charCodeAt(2) = %q, attendu 56832, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".charCodeAt(10)`, false)
	if err != nil || got != "NaN" {
		t.Fatalf("charCodeAt(10) = %q, err = %v", got, err)
	}

	// codePointAt avec caractère hors BMP (😀 = 0x1F600 = 128512)
	got, err = compileAndRun(t, `"a😀b".codePointAt(0)`, false)
	if err != nil || got != "97" {
		t.Fatalf("codePointAt(0) = %q, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".codePointAt(1)`, false)
	if err != nil || got != "128512" { // 0x1F600
		t.Fatalf("codePointAt(1) hors BMP = %q, attendu 128512, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".codePointAt(2)`, false)
	if err != nil || got != "56832" { // demi-codet bas
		t.Fatalf("codePointAt(2) = %q, attendu 56832, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".codePointAt(3)`, false)
	if err != nil || got != "98" {
		t.Fatalf("codePointAt(3) = %q, err = %v", got, err)
	}
	got, err = compileAndRun(t, `"a😀b".codePointAt(10)`, false)
	if err != nil || got != "undefined" {
		t.Fatalf("codePointAt(10) = %q, err = %v", got, err)
	}
}

func TestStringConstructorStaticMethods(t *testing.T) {
	// fromCharCode
	got, err := compileAndRun(t, `String.fromCharCode(65, 66, 67)`, false)
	if err != nil || got != "ABC" {
		t.Fatalf("fromCharCode = %q, err = %v", got, err)
	}

	// fromCodePoint hors BMP (128512 = 0x1F600 😀, 128169 = 0x1F4A9 💩)
	got, err = compileAndRun(t, `String.fromCodePoint(97, 128512, 98)`, false)
	if err != nil || got != "a😀b" {
		t.Fatalf("fromCodePoint hors BMP = %q, err = %v", got, err)
	}

	// fromCodePoint RangeError sur codepoint invalide
	_, err = compileAndRun(t, `String.fromCodePoint(-1)`, false)
	if err == nil || !strings.Contains(err.Error(), "RangeError") {
		t.Fatalf("fromCodePoint(-1) attendu RangeError, obtenu: %v", err)
	}
	_, err = compileAndRun(t, `String.fromCodePoint(0x110000)`, false)
	if err == nil || !strings.Contains(err.Error(), "RangeError") {
		t.Fatalf("fromCodePoint(0x110000) attendu RangeError, obtenu: %v", err)
	}

	// raw
	got, err = compileAndRun(t, `String.raw({ raw: ["a", "b", "c"] }, 1, 2)`, false)
	if err != nil || got != "a1b2c" {
		t.Fatalf("String.raw = %q, err = %v", got, err)
	}
}

func TestStringPrototypeMethods(t *testing.T) {
	// indexOf / lastIndexOf / includes / startsWith / endsWith
	got, err := compileAndRun(t, `"hello world hello".indexOf("hello", 1)`, false)
	if err != nil || got != "12" {
		t.Fatalf("indexOf = %q", got)
	}
	got, err = compileAndRun(t, `"hello world hello".lastIndexOf("hello", 10)`, false)
	if err != nil || got != "0" {
		t.Fatalf("lastIndexOf = %q", got)
	}
	got, err = compileAndRun(t, `"hello world".includes("world")`, false)
	if err != nil || got != "true" {
		t.Fatalf("includes = %q", got)
	}
	got, err = compileAndRun(t, `"hello world".startsWith("hello")`, false)
	if err != nil || got != "true" {
		t.Fatalf("startsWith = %q", got)
	}
	got, err = compileAndRun(t, `"hello world".endsWith("world")`, false)
	if err != nil || got != "true" {
		t.Fatalf("endsWith = %q", got)
	}

	// slice / substring / substr / concat / repeat
	got, err = compileAndRun(t, `"abcdef".slice(1, 4)`, false)
	if err != nil || got != "bcd" {
		t.Fatalf("slice = %q", got)
	}
	got, err = compileAndRun(t, `"abcdef".slice(-3, -1)`, false)
	if err != nil || got != "de" {
		t.Fatalf("slice neg = %q", got)
	}
	got, err = compileAndRun(t, `"abcdef".substring(4, 1)`, false)
	if err != nil || got != "bcd" {
		t.Fatalf("substring swap = %q", got)
	}
	got, err = compileAndRun(t, `"abcdef".substr(2, 3)`, false)
	if err != nil || got != "cde" {
		t.Fatalf("substr = %q", got)
	}
	got, err = compileAndRun(t, `"a".concat("b", "c")`, false)
	if err != nil || got != "abc" {
		t.Fatalf("concat = %q", got)
	}
	got, err = compileAndRun(t, `"ab".repeat(3)`, false)
	if err != nil || got != "ababab" {
		t.Fatalf("repeat = %q", got)
	}

	// padStart / padEnd
	got, err = compileAndRun(t, `"5".padStart(3, "0")`, false)
	if err != nil || got != "005" {
		t.Fatalf("padStart = %q", got)
	}
	got, err = compileAndRun(t, `"5".padEnd(3, "0")`, false)
	if err != nil || got != "500" {
		t.Fatalf("padEnd = %q", got)
	}

	// trim / trimStart / trimEnd
	got, err = compileAndRun(t, `"  foo  ".trim()`, false)
	if err != nil || got != "foo" {
		t.Fatalf("trim = %q", got)
	}
	got, err = compileAndRun(t, `"  foo  ".trimStart()`, false)
	if err != nil || got != "foo  " {
		t.Fatalf("trimStart = %q", got)
	}
	got, err = compileAndRun(t, `"  foo  ".trimEnd()`, false)
	if err != nil || got != "  foo" {
		t.Fatalf("trimEnd = %q", got)
	}

	// toLowerCase / toUpperCase
	got, err = compileAndRun(t, `"HeLLo".toLowerCase()`, false)
	if err != nil || got != "hello" {
		t.Fatalf("toLowerCase = %q", got)
	}
	got, err = compileAndRun(t, `"HeLLo".toUpperCase()`, false)
	if err != nil || got != "HELLO" {
		t.Fatalf("toUpperCase = %q", got)
	}

	// split
	got, err = compileAndRun(t, `"a,b,c".split(",").join("-")`, false)
	if err != nil || got != "a-b-c" {
		t.Fatalf("split = %q", got)
	}
	got, err = compileAndRun(t, `"abc".split("").join(",")`, false)
	if err != nil || got != "a,b,c" {
		t.Fatalf("split empty = %q", got)
	}

	// replace / replaceAll
	got, err = compileAndRun(t, `"aba".replace("a", "x")`, false)
	if err != nil || got != "xba" {
		t.Fatalf("replace = %q", got)
	}
	got, err = compileAndRun(t, `"aba".replaceAll("a", "x")`, false)
	if err != nil || got != "xbx" {
		t.Fatalf("replaceAll = %q", got)
	}
}

func TestNumberBuiltins(t *testing.T) {
	got, err := compileAndRun(t, `(123.456).toFixed(2)`, false)
	if err != nil || got != "123.46" {
		t.Fatalf("toFixed = %q", got)
	}
	got, err = compileAndRun(t, `(123.456).toPrecision(4)`, false)
	if err != nil || got != "123.5" {
		t.Fatalf("toPrecision = %q", got)
	}
	got, err = compileAndRun(t, `(123.456).toExponential(2)`, false)
	if err != nil || got != "1.23e+02" {
		t.Fatalf("toExponential = %q", got)
	}
}
