# M05 — Décomposition (compilateur)

Statut : **close**. Contexte 32k.

## Objectif

`const [a,b] = …` et `const {x} = …` dans un `for-of` / `for-in` **compilent**. Agrégat for-of mesuré : ~300 échecs « initialiseur manquant » / « cible for-in/for-of ». Un palier : patterns **simples** (identifiants, pas rest, pas défaut, pas imbriqué).

## Hors périmètre

Rest `...x`, défauts `= 1`, imbrication `[[a]]`, assignation déstructurée hors déclaration (`[a]=x` comme statement), iterators (M09).

## Lire (borné)

- `Grep "destructur|BindingPattern|for-of|ForOf" /devhoros/pkg/js55/parser/ /devhoros/pkg/js55/engine/compile.go`
- Read 80 lignes autour de **un** match compile et **un** match parser.
- `/devhoros/pkg/js55/conformance/cluster.go` (signatures seulement, ~120 lignes max)

## Écrire

- Parser + émission bytecode pour `const [a] = arr` et `let {x} = obj`.
- Tests unitaires vs Node (oracle M01) : 8 programmes, **sans** `for-of` d’abord, puis 4 `for (const [k] of [[1],[2]])`.
- Ne pas toucher le golden `manifest.for-of.golden` sauf si le ratchet du RelPrefix `test/language/statements/for-of` est relancé **et** sans régression. Relance optionnelle, pas obligatoire pour clore.

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestDestructur|TestT4_2' ./pkg/js55/engine/
```

## Voie C / sgoiter / c2archtsim

Non. Front compilateur.

## Clôture

12 programmes compilent et matchent Node. Les messages « initialiseur manquant » sur ces formes ont disparu. Suivante : M09.

## Preuve (2026-09-01)

- En-tête `for-of` : une déclaration `const [k]` / `const {x}` sans `=` n’appelle plus `varDecl` (qui exigeait l’initialiseur). `storeLoopLeft` décompose la valeur d’itération (`ArrayPattern` / `ObjectPattern`, déclaration ou assignation).
- Parser inchangé (`parseBindingTarget` produisait déjà le motif).
- `TestDestructuring` : 4 programmes `for-of` (const tableau, const objet, let tableau, assignation `[k]`). T4_2 les reprend. RelPrefix `for-of` : 186 → **620**/1442, ratchet sans régression. L’agrégat « initialiseur manquant » a disparu. Premier agrégat restant : `Error | [object Object]` (280) — M09.
