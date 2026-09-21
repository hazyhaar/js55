# M02 — Surface isolat = node:vm

Statut : ouverte. Dépend M01. Contexte 32k : fiche + fichiers listés.

## Objectif

L’hôte Go évalue comme `vm.createContext` + `vm.runInContext` : contexte propre, pas d’état global partagé, même valeur de complétion que Node sur un corpus court.

## Hors périmètre

`timeout` / `breakOnSigint` (M13), `compileFunction`, `cachedData`, modules ESM, `vm.constants`.

## Lire (borné)

- `/devhoros/pkg/js55/isolate/isolate.go` (164 lignes, entier OK)
- `/devhoros/pkg/js55/js55.go` (entier, court)
- `/devhoros/pkg/js55/engine/testdata/eval_oracle.js`

`Grep "^func "` dans `isolate.go` seulement.

## Écrire

- Tests sous `isolate/` : `TestVmCreateContextIsolation` (deux isolats, `var x=1` dans A n’apparaît pas dans B).
- `TestVmRunInContextCompletion` : 12 sources, oracle Node via le protocole M01, `iso.Eval` vs `vm.runInContext`.
- Si l’API publique manque un équivalent `RunInContext(src, ctx)` distinct de `Eval` global, l’ajouter **mince** dans `isolate.go` — pas de nouveau package.

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 ./pkg/js55/isolate/
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run TestT4_2 ./pkg/js55/engine/
```

## Voie C / sgoiter / c2archtsim

Non. Cloisonnement de tas Go.

## Clôture

Deux isolats étanches, 12 programmes bit-égaux à Node. Suivante utile : M03 (débloque le harnais Test262 qui parle de `arguments`).
