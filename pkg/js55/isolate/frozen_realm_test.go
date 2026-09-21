// SPDX-License-Identifier: BUSL-1.1
package isolate

import (
	"context"
	"sync"
	"testing"
)

// TestFrozenRealm_AllocationsCount valide l'éradication des 3 471 allocations physiques
// au profit du Frozen Root Realm partagé en Génération 0.
func TestFrozenRealm_AllocationsCount(t *testing.T) {
	// Préchauffer le RootRealm
	warm, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	_ = warm.Close()

	allocs := testing.AllocsPerRun(100, func() {
		iso, err := New(Config{})
		if err != nil {
			t.Fatal(err)
		}
		_ = iso.Close()
	})

	t.Logf("Allocations mesurées par instanciation d'isolat : %.1f (seuil cible < 25, historique = 3471)", allocs)
	if allocs > 25 {
		t.Fatalf("Trop d'allocations par isolat : %.1f > 25 (régression Frozen Realm)", allocs)
	}
}

// TestSecurity_CrossIsolatePrototypePollution vérifie l'étanchéité Copy-on-Write (CoW).
// Une modification hostile de prototypes builtins dans un isolat ne doit jamais polluer un autre isolat.
func TestSecurity_CrossIsolatePrototypePollution(t *testing.T) {
	ctx := context.Background()

	isoAttacker, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer isoAttacker.Close()

	// Tentatives multiples de pollution de prototype et d'écrasement de méthodes intrinsèques
	pollutionScript := `
		Object.prototype.polluted = "ATTACK_SUCCESSFUL";
		Array.prototype.customMethod = function() { return 42; };
		Function.prototype.hacked = true;
		Math.sin = function() { return 999; };
		JSON.parse = function() { return "hijacked"; };
	`
	_, err = isoAttacker.EvalContext(ctx, pollutionScript)
	if err != nil {
		t.Fatalf("L'attaquant a échoué à exécuter son script de pollution : %v", err)
	}

	// Vérifier que l'attaquant voit ses propres mutations
	val, err := isoAttacker.EvalContext(ctx, `({}).polluted`)
	if err != nil || isoAttacker.VM().ToStringValue(val).GoString() != "ATTACK_SUCCESSFUL" {
		t.Fatalf("L'attaquant n'a pas pu lire sa propre pollution locale : %v", err)
	}

	// Créer un isolat victime après l'attaque
	isoVictim, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer isoVictim.Close()

	checkVictimScript := `
		var results = {
			polluted: typeof ({}).polluted,
			arrayMethod: typeof [].customMethod,
			fnHacked: typeof (function(){}).hacked,
			mathSin: Math.sin(0),
			jsonVal: JSON.parse('{"a":1}').a
		};
		results.polluted === "undefined" &&
		results.arrayMethod === "undefined" &&
		results.fnHacked === "undefined" &&
		results.mathSin === 0 &&
		results.jsonVal === 1
	`
	v, err := isoVictim.EvalContext(ctx, checkVictimScript)
	if err != nil {
		t.Fatalf("Échec vérification victime : %v", err)
	}
	if isoVictim.VM().ToStringValue(v).GoString() != "true" {
		t.Fatal("ÉCHEC SÉCURITÉ : Pollution de prototype détectée dans un isolat tiers !")
	}
}

// TestIsolate_Pool_ZeroAllocs vérifie le recyclage avec sync.Pool et iso.Reset().
func TestIsolate_Pool_ZeroAllocs(t *testing.T) {
	ctx := context.Background()
	pool := sync.Pool{
		New: func() any {
			iso, err := New(Config{})
			if err != nil {
				panic(err)
			}
			return iso
		},
	}

	// 1. Mesure du Reset() pur sans réallocation
	isoInit := pool.Get().(*Isolate)
	_, _ = isoInit.EvalContext(ctx, `var x = 1 + 2;`)
	resetAllocs := testing.AllocsPerRun(100, func() {
		_ = isoInit.Reset()
	})
	t.Logf("Allocations mesurées par iso.Reset() pur : %.1f", resetAllocs)
	if resetAllocs > 1 {
		t.Fatalf("iso.Reset() alloue de la mémoire : %.1f > 1", resetAllocs)
	}
	pool.Put(isoInit)

	// 2. Mesure du cycle complet en pool avec chunk précompilé
	isoWorker := pool.Get().(*Isolate)
	chunk, err := isoWorker.Compile(`var y = 10 * 5; y;`, "pool_script", false)
	if err != nil {
		t.Fatal(err)
	}
	_ = isoWorker.Reset()
	pool.Put(isoWorker)

	allocs := testing.AllocsPerRun(100, func() {
		iso := pool.Get().(*Isolate)
		v, err := iso.Execute(ctx, chunk)
		if err != nil {
			t.Fatal(err)
		}
		if v.ToInt() != 50 {
			t.Fatalf("valeur attendue 50, obtenu %v", v)
		}
		if err := iso.Reset(); err != nil {
			t.Fatal(err)
		}
		pool.Put(iso)
	})

	t.Logf("Allocations par cycle d'exécution précompilée en pool : %.1f", allocs)
	if allocs > 15 {
		t.Fatalf("Trop d'allocations par cycle pool : %.1f > 15", allocs)
	}
}
