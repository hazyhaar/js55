# M01 — Oracle node:vm (erreurs + affichage)

Statut : ouverte. Contexte 32k : cette fiche + les trois fichiers listés, rien d’autre.

## Objectif

L’oracle Node compare aussi le **nom** d’exception, pas seulement `String(valeur)`. Aujourd’hui `eval_oracle.js` renvoie `{error: constructor.name}` et T4_2 **ignore** les programmes que Node refuse. Un script qui jette en js55 et réussit en Node (ou l’inverse) passe inaperçu.

## Hors périmètre

API `vm.Script`, timeout isolat (M13), tout builtin, tout Test262.

## Lire (borné)

- `/devhoros/pkg/js55/engine/testdata/eval_oracle.js` (fichier entier, 44 lignes)
- `/devhoros/pkg/js55/engine/vm_test.go` offset 182 limit 70 (`TestT4_2_DifferentialVsNode`)
- `/devhoros/pkg/js55/engine/conv.go` offset 77 limit 50 (`toDisplayString`)

## Écrire

- `eval_oracle.js` : chaque résultat = `{value}` ou `{error, name}` (`e.name`, pas seulement `constructor.name`). `display` inchangé pour les succès.
- `vm_test.go` : si Node jette, js55 doit jeter le **même** `name` ; si Node réussit, js55 ne doit pas jeter. Ajouter 8 programmes d’erreur (`throw new TypeError`, `a.b` sur `undefined`, `1n+1`, recursion bornée hors quota).

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestT4_2_DifferentialVsNode|TestGCModeAgreement' ./pkg/js55/engine/
```

Zéro divergence. Node absent = skip, pas un vert fictif.

## Voie C / sgoiter / c2archtsim

Non. Protocole JSON vers `node:vm`.

## Clôture

T4_2 couvre succès **et** échecs nommés. Cette fiche passe à `close`. Suivante : M02.
