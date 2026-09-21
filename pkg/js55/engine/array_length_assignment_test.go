// SPDX-License-Identifier: BUSL-1.1
package engine

import "testing"

func TestArrayLengthAssignmentRenderCycle(t *testing.T) {
	for _, src := range []string{
		`var lights=[];function frame(){lights.length=0;lights.push(1);return lights.length;}frame()===1 && frame()===1 && frame()===1`,
		`var a=[1,2,3];a.length=1;var rejected=false;try{a.length=-1;}catch(e){rejected=e instanceof RangeError;}a.push(4);rejected && a.length===2 && a[0]===1 && a[1]===4 && a[2]===undefined`,
		`var a=[1,2];a.length=0;a.length=2;a[0]===undefined && a[1]===undefined && a.length===2`,
	} {
		got,err:=compileAndRun(t,src,false)
		if err!=nil || got!="true" {t.Fatalf("%s: %q %v",src,got,err)}
	}
}
