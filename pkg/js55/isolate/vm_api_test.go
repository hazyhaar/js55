// SPDX-License-Identifier: Apache-2.0 OR MIT

package isolate

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

func TestVmCreateContextIsolation(t *testing.T) {
	a, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunInContext(context.Background(), `var x = 1;`); err != nil {
		t.Fatal(err)
	}
	if _, err := b.RunInContext(context.Background(), `x`); err == nil {
		t.Fatal("x d'un isolat visible dans l'autre")
	}
}

func TestVmRunInContextCompletion(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent")
	}
	programs := []string{
		`1 + 2`,
		`7 / 2`,
		`2 ** 10`,
		`-3 * -4`,
		`5 & 3`,
		`1 < 2`,
		`"abc".length`,
		`var a = 3; a * 4`,
		`1 && 2`,
		`[1, 2, 3].length`,
		`null ?? 7`,
		`typeof 1 === "number"`,
	}
	in, err := json.Marshal(map[string]any{"programs": programs})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "/devhoros/pkg/js55/engine/testdata/eval_oracle.js")
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("oracle : %v", err)
	}
	var got struct {
		Results []struct {
			Value   string `json:"value"`
			Display string `json:"display"`
			Name    string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != len(programs) {
		t.Fatalf("%d résultats", len(got.Results))
	}
	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i, src := range programs {
		ref := got.Results[i]
		if ref.Name != "" {
			t.Fatalf("programme %d : Node jette %s (%s)", i, ref.Name, src)
		}
		v, err := iso.RunInContext(ctx, src)
		if err != nil {
			t.Errorf("programme %d : %v\n  %s", i, err, src)
			continue
		}
		want := ref.Display
		if want == "" {
			want = ref.Value
		}
		if v.String() != want {
			t.Errorf("programme %d : js55 %q node %q\n  %s", i, v.String(), want, src)
		}
	}
}

func TestVmTimeout(t *testing.T) {
	iso, err := New(Config{GasLimit: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	_, err = iso.EvalTimeout(context.Background(), `while (true) {}`, 50*time.Millisecond)
	if err == nil {
		t.Fatal("timeout attendu")
	}
	iso2, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	v, err := iso2.EvalTimeout(context.Background(), `1+2`, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToInt() != 3 {
		t.Errorf("1+2 = %v", v)
	}
}
