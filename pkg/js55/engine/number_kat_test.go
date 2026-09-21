// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"encoding/json"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestNumberToStringVsNode(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent : le KAT Number::toString ne peut pas être mesuré")
	}
	samples := []float64{
		0,
		math.Copysign(0, -1),
		1,
		-1,
		0.1,
		func() float64 { x := 0.1; y := 0.2; return x + y }(),
		1e21,
		1e22,
		1e-6,
		1e-7,
		123456789012345680000,
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		42,
		-0.5,
		1.5e-20,
		9.007199254740992e15,
		1e40,
		1e100,
		math.MaxFloat64,
	}
	bits := make([]string, len(samples))
	for i, f := range samples {
		bits[i] = strconv.FormatUint(math.Float64bits(f), 16)
	}
	src := `
const bits = ` + mustJSON(bits) + `;
function fromBits(hex) {
  const buf = Buffer.from(hex.padStart(16, '0'), 'hex');
  return buf.readDoubleBE(0);
}
const out = bits.map((h) => String(fromBits(h)));
process.stdout.write(JSON.stringify(out));
`
	cmd := exec.Command("node", "-e", src)
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("oracle node : %v", err)
	}
	var nodeOut []string
	if err := json.Unmarshal(raw, &nodeOut); err != nil {
		t.Fatalf("décodage oracle : %v\n%s", err, raw)
	}
	if len(nodeOut) != len(samples) {
		t.Fatalf("%d résultats node pour %d échantillons", len(nodeOut), len(samples))
	}
	for i, f := range samples {
		got := numberToString(f)
		want := nodeOut[i]
		if got != want {
			t.Errorf("échantillon %d bits=%s js55=%q node=%q", i, bits[i], got, want)
		}
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestNumberToStringSentinels(t *testing.T) {
	if numberToString(math.NaN()) != "NaN" {
		t.Fatal("NaN")
	}
	if numberToString(math.Inf(1)) != "Infinity" {
		t.Fatal("Infinity")
	}
	if numberToString(math.Inf(-1)) != "-Infinity" {
		t.Fatal("-Infinity")
	}
	if numberToString(math.Copysign(0, -1)) != "0" {
		t.Fatal("-0")
	}
	if strings.Contains(numberToString(1), "INF") {
		t.Fatal("forme INF interdite")
	}
}
