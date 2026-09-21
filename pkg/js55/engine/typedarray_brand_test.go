package engine

import "testing"

func TestArrayIsArrayExcludesTypedViews(t *testing.T) {
	for _, src := range []string{
		`!Array.isArray(new Float32Array([1,2,3])) && Array.isArray([1,2,3])`,
		`!Array.isArray(new Uint8Array(new ArrayBuffer(8))) && !Array.isArray(new DataView(new ArrayBuffer(8)))`,
		`var a=[];a.BYTES_PER_ELEMENT=4;Object.setPrototypeOf(a,Float32Array.prototype);Array.isArray(a)`,
		`var a=new Float32Array([1,2,3]);Object.setPrototypeOf(a,Array.prototype);!Array.isArray(a)`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}

func TestTypedArraySetCopiesAndRejectsOverflow(t *testing.T) {
	for _, src := range []string{
		`var a=new Float32Array(4);a.set([1,2],1);a[0]===0 && a[1]===1 && a[2]===2 && a[3]===0`,
		`var a=new Float32Array([1,2,3]);a.set(a);a[0]===1 && a[2]===3`,
		`var a=new Float32Array([1,2,3]);var rejected=false;try{a.set([4,5],2);}catch(e){rejected=e instanceof RangeError;}a.set([6],1);rejected && a[0]===1 && a[1]===6 && a[2]===3`,
		`var a=new Uint8Array(new ArrayBuffer(4));a.set([1,2],1);a[0]===0 && a[1]===1 && a[2]===2 && a[3]===0`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}

func TestTypedArrayFloat32Storage(t *testing.T) {
	for _, src := range []string{
		`var a=new Float32Array([1.337]);a[0]===1.3370000123977661`,
		`var a=new Float32Array(1);a[0]=1.337;a[1]=2;a.length===1 && a[0]===1.3370000123977661`,
		`var a=new Float32Array(1);a.set([1.337]);a[0]===1.3370000123977661`,
		`var a=new Uint8Array([257,-1]);a[0]===1 && a[1]===255`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}
