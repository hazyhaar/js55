// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFullSuiteExecution exécute la suite avec le MOTEUR RÉEL et confronte le
// résultat au manifeste doré. Le doré protégeait initialement une ligne de base
// à zéro cas passant, le moteur n'existant pas encore ; il protège désormais le
// compte réellement atteint. C'est cette bascule qui donne son sens au ratchet :
// il ne garde plus l'absence de moteur, il garde la conformité acquise.
func TestFullSuiteExecution(t *testing.T) {
	if os.Getenv("JS55_FAST") == "1" || testing.Short() {
		t.Skip("abréviation locale : JS55_FAST=1 ou -short ; la CI exécute la suite complète")
	}
	test262Root := "/devhoros/pkg/js55/testdata/test262"
	if _, err := os.Stat(test262Root); os.IsNotExist(err) {
		t.Skipf("Arbre test262 introuvable sous %s", test262Root)
	}

	cfg := RunnerConfig{
		Test262Root: test262Root,
		Engine:      NewJS55Engine(),
	}

	start := time.Now()
	rep, err := RunSuite(cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RunSuite a échoué : %v", err)
	}

	t.Logf("Exécution complète terminée en %v", elapsed)
	t.Log(rep.Summary())

	// Le parcours complet doit tenir sous deux minutes.
	if elapsed > 2*time.Minute {
		t.Errorf("Dépassement du budget temps de 2 minutes : %v", elapsed)
	}

	// Recoupements de structure et volumétrie mesurée
	if rep.Stats.TotalFiles == 0 || rep.Stats.TotalCases == 0 {
		t.Fatalf("Aucun cas mesuré lors de l'exécution")
	}

	// Vérification des 3 sous-ensembles requis
	if rep.ProxyStats.TotalFiles == 0 || rep.ProxyStats.TotalCases == 0 {
		t.Errorf("Compteurs Proxy vides")
	}
	if rep.ReflectStats.TotalFiles == 0 || rep.ReflectStats.TotalCases == 0 {
		t.Errorf("Compteurs Reflect vides")
	}
	if rep.Intl402Stats.TotalFiles == 0 || rep.Intl402Stats.TotalCases == 0 {
		t.Errorf("Compteurs intl402 vides")
	}

	// Emplacement du manifeste doré
	goldenPath := filepath.Join("testdata", "manifest.golden")
	goldenData, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		// Génération initiale du manifeste doré si inexistant
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("Création du répertoire testdata impossible : %v", err)
		}
		f, err := os.Create(goldenPath)
		if err != nil {
			t.Fatalf("Création de manifest.golden impossible : %v", err)
		}
		defer f.Close()
		if err := WriteManifest(f, rep.Cases); err != nil {
			t.Fatalf("Écriture du manifeste doré impossible : %v", err)
		}
		t.Logf("Manifeste doré généré sous %s avec %d cas", goldenPath, len(rep.Cases))
		return
	} else if err != nil {
		t.Fatalf("Lecture de manifest.golden impossible : %v", err)
	}

	// Lecture et validation du ratchet contre le doré existant
	goldenCases, err := ParseManifest(strings.NewReader(string(goldenData)))
	if err != nil {
		t.Fatalf("Parsing du manifeste doré impossible : %v", err)
	}

	ratchetRep, err := VerifyRatchet(goldenCases, rep.Cases)
	if err != nil {
		t.Fatalf("Échec du gate ratchet : %v", err)
	}

	if ratchetRep.HasRegressions() {
		t.Errorf("Régressions détectées : %s", ratchetRep.Error())
	}
}

func TestNilEngine_EvalVarX(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "harness"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "test"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness", "assert.js"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness", "sta.js"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	src := "/*---\nflags: [raw]\n---*/\nvar x=1;\n"
	if err := os.WriteFile(filepath.Join(root, "test", "varx.js"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	rep, err := RunSuite(RunnerConfig{Test262Root: root})
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if rep.Stats.Pass == 0 || rep.Stats.Fail != 0 {
		t.Fatalf("défaut JS55Engine: %s", rep.Stats)
	}
}
