// SPDX-License-Identifier: Apache-2.0 OR MIT

package isolate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hazyhaar/js55/pkg/js55/engine"
)

func TestIsolateEval(t *testing.T) {
	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	v, err := iso.Eval(context.Background(), `var a = 2; a * 21`)
	if err != nil {
		t.Fatalf("évaluation : %v", err)
	}
	if got := v.ToInt(); got != 42 {
		t.Errorf("résultat %d, 42 attendu", got)
	}
}

// TestIsolatesAreSealed vérifie l'étanchéité : une globale posée dans un isolat
// n'existe pas dans l'autre. C'est ce qui rend le modèle sûr sans verrou.
func TestIsolatesAreSealed(t *testing.T) {
	a, _ := New(Config{})
	b, _ := New(Config{})

	if _, err := a.Eval(context.Background(), `var partagee = 1;`); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Eval(context.Background(), `partagee`); err == nil {
		t.Error("une globale d'un isolat est visible depuis un autre : l'étanchéité est rompue")
	}
}

func TestGasLimitStopsInfiniteLoop(t *testing.T) {
	iso, _ := New(Config{GasLimit: 50000})
	_, err := iso.Eval(context.Background(), `while (true) {}`)
	if err == nil {
		t.Fatal("une boucle sans fin devrait épuiser le quota")
	}
	if !errors.Is(err, ErrExecutionQuotaExceeded) {
		t.Errorf("erreur inattendue : %v", err)
	}
}

func TestDepthLimitStopsInfiniteRecursion(t *testing.T) {
	iso, _ := New(Config{MaxDepth: 32})
	_, err := iso.Eval(context.Background(), `function f() { return f(); } f();`)
	if err == nil {
		t.Fatal("une récursion sans fin devrait être bornée")
	}
	if !strings.Contains(err.Error(), "Maximum call stack") {
		t.Errorf("erreur inattendue : %v", err)
	}
}

func TestInterruptRefusesExecution(t *testing.T) {
	iso, _ := New(Config{})
	chunk, err := iso.Compile(`1 + 1`, "t", false)
	if err != nil {
		t.Fatal(err)
	}
	iso.Interrupt()
	if _, err := iso.Execute(context.Background(), chunk); err == nil {
		t.Error("un isolat interrompu devrait refuser d'exécuter")
	} else if !errors.Is(err, ErrInterrupted) {
		t.Errorf("erreur inattendue : %v", err)
	}
}

func TestInterruptStopsRunningLoop(t *testing.T) {
	iso, err := New(Config{GasLimit: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		_, e := iso.Eval(context.Background(), `while (true) {}`)
		errCh <- e
	}()
	time.Sleep(50 * time.Millisecond)
	iso.Interrupt()
	select {
	case e := <-errCh:
		if !errors.Is(e, ErrInterrupted) {
			t.Fatalf("boucle interrompue en vol : %v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Interrupt n'a pas arrêté la boucle en moins de 2s")
	}

	iso2, err := New(Config{GasLimit: 50000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = iso2.Eval(context.Background(), `while (true) {}`)
	if !errors.Is(err, ErrExecutionQuotaExceeded) {
		t.Fatalf("sans Interrupt, quota attendu, obtenu %v", err)
	}
	if errors.Is(err, ErrInterrupted) {
		t.Fatal("sans Interrupt, ErrInterrupted est un faux positif")
	}
}

// TestStressModeAgreement exige le même résultat avec et sans mode stress du
// ramasse-miettes.
func TestStressModeAgreement(t *testing.T) {
	srcs := []string{
		`var o = {a: 1, b: 2}; o.a + o.b`,
		`function mk(n) { return function () { return n * 3; }; } mk(14)()`,
		`var s = ""; for (var i = 0; i < 40; i++) { s += "y"; } s.length`,
	}
	for _, src := range srcs {
		normal, errN := mustEval(t, Config{}, src)
		stress, errS := mustEval(t, Config{StressGC: true}, src)
		if (errN == nil) != (errS == nil) {
			t.Errorf("%s : erreur en un seul mode\n  normal : %v\n  stress : %v", src, errN, errS)
			continue
		}
		if normal != stress {
			t.Errorf("%s : %q en mode normal, %q en mode stress", src, normal, stress)
		}
	}
}

func mustEval(t *testing.T, cfg Config, src string) (string, error) {
	t.Helper()
	iso, err := New(cfg)
	if err != nil {
		return "", err
	}
	v, err := iso.Eval(context.Background(), src)
	if err != nil {
		return "", err
	}
	_ = engine.Undefined
	return v.String(), nil
}
