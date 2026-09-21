// SPDX-License-Identifier: BUSL-1.1
package conformance

import (
	"os"
	"testing"
)

// Instrument T4.1 du plan : passage de la suite de conformité avec le moteur
// réel, et non plus avec le moteur vide.
//
// Le chiffre publié est un COMPTE, jamais un pourcentage d'appréciation
// (interdit I7). Le ratchet du pilote reste le gate ; ce test-ci mesure et
// publie, il ne juge pas.

const suiteRoot = "../testdata/test262"

func TestT4_1_ConformanceWithJS55Engine(t *testing.T) {
	if _, err := os.Stat(suiteRoot); err != nil {
		t.Skipf("arbre test262 absent (%s) ; le reconstituer par testdata/fetch_test262.sh", suiteRoot)
	}
	if testing.Short() || os.Getenv("JS55_FAST") == "1" {
		t.Skip("abréviation locale : JS55_FAST=1 ou -short ; la CI exécute la suite complète")
	}

	report, err := RunSuite(RunnerConfig{
		Test262Root: suiteRoot,
		Engine:      NewJS55Engine(),
	})
	if err != nil {
		t.Fatalf("exécution de la suite : %v", err)
	}

	t.Logf("\n%s", report.Summary())

	if report.Stats.Pass == 0 {
		t.Error("aucun cas ne passe : le moteur n'est pas branché sur le pilote")
	}
	// Le compte est publié pour être consigné au plan. Aucun seuil n'est posé
	// ici : le ratchet du manifeste doré est le gate, et il vit dans son propre
	// test.
}

// TestT4_1_StressModeAgreementOnSample vérifie sur un échantillon que le verdict
// ne dépend pas du mode de ramasse-miettes. La suite entière en mode stress
// serait trop lente ; l'échantillon est déterministe et nommé.
func TestT4_1_StressModeAgreementOnSample(t *testing.T) {
	if _, err := os.Stat(suiteRoot); err != nil {
		t.Skipf("arbre test262 absent (%s)", suiteRoot)
	}

	sample := []string{
		`var x = 1; if (x !== 1) { throw new Error("x"); }`,
		`function f(a) { return a * 2; } if (f(3) !== 6) { throw new Error("f"); }`,
		`var o = {a: 1}; if (o.a !== 1) { throw new Error("o"); }`,
		`var s = ""; for (var i = 0; i < 50; i++) { s += "x"; } if (s.length !== 50) { throw new Error("s"); }`,
		`function mk(n) { return function () { return n; }; } if (mk(7)() !== 7) { throw new Error("mk"); }`,
	}

	for i, src := range sample {
		normal := (&JS55Engine{Gas: 5_000_000, MaxDepth: 256}).Eval(src, false)
		stress := (&JS55Engine{Gas: 5_000_000, MaxDepth: 256, Stress: true}).Eval(src, false)

		if (normal == nil) != (stress == nil) {
			t.Errorf("échantillon %d : verdict différent selon le mode\n  source : %s\n  normal : %v\n  stress : %v",
				i, src, normal, stress)
		}
	}
}
