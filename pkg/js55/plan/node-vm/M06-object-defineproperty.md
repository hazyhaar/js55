# M06 — `Object.defineProperty` (premier agrégat)

Statut : **close**. Contexte 32k.

## Objectif

Le premier agrégat Object mesuré (1660 « TypeError | undefined is not a function ») vient presque toujours d’un helper Test262 (`verifyProperty`, `isConfigurable`, etc.) qui appelle une méthode absente. Identifier **une** fonction manquante par `ClusterBySignature` sur un **sous-préfixe**, l’implémenter, ratchet.

## Hors périmètre

Arbre `test/built-ins/Object` entier (6802 fichiers, débordement JSON). `JSON.stringify` (M07). Getters/setters exhaustifs. M06bis (agrégat suivant).

## Lire (borné)

- `/devhoros/pkg/js55/conformance/cluster.go`
- `/devhoros/pkg/js55/engine/builtins_object.go` : `Grep "^func "` puis Read `InstallObjectBuiltins` 80 lignes
- `/devhoros/pkg/js55/engine/builtins_object_descriptors.go` : `Grep "^func "` puis Read 80 lignes

## Faire

1. Dump **un** RelPrefix étroit, pas Object racine. Exemples : `test/built-ins/Object/defineProperty` ou `getOwnPropertyDescriptor`.
2. Premier agrégat seulement. Si c’est `undefined is not a function`, extraire le **nom** depuis 3 exemples du cluster (lire 3 fichiers test262, 40 lignes de frontmatter+corps chacun).
3. Implémenter cette unique méthode (souvent `defineProperty` incomplet, `propertyIsEnumerable`, `hasOwnProperty` déjà posé).
4. Test unitaire vs Node : 6 descripteurs (writable/enumerable/configurable data).

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestDefineProperty|TestT4_2' ./pkg/js55/engine/
GOTOOLCHAIN=go1.27.0 go test -count=1 -run TestBuiltinsObjectDefine -timeout 60s ./pkg/js55/conformance/
```

Le second test n’existe peut-être pas : alors `runLanguagePrefix` sur `test/built-ins/Object/defineProperty` seulement, golden dédié. Jamais `TestBuiltinsObject` racine.

## Voie C / sgoiter / c2archtsim

Non. Descripteurs = sémantique.

## Clôture

6 cas Node + RelPrefix étroit ratchet sans régression. Si 1660 n’a pas bougé, consigner le **nouveau** premier agrégat en bas de cette fiche, ne pas enchaîner M06bis dans la même session.

## Preuve (2026-09-01)

- Tas : attributs parallèles aux slots (`writable` / `enumerable` / `configurable`) ; `defineProperty` pose une propriété de données (défauts ES : bits absents = faux) ; accesseurs du descripteur via `KindAccessor` ; champs du descripteur lus sur la chaîne de prototypes.
- `Object.keys` n’émet que l’énumérable ; `getOwnPropertyNames` émet toutes les clés propres.
- Assignation sloppy sur non-inscriptible : no-op. `delete` refuse le non-configurable.
- 6 programmes Node dans T4_2 (valeur, énumérable vrai/faux, inscriptible vrai/faux, configurable faux). Unitaires `TestDefineProperty*`. RelPrefix `test/built-ins/Object/defineProperty` : golden `/devhoros/pkg/js55/conformance/testdata/manifest.object-defineproperty.golden`, **802/2250** pass (ratchet vert).
- L’agrégat 1660 « undefined is not a function » a bougé (84 restants). Premier agrégat actuel : **809** `TypeError | [object Object] is not a function` (ex. `15.2.3.6-0-2.js` = `Object.defineProperty.length`, `15.2.3.6-3-100.js` = `verifyProperty`). M06bis, pas cette session.
