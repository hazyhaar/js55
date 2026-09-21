package engine

import (
	"fmt"
	"github.com/hazyhaar/js55/pkg/js55/parser"
	"os"
	"testing"
)

func TestCheckBundle(t *testing.T) {
	b, _ := os.ReadFile("/devhoros/GAFP/audits/comparatif-chromium-js55-20260909/mutations/input-three-source")
	p, err := parser.Parse(string(b), parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := CompileMode(NewHeap(), p, "binding", false)
	if err != nil {
		t.Fatal(err)
	}

	var find func(chk *Chunk)
	find = func(chk *Chunk) {
		for _, k := range chk.Consts {
			if k.Kind == ConstFunction {
				if k.Fn.Name == "computeVertexNormals" || k.Fn.Name == "normalizeNormals" {
					fmt.Printf("FOUND %s!\n", k.Fn.Name)
					h := hashChunk(k.Fn)
					fmt.Printf("Hash: %q\n", h)
				}
				find(k.Fn)
			}
		}
	}
	find(c)
}
