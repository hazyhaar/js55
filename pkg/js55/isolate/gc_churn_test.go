// SPDX-License-Identifier: BUSL-1.1

package isolate_test

import (
	"context"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

func TestGCChurn_MemoryQuotaRecycled(t *testing.T) {
	// 2 Mo quota with 100,000 iterations creating temporary objects
	iso, err := isolate.New(isolate.Config{
		MaxMemoryBytes: 2 * 1024 * 1024, // 2 Mo
		GasLimit:       10_000_000,
	})
	if err != nil {
		t.Fatalf("failed to create isolate: %v", err)
	}

	ctx := context.Background()

	script := `
let sum = 0;
for (let i = 0; i < 50000; i = i + 1) {
    let tmp = [i, i + 1];
    sum = sum + tmp[0];
}
sum;
`

	val, err := iso.EvalContext(ctx, script)
	if err != nil {
		t.Fatalf("Le script avec fort churn a été tué à tort : %v", err)
	}

	expected := int32(49999 * 50000 / 2)
	if val.ToInt() != expected {
		t.Fatalf("Attendu %d, obtenu %d", expected, val.ToInt())
	}
	t.Logf("✓ Fort churn mémoire (50k objets temporaires) recyclé avec succès sous quota 2 Mo : sum = %d (Mémoire finale : %d Ko)", val.ToInt(), iso.AllocatedMemory()/1024)
}
