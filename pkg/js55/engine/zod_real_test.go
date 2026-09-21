// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

func TestZodPublishedEval(t *testing.T) {
	b, err := os.ReadFile("../testdata/zod.umd.js")
	if err != nil {
		t.Skip(err)
	}
	if len(b) < 10000 {
		t.Fatalf("zod.umd.js trop court (%d)", len(b))
	}
	src := string(b) + `;
		if (typeof Zod === "undefined") { throw new Error("Zod global absent"); }
		var z = Zod.z;
		if (typeof z === "undefined" || typeof z.string !== "function") { throw new Error("z.string absent"); }
		if (z.string().parse("hi") !== "hi") { throw new Error("parse string"); }
		if (z.number().parse(3) !== 3) { throw new Error("parse number"); }
		var o = z.object({a: z.string()}).parse({a: "x"});
		if (o.a !== "x") { throw new Error("parse object"); }
	`
	if _, err := compileAndRun(t, src, false); err != nil {
		t.Fatalf("zod publié : %v", err)
	}
}

func TestZodDifferentialVsNode(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent")
	}
	b, err := os.ReadFile("../testdata/zod.umd.js")
	if err != nil {
		t.Skip(err)
	}
	prefix := string(b) + "; var z = Zod.z;\n"
	cases := []string{
		`z.string().parse("hi")`,
		`z.number().parse(3)`,
		`z.string().parse(1)`,
		`z.number().parse("x")`,
	}
	var srcs []string
	for _, c := range cases {
		srcs = append(srcs, prefix+c)
	}
	in, err := json.Marshal(map[string]any{"programs": srcs})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "testdata/eval_oracle.js")
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("oracle : %v", err)
	}
	var got struct {
		Results []nodeResult `json:"results"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != len(cases) {
		t.Fatalf("%d résultats", len(got.Results))
	}
	for i, c := range cases {
		mine, name, msg, err := compileAndEncode(t, srcs[i], false)
		why := t42Diverge(got.Results[i], mine, name, msg, err)
		if why == "" {
			continue
		}
		if i >= 2 {
			t.Logf("hostile mesuré %s : %s (js55 %v / Node %s)", c, why, err, got.Results[i].Name)
			continue
		}
		t.Errorf("%s : %s\n  js55 %q %v\n  node %+v", c, why, mine, err, got.Results[i])
	}
}
