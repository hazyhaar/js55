// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func TestExportDefaultNamedEvaluationMeasure(t *testing.T) {
	src := `export default function(){}`
	_, err := parser.Parse(src, parser.Options{})
	t.Logf("script sans Module : %v", err)
	prog, err := parser.Parse(src, parser.Options{Module: true})
	if err != nil {
		t.Fatalf("parseur module : %v", err)
	}
	h := NewHeap()
	chunk, err := Compile(h, prog, "mod")
	if err != nil {
		t.Fatalf("le compilateur a refusé export default : %v", err)
	}
	if !chunk.ExportDefault {
		t.Errorf("chunk.ExportDefault attendu true")
	}
	vm := NewVM(h)
	if _, err := vm.Run(chunk); err != nil {
		t.Fatalf("exécution échouée : %v", err)
	}
	if _, ok := vm.ModuleExportDefault(); !ok {
		t.Errorf("export default attendu")
	}
	t.Logf("export default : parseur module OK, compilation et exécution OK")
}
