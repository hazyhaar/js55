// SPDX-License-Identifier: BUSL-1.1

package engine_test

import (
	"context"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

func TestRopePropertyKey_NoPanic(t *testing.T) {
	iso, err := isolate.New(isolate.Config{
		MaxMemoryBytes: 16 * 1024 * 1024,
		GasLimit:       10_000_000,
	})
	if err != nil {
		t.Fatalf("failed to create isolate: %v", err)
	}

	ctx := context.Background()

	// Script that creates a deeply nested Rope string and uses it as an object key
	script := `
let o = {};
let p = "prefix_";
for (let i = 0; i < 20; i = i + 1) {
    p = p + "_longer_and_longer_string_segment_" + i;
}
o[p] = 42;
let res = o[p];
res;
`

	val, err := iso.EvalContext(ctx, script)
	if err != nil {
		t.Fatalf("Échec évaluation clé Rope: %v", err)
	}

	if val.ToInt() != 42 {
		t.Fatalf("Attendu 42, obtenu %v", val.ToInt())
	}
	t.Logf("✓ Clé de propriété Corde (Rope) évaluée avec succès sans panique : res = %d", val.ToInt())
}
