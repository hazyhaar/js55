// SPDX-License-Identifier: BUSL-1.1

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
	v, err := iso.EvalContext(context.Background(), `var a = 2; a * 21`)
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

	if _, err := a.EvalContext(context.Background(), `var partagee = 1;`); err != nil {
		t.Fatal(err)
	}
	if _, err := b.EvalContext(context.Background(), `partagee`); err == nil {
		t.Error("une globale d'un isolat est visible depuis un autre : l'étanchéité est rompue")
	}
}

func TestGasLimitStopsInfiniteLoop(t *testing.T) {
	iso, _ := New(Config{GasLimit: 50000})
	_, err := iso.EvalContext(context.Background(), `while (true) {}`)
	if err == nil {
		t.Fatal("une boucle sans fin devrait épuiser le quota")
	}
	if !errors.Is(err, ErrExecutionQuotaExceeded) {
		t.Errorf("erreur inattendue : %v", err)
	}
}

func TestDepthLimitStopsInfiniteRecursion(t *testing.T) {
	iso, _ := New(Config{MaxDepth: 32})
	_, err := iso.EvalContext(context.Background(), `function f() { return f(); } f();`)
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
		_, e := iso.EvalContext(context.Background(), `while (true) {}`)
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
	_, err = iso2.EvalContext(context.Background(), `while (true) {}`)
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
	v, err := iso.EvalContext(context.Background(), src)
	if err != nil {
		return "", err
	}
	_ = engine.Undefined
	return v.String(), nil
}

// TestIsolate_Close_ReturnsErrClosed exige qu'après Close, toute évaluation
// rende ErrClosed de façon déterministe, sans panique sur un tas nil et sans
// confusion avec ErrInterrupted. Le périmètre couvre Eval, EvalContext,
// Compile, Execute et les accesseurs de tas et d'interpréteur.
func TestIsolate_Close_ReturnsErrClosed(t *testing.T) {
	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Une unité compilée avant Close fournit une cible opposable à Execute.
	chunk, err := iso.Compile(`1 + 1`, "close_probe", false)
	if err != nil {
		t.Fatalf("Compile avant Close : %v", err)
	}
	if err := iso.Close(); err != nil {
		t.Fatalf("Close : %v", err)
	}
	for _, src := range []string{"1+1", "'chaine'"} {
		v, err := iso.Eval(src)
		if !errors.Is(err, ErrClosed) {
			t.Errorf("Eval(%q) = %v, ErrClosed attendu", src, err)
		}
		if v != engine.Undefined {
			t.Errorf("Eval(%q) = %v, Undefined attendu", src, v)
		}
		if _, err := iso.EvalContext(context.Background(), src); !errors.Is(err, ErrClosed) {
			t.Errorf("EvalContext(%q) = %v, ErrClosed attendu", src, err)
		}
		if _, err := iso.Compile(src, "test", false); !errors.Is(err, ErrClosed) {
			t.Errorf("Compile(%q) = %v, ErrClosed attendu", src, err)
		}
	}
	if _, err := iso.Execute(context.Background(), chunk); !errors.Is(err, ErrClosed) {
		t.Errorf("Execute après Close = %v, ErrClosed attendu", err)
	}
	assertClosedPanic(t, "Heap()", func() { _ = iso.Heap() })
	assertClosedPanic(t, "VM()", func() { _ = iso.VM() })
}

// assertClosedPanic exige qu'un accès après Close lève une panique typée
// ErrClosed, et non une panique nil inintelligible.
func assertClosedPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		err, ok := r.(error)
		if !ok || !errors.Is(err, ErrClosed) {
			t.Errorf("%s après Close a paniqué %v, ErrClosed attendu", name, r)
		}
	}()
	fn()
}

// TestIsolate_StepLimit_GasLimit_Resolution fixe la règle de résolution : le
// minimum des deux quotas positifs l'emporte, et un quota négatif n'annule ni
// ne masque le quota positif de l'autre champ.
func TestIsolate_StepLimit_GasLimit_Resolution(t *testing.T) {
	isoMin, err := New(Config{StepLimit: 500, GasLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if got := isoMin.VM().GasLeft; got != 500 {
		t.Errorf("min des quotas : GasLeft = %d, 500 attendu", got)
	}

	isoNeg, err := New(Config{StepLimit: 500, GasLimit: -1})
	if err != nil {
		t.Fatal(err)
	}
	if got := isoNeg.VM().GasLeft; got != 500 {
		t.Errorf("quota négatif ignoré : GasLeft = %d, 500 attendu", got)
	}
}

// TestIsolate_EvalContext_Cancellation exige que l'annulation du contexte en
// vol coupe une boucle JS serrée par delà le quota d'instructions.
func TestIsolate_EvalContext_Cancellation(t *testing.T) {
	iso, err := New(Config{GasLimit: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = iso.EvalContext(ctx, `while (true) {}`)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("annulation en vol : %v, ErrInterrupted attendu", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("annulation trop lente : %s", elapsed)
	}
}

// TestIsolate_EvalContext_ReuseAfterCancellation exige qu'une annulation
// éphémère du contexte ne condamne pas l'isolat : le verrou d'interruption posé
// par le chien de garde doit redescendre et l'évaluation suivante doit rendre
// son résultat sans trace d'ErrInterrupted.
func TestIsolate_EvalContext_ReuseAfterCancellation(t *testing.T) {
	iso, err := New(Config{GasLimit: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if _, err := iso.EvalContext(ctx, `while (true) {}`); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("annulation en vol : %v, ErrInterrupted attendu", err)
	}

	v, err := iso.Eval(`1 + 1`)
	if err != nil {
		t.Fatalf("réemploi après annulation : %v, erreur nulle attendue (verrou non redescendu)", err)
	}
	if v.ToInt() != 2 {
		t.Fatalf("réemploi après annulation : %v, 2 attendu", v.ToInt())
	}
}

// TestIsolate_EvalTimeout_ReuseAfterTimeout exige qu'un dépassement de délai
// mur ne condamne pas l'isolat : le chien de garde de EvalContext redescend le
// verrou et l'évaluation suivante rend son résultat, dix fois de suite.
func TestIsolate_EvalTimeout_ReuseAfterTimeout(t *testing.T) {
	iso, err := New(Config{GasLimit: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	defer iso.Close()

	for i := 0; i < 10; i++ {
		_, err := iso.EvalTimeout(context.Background(), `while (true) {}`, 10*time.Millisecond)
		if err == nil {
			t.Fatal("attendu une interruption par timeout")
		}
		val, err := iso.Eval(`1 + 1`)
		if err != nil {
			t.Fatalf("échec réemploi à l'itération %d : %v", i, err)
		}
		if !val.IsNumber() || val.ToFloat() != 2 {
			t.Fatalf("résultat inattendu à l'itération %d : %v", i, val)
		}
	}
}
