package engine

import "testing"

func TestForInRendererLexicalAndNullishRecovery(t *testing.T) {
	// Reduced from WebGLGeometries.update in the unchanged GAFP Three r128.
	src := `(function(t){
var seen='';
for(const t in {position:1,skip:2}) { if(t==='skip') continue; seen+=t; }
for(let t in {stop:1}) { break; }
for(const k in undefined) { throw new Error('undefined enumerated'); }
for(const k in null) { throw new Error('null enumerated'); }
var rejected=false; try { Object.keys(null); } catch(e) { rejected=true; }
for(const t in {normal:1}) { seen+=t; }
return t.morphAttributes.ok+':'+seen+':'+rejected;
})({morphAttributes:{ok:'retained'}})`
	for _, stress := range []bool{false, true} {
		got, err := compileAndRun(t, src, stress)
		if err != nil || got != "retained:positionnormal:true" {
			t.Fatalf("stress=%v got=%q err=%v", stress, got, err)
		}
	}
}
