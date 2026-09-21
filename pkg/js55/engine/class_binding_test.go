package engine

import "testing"

// Reduced from the Three.js UMD bundle used by GAFP: a hoisted factory
// closes over a class declared later in the enclosing function.
func TestClassBindingCapturedBeforeDeclaration(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{"factory", `(function(){ function create(){ return new Ce(); } class Ce {} return create() instanceof Ce; })()`, "true"},
		{"self", `(function(){ class Ce { clone(){ return new Ce(); } } return (new Ce()).clone() instanceof Ce; })()`, "true"},
		{"temporal_recovery", `(function(){ function create(){ return new Ce(); } var rejected=false; try { create(); } catch(e) { rejected=e instanceof ReferenceError; } class Ce {} return rejected && create() instanceof Ce; })()`, "true"},
		{"global_temporal_recovery", `var rejected=false; try { new Ce(); } catch(e) { rejected=e instanceof ReferenceError; } class Ce {} rejected && (new Ce() instanceof Ce)`, "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compileAndRun(t, tc.source, false)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
