# M12 — Scan lexeur (c2archtsim, si pertinent)

Statut : ouverte. **Mesure obligatoire. Abort si pas chaud.**

## Objectif

Le lexeur JS classe les octets (espace, ident, chiffre) par une LUT 256. Doctrine `PROTOCOLE_DOGFOODING_ARCHTIME_SIMD.md` : scan js55/lexer. Un noyau C LUT + sgoiter **uniquement** si `skipSpace` / classification est mesurable sur le chemin chaud.

## Hors périmètre

Grammaire, regexp vs division, templates, Maglev, réécrire `Next()`.

## Lire (borné)

- `/devhoros/pkg/js55/lexer/lexer.go` offset 91 limit 50 (`isWhitespace`, `isIdentStart`)
- `/devhoros/c2simd/spec/PROTOCOLE_DOGFOODING_ARCHTIME_SIMD.md` (66 lignes)
- Un finding existant LUT : `Grep "lut16|class_of" /devhoros/c2simd/sgoiter/spec/findings/` — un fichier seulement

## Faire

1. Banc : `go test -bench=BenchmarkLex -count=3 ./pkg/js55/lexer/` (créer un bench 1 Mo de JS si absent). Si `skipSpace` < 15 % CPU (`-cpuprofile` une passe) → **abort**, noter « non pertinent », close.
2. Sinon : C `c2simd/sources/c2archtsim/c2_js_byteclass.c` : `uint8_t class[256]` en `.rodata`, scan `len` octets → masque/classes, zéro alloc.
3. `sgoiter -in -out` vers `c2simd/c2pkg/c2archtsim/`, oracle `gcc -O2`, finding, `GOEXPERIMENT=simd` bench vs scalaire.
4. Brancher **seulement** `skipSpace` ASCII ; repli scalaire unicode inchangé.

## Oracle

`Test*VsCOracle` + `go test -race -count=1 ./pkg/js55/lexer/` + `cue vet`. Jamais un chiffre SIMD sous go1.26.

## Voie C / sgoiter / c2archtsim

Oui, **après** l’étape 1. Custom `c2archtsim`, pas un kernel Maglev V8 existant.

## Clôture

Soit abort mesuré, soit LUT transpilée + parité C + lexer non régressé.
