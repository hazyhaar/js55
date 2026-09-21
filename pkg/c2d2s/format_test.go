// SPDX-License-Identifier: Apache-2.0 OR MIT

package c2d2s

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestU128ToDecSmall(t *testing.T) {
	buf := make([]byte, 48)
	n := u128_to_dec(0, 12345, buf)
	got := string(buf[:n])
	if got != "12345" {
		t.Fatalf("u128_to_dec(12345)=%q", got)
	}
	n = u128_to_dec(0, 10, buf)
	if string(buf[:n]) != "10" {
		t.Fatalf("u128_to_dec(10)=%q", buf[:n])
	}
}

func TestFormatSentinels(t *testing.T) {
	cases := []struct {
		f    float64
		want string
	}{
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
		{0, "0"},
		{math.Copysign(0, -1), "0"},
		{1, "1"},
		{-1, "-1"},
		{42, "42"},
	}
	for _, tc := range cases {
		got := Format(tc.f)
		if got != tc.want {
			t.Errorf("Format(%v) = %q, attendu %q", tc.f, got, tc.want)
		}
	}
}

func TestFormatVsNode(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent")
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
		1e308,
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		math.Float64frombits(0x826f6856b2d48017),
		7.691318327087494e+134,
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
	for i, f := range samples {
		got := Format(f)
		want := nodeOut[i]
		if got != want {
			t.Errorf("échantillon %d bits=%s c2d2s=%q node=%q", i, bits[i], got, want)
		}
	}
}

func TestFormatVsNodeRandom(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent")
	}
	rng := rand.New(rand.NewSource(0xC2D25))
	const n = 20016
	samples := make([]float64, 0, n+4)
	samples = append(samples,
		math.SmallestNonzeroFloat64,
		math.MaxFloat64,
		math.Float64frombits(0x826f6856b2d48017),
		func() float64 { x := 0.1; y := 0.2; return x + y }(),
	)
	for len(samples) < n {
		u := rng.Uint64()
		f := math.Float64frombits(u)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			continue
		}
		samples = append(samples, f)
	}
	bits := make([]string, len(samples))
	for i, f := range samples {
		bits[i] = strconv.FormatUint(math.Float64bits(f), 16)
	}
	src := `
const bits = JSON.parse(require('fs').readFileSync(0, 'utf8'));
function fromBits(hex) {
  const buf = Buffer.from(hex.padStart(16, '0'), 'hex');
  return buf.readDoubleBE(0);
}
const out = bits.map((h) => String(fromBits(h)));
process.stdout.write(JSON.stringify(out));
`
	cmd := exec.Command("node", "-e", src)
	cmd.Stdin = bytes.NewReader([]byte(mustJSON(bits)))
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
	if len(samples) < 20016 {
		t.Fatalf("%d échantillons, 20016 exigés", len(samples))
	}
	fail := 0
	for i, f := range samples {
		got := Format(f)
		want := nodeOut[i]
		if got != want {
			if fail < 8 {
				t.Errorf("bits=%s c2d2s=%q node=%q", bits[i], got, want)
			}
			fail++
		}
	}
	if fail > 0 {
		t.Fatalf("%d divergences / %d", fail, len(samples))
	}
}

func TestFormatVsCOracle(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc absent")
	}
	cSrc := filepath.Join("..", "..", "c2simd", "sources", "c2d2s", "d2s.c")
	if _, err := os.Stat(cSrc); err != nil {
		cSrc = "/devhoros/c2simd/sources/c2d2s/d2s.c"
	}
	harness := `
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
uint32_t c2_ecma_d2s_buffered(double f, char *dst);
int main(int argc, char **argv) {
    if (argc < 2) return 1;
    double val = atof(argv[1]);
    char buf[64];
    c2_ecma_d2s_buffered(val, buf);
    printf("%s\n", buf);
    return 0;
}
`
	dir := t.TempDir()
	hFile := filepath.Join(dir, "harness.c")
	bin := filepath.Join(dir, "oracle")
	if err := os.WriteFile(hFile, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("gcc", "-O2", "-o", bin, hFile, cSrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc : %v\n%s", err, string(out))
	}
	samples := []string{"0", "1", "-1", "42", "inf", "-inf", "nan"}
	buf := make([]byte, 40)
	for _, arg := range samples {
		out, err := exec.Command(bin, arg).Output()
		if err != nil {
			t.Fatalf("oracle %s : %v", arg, err)
		}
		cOut := strings.TrimSpace(string(out))
		var f float64
		switch arg {
		case "inf":
			f = math.Inf(1)
		case "-inf":
			f = math.Inf(-1)
		case "nan":
			f = math.NaN()
		default:
			f, _ = strconv.ParseFloat(arg, 64)
		}
		n := C2_ecma_d2s_buffered(f, buf)
		goOut := string(buf[:n])
		if goOut != cOut {
			t.Errorf("%s : go=%q c=%q", arg, goOut, cOut)
		}
	}
}

func TestEcmaD2sBitsVsCOracle(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc absent")
	}
	cSrc := "/devhoros/c2simd/sources/c2d2s/d2s.c"
	harness := `
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
uint32_t c2_ecma_d2s_bits(uint64_t bits, double mag, char *dst);
int main(int argc, char **argv) {
    uint64_t bits;
    double mag;
    uint64_t absbits;
    char buf[64];
    if (argc < 2) return 1;
    bits = strtoull(argv[1], NULL, 16);
    memcpy(&mag, &bits, 8);
    absbits = bits;
    if (mag < 0.0 && mag == mag && mag + mag != mag) {
        absbits = bits & 0x7fffffffffffffffULL;
        mag = -mag;
    }
    c2_ecma_d2s_bits(absbits, mag, buf);
    printf("%s\n", buf);
    return 0;
}
`
	dir := t.TempDir()
	hFile := filepath.Join(dir, "harness.c")
	bin := filepath.Join(dir, "oracle")
	if err := os.WriteFile(hFile, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("gcc", "-O2", "-o", bin, hFile, cSrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc : %v\n%s", err, string(out))
	}
	samples := []float64{0, 1, -1, 0.5, 42, 1e-7, 1e-6, 0.3, 1e40, 1e100, math.MaxFloat64, math.Inf(1), math.Inf(-1)}
	buf := make([]byte, 64)
	for _, f := range samples {
		hex := strconv.FormatUint(math.Float64bits(f), 16)
		out, err := exec.Command(bin, hex).Output()
		if err != nil {
			t.Fatalf("oracle %g : %v", f, err)
		}
		cOut := strings.TrimSpace(string(out))
		bits := math.Float64bits(f)
		mag := f
		if finiteNeg(f) {
			bits &= 0x7fffffffffffffff
			mag = -f
		}
		n := C2_ecma_d2s_bits(bits, mag, buf)
		goOut := string(buf[:n])
		if goOut != cOut {
			t.Errorf("%g bits=%s go=%q c=%q", f, hex, goOut, cOut)
		}
	}
}

func TestAppendNoAlloc(t *testing.T) {
	dst := make([]byte, 0, 80)
	allocs := testing.AllocsPerRun(100, func() {
		_ = Append(dst[:0], 3.14159)
		_ = Append(dst[:0], 42)
	})
	if allocs != 0 {
		t.Fatalf("Append avec cap libre : Allocs/op = %.2f, attendu 0", allocs)
	}
}

func TestFormatZeroAlloc(t *testing.T) {
	buf := make([]byte, 40)
	allocs := testing.AllocsPerRun(1000, func() {
		_ = C2_ecma_d2s_buffered(3.1415926535, buf)
		_ = C2_ecma_d2s_buffered(math.Inf(1), buf)
		_ = C2_ecma_d2s_buffered(1e21, buf)
	})
	if allocs != 0 {
		t.Fatalf("Allocs/op = %.2f, attendu 0", allocs)
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
