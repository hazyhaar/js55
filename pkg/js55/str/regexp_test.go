// SPDX-License-Identifier: Apache-2.0 OR MIT

package str

import (
	"testing"
)

func TestRegExpEngine_ExecutionAndBacktracking(t *testing.T) {
	// Bytecode sequence matching "a" followed by "b"
	// OpCheckCharacter 'a', OpCheckCharacter 'b', OpSucceed
	bc := []byte{
		byte(OpCheckCharacter), 'a',
		byte(OpCheckCharacter), 'b',
		byte(OpSucceed),
	}

	engine := NewRegExpEngine(bc, 0)

	// Test 1: Match "ab"
	sMatch := FromGo("ab")
	matched, _, err := engine.Exec(sMatch, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !matched {
		t.Fatalf("Expected regexp to match 'ab'")
	}

	// Test 2: Mismatch "ac"
	sMismatch := FromGo("ac")
	matched, _, err = engine.Exec(sMismatch, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if matched {
		t.Fatalf("Expected regexp to fail on 'ac'")
	}
}
