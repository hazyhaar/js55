// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"strings"
	"testing"
)

// TestRatchet_Nominal vérifie qu'un ensemble identique valide le gate.
func TestRatchet_Nominal(t *testing.T) {
	golden := []TestCase{
		{RelPath: "test/language/test1.js", Mode: ModeSloppy, Verdict: VerdictFail},
		{RelPath: "test/language/test1.js", Mode: ModeStrict, Verdict: VerdictFail},
	}
	current := []TestCase{
		{RelPath: "test/language/test1.js", Mode: ModeSloppy, Verdict: VerdictFail},
		{RelPath: "test/language/test1.js", Mode: ModeStrict, Verdict: VerdictFail},
	}

	report, err := VerifyRatchet(golden, current)
	if err != nil {
		t.Fatalf("Le gate aurait dû être vert : %v", err)
	}
	if report.HasRegressions() {
		t.Errorf("Aucune régression attendue")
	}
}

// TestRatchet_Progression vérifie qu'un cas devenant passant est accepté comme une progression sans échec du gate.
func TestRatchet_Progression(t *testing.T) {
	golden := []TestCase{
		{RelPath: "test/language/test1.js", Mode: ModeSloppy, Verdict: VerdictFail},
	}
	current := []TestCase{
		{RelPath: "test/language/test1.js", Mode: ModeSloppy, Verdict: VerdictPass},
	}

	report, err := VerifyRatchet(golden, current)
	if err != nil {
		t.Fatalf("Une progression ne doit pas faire échouer le ratchet : %v", err)
	}
	if len(report.Progressions) != 1 {
		t.Fatalf("1 progression attendue, obtenu %d", len(report.Progressions))
	}
	if report.Progressions[0].RelPath != "test/language/test1.js" {
		t.Errorf("Progression sur le mauvais chemin : %s", report.Progressions[0].RelPath)
	}
}

// TestRatchet_PreuveDePanneParRegression prouve mécaniquement la défaillance du gate lorsqu'un cas passant est perdu.
// Règle 8 : PREUVE DU GATE PAR SA PROPRE PANNE.
func TestRatchet_PreuveDePanneParRegression(t *testing.T) {
	// Doré de référence contenant un cas passant
	golden := []TestCase{
		{RelPath: "test/language/expressions/addition/valid-sum.js", Mode: ModeStrict, Verdict: VerdictPass},
		{RelPath: "test/language/expressions/addition/valid-sum.js", Mode: ModeSloppy, Verdict: VerdictPass},
		{RelPath: "test/language/expressions/bitwise/and.js", Mode: ModeSloppy, Verdict: VerdictFail},
	}

	// Cas 1 : Régression par passage de 'pass' à 'fail'
	currentWithFailure := []TestCase{
		{RelPath: "test/language/expressions/addition/valid-sum.js", Mode: ModeStrict, Verdict: VerdictFail}, // Régression !
		{RelPath: "test/language/expressions/addition/valid-sum.js", Mode: ModeSloppy, Verdict: VerdictPass},
		{RelPath: "test/language/expressions/bitwise/and.js", Mode: ModeSloppy, Verdict: VerdictFail},
	}

	report, err := VerifyRatchet(golden, currentWithFailure)
	if err == nil {
		t.Fatalf("PREUVE DE PANNE : Le gate aurait DÛ ÉCHOUER sur la régression 'valid-sum.js [strict]' !")
	}

	if !report.HasRegressions() {
		t.Fatalf("Le rapport doit signaler des régressions")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "test/language/expressions/addition/valid-sum.js") {
		t.Errorf("Le message d'erreur doit nommer le cas perdu, obtenu : %s", errMsg)
	}
	if !strings.Contains(errMsg, "strict") {
		t.Errorf("Le message d'erreur doit préciser le mode strict, obtenu : %s", errMsg)
	}

	// Cas 2 : Régression par disparition pure et simple du cas passant
	currentWithMissing := []TestCase{
		{RelPath: "test/language/expressions/addition/valid-sum.js", Mode: ModeSloppy, Verdict: VerdictPass},
		{RelPath: "test/language/expressions/bitwise/and.js", Mode: ModeSloppy, Verdict: VerdictFail},
	}

	reportMissing, errMissing := VerifyRatchet(golden, currentWithMissing)
	if errMissing == nil {
		t.Fatalf("PREUVE DE PANNE : Le gate aurait DÛ ÉCHOUER sur la disparition du cas 'valid-sum.js [strict]' !")
	}
	if len(reportMissing.Regressions) != 1 {
		t.Errorf("1 régression attendue pour cas manquant, obtenu %d", len(reportMissing.Regressions))
	}
}
