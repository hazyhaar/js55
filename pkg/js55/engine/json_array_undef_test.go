// SPDX-License-Identifier: BUSL-1.1
package engine

import "testing"

func TestJSONArrayUndefined(t *testing.T) {
	got, err := compileAndRun(t, `JSON.stringify([undefined])`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[null]" {
		t.Errorf("obtenu %q, [null] attendu", got)
	}
}
