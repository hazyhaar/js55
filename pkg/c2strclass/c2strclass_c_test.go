package c2strclass

// Oracle de parité du protocole P4 (/devhoros/pkg/js55/PLAN_MOTEUR.md §J2.3).
//
// Le Go de ce paquet est ÉMIS par sgoiter depuis
// /devhoros/c2simd/sources/c2strclass/c2strclass.c. Ce test compile la même
// source C avec gcc -O2 et compare les deux sorties par ÉGALITÉ STRICTE, octet
// par octet. Aucune tolérance n'intervient : le terme « bit-exact » est donc
// employé ici à bon droit.
//
// Aucun code C n'est écrit en dur dans ce fichier au titre de la source testée :
// le driver ci-dessous se contente d'appeler la source réelle du dépôt, dont le
// chemin absolu est cSourcePath (interdit I2 du plan).

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const cSourcePath = "/devhoros/c2simd/sources/c2strclass/c2strclass.c"
const cHeaderDir = "/devhoros/c2simd/sources/c2strclass"

// driver lit un corpus sur son entrée standard et écrit les résultats des quatre
// noyaux sur sa sortie standard, en binaire. Il n'implémente aucune logique
// propre : toute la sémantique vient de c2strclass.c.
const driver = `
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include "c2strclass.h"

int main(void) {
    static uint8_t buf[1 << 16];
    static uint8_t out[1 << 16];
    static uint16_t u16[1 << 15];
    uint32_t n;
    while (fread(&n, sizeof(n), 1, stdin) == 1) {
        if (n > sizeof(buf)) return 2;
        if (n && fread(buf, 1, n, stdin) != n) return 3;

        c2strclass_utf8_classify(buf, n, out);
        fwrite(out, 1, n, stdout);

        c2strclass_lex_class(buf, n, out);
        fwrite(out, 1, n, stdout);

        uint8_t tmp[sizeof(buf)];
        memcpy(tmp, buf, n);
        c2strclass_ascii_upper(tmp, n);
        fwrite(tmp, 1, n, stdout);

        size_t m = n / 2;
        memcpy(u16, buf, m * 2);
        uint8_t r = c2strclass_is_latin1_u16(u16, m);
        fwrite(&r, 1, 1, stdout);
    }
    return 0;
}
`

// buildCOracle compile le driver contre la source C réelle et rend le chemin du
// binaire produit, hors de l'arbre du dépôt (interdit I6).
func buildCOracle(t *testing.T) string {
	t.Helper()

	if _, err := os.Stat(cSourcePath); err != nil {
		t.Skipf("source C absente (%s) : la parité ne peut pas être mesurée", cSourcePath)
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc absent : la parité ne peut pas être mesurée")
	}

	dir := t.TempDir()
	drvPath := filepath.Join(dir, "driver.c")
	if err := os.WriteFile(drvPath, []byte(driver), 0o644); err != nil {
		t.Fatalf("écriture du driver : %v", err)
	}
	bin := filepath.Join(dir, "oracle")

	cmd := exec.Command("gcc", "-O2", "-Wall", "-Wextra", "-I", cHeaderDir,
		"-o", bin, drvPath, cSourcePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compilation de l'oracle C : %v\n%s", err, out)
	}
	return bin
}

// goSide calcule les mêmes sorties que le driver, avec le Go émis.
func goSide(bufs [][]byte) []byte {
	var w bytes.Buffer
	for _, b := range bufs {
		n := uint64(len(b))
		out := make([]byte, len(b))

		C2strclass_utf8_classify(b, n, out)
		w.Write(out)

		C2strclass_lex_class(b, n, out)
		w.Write(out)

		tmp := append([]byte(nil), b...)
		C2strclass_ascii_upper(tmp, n)
		w.Write(tmp)

		m := len(b) / 2
		u16 := make([]uint16, m)
		for i := 0; i < m; i++ {
			u16[i] = binary.LittleEndian.Uint16(b[i*2:])
		}
		w.WriteByte(C2strclass_is_latin1_u16(u16, uint64(m)))
	}
	return w.Bytes()
}

func runCOracle(t *testing.T, bin string, bufs [][]byte) []byte {
	t.Helper()
	var in bytes.Buffer
	for _, b := range bufs {
		_ = binary.Write(&in, binary.LittleEndian, uint32(len(b)))
		in.Write(b)
	}
	cmd := exec.Command(bin)
	cmd.Stdin = &in
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("exécution de l'oracle C : %v", err)
	}
	return out
}

// compare localise la première divergence plutôt que de rendre un verdict nu.
func compare(t *testing.T, name string, got, want []byte, bufs [][]byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	if len(got) != len(want) {
		t.Fatalf("%s : longueurs %d (Go) contre %d (C)", name, len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s : divergence au décalage %d — Go %#02x, C %#02x (corpus de %d tampons)",
				name, i, got[i], want[i], len(bufs))
		}
	}
}

// TestParityAllSizes couvre chaque reste de boucle vectorielle.
func TestParityAllSizes(t *testing.T) {
	bin := buildCOracle(t)
	var bufs [][]byte
	rnd := rand.New(rand.NewSource(20260829))
	for n := 0; n <= 128; n++ {
		b := make([]byte, n)
		rnd.Read(b)
		bufs = append(bufs, b)
	}
	compare(t, "tailles 0..128", goSide(bufs), runCOracle(t, bin, bufs), bufs)
}

// TestParityRandom tire un corpus déterministe de grande taille.
func TestParityRandom(t *testing.T) {
	bin := buildCOracle(t)
	rnd := rand.New(rand.NewSource(0xC2571A55))
	var bufs [][]byte
	for i := 0; i < 4096; i++ {
		b := make([]byte, rnd.Intn(512))
		rnd.Read(b)
		bufs = append(bufs, b)
	}
	compare(t, "aléatoire déterministe", goSide(bufs), runCOracle(t, bin, bufs), bufs)
}

// TestParityAdversarial nomme les cas de bord qui font effectivement diverger
// une implémentation approximative.
func TestParityAdversarial(t *testing.T) {
	bin := buildCOracle(t)

	all := func(n int, v byte) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = v
		}
		return b
	}
	rangeBytes := func(lo, hi int) []byte {
		b := make([]byte, 0, hi-lo+1)
		for v := lo; v <= hi; v++ {
			b = append(b, byte(v))
		}
		return b
	}

	bufs := [][]byte{
		{},                       // tampon vide
		rangeBytes(0, 255),       // toutes les valeurs d'octet
		rangeBytes(0x7A, 0x7C),   // frontière 'z' / '{'
		rangeBytes(0x39, 0x3A),   // frontière '9' / ':'
		rangeBytes(0x7F, 0x80),   // frontière ASCII / non-ASCII
		rangeBytes(0xF5, 0xFF),   // débuts UTF-8 interdits
		all(64, 0x80),            // continuations pures
		all(64, 0x41),            // ASCII pur
		{0xF0, 0x9F, 0x98},       // séquence UTF-8 tronquée
		{'a', 0x00, 'b'},         // NUL en milieu de tampon
		{0xFF, 0x00, 0x00, 0x01}, // U+00FF puis U+0100 en UTF-16 petit-boutiste
		{0xFF, 0x00, 0xFF, 0x00}, // deux fois U+00FF : reste latin-1
	}
	compare(t, "adversarial", goSide(bufs), runCOracle(t, bin, bufs), bufs)
}
