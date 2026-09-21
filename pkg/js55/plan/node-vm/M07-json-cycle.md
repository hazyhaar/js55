# M07 — `JSON.stringify` et cycles

Statut : ouverte. Contexte 32k.

## Objectif

`JSON.stringify` sur un cycle doit jeter `TypeError` comme Node, sans déborder la pile Go. Cause suspecte déjà au sol : `jsonStringifyObject` appelle `BuiltinJSONStringify` qui **recrée** `seen` (`builtins_json_ext.go` vers 114–134). Les tests Object racine explosent encore.

## Hors périmètre

`replacer` fonction, `space` pretty-print, `toJSON` exotique, parse SIMD (pas pertinent ici).

## Lire (borné)

- `/devhoros/pkg/js55/engine/builtins_json_ext.go` (entier si < 200 lignes)
- `Grep "JSON.stringify|BuiltinJSON" /devhoros/pkg/js55/engine/vm.go` — Read 30 lignes si match

## Écrire

- Un seul `seen` threadé dans toute la récursion.
- Cycle → `TypeError` (nom M01), pas une panique.
- `undefined` dans un objet omis, dans un tableau → `null` (déjà esquissé).
- Tests vs Node : cycle objet, cycle tableau, diamant sans cycle, `undefined` dans tableau.

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestJSONStringify|TestT4_2' ./pkg/js55/engine/
```

Ne pas relancer `./conformance` Object racine dans cette mission. Si un RelPrefix `test/built-ins/JSON/stringify` existe, ratchet optionnel.

## Voie C / sgoiter / c2archtsim

Non. Le scan JSON.parse n’est pas ce palier. Un noyau C de stringify n’aide pas la détection de cycle sur le tas js55.

## Clôture

Quatre programmes Node + zéro débordement de pile sur le cycle. Suivante : M06 si encore bloqué par stringify dans Object, sinon M08.
