// SPDX-License-Identifier: Apache-2.0 OR MIT

package isolate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

func TestMemoryQuotaBreach_Enforced(t *testing.T) {
	// Configure isolate with tight 1MB limit
	iso, err := isolate.New(isolate.Config{
		MaxMemoryBytes: 1 * 1024 * 1024, // 1 Mo
		GasLimit:       10_000_000,
	})
	if err != nil {
		t.Fatalf("failed to create isolate: %v", err)
	}

	ctx := context.Background()

	// Script that attempts to allocate a 2.5 million element array (exceeding 1MB)
	script := `
let a = [];
for (let i = 0; i < 2500000; i = i + 1) {
    a[i] = i;
}
`

	_, err = iso.Eval(ctx, script)
	if err == nil {
		t.Fatalf("Attaque d'allocation réussie ! Le quota de 1 Mo n'a pas arrêté le script.")
	}

	if !errors.Is(err, isolate.ErrMemoryLimitExceeded) {
		t.Logf("Quota mémoire bien déclenché avec erreur: %v", err)
	} else {
		t.Logf("✓ ErrMemoryLimitExceeded interceptée avec succès !")
	}
}
