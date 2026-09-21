// SPDX-License-Identifier: BUSL-1.1

package conformance

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizeSignature(t *testing.T) {
	got := NormalizeSignature("runtime TypeError: exception non interceptée : TypeError: Cannot read properties of undefined (ligne 132)")
	if !strings.HasPrefix(got, "TypeError | ") {
		t.Fatalf("type attendu TypeError, obtenu %q", got)
	}
	if strings.Contains(got, "ligne") {
		t.Fatalf("numéro de ligne encore présent : %q", got)
	}
	if NormalizeSignature("") != "fail-silent" {
		t.Fatal("vide → fail-silent")
	}
	if len(NormalizeSignature(strings.Repeat("x", 200))) > len("Error | ")+sigMsgMax+8 {
		t.Fatal("troncature trop longue")
	}
}

func TestClusterBySignatureOrderAndExamples(t *testing.T) {
	cases := []TestCase{
		{RelPath: "a.js", Mode: ModeSloppy, Verdict: VerdictFail, Error: "runtime TypeError: foo (ligne 1)"},
		{RelPath: "b.js", Mode: ModeStrict, Verdict: VerdictFail, Error: "runtime TypeError: foo (ligne 9)"},
		{RelPath: "c.js", Mode: ModeSloppy, Verdict: VerdictFail, Error: "runtime TypeError: foo (ligne 3)"},
		{RelPath: "d.js", Mode: ModeSloppy, Verdict: VerdictFail, Error: "runtime ReferenceError: bar"},
		{RelPath: "e.js", Mode: ModeSloppy, Verdict: VerdictPass, Error: ""},
	}
	aggs := ClusterBySignature(cases)
	if len(aggs) != 2 {
		t.Fatalf("%d agrégats, 2 attendus", len(aggs))
	}
	if aggs[0].Size != 3 || aggs[1].Size != 1 {
		t.Fatalf("tailles %d %d, attendu 3 puis 1", aggs[0].Size, aggs[1].Size)
	}
	if len(aggs[0].Examples) != 3 {
		t.Fatalf("%d exemples, 3 attendus", len(aggs[0].Examples))
	}
	var buf bytes.Buffer
	if err := WriteAggregates(&buf, "t", aggs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "AGRÉGATS t (2 signatures)") {
		t.Fatalf("en-tête manquant : %s", buf.String())
	}
}
