// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"strings"
	"testing"
)

func TestHarnessLoader_RealHarness(t *testing.T) {
	harnessDir := "/devhoros/pkg/js55/testdata/test262/harness"
	hl := NewHarnessLoader(harnessDir)

	if err := hl.Preload(); err != nil {
		t.Fatalf("Échec du préchargement du harnais : %v", err)
	}

	assertCode, err := hl.Load("assert.js")
	if err != nil {
		t.Fatalf("Chargement de assert.js impossible : %v", err)
	}
	if !strings.Contains(assertCode, "Test262Error") {
		t.Errorf("assert.js ne contient pas Test262Error")
	}

	staCode, err := hl.Load("sta.js")
	if err != nil {
		t.Fatalf("Chargement de sta.js impossible : %v", err)
	}
	if len(staCode) == 0 {
		t.Errorf("sta.js est vide")
	}
}

func TestAssembleSource_Raw(t *testing.T) {
	harnessDir := "/devhoros/pkg/js55/testdata/test262/harness"
	hl := NewHarnessLoader(harnessDir)

	meta := &Metadata{
		Flags: []string{"raw"},
	}
	body := "var x = 1;"
	src, err := AssembleSource(hl, meta, body, ModeSloppy)
	if err != nil {
		t.Fatalf("AssembleSource a échoué : %v", err)
	}
	if src != body {
		t.Errorf("Mode raw altéré : attendu %q, obtenu %q", body, src)
	}
}

func TestAssembleSource_StrictAndIncludes(t *testing.T) {
	harnessDir := "/devhoros/pkg/js55/testdata/test262/harness"
	hl := NewHarnessLoader(harnessDir)

	meta := &Metadata{
		Includes: []string{"propertyHelper.js"},
	}
	body := "var testVar = 42;"
	src, err := AssembleSource(hl, meta, body, ModeStrict)
	if err != nil {
		t.Fatalf("AssembleSource a échoué : %v", err)
	}

	// Doit contenir assert.js, sta.js, propertyHelper.js puis "use strict";\nvar testVar = 42;
	if !strings.Contains(src, "Test262Error") {
		t.Errorf("assert.js manquant dans l'assemblage")
	}
	if !strings.Contains(src, "verifyProperty") {
		t.Errorf("propertyHelper.js manquant dans l'assemblage")
	}
	if !strings.Contains(src, "\"use strict\";\nvar testVar = 42;") {
		t.Errorf("Corps strict mal formé dans l'assemblage : %s", src)
	}
}

func TestAssembleSource_Async(t *testing.T) {
	harnessDir := "/devhoros/pkg/js55/testdata/test262/harness"
	hl := NewHarnessLoader(harnessDir)

	meta := &Metadata{
		Flags: []string{"async"},
	}
	body := "print('Test262:AsyncTestComplete');"
	src, err := AssembleSource(hl, meta, body, ModeSloppy)
	if err != nil {
		t.Fatalf("AssembleSource a échoué : %v", err)
	}

	if !strings.Contains(src, "Test262:AsyncTestComplete") {
		t.Errorf("doneprintHandle ou corps manquant : %s", src)
	}
}
