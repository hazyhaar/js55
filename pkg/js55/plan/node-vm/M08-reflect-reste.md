# M08 — Reflect, un agrégat restant

Statut : ouverte. Dépend M03. Contexte 32k.

## Objectif

Golden actuel : 68/306 (`manifest.reflect.golden`). Un **seul** agrégat par session. Ordre imposé par la mesure N0 : (1) `arguments is not defined` — doit déjà chuter après M03 ; si oui, passer à (2) apply array-like (72) : `Reflect.apply(fn, this, {0:1, length:1})`.

## Hors périmètre

Soldé 153/153. Proxy traps manquants. `Reflect.construct` new.target exotique.

## Lire (borné)

- `/devhoros/pkg/js55/engine/builtins_reflect.go` : `Grep "^func "` puis Read `apply` / `arrayValues` 80 lignes
- `/devhoros/pkg/js55/engine/p468_test.go` offset 85 limit 50
- `/devhoros/pkg/js55/conformance/testdata/manifest.reflect.golden` : ne pas le lire en entier ; `Grep` les `fail` si besoin d’un échantillon

## Écrire

- Si agrégat (2) : `arrayValues` accepte array-like (`length` + indices), comme `Function.prototype.apply`.
- Test unitaire vs Node : 6 appels `Reflect.apply` / `fn.apply`.
- Relancer **uniquement** `TestBuiltinsReflect_JS55Engine`, mettre à jour le golden si progression, ratchet sans régression.

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestReflect|TestFunctionCallBind' ./pkg/js55/engine/
GOTOOLCHAIN=go1.27.0 go test -count=1 -run TestBuiltinsReflect_JS55Engine -timeout 120s ./pkg/js55/conformance/
```

## Voie C / sgoiter / c2archtsim

Non.

## Clôture

Golden Reflect `pass` strictement ≥ 68, zéro régression. Noter le nouveau compteur en bas de fiche. Ne pas enchaîner un second agrégat.
