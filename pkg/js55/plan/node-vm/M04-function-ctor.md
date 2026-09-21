# M04 — Constructeur `Function`

Statut : ouverte. Dépend M03. Contexte 32k.

## Objectif

`new Function("a","return a+1")` et `Function("return 2")` compilent une source et appellent comme Node. Aujourd’hui : `TypeError: Function constructor is not implemented` (`builtins_function.go:20`).

## Hors périmètre

`eval` indirect, `new Function` en mode strict vs sloppy exhaustif Test262, `Function.prototype.toString` source fidèle, ASM.

## Lire (borné)

- `/devhoros/pkg/js55/engine/builtins_function.go` (entier, 80 lignes)
- `Grep "func Parse|type Options" /devhoros/pkg/js55/parser/` puis Read 40 lignes
- `Grep "func Compile" /devhoros/pkg/js55/engine/compile.go` puis Read 40 lignes

## Écrire

- Native du ctor : joindre les paramètres + corps comme ES (`body` = dernier arg, params = précédents), `parser.Parse`, `Compile`, fonction sur le tas.
- Rejet : body non parseable → `SyntaxError` (nom M01).
- Tests vs Node : `Function("return 1")()`, `new Function("a","b","return a+b")(2,3)`, `Function("return arguments[0]")(9)` (s’appuie sur M03).

## Oracle

```
GOTOOLCHAIN=go1.27.0 go test -race -count=1 -run 'TestFunctionCtor|TestT4_2|TestFunctionCallBind' ./pkg/js55/engine/
```

## Voie C / sgoiter / c2archtsim

Non. Réutilise le compilateur existant.

## Clôture

Trois programmes ci-dessus = Node. Le message « not implemented » a disparu. Suivante : M08 si Reflect encore bloqué par apply, sinon M05.
