// SPDX-License-Identifier: BUSL-1.1

package isolate_test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/hazyhaar/js55/pkg/js55"
)

func TestBanc10kIsolates(t *testing.T) {
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	const N = 10_000
	t.Logf("--- DÉMARRAGE DU BANC 10 000 ISOLATS JS55 (CONCURRENCE RÉELLE) ---")
	start := time.Now()

	isolates := make([]*js55.Isolate, N)
	ctx := context.Background()

	// 1. Instanciation parallèle de 10 000 isolats sur 50 Goroutines
	const numWorkers = 50
	var wg sync.WaitGroup
	chunkSize := N / numWorkers

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			startIdx := workerID * chunkSize
			endIdx := startIdx + chunkSize
			if workerID == numWorkers-1 {
				endIdx = N
			}
			for i := startIdx; i < endIdx; i++ {
				iso, err := js55.NewIsolate(js55.Config{
					GasLimit:       100_000,
					MaxDepth:       100,
					MaxMemoryBytes: 1024 * 1024,
				})
				if err != nil {
					t.Errorf("Erreur création isolat %d: %v", i, err)
					return
				}
				isolates[i] = iso
			}
		}(w)
	}
	wg.Wait()
	elapsedCreate := time.Since(start)

	runtime.GC()
	runtime.ReadMemStats(&m2)
	allocMB := float64(m2.Alloc-m1.Alloc) / (1024 * 1024)
	sysMB := float64(m2.Sys-m1.Sys) / (1024 * 1024)

	t.Logf("✓ Création parallèle de %d Isolats en %v (%.2f µs / isolat)", N, elapsedCreate, float64(elapsedCreate.Microseconds())/N)
	t.Logf("✓ Mémoire consommée : %.2f Mo Alloc (soit %.2f Ko / isolat) | %.2f Mo Sys", allocMB, (allocMB*1024)/N, sysMB)

	// 2. Évaluation concurrente réelle de 10 000 scripts isolés sur 50 Goroutines
	startExec := time.Now()
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			startIdx := workerID * chunkSize
			endIdx := startIdx + chunkSize
			if workerID == numWorkers-1 {
				endIdx = N
			}
			for i := startIdx; i < endIdx; i++ {
				res, err := isolates[i].EvalContext(ctx, fmt.Sprintf("let x = %d; x * 2;", i))
				if err != nil {
					t.Errorf("Erreur eval isolat %d: %v", i, err)
					return
				}
				expected := int32(i * 2)
				if res.ToInt() != expected {
					t.Errorf("Isolat %d: attendu %d, obtenu %d", i, expected, res.ToInt())
					return
				}
			}
		}(w)
	}
	wg.Wait()
	elapsedExec := time.Since(startExec)
	t.Logf("✓ Évaluation concurrente réelle de %d scripts JS isolés en %v (%.2f µs / exécution)", N, elapsedExec, float64(elapsedExec.Microseconds())/N)
}
