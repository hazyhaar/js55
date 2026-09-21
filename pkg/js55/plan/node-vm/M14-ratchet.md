# M14 — Ratchet de clôture de palier

Statut : ouverte. À jouer **après** M01–M09 (et M10–M13 s’ils ont touché du Go). Contexte 32k : pas de nouveau code métier.

## Objectif

Constater, sans élargir le périmètre, que les goldens touchés n’ont pas régressé et que T4_2 tient. Mettre à jour `00-INDEX.md` (statuts).

## Hors périmètre

Nouvel agrégat, nouveau builtin, `git commit` (sauf demande explicite de l’usager), `./conformance` intégral, Object racine, Array racine.

## Lire

- `00-INDEX.md`
- Goldens **nommés** par les missions closes uniquement.

## Faire

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 ./pkg/js55/engine/ ./pkg/js55/isolate/ ./pkg/c2d2s/
GOTOOLCHAIN=go1.27.0 go test -count=1 -timeout 180s -run 'TestT4_2|TestBuiltinsReflect_JS55Engine|TestLanguageForOf|TestLanguageComments' ./pkg/js55/conformance/
```

Si un paquet C a été créé (M10–M12) : `go test -race -count=1` de ce paquet sous `GOEXPERIMENT=simd` depuis `/devhoros/c2simd`.

Dump cluster : seulement `JS55_CLUSTER=1` et un RelPrefix, jamais par défaut.

## Oracle

Tous les packages listés verts. Compteurs Reflect / for-of consignés dans l’INDEX (≥ 68 et ≥ 186).

## Voie C / sgoiter / c2archtsim

Non, sauf `cue vet` si un finding a été ajouté.

## Clôture

INDEX à jour. Note : T4_2 = 0 divergence. Pas de push.
