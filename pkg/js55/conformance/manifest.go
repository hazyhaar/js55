// SPDX-License-Identifier: BUSL-1.1

package conformance

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// TestCase décrit un cas d'exécution unitaire issu d'un fichier de test et d'un mode spécifique.
type TestCase struct {
	RelPath string  // Chemin relatif depuis testdata/test262 (ex: test/language/...)
	Mode    Mode    // Mode d'exécution (sloppy ou strict)
	Verdict Verdict // Verdict normatif (pass, fail, skip, error)
	Error   string  // Message d'erreur optionnel
}

// Key retourne l'identifiant unique stable du cas de test.
func (tc TestCase) Key() string {
	return tc.RelPath + "\t" + string(tc.Mode)
}

// Line formate le cas selon le standard du manifeste doré.
func (tc TestCase) Line() string {
	return fmt.Sprintf("%s\t%s\t%s", tc.RelPath, tc.Mode, tc.Verdict)
}

// WriteManifest écrit la liste triée des cas de test dans le flux de sortie.
func WriteManifest(w io.Writer, cases []TestCase) error {
	bw := bufio.NewWriter(w)
	for _, c := range cases {
		if _, err := bw.WriteString(c.Line() + "\n"); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ParseManifest lit et parse un manifeste au format TSV (<chemin>\t<mode>\t<verdict>).
func ParseManifest(r io.Reader) ([]TestCase, error) {
	var cases []TestCase
	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			return nil, fmt.Errorf("ligne %d invalide dans le manifeste (%q) : 3 colonnes requises", lineNum, line)
		}
		cases = append(cases, TestCase{
			RelPath: parts[0],
			Mode:    Mode(parts[1]),
			Verdict: Verdict(parts[2]),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cases, nil
}
