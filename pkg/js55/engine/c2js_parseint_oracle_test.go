// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 HazyHaar. See LICENSE and NOTICE.

package engine

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// The c2js_parseint_*_gen.go files are transpiled by sgoiter from the C
// sources of /devhoros/c2simd/sources/js55. This oracle compiles the same
// sources with gcc -O2 and a driver, then requires the C output and the output
// of the transpiled Go to be identical byte for byte on the same inputs.

const c2jsOracleDriver = `
#include <stdint.h>
#include <stdio.h>
int32_t c2js_parseint_is_digit(int32_t, int32_t);
int32_t c2js_parseint_digit(int32_t, int32_t);
int32_t c2js_parseint_is_binary_digit(int32_t);
int32_t c2js_parseint_radix_ok(int32_t);
int32_t c2js_parseint_skip_zeros(const uint8_t *, int32_t);
int32_t c2js_parseint_r16_exact(const uint8_t *, int32_t, uint64_t *);
static uint64_t st = 0x9E3779B97F4A7C15ull;
static uint64_t next(void) { st ^= st << 13; st ^= st >> 7; st ^= st << 17; return st; }
int main(void) {
	static const char alpha[] = "0000123456789abcdefABCDEFxgz";
	for (int32_t r = 2; r <= 36; r++)
		for (int32_t c = -300; c <= 70000; c += 7)
			printf("d %d %d %d %d\n", c, r, c2js_parseint_is_digit(c, r), c2js_parseint_digit(c, r));
	for (int32_t c = -300; c <= 300; c++) printf("b %d %d\n", c, c2js_parseint_is_binary_digit(c));
	for (int32_t r = -100; r <= 100; r++) printf("r %d %d\n", r, c2js_parseint_radix_ok(r));
	uint8_t s[48];
	for (int k = 0; k < 5000; k++) {
		int32_t n = (int32_t)(next() % 41);
		for (int32_t i = 0; i < n; i++) s[i] = (uint8_t)alpha[next() % (sizeof(alpha) - 1)];
		uint64_t num = 0;
		int32_t used = c2js_parseint_r16_exact(s, n, &num);
		printf("s %d %d %d %llu\n", n, c2js_parseint_skip_zeros(s, n), used, (unsigned long long)num);
	}
	return 0;
}
`

func c2jsOracleSources(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	for _, dir := range []string{
		filepath.Join(filepath.Dir(file), "..", "..", "..", "c2simd", "sources", "js55"),
		"/devhoros/c2simd/sources/js55",
	} {
		if _, err := os.Stat(filepath.Join(dir, "c2js_parseint_is_digit.c")); err == nil {
			return dir
		}
	}
	t.Skip("sources C de c2js_parseint absentes (dépôt publié sans c2simd)")
	return ""
}

func TestC2jsParseintVsGCCOracle(t *testing.T) {
	src := c2jsOracleSources(t)
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc absent")
	}
	work := t.TempDir()
	driver := filepath.Join(work, "driver.c")
	if err := os.WriteFile(driver, []byte(c2jsOracleDriver), 0o600); err != nil {
		t.Fatal(err)
	}
	sources, _ := filepath.Glob(filepath.Join(src, "c2js_parseint_*.c"))
	if len(sources) != 6 {
		t.Fatalf("attendu 6 sources C, trouvé %d dans %s", len(sources), src)
	}
	bin := filepath.Join(work, "oracle")
	args := append([]string{"-O2", "-Wall", "-Wextra", "-Werror", "-fsanitize=address,undefined", "-o", bin, driver}, sources...)
	if out, err := exec.Command(gcc, args...).CombinedOutput(); err != nil {
		t.Fatalf("gcc : %v\n%s", err, out)
	}
	want, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("oracle C : %v", err)
	}

	var got bytes.Buffer
	for r := int32(2); r <= 36; r++ {
		for c := int32(-300); c <= 70000; c += 7 {
			fmt.Fprintf(&got, "d %d %d %d %d\n", c, r, C2js_parseint_is_digit(c, r), C2js_parseint_digit(c, r))
		}
	}
	for c := int32(-300); c <= 300; c++ {
		fmt.Fprintf(&got, "b %d %d\n", c, C2js_parseint_is_binary_digit(c))
	}
	for r := int32(-100); r <= 100; r++ {
		fmt.Fprintf(&got, "r %d %d\n", r, C2js_parseint_radix_ok(r))
	}
	const alpha = "0000123456789abcdefABCDEFxgz"
	st := uint64(0x9E3779B97F4A7C15)
	next := func() uint64 { st ^= st << 13; st ^= st >> 7; st ^= st << 17; return st }
	s := make([]byte, 48)
	for k := 0; k < 5000; k++ {
		n := int32(next() % 41)
		for i := int32(0); i < n; i++ {
			s[i] = alpha[next()%uint64(len(alpha))]
		}
		var num uint64
		used := C2js_parseint_r16_exact(s, n, &num)
		fmt.Fprintf(&got, "s %d %d %d %d\n", n, C2js_parseint_skip_zeros(s, n), used, num)
	}

	if !bytes.Equal(got.Bytes(), want) {
		g, w := got.Bytes(), want
		i := 0
		for i < len(g) && i < len(w) && g[i] == w[i] {
			i++
		}
		lo := max(0, i-80)
		t.Fatalf("écart Go/gcc à l'octet %d\n  go  : %q\n  gcc : %q", i, g[lo:min(len(g), i+80)], w[lo:min(len(w), i+80)])
	}
	t.Logf("parité bit-exacte Go/gcc : %d octets", len(want))
}
