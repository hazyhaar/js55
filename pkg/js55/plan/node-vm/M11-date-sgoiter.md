# M11 — Date TimeClip / parse (C si divergence)

Statut : ouverte. Contexte 32k. **Mesure d’abord.**

## Objectif

`Date.parse` / `Date.UTC` / `toISOString` sur un corpus fixe = Node. `Date.now` n’est pas un oracle (horloge). Go : `builtins_date.go`. Même discipline que M10 : C seulement si Node diverge sur un vecteur reproductible.

## Hors périmètre

Fuseaux `Intl`, `Temporal` (déjà un stub), horloge temps réel, SIMD.

## Lire (borné)

- `/devhoros/pkg/js55/engine/builtins_date.go` : `Grep "^func "` puis Read 80 autour de `parse` / `UTC`
- RelPrefix tests : `TestBuiltinsDateParse`, `TestBuiltinsDateNow` (Now = présence, pas la valeur)

## Faire

1. Corpus figé vs Node : ISO `2020-01-15T00:00:00.000Z`, `Date.UTC(2020,0,15)`, invalid → `NaN`, TimeClip hors ±8.64e15 → `NaN`.
2. Vert → clore sans C.
3. Divergence TimeClip ou parse ISO : noyau C `c2simd/sources/c2date/timeclip.c` (entier 64-bit, **formule anti-wrap** `addr < limit && len <= limit-addr` si bornes), sgoiter, `TestTimeClipVsCOracle`, finding, branchement.

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestDate|TestBuiltinsDateParse' ./pkg/js55/engine/ ./pkg/js55/conformance/
```

## Voie C / sgoiter / c2archtsim

sgoiter si divergence. Pas c2archtsim.

## Clôture

Corpus Node pair, ou noyau C + deux oracles. `Date.now` non comparé.
