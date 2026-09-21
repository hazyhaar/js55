# M13 — Timeout `node:vm` et file de microtâches

Statut : ouverte. Dépend M02. Contexte 32k.

## Objectif

`vm.runInContext(src, ctx, {timeout: N})` : Node coupe par `ERR_SCRIPT_EXECUTION_TIMEOUT`. L’isolat a déjà un *gas* (`GasLimit`) et une file de microtâches (`isolate/microtask.go`). Aligner : timeout mur → interruption, `Promise.then` drainé après `Run` comme aujourd’hui `TestMicrotaskDrainAfterRun`, sans exécuter les timers macrotask (hors périmètre).

## Hors périmètre

`setTimeout` / `setImmediate` Node complets, `breakOnSigint`, worker threads.

## Lire (borné)

- `/devhoros/pkg/js55/isolate/isolate.go`
- `/devhoros/pkg/js55/isolate/microtask.go`
- `/devhoros/pkg/js55/engine/p468_test.go` offset 62 limit 25
- `Grep "GasLeft|Interrupted" /devhoros/pkg/js55/engine/vm.go` puis Read 40 lignes

## Écrire

- API mince : `EvalTimeout(ctx, src, d time.Duration)` ou option sur `Config` déjà existante — ne pas dupliquer le gas. Le délai mur **et** le gas doivent tous deux interrompre.
- Tests : (1) boucle infinie + 50 ms → erreur d’interruption, pas un hang ; (2) `Promise.resolve().then` complète après Eval ; (3) script court << timeout = même valeur que Node.

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestVmTimeout|TestMicrotask|TestT4_5' ./pkg/js55/isolate/ ./pkg/js55/engine/
```

Invariant cycle de vie : après timeout, un **nouvel** Eval sur un **nouvel** isolat réussit (pas un isolat coincé).

## Voie C / sgoiter / c2archtsim

Non.

## Clôture

Trois tests ci-dessus. Pas de `sleep` dans le moteur, seulement deadline + gas.
