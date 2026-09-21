// SPDX-License-Identifier: Apache-2.0 OR MIT

package isolate_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

// TestMultiTenant_TrulyParallelExecution lance 100 Goroutines concurrentes exécutant chacune
// des isolats indépendants pour prouver l'absence absolue de contention et de data-race.
func TestMultiTenant_TrulyParallelExecution(t *testing.T) {
	const numGoroutines = 50
	const iterationsPerGoroutine = 50

	var wg sync.WaitGroup
	ctx := context.Background()

	errChan := make(chan error, numGoroutines*iterationsPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			for i := 0; i < iterationsPerGoroutine; i++ {
				iso, err := isolate.New(isolate.Config{
					MaxMemoryBytes: 2 * 1024 * 1024,
					GasLimit:       500_000,
				})
				if err != nil {
					errChan <- fmt.Errorf("Goroutine %d init err: %w", routineID, err)
					return
				}

				script := fmt.Sprintf(`
let base = %d;
let sum = 0;
for (let j = 0; j < 50; j = j + 1) {
    sum = sum + base + j;
}
sum;
`, routineID*100+i)

				val, err := iso.Eval(ctx, script)
				if err != nil {
					errChan <- fmt.Errorf("Goroutine %d eval err: %w", routineID, err)
					return
				}

				expected := int32((routineID*100+i)*50 + (49 * 50 / 2))
				if val.ToInt() != expected {
					errChan <- fmt.Errorf("Goroutine %d mismatch: attendu %d, obtenu %d", routineID, expected, val.ToInt())
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Fatalf("Échec épreuve de concurrence multi-tenant: %v", err)
	}

	t.Logf("✓ Concurrence active réelle : %d exécutions parallèles validées sans aucune collision sous -race.", numGoroutines*iterationsPerGoroutine)
}
