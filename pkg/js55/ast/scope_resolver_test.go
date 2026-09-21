// SPDX-License-Identifier: BUSL-1.1

package ast

import (
	"testing"
)

func TestScopeResolver_HoistingAndBindings(t *testing.T) {
	sr := NewScopeResolver()

	sr.Declare("varX", BindingVar)
	sr.Declare("letY", BindingLet)
	sr.Declare("constZ", BindingConst)
	sr.Declare("fnW", BindingFunction)

	if !sr.IsHoisted("varX") {
		t.Fatalf("Expected varX to be hoisted")
	}
	if !sr.IsHoisted("fnW") {
		t.Fatalf("Expected fnW to be hoisted")
	}
	if sr.IsHoisted("letY") {
		t.Fatalf("Expected letY NOT to be hoisted (TDZ enforcement)")
	}
	if sr.IsHoisted("constZ") {
		t.Fatalf("Expected constZ NOT to be hoisted (TDZ enforcement)")
	}
}
