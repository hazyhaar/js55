// SPDX-License-Identifier: BUSL-1.1

package conformance

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Stats regroupe les compteurs de volumétrie et de résultats.
type Stats struct {
	TotalFiles int
	TotalCases int
	Pass       int
	Fail       int
	Skip       int
	Error      int
}

// Add intègre le résultat d'un cas de test dans les statistiques.
func (s *Stats) Add(v Verdict) {
	s.TotalCases++
	switch v {
	case VerdictPass:
		s.Pass++
	case VerdictFail:
		s.Fail++
	case VerdictSkip:
		s.Skip++
	case VerdictError:
		s.Error++
	}
}

// String produit une synthèse textuelle lisible des compteurs.
func (s Stats) String() string {
	return fmt.Sprintf("fichiers=%d, cas=%d (pass=%d, fail=%d, skip=%d, error=%d)",
		s.TotalFiles, s.TotalCases, s.Pass, s.Fail, s.Skip, s.Error)
}

// RunnerConfig définit la configuration d'exécution de la suite de conformité.
type RunnerConfig struct {
	Test262Root         string          // Répertoire racine contenant test/ et harness/
	RelPrefix           string          // Si non vide, ne parcourt que ce préfixe (ex. test/language/comments/)
	Engine              Engine          // Implémentation du moteur JS à tester
	Workers             int             // Nombre de goroutines de calcul (défaut runtime.NumCPU())
	UnsupportedFeatures map[string]bool // Liste de fonctionnalités ignorées (marquées skip)
}

// Report regroupe les résultats complets de l'exécution de la suite test262.
type Report struct {
	Cases        []TestCase
	Stats        Stats
	ProxyStats   Stats
	ReflectStats Stats
	Intl402Stats Stats
	Duration     time.Duration
}

// Summary génère le récapitulatif textuel complet avec les 4 compteurs distincts demandés.
func (r *Report) Summary() string {
	var sb strings.Builder
	sb.WriteString("============================================================\n")
	sb.WriteString("BILAN D'EXÉCUTION TEST262 (PILOTE DE CONFORMITÉ JS55)\n")
	sb.WriteString("============================================================\n")
	sb.WriteString(fmt.Sprintf("Durée d'exécution totale : %v\n", r.Duration))
	sb.WriteString(fmt.Sprintf("Total global           : %s\n", r.Stats))
	sb.WriteString(fmt.Sprintf("Sous-ensemble Proxy    : %s\n", r.ProxyStats))
	sb.WriteString(fmt.Sprintf("Sous-ensemble Reflect  : %s\n", r.ReflectStats))
	sb.WriteString(fmt.Sprintf("Sous-ensemble intl402  : %s\n", r.Intl402Stats))
	sb.WriteString("============================================================\n")
	return sb.String()
}

// RunSuite exécute la suite complète test262 selon la configuration fournie.
func RunSuite(cfg RunnerConfig) (*Report, error) {
	startTime := time.Now()

	if cfg.Engine == nil {
		cfg.Engine = NewJS55Engine()
	}
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}

	testDir := filepath.Join(cfg.Test262Root, "test")
	if cfg.RelPrefix != "" {
		testDir = filepath.Join(cfg.Test262Root, cfg.RelPrefix)
	}
	harnessDir := filepath.Join(cfg.Test262Root, "harness")

	// 1. Préchargement du harnais en cache mémoire
	hl := NewHarnessLoader(harnessDir)
	if err := hl.Preload(); err != nil {
		return nil, fmt.Errorf("échec du préchargement du harnais : %w", err)
	}

	// 2. Découverte de tous les fichiers de test .js hors _FIXTURE
	var testFiles []string
	err := filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".js") && !IsFixture(name) {
			testFiles = append(testFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("échec du parcours de %s : %w", testDir, err)
	}

	// 3. Traitement parallèle avec un pool de workers
	jobs := make(chan string, 2048)
	results := make(chan []TestCase, 2048)

	var wg sync.WaitGroup
	for w := 0; w < cfg.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				cases := executeTestFile(path, cfg.Test262Root, hl, cfg.Engine, cfg.UnsupportedFeatures)
				results <- cases
			}
		}()
	}

	// Producteur de chemins
	go func() {
		for _, f := range testFiles {
			jobs <- f
		}
		close(jobs)
	}()

	// Collecteur de résultats
	var collectorWg sync.WaitGroup
	var allCases []TestCase
	collectorWg.Add(1)
	go func() {
		defer collectorWg.Done()
		for res := range results {
			allCases = append(allCases, res...)
		}
	}()

	wg.Wait()
	close(results)
	collectorWg.Wait()

	// 4. Tri déterministe absolu (chemin croissant, puis mode croissant)
	sort.Slice(allCases, func(i, j int) bool {
		if allCases[i].RelPath != allCases[j].RelPath {
			return allCases[i].RelPath < allCases[j].RelPath
		}
		return allCases[i].Mode < allCases[j].Mode
	})

	// 5. Calcul des statistiques et sous-ensembles
	rep := &Report{
		Cases:    allCases,
		Duration: time.Since(startTime),
	}

	// Comptabilisation des fichiers par sous-ensemble
	fileSeen := make(map[string]bool)
	proxyFiles := make(map[string]bool)
	reflectFiles := make(map[string]bool)
	intl402Files := make(map[string]bool)

	for _, c := range allCases {
		rep.Stats.Add(c.Verdict)
		fileSeen[c.RelPath] = true

		if strings.HasPrefix(c.RelPath, "test/built-ins/Proxy/") {
			rep.ProxyStats.Add(c.Verdict)
			proxyFiles[c.RelPath] = true
		} else if strings.HasPrefix(c.RelPath, "test/built-ins/Reflect/") {
			rep.ReflectStats.Add(c.Verdict)
			reflectFiles[c.RelPath] = true
		} else if strings.HasPrefix(c.RelPath, "test/intl402/") {
			rep.Intl402Stats.Add(c.Verdict)
			intl402Files[c.RelPath] = true
		}
	}

	rep.Stats.TotalFiles = len(fileSeen)
	rep.ProxyStats.TotalFiles = len(proxyFiles)
	rep.ReflectStats.TotalFiles = len(reflectFiles)
	rep.Intl402Stats.TotalFiles = len(intl402Files)

	return rep, nil
}

func executeTestFile(fullPath, rootDir string, hl *HarnessLoader, eng Engine, unsupported map[string]bool) []TestCase {
	relPath, err := filepath.Rel(rootDir, fullPath)
	if err != nil {
		relPath = fullPath
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return []TestCase{{
			RelPath: relPath,
			Mode:    ModeSloppy,
			Verdict: VerdictError,
			Error:   fmt.Sprintf("lecture impossible : %v", err),
		}}
	}

	meta, body, err := ParseFrontmatter(string(data))
	if err != nil {
		return []TestCase{{
			RelPath: relPath,
			Mode:    ModeSloppy,
			Verdict: VerdictError,
			Error:   fmt.Sprintf("parsing frontmatter impossible : %v", err),
		}}
	}

	// Vérification des fonctionnalités non supportées
	if unsupported != nil && meta != nil {
		for _, feat := range meta.Features {
			if unsupported[feat] {
				var cases []TestCase
				for _, m := range meta.Modes() {
					cases = append(cases, TestCase{
						RelPath: relPath,
						Mode:    m,
						Verdict: VerdictSkip,
					})
				}
				return cases
			}
		}
	}

	modes := meta.Modes()
	cases := make([]TestCase, 0, len(modes))
	for _, mode := range modes {
		src, err := AssembleSource(hl, meta, body, mode)
		if err != nil {
			cases = append(cases, TestCase{
				RelPath: relPath,
				Mode:    mode,
				Verdict: VerdictError,
				Error:   fmt.Sprintf("assemblage harnais impossible : %v", err),
			})
			continue
		}

		evalErr := eng.Eval(src, mode == ModeStrict)
		verdict := EvaluateVerdict(meta, evalErr)
		var errMsg string
		if evalErr != nil {
			errMsg = evalErr.Error()
		}

		cases = append(cases, TestCase{
			RelPath: relPath,
			Mode:    mode,
			Verdict: verdict,
			Error:   errMsg,
		})
	}

	return cases
}
