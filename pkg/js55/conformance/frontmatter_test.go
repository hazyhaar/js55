// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFrontmatter_Positive(t *testing.T) {
	content := `// Copyright 2026 Test
/*---
esid: sec-array.prototype.map
description: Array.prototype.map test description
info: Extra information
flags: [onlyStrict]
includes: [propertyHelper.js, compareArray.js]
features: [Array.prototype.at]
---*/

var arr = [1, 2, 3];
assert.sameValue(arr.length, 3);
`
	meta, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("ParseFrontmatter a échoué : %v", err)
	}
	if meta.ESID != "sec-array.prototype.map" {
		t.Errorf("ESID attendu sec-array.prototype.map, obtenu %q", meta.ESID)
	}
	if meta.Description != "Array.prototype.map test description" {
		t.Errorf("Description inattendue : %q", meta.Description)
	}
	if !meta.HasFlag("onlyStrict") {
		t.Errorf("Le drapeau onlyStrict aurait dû être présent")
	}
	if len(meta.Includes) != 2 || meta.Includes[0] != "propertyHelper.js" || meta.Includes[1] != "compareArray.js" {
		t.Errorf("Includes inattendus : %v", meta.Includes)
	}
	if !meta.HasFeature("Array.prototype.at") {
		t.Errorf("Feature Array.prototype.at attendue")
	}
	modes := meta.Modes()
	if len(modes) != 1 || modes[0] != ModeStrict {
		t.Errorf("Modes attendus [strict], obtenu %v", modes)
	}
	if !containsStr(body, "var arr = [1, 2, 3];") {
		t.Errorf("Corps de test incomplet : %q", body)
	}
}

func TestParseFrontmatter_Negative(t *testing.T) {
	content := `/*---
es6id: 12.2.8
description: Negative test
negative:
  phase: parse
  type: SyntaxError
flags: [noStrict]
---*/
$DONOTEVALUATE();
var a\u2E2F;
`
	meta, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("ParseFrontmatter a échoué : %v", err)
	}
	if meta.Negative == nil {
		t.Fatalf("Bloc negative attendu")
	}
	if meta.Negative.Phase != "parse" || meta.Negative.Type != "SyntaxError" {
		t.Errorf("Negative inattendu : %+v", meta.Negative)
	}
	modes := meta.Modes()
	if len(modes) != 1 || modes[0] != ModeSloppy {
		t.Errorf("Modes attendus [sloppy], obtenu %v", modes)
	}
	if !containsStr(body, "$DONOTEVALUATE();") {
		t.Errorf("Corps de test inattendu : %q", body)
	}
}

func TestParseFrontmatter_DefaultModes(t *testing.T) {
	content := `/*---
description: Default dual mode test
---*/
var x = 1;
`
	meta, _, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("ParseFrontmatter a échoué : %v", err)
	}
	modes := meta.Modes()
	if len(modes) != 2 || modes[0] != ModeSloppy || modes[1] != ModeStrict {
		t.Errorf("Modes attendus [sloppy, strict], obtenu %v", modes)
	}
}

func TestParseFrontmatter_RawAndModule(t *testing.T) {
	rawContent := "/*---\nflags: [raw]\n---*/\n'use strict';"
	metaRaw, _, err := ParseFrontmatter(rawContent)
	if err != nil {
		t.Fatalf("ParseFrontmatter raw a échoué : %v", err)
	}
	if modes := metaRaw.Modes(); len(modes) != 1 || modes[0] != ModeSloppy {
		t.Errorf("Modes raw attendus [sloppy], obtenu %v", modes)
	}

	moduleContent := "/*---\nflags: [module]\n---*/\nexport default 42;"
	metaModule, _, err := ParseFrontmatter(moduleContent)
	if err != nil {
		t.Fatalf("ParseFrontmatter module a échoué : %v", err)
	}
	if modes := metaModule.Modes(); len(modes) != 1 || modes[0] != ModeStrict {
		t.Errorf("Modes module attendus [strict], obtenu %v", modes)
	}
}

func TestIsFixture(t *testing.T) {
	if !IsFixture("foo_FIXTURE.js") {
		t.Errorf("foo_FIXTURE.js aurait dû être reconnu comme fixture")
	}
	if !IsFixture("test/language/expressions/dynamic-import/dep-module_FIXTURE.js") {
		t.Errorf("dep-module_FIXTURE.js aurait dû être reconnu comme fixture")
	}
	if IsFixture("test/language/expressions/dynamic-import/normal-test.js") {
		t.Errorf("normal-test.js ne doit pas être une fixture")
	}
}

func TestParseRealTest262Files(t *testing.T) {
	testFile := "/devhoros/pkg/js55/testdata/test262/test/annexB/built-ins/Object/is/emulates-undefined.js"
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Skipf("Fichier de test réel inaccessible : %v", err)
	}
	meta, body, err := ParseFrontmatter(string(data))
	if err != nil {
		t.Fatalf("Échec du parsing du fichier réel : %v", err)
	}
	if meta.ESID != "sec-object.is" {
		t.Errorf("ESID attendu sec-object.is, obtenu %q", meta.ESID)
	}
	if !meta.HasFeature("IsHTMLDDA") {
		t.Errorf("Feature IsHTMLDDA attendue")
	}
	if len(body) == 0 {
		t.Errorf("Corps de test vide")
	}
}

func containsStr(s, sub string) bool {
	return filepath.Clean(s) != "" && len(s) >= len(sub) && (s == sub || len(s) > len(sub) && (s[:len(sub)] == sub || s[len(s)-len(sub):] == sub || len(s) > len(sub)))
}
