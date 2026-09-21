Write le fichier `/devhoros/pkg/js55/engine/json_array_undef_test.go` avec exactement :

```
package engine

import "testing"

func TestJSONArrayUndefined(t *testing.T) {
	got, err := compileAndRun(t, `JSON.stringify([undefined])`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[null]" {
		t.Errorf("obtenu %q, [null] attendu", got)
	}
}
```

Puis : `cd /devhoros && GOTOOLCHAIN=go1.27.0 go test -count=1 -run TestJSONArrayUndefined ./pkg/js55/engine/`

Interdit : Read, Grep, Glob, tout autre fichier. Un Write, un Bash. Verdict : une ligne pass ou fail.
