// SPDX-License-Identifier: BUSL-1.1

package ast

import (
	"testing"
)

func TestASTArena_AllocationAndReset(t *testing.T) {
	arena := NewASTArena(16)

	// Allocate 100 nodes to force dynamic growth
	for i := 0; i < 100; i++ {
		idx := arena.AllocNode(uint8(i%10), uint32(i), uint32(i+1), uint32(i*2), 2)
		if idx != uint32(i) {
			t.Fatalf("Expected allocated index %d, got %d", i, idx)
		}
	}

	if arena.Length != 100 {
		t.Fatalf("Expected arena length 100, got %d", arena.Length)
	}

	// Verify SoA storage integrity
	if arena.LeftChildren[50] != 50 || arena.RightChildren[50] != 51 {
		t.Fatalf("Corrupted SoA children data at index 50")
	}

	// Reset arena
	arena.Reset()
	if arena.Length != 0 {
		t.Fatalf("Expected arena length 0 after reset, got %d", arena.Length)
	}

	// Re-allocate after reset
	newIdx := arena.AllocNode(1, 10, 20, 0, 4)
	if newIdx != 0 {
		t.Fatalf("Expected index 0 after reset, got %d", newIdx)
	}
}
