// SPDX-License-Identifier: Apache-2.0 OR MIT

package c2strsimd

import (
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStrposStdLibParity(t *testing.T) {
	haystack := []byte("The quick brown fox jumps over the lazy dog. PHP 127 Sovereign Runtime.")
	tests := []string{"quick", "fox", "PHP", "Runtime", "dog.", "nonexistent", "T", "."}

	for _, needle := range tests {
		nBytes := []byte(needle)
		got := C2_strpos(haystack, uint64(len(haystack)), nBytes, uint64(len(nBytes)))
		expected := int64(strings.Index(string(haystack), needle))
		if got != expected {
			t.Fatalf("needle %q: got %d, want %d", needle, got, expected)
		}
	}
}

func TestToUpperToLowerStdLibParity(t *testing.T) {
	input := []byte("PHP 127 Sgoiter Transpiler -- Clean Room Kernel -- 2026")
	dstUp := make([]byte, len(input))
	dstLow := make([]byte, len(input))

	C2_strtoupper(dstUp, input, uint64(len(input)))
	if string(dstUp) != strings.ToUpper(string(input)) {
		t.Fatalf("ToUpper got %q, want %q", string(dstUp), strings.ToUpper(string(input)))
	}

	C2_strtolower(dstLow, input, uint64(len(input)))
	if string(dstLow) != strings.ToLower(string(input)) {
		t.Fatalf("ToLower got %q, want %q", string(dstLow), strings.ToLower(string(input)))
	}
}

func TestMbStrlenStdLibParity(t *testing.T) {
	utf8Str := []byte("Antigravity 🚀 — Go 1.27 Architecture Souveraine — été 2026")
	got := C2_mb_strlen_utf8(utf8Str, uint64(len(utf8Str)))
	expected := uint64(utf8.RuneCount(utf8Str))
	if got != expected {
		t.Fatalf("mb_strlen: got %d, want %d", got, expected)
	}
}

func TestStrsimdVsCOracle(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc non disponible")
	}

	cSrcPath, err := filepath.Abs(filepath.Join("..", "..", "sources", "c2strsimd.c"))
	if err != nil || cSrcPath == "" {
		t.Skip("source C strsimd.c introuvable")
	}

	testHarness := `
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

int64_t c2_strpos(const uint8_t *haystack, uint64_t h_len, const uint8_t *needle, uint64_t n_len);
void c2_strtoupper(uint8_t *dst, const uint8_t *src, uint64_t len);
uint64_t c2_mb_strlen_utf8(const uint8_t *src, uint64_t byte_len);

int main(int argc, char **argv) {
    if (argc < 2) return 1;
    const char *text = argv[1];
    uint64_t len = (uint64_t)strlen(text);

    uint64_t ulen = c2_mb_strlen_utf8((const uint8_t*)text, len);
    printf("UTF8_LEN=%llu\n", (unsigned long long)ulen);

    char *up = (char*)malloc(len + 1);
    c2_strtoupper((uint8_t*)up, (const uint8_t*)text, len);
    up[len] = '\0';
    printf("UPPER=%s\n", up);
    free(up);

    if (argc >= 3) {
        const char *needle = argv[2];
        uint64_t nlen = (uint64_t)strlen(needle);
        int64_t pos = c2_strpos((const uint8_t*)text, len, (const uint8_t*)needle, nlen);
        printf("POS=%lld\n", (long long)pos);
    }
    return 0;
}
`
	tmpDir := t.TempDir()
	harnessFile := filepath.Join(tmpDir, "harness.c")
	binFile := filepath.Join(tmpDir, "oracle_strsimd")
	if err := os.WriteFile(harnessFile, []byte(testHarness), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("gcc", "-O2", "-Wall", "-Wextra", "-Werror", "-o", binFile, harnessFile, cSrcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gcc compile failed: %v\n%s", err, string(out))
	}

	rng := rand.New(rand.NewSource(0x12345678))
	sampleTexts := []string{
		"Hello World from Go 1.27 and C2SIMD!",
		"Développement souverain sous /devhoros avec sgoiter.",
		"Quick brown fox jumps over lazy dog.",
		"Zéro-allocation et parité mécanique bit-exacte.",
	}

	for _, text := range sampleTexts {
		needle := text[rng.Intn(len(text)/2) : len(text)/2+rng.Intn(len(text)/2)]
		runCmd := exec.Command(binFile, text, needle)
		out, err := runCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("oracle execution failed: %v\n%s", err, string(out))
		}

		goPos := C2_strpos([]byte(text), uint64(len(text)), []byte(needle), uint64(len(needle)))
		goLen := C2_mb_strlen_utf8([]byte(text), uint64(len(text)))
		dstUp := make([]byte, len(text))
		C2_strtoupper(dstUp, []byte(text), uint64(len(text)))

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, l := range lines {
			if strings.HasPrefix(l, "UTF8_LEN=") {
				cLen, parseErr := strconv.ParseUint(strings.TrimPrefix(l, "UTF8_LEN="), 10, 64)
				if parseErr != nil {
					t.Fatalf("Échec parsing oracle C len: %v", parseErr)
				}
				if goLen != cLen {
					t.Fatalf("UTF8 len divergence pour %q: Go=%d Oracle_C=%d", text, goLen, cLen)
				}
			} else if strings.HasPrefix(l, "POS=") {
				cPos, parseErr := strconv.ParseInt(strings.TrimPrefix(l, "POS="), 10, 64)
				if parseErr != nil {
					t.Fatalf("Échec parsing oracle C pos: %v", parseErr)
				}
				if goPos != cPos {
					t.Fatalf("POS divergence pour %q dans %q: Go=%d Oracle_C=%d", needle, text, goPos, cPos)
				}
			} else if strings.HasPrefix(l, "UPPER=") {
				cUp := strings.TrimPrefix(l, "UPPER=")
				if string(dstUp) != cUp {
					t.Fatalf("UPPER divergence: Go=%q Oracle_C=%q", string(dstUp), cUp)
				}
			}
		}
	}
}

func TestStrsimdZeroAlloc(t *testing.T) {
	haystack := []byte("The quick brown fox jumps over the lazy dog.")
	needle := []byte("fox")
	dst := make([]byte, len(haystack))

	allocs := testing.AllocsPerRun(1000, func() {
		_ = C2_strpos(haystack, uint64(len(haystack)), needle, uint64(len(needle)))
		_ = C2_mb_strlen_utf8(haystack, uint64(len(haystack)))
		C2_strtoupper(dst, haystack, uint64(len(haystack)))
	})
	if allocs != 0 {
		t.Fatalf("Allocs/op = %.2f, attendu 0", allocs)
	}
}
