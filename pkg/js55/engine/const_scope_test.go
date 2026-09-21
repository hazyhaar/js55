// SPDX-License-Identifier: BUSL-1.1
package engine

import "testing"

func TestConstBindingUsesDeclarationScope(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{"parameter_shadow", `const n=1; var f=function(n){n++;return n;}; f(n)`, "2"},
		{"local_shadow", `const n=1; var f=function(){var n=2;n++;return n;}; f()`, "3"},
		{"closure_local_shadow", `const n=1; var f=function(){var n=2;function g(){n++;}g();return n;}; f()`, "3"},
		{"capture_reject_and_read", `(function(){function change(){n=2;}const n=1;var rejected=false;try{change();}catch(e){rejected=e instanceof TypeError;}return rejected && n===1;})()`, "true"},
		{"global_capture_reject_and_read", `function change(){n=2;}const n=1;var rejected=false;try{change();}catch(e){rejected=e instanceof TypeError;}rejected && n===1`, "true"},
		{"block_shadow", `(function(s){for(let s=0;s<2;s++){const n=s;} {const s=4;} s++;return s;})(5)`, "6"},
		{"block_capture", `(function(){let n=1;var f;{const n=2;f=function(){return n;};}n++;return n===2 && f()===2;})()`, "true"},
		{"block_reject_recovery", `(function(){let n=1;var rejected=false;{const n=2;try{n=3;}catch(e){rejected=e instanceof TypeError;}}n++;return rejected && n===2;})()`, "true"},
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
