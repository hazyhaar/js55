package engine

import "testing"

func TestMathFunctionsUsedByThree(t *testing.T) {
	for _, src := range []string{
		`Math.asin(0)===0 && Math.acos(1)===0 && Math.atan(0)===0 && Math.atan2(0,1)===0`,
		`Math.atan2("1","1")===Math.PI/4 && Number.isNaN(Math.asin(2)) && Math.asin(1)===Math.PI/2`,
		`Object.is(Math.asin(-0),-0) && Object.is(Math.atan2(-0,1),-0)`,
		`Math.fround(1.337)===1.3370000123977661 && Math.hypot(3,4,12)===13`,
		`var rejected=false;try{Math.pow(1n,1);}catch(e){rejected=e instanceof TypeError;}rejected && Math.pow(2,3)===8`,
		`var rejected=false;try{+1n;}catch(e){rejected=e instanceof TypeError;}rejected && +"2"===2`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}
