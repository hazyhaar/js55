Write le fichier `/devhoros/pkg/js55/engine/defineproperty_value_test.go` avec exactement :

```
package engine

import "testing"

func TestDefinePropertyValue(t *testing.T) {
	got, err := compileAndRun(t, `var o={}; Object.defineProperty(o,"a",{value:7}); o.a`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "7" {
		t.Errorf("obtenu %q, 7 attendu", got)
	}
}
```

Puis : `cd /devhoros && GOTOOLCHAIN=go1.27.0 go test -count=1 -run TestDefinePropertyValue ./pkg/js55/engine/`

Interdit : Read, Grep, Glob, tout autre fichier. Un Write, un Bash. Verdict : une ligne pass ou fail.
