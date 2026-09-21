# M03 — Objet `arguments`

Statut : ouverte. Dépend M01. Contexte 32k.

## Objectif

Une fonction non-flèche expose `arguments` (objet array-like : `length`, indices, `callee` selon mode). Agrégats mesurés : Reflect « arguments is not defined » (88) et Object (1108). Un seul palier : **création + indices + length** en sloppy. Pas `callee` strict poison, pas mapped vs unmapped exhaustif.

## Hors périmètre

`Function` ctor (M04), `Reflect.apply` array-like générique (M08), rest/spread.

## Lire (borné)

- `Grep "arguments"` dans `/devhoros/pkg/js55/engine/` `--include=*.go` (pas le parser entier).
- `/devhoros/pkg/js55/engine/builtins_function.go` (80 lignes, entier)
- Opcode : `Grep "^func |Op[A-Z]" /devhoros/pkg/js55/engine/opcode.go` puis Read 40 lignes autour du match Call.

Si `compile.go` est requis : `Grep "func compile|func emitCall|arguments"` puis Read 80 lignes. Jamais le fichier entier.

## Écrire

- Liaison `arguments` à l’entrée d’appel (objet réel, pas un tableau JS nu si Node expose `callee`).
- Test unitaire `TestArgumentsObjectVsNode` : 10 sources dans T4_2 / oracle M01 (`function f(a){return arguments[0]+arguments.length} f(7,8,9)`).
- Ne pas régénérer le golden Reflect entier. Ajouter les 10 programmes à `programs` de `vm_test.go` si la liste reste petite ; sinon fichier `testdata/arguments_oracle.js` lu par un test dédié.

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestArguments|TestT4_2|TestFunctionCallBind' ./pkg/js55/engine/
```

Option (hors clôture) : `JS55_CLUSTER=1` sur `test/built-ins/Reflect` et noter si l’agrégat 88 a bougé. Ne pas échouer la mission sur 88>0.

## Voie C / sgoiter / c2archtsim

Non. Objet de cadre d’appel.

## Clôture

10 programmes Node bit-égaux. Suivante : M04 ou M08.
