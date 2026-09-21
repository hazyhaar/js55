// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"fmt"
	"github.com/hazyhaar/js55/pkg/js55/parser"
	"testing"
)

func TestArchtimeGenericRegressions(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"strict readonly then restore", `var o={x:1};Object.defineProperty(o,'x',{writable:false});var caught='';try{(function(){'use strict';o.x=2})()}catch(e){caught=e.name}var preserved=o.x;Object.defineProperty(o,'x',{writable:true});(function(){'use strict';o.x=3})();JSON.stringify([caught,preserved,o.x]);`, `["TypeError",1,3]`},
		{"sloppy readonly then restore", `var o={x:1};Object.defineProperty(o,'x',{writable:false});o.x=2;var preserved=o.x;Object.defineProperty(o,'x',{writable:true});o.x=3;JSON.stringify([preserved,o.x]);`, `[1,3]`},
		{"method getter throw then restore", `var fail=true,calls=0;var o={x:37};Object.defineProperty(o,'m',{get:function(){calls++;if(fail)throw Error('gate');return function(n){return this.x+n}},configurable:true});var caught='';try{o.m(5)}catch(e){caught=e.message}fail=false;var answer=o.m(5);JSON.stringify([caught,calls,answer]);`, `["gate",2,42]`},
	}
	for _, disable := range []bool{true, false} {
		for _, stress := range []bool{false, true} {
			for _, tc := range cases {
				t.Run(fmt.Sprintf("%s/disabled=%t/stress=%t", tc.name, disable, stress), func(t *testing.T) {
					h := NewHeap()
					vm := NewVM(h)
					vm.DisableArchtimeGeometry = disable
					h.SetStress(stress)
					p, err := parser.Parse(tc.src, parser.Options{})
					if err != nil {
						t.Fatal(err)
					}
					c, err := Compile(h, p, "generic-regression")
					if err != nil {
						t.Fatal(err)
					}
					v, err := vm.Run(c)
					if err != nil {
						t.Fatal(err)
					}
					s := vm.StringOf(v)
					if s == nil || s.GoString() != tc.want {
						t.Fatalf("want %s got %v", tc.want, vm.toDisplayString(v))
					}
				})
			}
		}
	}
}
