// SPDX-License-Identifier: BUSL-1.1

package isolate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

// TestPropertyGrowthQuotaAttack vérifie qu'un script créant massivement des propriétés uniques
// et des transitions de formes est rigoureusement intercepté par le QuotaTracker.
func TestPropertyGrowthQuotaAttack(t *testing.T) {
	iso, err := isolate.New(isolate.Config{
		MaxMemoryBytes: 512 * 1024, // Quota strict de 512 Ko
		GasLimit:       10_000_000,
	})
	if err != nil {
		t.Fatalf("failed to create isolate: %v", err)
	}

	ctx := context.Background()

	// Script d'attaque créant des milliers de propriétés calculées uniques
	script := `
let o = {};
for (let i = 0; i < 20000; i = i + 1) {
    let key = "prop_" + i + "_extended_unique_key_name";
    o[key] = i;
}
`

	_, evalErr := iso.EvalContext(ctx, script)
	if evalErr == nil {
		t.Fatalf("L'attaque de croissance de propriétés aurait dû être tuée par le quota mémoire !")
	}

	if !errors.Is(evalErr, isolate.ErrMemoryLimitExceeded) {
		t.Fatalf("Attendu ErrMemoryLimitExceeded, obtenu : %v", evalErr)
	}

	t.Logf("✓ Attaque de saturation de propriétés interceptée avec succès par le QuotaTracker : %v (Mémoire facturée : %d Ko)", evalErr, iso.AllocatedMemory()/1024)
}
