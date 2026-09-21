# M09 — `for-of` après décomposition

Statut : **close**. Dépend M05. Contexte 32k.

## Objectif

`for (const x of iterable)` et `for (const [x] of pairs)` (formes M05) s’exécutent comme Node : protocole `@@iterator` / `next`. Golden for-of : 186/1442. Un agrégat après M05, pas l’arbre entier.

## Hors périmètre

`async` for-await, `yield*`, fermeture `return()` de l’itérateur en `break` (sauf si l’agrégat n°1 l’exige), Map/Set iterators complets.

## Lire (borné)

- `/devhoros/pkg/js55/engine/builtins_iterator.go` : `Grep "^func "` puis Read 80
- `Grep "for-of|ForOf|OpFor" /devhoros/pkg/js55/engine/compile.go` puis Read 80
- `/devhoros/pkg/js55/conformance/language_subset_test.go` offset 74 limit 10

## Écrire

- Si l’agrégat n°1 est encore la décomposition : **stop**, renvoyer à M05, ne pas coder ici.
- Sinon : le trou nommé (souvent `next is not a function` ou close). Un correctif.
- 8 programmes vs Node : tableau, string, objet avec `Symbol.iterator`, `break`.

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestForOf|TestT4_2' ./pkg/js55/engine/
GOTOOLCHAIN=go1.27.0 go test -count=1 -run TestLanguageForOf_JS55Engine -timeout 120s ./pkg/js55/conformance/
```

Ratchet `manifest.for-of.golden` : pass ≥ 186, zéro régression.

## Voie C / sgoiter / c2archtsim

Non.

## Clôture

8 programmes Node + golden for-of non régressé. Un agrégat seulement.

## Preuve (2026-09-01)

M05 a fait disparaître l’agrégat « initialiseur manquant ». Palier protocole :

- `arguments[Symbol.iterator]` = `Array.prototype.values`.
- `next` d’itérateur de tableau : `length` array-like + `[[Get]]` (accesseurs d’indice).
- `defineProperty` d’un indice de tableau étend `length` (éléments denses).
- Clé `Symbol.*` : `keyString` utilise le descripteur du symbole, pas `[object Object]`.

`TestForOf` : 8 programmes (tableau, chaîne, `break`, `arguments`, objet `Symbol.iterator`, décomposition, vide, générateur). RelPrefix live **641/1442** (plancher golden 106, ratchet ≥ 186). Map/Set restent « not iterable » (hors périmètre). Premier agrégat restant : `Error | [object Object]` (270, surtout TDZ / `Test262Error`).
