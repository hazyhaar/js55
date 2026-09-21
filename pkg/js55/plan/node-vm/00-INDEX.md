# Parité node:vm — plan par missions 32k

Objectif : l’isolat `js55` rend, pour un script évalué comme `node:vm`, la même valeur de complétion et le même nom d’exception que Node. Ce n’est pas Maglev, pas `c2jsc`, pas les 102 922 cas Test262 au vert, pas intl402.

Oracle de vérité : `node` + `/devhoros/pkg/js55/engine/testdata/eval_oracle.js` (`vm.runInContext`). Test262 n’est qu’un microscope par agrégat (`ClusterBySignature`), jamais un but.

## Session Ornith (32k)

Voie Ornith = grain Gemini : une fonction C++ recopiée dans la fiche → un `.c` ≤ 2000 jetons. File : `micro/INDEX.md`. Règle : `micro/00-REGLE.md`. Les `M01`–`M14` (enquête Go) sont hors grain.

## Voie C / sgoiter / c2archtsim

| Voie | Quand | Interdit |
| :--- | :--- | :--- |
| Go grain B (`pkg/js55`) | Sémantique ECMA, builtins, isolat | Manuscrit qualifié de transpilé |
| C + `sgoiter -in … -out *_gen.go` | Noyau numérique / parse / codec à parité bit | Éditer `*_gen.go` |
| `c2archtsim` custom | Scan, LUT, UTF, recherche d’octets, **après mesure** | Maglev, descripteurs, `arguments` |

Le binaire s’appelle `sgoiter` (transpileur C → Go). Cycle : source C sous `c2simd/sources/` → épreuve à blanc → oracle `gcc -O2` (`Test*VsCOracle`) → fiche `F-sgoiter-*.cue` → `cue vet`. `c2d2s` (`Number::toString`) est **clos**.

## Ordre

| Id | Fichier | Voie | Dépend | Statut |
| :--- | :--- | :--- | :--- | :--- |
| M01 | `M01-oracle-node-vm.md` | Go | — | **close** T4_2 **155** prog., 0 divergence, noms d’exception |
| M02 | `M02-isolate-vm-api.md` | Go | M01 | **close** |
| M03 | `M03-arguments.md` | Go | M01 | **close** length + indices |
| M04 | `M04-function-ctor.md` | Go | M03 | **close** |
| M05 | `M05-destructuring.md` | Go | — | **close** décomposition en tête de for-of ; RelPrefix **1442/1442** |
| M06 | `M06-object-defineproperty.md` | Go | — | **close** RelPrefix defineProperty 802/2250 ; T4_2 × 6 descripteurs |
| M07 | `M07-json-cycle.md` | Go | — | **close** |
| M08 | `M08-reflect-reste.md` | Go | M03 | **close** apply array-like ; Reflect 70/306 |
| M09 | `M09-for-of.md` | Go | M05 | **close** protocole @@iterator/next ; RelPrefix **1442/1442** |
| M10 | `M10-parseint-sgoiter.md` | C si divergence | — | mesure : parseInt 74/110, **pas de C** |
| M11 | `M11-date-sgoiter.md` | C si divergence | — | non mesurée |
| M12 | `M12-lexer-archtsim.md` | c2archtsim si chaud | mesure | **abort** (pas de banc) |
| M13 | `M13-vm-timeout.md` | Go | M02 | **close** EvalTimeout |
| M14 | `M14-ratchet.md` | Go | paliers | **close** engine+isolate `-race` vert |

## Solde 2026-09-02 (Phase 0 sandbox)

- Parseur : déréférence nulle de `collectVarDeclaredNames` sur `try` sans `finally` (cinq fichiers staging/intl402) réparée ; `TestParseAuditPanicFiles` vert.
- T4_2 : 155 programmes, 0 divergence, modes normal et stress.
- RelPrefix `for-of` : 1442/1442 (manifestes `for-of` et `stmt-for-of`).
- Isolat : cloisonnement, complétion, `EvalTimeout` verts.
- c2cred : Firecracker v1.16.1 vendorié ; boot ELF + initramfs jusqu’à `/init` en **58,7 ms** (plafond publié 125 ms) ; série allumée donc majorant.
- Réseau guest : ARP captif e1000 seulement ; pas de TCP/UDP, pas de TAP, pas de gVisor (`vmm_net_pkt` non câblé sur le NIC).
- RegExp js55 : pont `regexp` Go (RE2) ; `c2pcre` transpilé mais **non importé**.

Hors plan : Maglev, AOT `c2jsc` (Phase 6, après seuils Phase 4), `test/intl402`, arbre `Array/` entier (OOM), `test/built-ins/RegExp` entier, `fs`/`http` Node complets.

Preuve minimale d’une mission : un test unitaire nommé + `go test -race -count=1` du **package touché** (pas `./...` du module sauf M14). Golden Test262 : uniquement le `RelPrefix` nommé, ratchet sans régression.
