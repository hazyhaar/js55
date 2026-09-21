package engine

import "testing"

func TestArraySortForThreeIntersections(t *testing.T) {
	for _, src := range []string{
		`var a=[{distance:3,id:'a'},{distance:1,id:'b'},{distance:3,id:'c'}];a.sort(function(a,b){return a.distance-b.distance;});a.map(function(x){return x.id;}).join('')==='bac'`,
		`[10,2,1].sort().join(',')==='1,10,2'`,
		`var a=[2,1],rejected=false;try{a.sort(function(){throw new Error('reject');});}catch(e){rejected=true;}a.sort(function(a,b){return a-b;});rejected && a.join(',')==='1,2'`,
		`var a=[2,undefined,1];a.sort(function(a,b){return a-b;});a[0]===1 && a[1]===2 && a[2]===undefined`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: %q %v", src, got, err)
		}
	}
}
