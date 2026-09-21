// SPDX-License-Identifier: Apache-2.0 OR MIT

package c2strsimd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// Les noyaux traitent 32 octets par tour puis déroulent une queue scalaire. Les
// tailles ci-dessous encadrent chaque frontière de bloc, là où une queue mal
// bornée ou un reliquat de bloc se voient.
var parityLens = []int{0, 1, 2, 15, 16, 31, 32, 33, 47, 63, 64, 65, 95, 96, 97, 128, 129}

// lcgBytes rend une suite d'octets déterministe, reproduite à l'identique par
// le harnais C de TestStrsimdAvx2VsCOracleBoundaries.
func lcgBytes(n int, seed uint32) []byte {
	out := make([]byte, n)
	x := seed
	for i := 0; i < n; i++ {
		x = x*1103515245 + 12345
		out[i] = byte((x >> 16) & 0xFF)
	}
	return out
}

// refToUpper et refToLower sont la définition de référence sur la plage
// complète des 256 valeurs d'octet : seules les 26 lettres ASCII basculent, les
// octets >= 0x80 restent intacts. bytes.ToUpper ne convient pas comme oracle
// ici, car il interprète l'entrée comme de l'UTF-8.
func refToUpper(src []byte) []byte {
	out := make([]byte, len(src))
	for i, c := range src {
		if c >= 'a' && c <= 'z' {
			c -= 0x20
		}
		out[i] = c
	}
	return out
}

func refToLower(src []byte) []byte {
	out := make([]byte, len(src))
	for i, c := range src {
		if c >= 'A' && c <= 'Z' {
			c += 0x20
		}
		out[i] = c
	}
	return out
}

// TestMemchrBoundariesVsStdlib confronte C2_memchr à bytes.IndexByte pour
// chaque position possible de la sentinelle, autour des frontières de bloc.
func TestMemchrBoundariesVsStdlib(t *testing.T) {
	const target = byte(0x7E)
	for _, n := range parityLens {
		base := lcgBytes(n, 0x2A57F001)
		for i := range base {
			if base[i] == target {
				base[i] = 0
			}
		}
		if got, want := C2_memchr(base, uint64(n), target), int64(bytes.IndexByte(base, target)); got != want {
			t.Fatalf("n=%d sans sentinelle : got %d, want %d", n, got, want)
		}
		for pos := 0; pos < n; pos++ {
			buf := append([]byte(nil), base...)
			buf[pos] = target
			got := C2_memchr(buf, uint64(n), target)
			want := int64(bytes.IndexByte(buf, target))
			if got != want {
				t.Fatalf("n=%d pos=%d : got %d, want %d", n, pos, got, want)
			}
		}
	}
}

// TestCaseFoldBoundariesVsReference confronte les deux conversions de casse à
// la définition de référence sur des octets couvrant les 256 valeurs.
func TestCaseFoldBoundariesVsReference(t *testing.T) {
	for _, n := range parityLens {
		src := lcgBytes(n, 0x51F3C0DE)
		dstUp := make([]byte, n)
		dstLow := make([]byte, n)

		C2_strtoupper(dstUp, src, uint64(n))
		if want := refToUpper(src); !bytes.Equal(dstUp, want) {
			t.Fatalf("strtoupper n=%d : got %x, want %x", n, dstUp, want)
		}
		C2_strtolower(dstLow, src, uint64(n))
		if want := refToLower(src); !bytes.Equal(dstLow, want) {
			t.Fatalf("strtolower n=%d : got %x, want %x", n, dstLow, want)
		}
	}
}

// TestCaseFoldAsciiVsStdlib recoupe la conversion de casse avec la
// bibliothèque standard sur de l'ASCII pur, où les deux définitions coïncident.
func TestCaseFoldAsciiVsStdlib(t *testing.T) {
	alphabet := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-@[]{}`~")
	for _, n := range parityLens {
		src := make([]byte, n)
		for i := range src {
			src[i] = alphabet[i%len(alphabet)]
		}
		dstUp := make([]byte, n)
		dstLow := make([]byte, n)

		C2_strtoupper(dstUp, src, uint64(n))
		if want := bytes.ToUpper(src); !bytes.Equal(dstUp, want) {
			t.Fatalf("strtoupper ASCII n=%d : got %q, want %q", n, dstUp, want)
		}
		C2_strtolower(dstLow, src, uint64(n))
		if want := bytes.ToLower(src); !bytes.Equal(dstLow, want) {
			t.Fatalf("strtolower ASCII n=%d : got %q, want %q", n, dstLow, want)
		}
	}
}

// TestMbStrlenBoundariesVsStdlib confronte le comptage de points de code à
// unicode/utf8 sur des chaînes dont la longueur en octets traverse les
// frontières de bloc, séquences multi-octets comprises.
func TestMbStrlenBoundariesVsStdlib(t *testing.T) {
	motif := "aé€🚀z"
	for _, n := range parityLens {
		var b strings.Builder
		for b.Len() < n {
			b.WriteString(motif)
		}
		full := []byte(b.String())
		// Coupe à la frontière de rune la plus proche sous n octets, pour que
		// l'oracle utf8.RuneCount porte sur une chaîne bien formée.
		cut := n
		if cut > len(full) {
			cut = len(full)
		}
		for cut > 0 && !utf8.Valid(full[:cut]) {
			cut--
		}
		src := full[:cut]
		got := C2_mb_strlen_utf8(src, uint64(len(src)))
		want := uint64(utf8.RuneCount(src))
		if got != want {
			t.Fatalf("n=%d len=%d : got %d, want %d", n, len(src), got, want)
		}
	}
}

// TestStrsimdAvx2VsCOracleBoundaries confronte les noyaux transpilés au binaire
// gcc -O2 de la même source C, sur les mêmes octets, taille par taille. Les
// deux côtés dérivent leurs données du même générateur congruentiel, donc la
// comparaison porte sur des entrées identiques sans les transporter.
func TestStrsimdAvx2VsCOracleBoundaries(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc non disponible")
	}
	cSrcPath, err := filepath.Abs(filepath.Join("..", "..", "sources", "c2strsimd.c"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cSrcPath); err != nil {
		t.Fatalf("source C introuvable : %v", err)
	}

	harness := `
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>

int64_t c2_memchr(const uint8_t *src, uint64_t len, uint8_t target);
void c2_strtoupper(uint8_t *dst, const uint8_t *src, uint64_t len);
void c2_strtolower(uint8_t *dst, const uint8_t *src, uint64_t len);
uint64_t c2_mb_strlen_utf8(const uint8_t *src, uint64_t byte_len);

static void print_hex(const char *label, const uint8_t *p, unsigned long n) {
    unsigned long i;
    printf("%s=", label);
    for (i = 0; i < n; i++) printf("%02x", p[i]);
    printf("\n");
}

int main(int argc, char **argv) {
    unsigned long n, i;
    uint32_t x;
    uint8_t *buf, *up, *low;
    if (argc < 3) return 1;
    n = strtoul(argv[1], 0, 10);
    x = (uint32_t)strtoul(argv[2], 0, 10);
    buf = (uint8_t *)malloc(n ? n : 1);
    up = (uint8_t *)malloc(n ? n : 1);
    low = (uint8_t *)malloc(n ? n : 1);
    for (i = 0; i < n; i++) {
        x = x * 1103515245u + 12345u;
        buf[i] = (uint8_t)((x >> 16) & 0xFF);
    }
    printf("MEMCHR=%lld\n", (long long)c2_memchr(buf, n, (uint8_t)0x7E));
    printf("UTF8LEN=%llu\n", (unsigned long long)c2_mb_strlen_utf8(buf, n));
    c2_strtoupper(up, buf, n);
    c2_strtolower(low, buf, n);
    print_hex("UPPER", up, n);
    print_hex("LOWER", low, n);
    free(buf); free(up); free(low);
    return 0;
}
`
	dir := t.TempDir()
	harnessFile := filepath.Join(dir, "harness.c")
	binFile := filepath.Join(dir, "oracle_c2strsimd")
	if err := os.WriteFile(harnessFile, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("gcc", "-O2", "-Wall", "-Wextra", "-Werror", "-o", binFile, harnessFile, cSrcPath)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("gcc : %v\n%s", err, out)
	}

	const seed = uint32(0x2A57F001)
	for _, n := range parityLens {
		out, err := exec.Command(binFile, fmt.Sprint(n), fmt.Sprint(seed)).CombinedOutput()
		if err != nil {
			t.Fatalf("oracle n=%d : %v\n%s", n, err, out)
		}
		fields := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if k, v, ok := strings.Cut(line, "="); ok {
				fields[k] = v
			}
		}

		src := lcgBytes(n, seed)
		if got, want := fmt.Sprint(C2_memchr(src, uint64(n), 0x7E)), fields["MEMCHR"]; got != want {
			t.Fatalf("memchr n=%d : Go=%s oracle_C=%s", n, got, want)
		}
		if got, want := fmt.Sprint(C2_mb_strlen_utf8(src, uint64(n))), fields["UTF8LEN"]; got != want {
			t.Fatalf("mb_strlen n=%d : Go=%s oracle_C=%s", n, got, want)
		}
		dstUp := make([]byte, n)
		C2_strtoupper(dstUp, src, uint64(n))
		if got, want := fmt.Sprintf("%x", dstUp), fields["UPPER"]; got != want {
			t.Fatalf("strtoupper n=%d : Go=%s oracle_C=%s", n, got, want)
		}
		dstLow := make([]byte, n)
		C2_strtolower(dstLow, src, uint64(n))
		if got, want := fmt.Sprintf("%x", dstLow), fields["LOWER"]; got != want {
			t.Fatalf("strtolower n=%d : Go=%s oracle_C=%s", n, got, want)
		}
	}
}
