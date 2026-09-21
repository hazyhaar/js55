# M10 — `parseInt` / `parseFloat` vs Node (C si divergence)

Statut : ouverte. Contexte 32k. **Mesure d’abord.**

## Objectif

`parseInt` / `parseFloat` bit-égaux à Node sur un corpus (espaces, signes, radix 2–36, `0x`, trailing junk, `NaN`). Go existe déjà : `BuiltinParseInt` / `BuiltinParseFloat` dans `builtins_string_number.go`. Si T4_2 + RelPrefix parseInt/parseFloat sont déjà verts, **clore sans C**.

## Hors périmètre

`Number(string)` (autre conversion, `conv.go`), `BigInt`, SIMD.

## Lire (borné)

- `/devhoros/pkg/js55/engine/builtins_string_number.go` offset 151 limit 80
- `/devhoros/pkg/js55/engine/number_kat_test.go` (entier si court)
- RelPrefix goldens : ne pas les dump ; lancer les tests.

## Faire

1. ```
   GOTOOLCHAIN=go1.27.0 go test -count=1 -run 'TestBuiltinsParseInt|TestBuiltinsParseFloat|TestNumberToString' ./pkg/js55/conformance/ ./pkg/js55/engine/
   ```
2. Si vert et 12 KAT Node (radix, junk, `+0`, `-0`) passent : marquer `close`, **pas de sgoiter**.
3. Si divergence : extraire le vecteur, écrire **un** noyau C `c2simd/sources/c2parseint/parseint.c` (pas de libc locale, bornes explicites), `sgoiter -in … -out` dans `c2simd/c2pkg/c2parseint/`, `TestParseIntVsCOracle` + oracle Node, finding `F-sgoiter-parseint.cue`, brancher `BuiltinParseInt`.

## Oracle

KAT Node + `go test -race -count=1` du package du noyau si créé. `cue vet` sur le finding.

## Voie C / sgoiter / c2archtsim

sgoiter **seulement** si l’étape 2 échoue. Pas c2archtsim (pas un scan de flux).

## Clôture

Soit « déjà pair, rien à transpilier », soit noyau transpilé + oracles C et Node. Ne pas réécrire Go à la main **et** le qualifier de transpilé.
