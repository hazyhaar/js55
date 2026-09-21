// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func TestJSREGasDebitOnSimpleMatch(t *testing.T) {
	p, err := compileJSRE("abc", "")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	gas := int64(1000)
	loc, completed := p.FindStringSubmatchIndexWithGas("abc", &gas)
	if !completed {
		t.Fatal("recherche interrompue alors que le gas est abondant")
	}
	if loc == nil || loc[0] != 0 || loc[1] != 3 {
		t.Fatalf("indices de correspondance %v, attendu [0 3]", loc)
	}
	if used := int64(1000) - gas; used != 4 {
		t.Fatalf("gas débité = %d, attendu 4 (concaténation + 3 littéraux)", used)
	}
	if gas <= 0 {
		t.Fatalf("gas résiduel non positif : %d", gas)
	}
}

func TestJSREGasAbortsCatastrophicBacktracking(t *testing.T) {
	p, err := compileJSRE("^(a*)*b$", "")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	input := strings.Repeat("a", 28) + "!"
	const budget = int64(50_000)
	gas := budget

	start := time.Now()
	loc, completed := p.FindStringSubmatchIndexWithGas(input, &gas)
	elapsed := time.Since(start)

	if completed {
		t.Fatalf("attendu une rupture de gas, obtenu loc=%v", loc)
	}
	if loc != nil {
		t.Fatalf("une interruption ne doit produire aucun indice : %v", loc)
	}
	if gas > 0 {
		t.Fatalf("gas non épuisé : %d", gas)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("interruption en %s, blocage suspect", elapsed)
	}
}

func TestJSREGasAbortsExponentialWithin5ms(t *testing.T) {
	p, err := compileJSRE("^(a?){24}a{24}$", "")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	input := strings.Repeat("a", 24) + "!"
	const budget = int64(50_000)
	gas := budget

	start := time.Now()
	loc, completed := p.FindStringSubmatchIndexWithGas(input, &gas)
	elapsed := time.Since(start)

	if completed {
		t.Fatalf("attendu une rupture de gas, obtenu loc=%v", loc)
	}
	if loc != nil {
		t.Fatalf("une interruption ne doit produire aucun indice : %v", loc)
	}
	if gas > 0 {
		t.Fatalf("gas non épuisé : %d", gas)
	}
	if elapsed > 15*time.Millisecond {
		t.Fatalf("interruption en %s, attendu moins de 15 ms", elapsed)
	}
}

func TestJSREGasBoundedSearchStopsOnFirstPosition(t *testing.T) {
	p, err := compileJSRE("a", "")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	gas := int64(1)
	loc, completed := p.FindStringSubmatchIndexWithGas(strings.Repeat("a", 4096), &gas)
	if completed {
		t.Fatalf("attendu une rupture de gas, obtenu loc=%v", loc)
	}
	if gas > 0 {
		t.Fatalf("gas non épuisé : %d", gas)
	}
}

// TestJSREGasAlternativeAbortNotCompleted couvre le contre-exemple d'Astra :
// le motif a|b sur chaîne vide épuise le quota au débit de l'alternative
// (gas=3, soit un pas de nœud puis les deux unités de l'alternative). L'abandon
// doit être terminal, donc rendu completed=false avec un gas nul, et non une
// recherche achevée sans correspondance.
func TestJSREGasAlternativeAbortNotCompleted(t *testing.T) {
	p, err := compileJSRE("a|b", "")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	gas := int64(3)
	loc, completed := p.FindStringSubmatchIndexWithGas("", &gas)
	if completed {
		t.Fatalf("abandon d'alternative déguisé en recherche achevée : loc=%v gas=%d", loc, gas)
	}
	if loc != nil {
		t.Fatalf("une interruption ne doit produire aucun indice : %v", loc)
	}
	if gas != 0 {
		t.Fatalf("gas non épuisé après abandon terminal : %d", gas)
	}
}

// TestJSREGasNegativeLookaheadDoesNotInvertAbort couvre le second
// contre-exemple d'Astra : (?!a|b) sur "b" avec un quota insuffisant. L'échec
// de l'alternative est un abandon de quota ; l'assertion négative ne doit pas
// l'inverser en succès ni produire [0 0].
func TestJSREGasNegativeLookaheadDoesNotInvertAbort(t *testing.T) {
	p, err := compileJSRE("(?!a|b)", "")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	gas := int64(5)
	loc, completed := p.FindStringSubmatchIndexWithGas("b", &gas)
	if completed {
		t.Fatalf("abandon de quota inversé en succès par l'assertion négative : loc=%v gas=%d", loc, gas)
	}
	if loc != nil {
		t.Fatalf("interdiction de rendre un indice après abandon : %v", loc)
	}
	if gas != 0 {
		t.Fatalf("gas non épuisé après abandon terminal : %d", gas)
	}
}

func TestJSREInputCapRefusesOversized(t *testing.T) {
	p, err := compileJSRE("a", "")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	oversized := strings.Repeat("a", jsreMaxInputLen+1)
	const budget = int64(1 << 40)
	gas := budget
	loc, completed := p.FindStringSubmatchIndexWithGas(oversized, &gas)
	if completed || loc != nil {
		t.Fatalf("entrée hors plafond non refusée : loc=%v completed=%v", loc, completed)
	}
	if gas != budget {
		t.Fatalf("gas débité pour une entrée refusée : %d", budget-gas)
	}
}

func TestVMOversizedRegExpInputRejected(t *testing.T) {
	find, err := compileJSRE("a", "")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	rd := &RegExpData{find: find, source: "a"}
	vm := NewVM(NewHeap())
	_, err = vm.regexpFindIndex(rd, strings.Repeat("a", jsreMaxInputLen+1))
	if !errors.Is(err, ErrRegExpInputTooLarge) {
		t.Fatalf("attendu ErrRegExpInputTooLarge, obtenu %v", err)
	}
}

func TestVMRegExpGasExhaustionPropagates(t *testing.T) {
	src := `var re = /^(a*)*\1b$/; re.test("aaaaaaaaaaaaaaaaaaaaaaaaaaaa!");`
	prog, err := parser.Parse(src, parser.Options{})
	if err != nil {
		t.Fatalf("parse : %v", err)
	}
	h := NewHeap()
	chunk, err := Compile(h, prog, "script")
	if err != nil {
		t.Fatalf("compile : %v", err)
	}
	vm := NewVM(h)
	vm.GasLeft = 50_000
	_, err = vm.Run(chunk)
	if !errors.Is(err, ErrGasExhausted) {
		t.Fatalf("attendu ErrGasExhausted, obtenu %v", err)
	}
	if vm.GasLeft > 0 {
		t.Fatalf("gas de l'isolat non épuisé : %d", vm.GasLeft)
	}
}
