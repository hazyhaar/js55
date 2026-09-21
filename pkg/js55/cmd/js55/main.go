// SPDX-License-Identifier: BUSL-1.1

// Command js55 — Moteur d'exécution JavaScript / ECMAScript souverain 0-CGO Go 1.27.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hazyhaar/js55/pkg/js55"
	"github.com/hazyhaar/js55/pkg/js55/engine"
)

func main() {
	evalFlag := flag.String("e", "", "Évaluer une expression JavaScript")
	flag.Parse()

	iso, err := js55.NewIsolate(js55.Config{
		GasLimit:       10000000,
		MaxDepth:       1000,
		MaxMemoryBytes: 128 * 1024 * 1024,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "js55: erreur initialisation isolate: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	if *evalFlag != "" {
		res, err := iso.EvalContext(ctx, *evalFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Uncaught Error: %v\n", err)
			os.Exit(1)
		}
		if res != engine.Undefined {
			fmt.Println(iso.VM().ToStringValue(res).GoString())
		}
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("js55 v1.27 — Moteur d'exécution JavaScript souverain (Go 1.27, ARCHTIME-SIMD)")
		fmt.Println("Usage: js55 [-e 'expression'] [fichier.js]")
		return
	}

	scriptBytes, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "js55: erreur lecture fichier %s: %v\n", args[0], err)
		os.Exit(1)
	}

	chunk, err := iso.Compile(string(scriptBytes), args[0], false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Uncaught %v\n", err)
		os.Exit(1)
	}

	res, err := iso.Execute(ctx, chunk)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Uncaught %v\n", err)
		os.Exit(1)
	}
	if res != engine.Undefined {
		fmt.Println(iso.VM().ToStringValue(res).GoString())
	}
}
