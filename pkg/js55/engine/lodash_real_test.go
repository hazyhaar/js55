// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

func TestHoistVarClosedOverByInnerFunction(t *testing.T) {
	got, err := compileAndRun(t, `(function(){ function a(){ return x; } var x = 3; return a(); })()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "3" {
		t.Fatalf("obtenu %q, 3 attendu", got)
	}
}

func TestHoistSiblingFunctionInIIFE(t *testing.T) {
	got, err := compileAndRun(t, `(function(){ function a(){ return b(); } function b(){ return 7; } return a(); })()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "7" {
		t.Fatalf("obtenu %q, 7 attendu", got)
	}
}

func TestGlobalObjectMirrorsBuiltins(t *testing.T) {
	got, err := compileAndRun(t, `globalThis.Array === Array && this.Array === Array && Function("return this.Array")() === Array`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("obtenu %q, true attendu", got)
	}
}

func TestLodashMinVendorEval(t *testing.T) {
	b, err := os.ReadFile("../testdata/lodash.min.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 10000 {
		t.Fatalf("lodash.min.js trop court (%d) : pas un paquet réel", len(b))
	}
	src := string(b) + `;
		if (typeof _ === "undefined") { throw new Error("lodash global absent"); }
		if (!_.isEmpty({})) { throw new Error("isEmpty objet vide"); }
		if (_.isEmpty({a:1})) { throw new Error("isEmpty objet non vide"); }
		if (_.identity(7) !== 7) { throw new Error("identity"); }
	`
	if _, err := compileAndRun(t, src, false); err != nil {
		t.Fatalf("lodash.min.js: %v", err)
	}
}

func TestLodashPublishedEval(t *testing.T) {
	b, err := os.ReadFile("../testdata/lodash.published.min.js")
	if err != nil {
		t.Skip(err)
	}
	src := string(b) + `;
		if (typeof _ === "undefined") { throw new Error("lodash global absent"); }
		if (_.identity(7) !== 7) { throw new Error("identity"); }
		if (_.camelCase("Foo Bar") !== "fooBar") { throw new Error("camelCase"); }
		if (_.words("fred, barney, & pebbles").join(",") !== "fred,barney,pebbles") { throw new Error("words"); }
	`
	if _, err := compileAndRun(t, src, false); err != nil {
		t.Fatalf("lodash publié : %v", err)
	}
}

func TestLodashDifferentialVsNode(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent")
	}
	b, err := os.ReadFile("../testdata/lodash.published.min.js")
	if err != nil {
		t.Skip(err)
	}
	prefix := string(b) + ";\n"
	cases := []string{
		`_.map([1,2,3], function(n){ return n*2 }).join(",")`,
		`_.filter([1,2,3,4], function(n){ return n%2===0 }).join(",")`,
		`_.uniq([1,1,2,2,3]).join(",")`,
		`_.get({a:{b:4}}, "a.b")`,
		`_.compact([0,1,false,2,null]).join(",")`,
		`_.head([])`,
		`_.get({}, "missing")`,
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
		if why := t42Diverge(got.Results[i], mine, name, msg, err); why != "" {
			t.Errorf("%s : %s\n  js55 %q %v\n  node %+v", c, why, mine, err, got.Results[i])
		}
	}
}
