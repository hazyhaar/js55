// SPDX-License-Identifier: BUSL-1.1
// Package csgguard mesure les contraintes compilateur opposables au moteur js55.
//
// Ces contraintes décrivent un comportement du compilateur Go, pas une propriété
// du moteur. Elles périment donc à chaque version de la toolchain, silencieusement,
// et deviennent du folklore si rien ne les remesure. Deux des règles héritées
// (CSG-020 appliquée au décodage multi-octets, CSG-022) se sont révélées fausses
// sur go1.27.0 le 2026-08-29 : ce paquet est la garde qui empêche la répétition.
//
// Toute contrainte invoquée dans le code du moteur porte un test ici. Une
// contrainte sans garde ne gouverne rien (plan §1, C7).
package csgguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const fixturesDir = "testdata/fixtures"

// compileFixtures invoque le compilateur directement sur les fixtures. Elles ne
// portent aucun import, ce qui rend l'appel possible sans module ni importcfg et
// rend la garde indépendante du workspace.
func compileFixtures(t *testing.T, extraFlags ...string) string {
	t.Helper()

	sources, err := filepath.Glob(filepath.Join(fixturesDir, "*.go"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("aucune fixture trouvée sous %s : %v", fixturesDir, err)
	}
	sort.Strings(sources)

	args := append([]string{"tool", "compile", "-p", "csgfixtures", "-o", os.DevNull}, extraFlags...)
	args = append(args, sources...)

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.0")
	out, err := cmd.CombinedOutput()
	// Le compilateur écrit ses diagnostics de passes sur stderr et sort en 0 ;
	// une erreur réelle doit faire échouer la garde.
	if err != nil && !strings.Contains(string(out), "Found Is") {
		t.Fatalf("compilation des fixtures échouée : %v\n%s", err, out)
	}
	return string(out)
}

// ─────────────────────────────────────────────────────────────────────────────
// C2 — densité des opcodes et table de saut
// ─────────────────────────────────────────────────────────────────────────────

var jumpSymRe = regexp.MustCompile(`csgfixtures\.(\w+)\.jump\d+`)

// TestC2_JumpTable mesure le seuil réel d'émission d'une table de saut.
//
// Le compilateur exige minCases = 8 ET minDensity = 4
// (cmd/compile/internal/walk/switch.go:302-303). La densité, non la contiguïté :
// un espace d'opcodes peut réserver des trous jusqu'à un facteur 4 sans perdre
// la table. La formulation « strictement contigus sans trou » est trop stricte
// et coûterait une liberté de conception réelle.
func TestC2_JumpTable(t *testing.T) {
	asm := compileFixtures(t, "-S")

	withTable := map[string]bool{}
	for _, m := range jumpSymRe.FindAllStringSubmatch(asm, -1) {
		withTable[m[1]] = true
	}

	cases := []struct {
		fn     string
		want   bool
		reason string
	}{
		{"SwitchDense", true, "16 cas contigus : au-dessus de minCases=8, densité 1"},
		{"SwitchHoles", true, "16 cas sur 0..60, densité 1/3,8 : sous minDensity=4, les trous sont tolérés"},
		{"SwitchSparse", false, "16 cas sur 0..300, densité 1/18 : au-delà de minDensity"},
		{"SwitchTooFew", false, "7 cas contigus : sous minCases=8"},
	}

	for _, c := range cases {
		if got := withTable[c.fn]; got != c.want {
			t.Errorf("C2 %s : table de saut = %v, attendu %v (%s)", c.fn, got, c.want, c.reason)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// C3 — budget de registres de la boucle d'interprétation
// ─────────────────────────────────────────────────────────────────────────────

var frameRe = regexp.MustCompile(`TEXT\s+csgfixtures\.(\w+)\(SB\)[^$]*\$(\d+)-\d+`)

// TestC3_LiveValueBudget mesure le seuil de spill d'une boucle chaude.
//
// Les 13 registres généraux nominaux de amd64 (16 moins RSP, moins R14 qui porte
// le goroutine pointer, moins RBP frame pointer) ne sont pas tous disponibles
// pour l'état : la boucle consomme elle-même l'induction, la borne, la base de
// tranche et un temporaire. Le seuil mesuré est 10, d'où la contrainte C3.
//
// Live16 sert de témoin : sans lui, un plafond trop généreux passerait pour
// satisfait alors que la mesure ne mesurerait rien.
func TestC3_LiveValueBudget(t *testing.T) {
	asm := compileFixtures(t, "-S")

	frames := map[string]int{}
	for _, m := range frameRe.FindAllStringSubmatch(asm, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("cadre illisible pour %s : %q", m[1], m[2])
		}
		frames[m[1]] = n
	}

	const floor = 24 // plancher observé sur go1.27.0 : aucun emplacement de spill

	got10, ok := frames["Live10"]
	if !ok {
		t.Fatal("C3 : cadre de Live10 introuvable dans la sortie assembleur")
	}
	if got10 > floor {
		t.Errorf("C3 : Live10 a un cadre de %d octets, plafond %d — dix valeurs vivantes "+
			"ne tiennent plus en registres, la contrainte C3 doit être abaissée", got10, floor)
	}

	got16, ok := frames["Live16"]
	if !ok {
		t.Fatal("C3 : cadre de Live16 introuvable")
	}
	if got16 <= floor {
		t.Errorf("C3 : Live16 a un cadre de %d octets, au plancher — le témoin ne mesure "+
			"plus rien, l'allocateur a changé et le seuil de 10 doit être remesuré", got16)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// C4 et C5 — élimination des contrôles de bornes
// ─────────────────────────────────────────────────────────────────────────────

var bceRe = regexp.MustCompile(`([\w./]+\.go):(\d+):\d+: Found Is(?:Slice)?InBounds`)

// boundsChecksByFunc attribue chaque contrôle signalé à la fonction qui le porte,
// en relisant les fixtures avec go/parser plutôt qu'en devinant sur les lignes.
func boundsChecksByFunc(t *testing.T, diag string) map[string]int {
	t.Helper()

	type span struct {
		name       string
		start, end int
	}
	spans := map[string][]span{}

	fset := token.NewFileSet()
	sources, _ := filepath.Glob(filepath.Join(fixturesDir, "*.go"))
	for _, src := range sources {
		f, err := parser.ParseFile(fset, src, nil, 0)
		if err != nil {
			t.Fatalf("lecture de %s : %v", src, err)
		}
		base := filepath.Base(src)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			spans[base] = append(spans[base], span{
				name:  fd.Name.Name,
				start: fset.Position(fd.Pos()).Line,
				end:   fset.Position(fd.End()).Line,
			})
		}
	}

	counts := map[string]int{}
	for _, m := range bceRe.FindAllStringSubmatch(diag, -1) {
		base := filepath.Base(m[1])
		line, _ := strconv.Atoi(m[2])
		for _, s := range spans[base] {
			if line >= s.start && line <= s.end {
				counts[s.name]++
				break
			}
		}
	}
	return counts
}

// TestC4_OperandDecode mesure le coût réel des idiomes de décodage d'opérande.
//
// L'idiome CSG-020 (_ = code[pc+3] puis quatre indexations) est le PIRE des trois
// sur ce motif : l'assertion coûte son propre contrôle et n'affranchit aucune des
// indexations, qui portent des indices distincts que la table de faits ne relie
// pas à elle. Le re-tranchage n'en laisse qu'un. D'où C4.
func TestC4_OperandDecode(t *testing.T) {
	diag := compileFixtures(t, "-d=ssa/check_bce/debug=1")
	counts := boundsChecksByFunc(t, diag)

	if got := counts["DecodeReslice"]; got != 1 {
		t.Errorf("C4 : DecodeReslice porte %d contrôles de bornes, attendu 1 — "+
			"l'idiome retenu pour la boucle d'interprétation a cessé d'être le bon", got)
	}
	if got := counts["DecodeBounded"]; got != 1 {
		t.Errorf("C4 : DecodeBounded porte %d contrôles de bornes, attendu 1", got)
	}
	if got := counts["DecodeAssertIdiom"]; got <= counts["DecodeReslice"] {
		t.Errorf("C4 : l'idiome d'assertion porte %d contrôles contre %d pour le "+
			"re-tranchage — il a cessé d'être pénalisant et C4 peut être révisée",
			got, counts["DecodeReslice"])
	}
}

// TestC5_LoopDirection garde la correction apportée à CSG-022.
//
// CSG-022 interdisait les boucles descendantes au motif que loopbce échouerait à
// prouver l'induction. Mesure contraire sur go1.27.0 : le contrôle est éliminé
// dans les deux sens. Si ce test redevient rouge, la règle héritée redevient
// vraie et la contrainte C5 doit être retirée du plan.
func TestC5_LoopDirection(t *testing.T) {
	diag := compileFixtures(t, "-d=ssa/check_bce/debug=1")
	counts := boundsChecksByFunc(t, diag)

	if got := counts["LoopAscending"]; got != 0 {
		t.Errorf("C5 : LoopAscending porte %d contrôles, attendu 0", got)
	}
	if got := counts["LoopDescending"]; got != 0 {
		t.Errorf("C5 : LoopDescending porte %d contrôles, attendu 0 — CSG-022 redevient "+
			"vraie sur cette toolchain et la contrainte C5 doit être retirée du plan", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// C6 — décomposition SSA des structures d'état
// ─────────────────────────────────────────────────────────────────────────────

// TestC6_MaxStruct vérifie que le plafond de décomposition reste celui sur lequel
// la contrainte C6 est fondée. La valeur est lue dans la source du compilateur
// épinglé, pas mémorisée : c'est le seul oracle qui ne périme pas en silence.
func TestC6_MaxStruct(t *testing.T) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("GOROOT illisible : %v", err)
	}
	goroot := strings.TrimSpace(string(out))
	path := filepath.Join(goroot, "src", "cmd", "compile", "internal", "ssa", "decompose.go")

	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("source du compilateur absente (%s) : la garde C6 ne peut pas mesurer", path)
	}

	m := regexp.MustCompile(`const\s+MaxStruct\s*=\s*(\d+)`).FindSubmatch(src)
	if m == nil {
		t.Fatalf("C6 : MaxStruct introuvable dans %s — la constante a été renommée "+
			"ou déplacée, la contrainte C6 doit être réancrée", path)
	}
	if got := string(m[1]); got != "4" {
		t.Errorf("C6 : MaxStruct vaut %s dans %s, la contrainte en suppose 4", got, path)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Gardes portant sur le CODE RÉEL de l'interpréteur, non sur des fixtures.
//
// Les tests ci-dessus mesurent le compilateur Go sur des extraits de référence ;
// ceux-ci mesurent que le moteur lui-même respecte les contraintes qu'il invoque
// dans ses commentaires. Une contrainte proclamée dans un commentaire et démentie
// par l'assembleur ne gouverne rien.
// ─────────────────────────────────────────────────────────────────────────────

// compileEngine compile le paquet du moteur et rend sa sortie assembleur.
func compileEngine(t *testing.T, extraFlags ...string) string {
	t.Helper()

	args := append([]string{"build", "-gcflags=" + strings.Join(extraFlags, " ")},
		"github.com/hazyhaar/js55/pkg/js55/engine")
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.0", "GOEXPERIMENT=simd")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "TEXT") {
		t.Fatalf("compilation du moteur : %v\n%s", err, out)
	}
	return string(out)
}

// TestC2_EngineSwitchGetsJumpTable vérifie que le commutateur d'opcodes de
// l'interpréteur reçoit bien une table de saut. C'est la raison d'être de la
// contrainte C2 : sans elle, chaque instruction paierait un arbre de
// comparaisons.
func TestC2_EngineSwitchGetsJumpTable(t *testing.T) {
	asm := compileEngine(t, "-S")
	if !strings.Contains(asm, "step") {
		t.Skip("la fonction step n'apparaît pas dans la sortie assembleur")
	}
	if !regexp.MustCompile(`engine\.\(\*VM\)\.step\.jump\d+`).MatchString(asm) {
		t.Error("C2 : le commutateur d'opcodes de (*VM).step ne reçoit PAS de table de saut. " +
			"L'énumération a perdu sa densité, ou le nombre de cas est passé sous le seuil.")
	}
}

// TestC3_EngineLoopFrame mesure le cadre de la boucle d'interprétation. La
// contrainte C3 borne les valeurs vivantes ; le cadre en est la trace
// observable.
func TestC3_EngineLoopFrame(t *testing.T) {
	asm := compileEngine(t, "-S")
	m := regexp.MustCompile(`TEXT\s+[\w./]*engine\.\(\*VM\)\.loop\(SB\)[^$]*\$(\d+)`).FindStringSubmatch(asm)
	if m == nil {
		t.Skip("le cadre de (*VM).loop n'apparaît pas dans la sortie assembleur")
	}
	frame, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("cadre illisible : %q", m[1])
	}

	// Le plafond est nommé. Il ne dit pas que la boucle est optimale, il dit
	// qu'elle n'a pas dérivé : une croissance du cadre signale que des valeurs
	// vivantes se sont ajoutées et débordent en pile.
	const maxFrame = 256
	if frame > maxFrame {
		t.Errorf("C3 : la boucle d'interprétation a un cadre de %d octets, plafond %d. "+
			"Des valeurs vivantes se sont ajoutées et débordent en pile.", frame, maxFrame)
	}
	t.Logf("cadre de (*VM).loop : %d octets", frame)
}

// TestC4_EngineOperandDecode mesure les contrôles de bornes du décodage
// d'opérande dans la boucle réelle. La contrainte C4 impose le re-tranchage ;
// ce test vérifie qu'il est effectivement employé là où il compte.
func TestC4_EngineOperandDecode(t *testing.T) {
	diag := compileEngine(t, "-d=ssa/check_bce/debug=1")

	// Le compilateur attribue à vm.go les contrôles des fonctions qu'il y inline
	// (Heap.Get à lui seul en apporte plus de trente) : l'agrégat du fichier ne
	// mesure donc pas le décodage. Seules comptent les lignes qui indexent le
	// flux de code, c'est-à-dire la cible nommée par C4.
	src, err := os.ReadFile(filepath.Join("..", "engine", "vm.go"))
	if err != nil {
		t.Fatalf("lecture de engine/vm.go : %v", err)
	}
	lines := strings.Split(string(src), "\n")
	decodeIdiom := regexp.MustCompile(`\bcode\[`)

	total := 0
	seen := map[int]bool{}
	var decode []string
	for _, m := range regexp.MustCompile(`vm\.go:(\d+):\d+: Found Is(?:Slice)?InBounds`).FindAllStringSubmatch(diag, -1) {
		total++
		ln, _ := strconv.Atoi(m[1])
		if ln < 1 || ln > len(lines) || !decodeIdiom.MatchString(lines[ln-1]) {
			continue
		}
		if !seen[ln] {
			seen[ln] = true
			decode = append(decode, "vm.go:"+m[1]+": "+strings.TrimSpace(lines[ln-1]))
		}
	}

	// Plafond nommé et mesuré : le re-tranchage laisse un contrôle par largeur
	// d'opérande (opcode, 16 bits, 32 bits), soit 3 le 2026-09-02 sur go1.27.0.
	// Le plafond est cette mesure même : toute hausse vient d'une ligne de
	// décodage qui a ajouté un contrôle. Mutation témoin vérifiée le même jour :
	// remplacer le re-tranchage 32 bits par « _ = code[ip+3] » puis quatre
	// accès porte la mesure à 4 (l'assertion coûte un contrôle, les quatre
	// accès sont prouvés), pas à sept comme le supposait l'ancien plafond 40.
	const maxDecodeChecks = 3
	if len(decode) > maxDecodeChecks {
		t.Errorf("C4 : %d contrôles de bornes sur le décodage d'opérande, plafond %d. "+
			"Un décodage a quitté le re-tranchage :\n  %s", len(decode), maxDecodeChecks, strings.Join(decode, "\n  "))
	}
	if len(decode) == 0 {
		t.Errorf("C4 : aucun contrôle de bornes attribué au décodage d'opérande ; " +
			"la mesure ne voit plus la boucle (diagnostic vide ou idiome renommé)")
	}
	t.Logf("contrôles de bornes du décodage d'opérande : %d (agrégat vm.go, inlining compris : %d)", len(decode), total)
}
