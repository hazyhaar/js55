// SPDX-License-Identifier: BUSL-1.1
package parser

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// Instrument T1.1 du plan : passage de test262 en mode « parse seul ».
//
// Deux mesures distinctes, et deux seulement :
//   - un fichier non-negative doit s'analyser sans erreur ;
//   - un fichier negative de phase « parse » doit produire une SyntaxError.
//
// Rien d'autre n'est mesuré ici : l'exécution relève du pilote de conformité.
// Aucun pourcentage d'appréciation ne figure dans ce fichier, seulement des
// comptes (interdit I7).

const test262Root = "/devhoros/pkg/js55/testdata/test262"

var (
	fmRe     = regexp.MustCompile(`(?s)/\*---(.*?)---\*/`)
	flagsRe  = regexp.MustCompile(`(?m)^\s*flags:\s*\[([^\]]*)\]`)
	flagsYml = regexp.MustCompile(`(?m)^flags:\s*\n((?:\s+-\s*\w+\s*\n)+)`)
	negRe    = regexp.MustCompile(`(?m)^negative:`)
	phaseRe  = regexp.MustCompile(`(?m)^\s*phase:\s*(\w+)`)
	typeRe   = regexp.MustCompile(`(?m)^\s*type:\s*(\w+)`)
)

type meta struct {
	flags     map[string]bool
	negative  bool
	phase     string
	errorType string
}

func parseMeta(src []byte) (meta, bool) {
	m := fmRe.FindSubmatch(src)
	if m == nil {
		return meta{}, false
	}
	fm := m[1]
	out := meta{flags: map[string]bool{}}

	// Les deux formes YAML sont acceptées : liste entre crochets et séquence
	// sur plusieurs lignes. Ne reconnaître que la première fausse le compte —
	// 181 fichiers de la suite emploient la seconde.
	if f := flagsRe.FindSubmatch(fm); f != nil {
		for _, x := range strings.Split(string(f[1]), ",") {
			if x = strings.TrimSpace(x); x != "" {
				out.flags[x] = true
			}
		}
	} else if f := flagsYml.FindSubmatch(fm); f != nil {
		for _, line := range strings.Split(string(f[1]), "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
			if line != "" {
				out.flags[line] = true
			}
		}
	}

	if negRe.Match(fm) {
		out.negative = true
		if ph := phaseRe.FindSubmatch(fm); ph != nil {
			out.phase = string(ph[1])
		}
		if ty := typeRe.FindSubmatch(fm); ty != nil {
			out.errorType = string(ty[1])
		}
	}
	return out, true
}

type counts struct {
	total, ok, ko int
	failures      []string
}

func (c *counts) add(other counts) {
	c.total += other.total
	c.ok += other.ok
	c.ko += other.ko
	c.failures = append(c.failures, other.failures...)
}

// TestParseOnlyTest262 mesure le passage en analyse seule. Il ne fait pas
// échouer la suite sur un compte : le ratchet du pilote de conformité est le
// gate. Ici, la mesure est publiée pour être consignée au plan.
func TestParseOnlyTest262(t *testing.T) {
	if _, err := os.Stat(test262Root); err != nil {
		t.Skipf("arbre test262 absent (%s) ; le reconstituer par testdata/fetch_test262.sh", test262Root)
	}
	if testing.Short() {
		t.Skip("mesure longue")
	}

	var files []string
	root := filepath.Join(test262Root, "test", "language")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".js") && !strings.Contains(path, "_FIXTURE") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parcours de %s : %v", root, err)
	}
	sort.Strings(files)

	var (
		mu       sync.Mutex
		positive counts
		negative counts
		skipped  int
	)

	work := make(chan string, len(files))
	for _, f := range files {
		work <- f
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var lp, ln counts
			var lskip int
			for path := range work {
				src, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				md, ok := parseMeta(src)
				if !ok {
					lskip++
					continue
				}
				// Les tests d'exécution négative ne relèvent pas de l'analyse.
				if md.negative && md.phase != "parse" {
					lskip++
					continue
				}

				opt := Options{
					Module: md.flags["module"],
					Strict: md.flags["onlyStrict"],
				}
				body := string(src)
				if md.flags["onlyStrict"] && !md.flags["raw"] {
					body = "\"use strict\";\n" + body
				}

				_, perr := Parse(body, opt)
				rel := strings.TrimPrefix(path, test262Root+"/")

				if md.negative {
					ln.total++
					if perr != nil {
						ln.ok++
					} else {
						ln.ko++
						if len(ln.failures) < 20 {
							ln.failures = append(ln.failures, "accepté à tort : "+rel)
						}
					}
				} else {
					lp.total++
					if perr == nil {
						lp.ok++
					} else {
						lp.ko++
						if len(lp.failures) < 20 {
							lp.failures = append(lp.failures, rel+" — "+perr.Error())
						}
					}
				}
			}
			mu.Lock()
			positive.add(lp)
			negative.add(ln)
			skipped += lskip
			mu.Unlock()
		}()
	}
	wg.Wait()

	t.Logf("test262 test/language, analyse seule")
	t.Logf("  fichiers parcourus            : %d", len(files))
	t.Logf("  hors périmètre de l'analyse   : %d", skipped)
	t.Logf("  positifs  : %d analysés sur %d (%d refusés à tort)",
		positive.ok, positive.total, positive.ko)
	t.Logf("  négatifs  : %d refusés sur %d (%d acceptés à tort)",
		negative.ok, negative.total, negative.ko)

	for _, f := range positive.failures {
		t.Logf("    refus à tort  : %s", f)
	}
	for _, f := range negative.failures {
		t.Logf("    %s", f)
	}
}
