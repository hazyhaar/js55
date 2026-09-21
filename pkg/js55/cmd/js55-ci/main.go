// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command js55-ci — Orchestrateur CI Unifié du Moteur JavaScript Souverain js55.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func runCommand(name, command string, args ...string) error {
	fmt.Printf("[CI Garde] %s...\n", name)
	start := time.Now()
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("❌ ÉCHEC Garde %s (%v):\n%s\n", name, elapsed, string(out))
		return err
	}
	fmt.Printf("✅ PASS Garde %s en %v\n", name, elapsed)
	return nil
}

func runStep(num int, name, pkg, runFilter string) error {
	fmt.Printf("[CI Étape %d/5] %s (%s)...\n", num, name, pkg)
	start := time.Now()
	args := []string{"test", "-race", "-count=1"}
	if runFilter != "" {
		args = append(args, "-run", runFilter)
	}
	args = append(args, pkg)

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOEXPERIMENT=simd")
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("❌ ÉCHEC Étape %d (%v):\n%s\n", num, elapsed, string(out))
		return err
	}
	fmt.Printf("✅ PASS Étape %d en %v\n", num, elapsed)
	return nil
}

func ensureFreshBinaries() error {
	fmt.Printf("[CI Garde] Garde Fraîcheur & Reconstruction des Binaires...\n")
	start := time.Now()

	targets := []struct {
		binPath string
		workDir string
		srcPkg  string
	}{
		{"/devhoros/bin/js55", "/devhoros", "./pkg/js55/cmd/js55"},
		{"/devhoros/bin/c2jsc", "/devhoros/c2simd", "./cmd/c2jsc"},
		{"/devhoros/bin/js55-ci", "/devhoros", "./pkg/js55/cmd/js55-ci"},
	}

	for _, t := range targets {
		cmd := exec.Command("go", "build", "-o", t.binPath, t.srcPkg)
		cmd.Dir = t.workDir
		cmd.Env = append(os.Environ(), "GOEXPERIMENT=simd")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("❌ ÉCHEC Reconstruction %s: %v\n%s\n", t.binPath, err, string(out))
			return err
		}
	}
	fmt.Printf("✅ PASS Garde Fraîcheur Binaires (js55, c2jsc, js55-ci à jour) en %v\n", time.Since(start))
	return nil
}

func main() {
	fmt.Println("=================================================================")
	fmt.Println("   js55 UNIFIED CI PIPELINE (Go 1.27, ARCHTIME-SIMD, Zero-CGO)  ")
	fmt.Println("=================================================================")
	globalStart := time.Now()

	// 0. Gardes Mécaniques de Licence, de Propreté et de Fraîcheur des Binaires
	if err := runCommand("Garde Licence C2VMM", "/devhoros/bin/check-license-guard"); err != nil {
		os.Exit(1)
	}
	if err := runCommand("Garde Propreté Échafaudages C2VMM", "/devhoros/bin/check-cleanliness-guard"); err != nil {
		os.Exit(1)
	}
	if err := ensureFreshBinaries(); err != nil {
		os.Exit(1)
	}

	steps := []struct {
		name      string
		pkg       string
		runFilter string
	}{
		{"Gardes d'Architecture csgguard (Table O(1), Cadre 96B)", "github.com/hazyhaar/js55/pkg/js55/csgguard", ""},
		{"Suite V8 mjsunit (Arithmétique, Portées, Récursion)", "github.com/hazyhaar/js55/pkg/js55/ci", "TestCI_03_V8_Mjsunit_Harness"},
		{"Suite d'Oracle Contradictoire Node.js V8 (SIMD AVX2)", "github.com/hazyhaar/c2pkg/c2jsc_oracle", ""},
		{"Banc Haute Densité 10 000 VMs & Concurrence Active", "github.com/hazyhaar/js55/pkg/js55/isolate", ""},
		{"Suite Officielle ECMAScript TC39 Test262 & Ratchet", "github.com/hazyhaar/js55/pkg/js55/conformance", ""},
	}

	for i, step := range steps {
		if err := runStep(i+1, step.name, step.pkg, step.runFilter); err != nil {
			fmt.Printf("\n❌ PIPELINE CI INTERROMPU À L'ÉTAPE %d EN %v\n", i+1, time.Since(globalStart))
			os.Exit(1)
		}
	}

	fmt.Println("=================================================================")
	fmt.Printf("🎉 CI UNIFIÉE VALIDÉE AVEC SUCCÈS À 100%% EN %v\n", time.Since(globalStart))
	fmt.Println("=================================================================")
}
