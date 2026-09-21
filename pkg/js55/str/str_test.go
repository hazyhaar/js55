package str

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"sync"
	"testing"
	"time"
)

// ─── Corpus adversarial ─────────────────────────────────────────────────────

// corpus nomme les cas de bord qui font effectivement diverger une
// implémentation approximative de la représentation des chaînes.
func corpus() [][]uint16 {
	c := [][]uint16{
		{},                       // vide
		{0x41},                   // ASCII
		{0x61, 0x62, 0x63},       // ASCII multiple
		{0x00},                   // NUL
		{0x00, 0x41, 0x00},       // NUL en milieu
		{0xFF},                   // U+00FF, dernière valeur Latin-1
		{0x100},                  // U+0100, première hors Latin-1
		{0xFF, 0x100},            // franchit la frontière
		{0x7F, 0x80},             // frontière ASCII
		{0xD83D, 0xDE00},         // 😀, paire de demi-codets
		{0xD83D},                 // demi-codet HAUT isolé — licite en JavaScript
		{0xDE00},                 // demi-codet BAS isolé — licite aussi
		{0xDE00, 0xD83D},         // paire inversée : deux isolés, pas un couple
		{0x41, 0xD83D, 0x42},     // isolé entouré d'ASCII
		{0xFEFF},                 // marque d'ordre d'octets en corps de chaîne
		{0x2028},                 // séparateur de ligne
		{0x2029},                 // séparateur de paragraphe
		{0xFFFD},                 // caractère de remplacement, en propre
		{0x41, 0x300},            // A suivi d'un diacritique combinant
		{0x7A, 0x7B},             // frontière 'z' / '{' — celle de la casse ASCII
		{0x39, 0x3A},             // frontière '9' / ':'
		{0x5A, 0x5B, 0x60, 0x61}, // 'Z' '[' '`' 'a' — les bords du bloc de casse
	}
	// Une chaîne longue purement Latin-1 et une longue avec une seule unité
	// large : les deux formes doivent se comporter identiquement.
	long1 := make([]uint16, 300)
	for i := range long1 {
		long1[i] = uint16(i % 256)
	}
	long2 := make([]uint16, 300)
	copy(long2, long1)
	long2[150] = 0x4E2D
	return append(c, long1, long2)
}

func fromUnits(u []uint16) *String { return FromUTF16(u) }

// ─── T2.2 : invariant métamorphique de représentation ───────────────────────

// observation rassemble tout ce qu'une chaîne expose. Deux chaînes de même
// contenu doivent produire la même observation, quelle que soit leur FORME.
type observation struct {
	Length     int      `json:"length"`
	CharCodes  []uint16 `json:"charCodes"`
	CodePoints []int32  `json:"codePoints"`
	SliceMid   []uint16 `json:"slice_1_n1"`
	IndexSelf  int      `json:"indexOf_self"`
	IndexT     int      `json:"indexOf_t"`
	Concat     []uint16 `json:"concat"`
	CmpT       int      `json:"cmp_t"`
	EqT        bool     `json:"eq_t"`
	IsASCII    bool     `json:"isAscii"`
	UpperASCII []uint16 `json:"upperAscii"`
	Hash       uint32   `json:"-"` // l'oracle Node n'a pas d'équivalent
}

func observe(s, t *String) observation {
	n := s.Len()
	o := observation{
		Length:     n,
		CharCodes:  []uint16{},
		CodePoints: []int32{},
		IndexSelf:  s.IndexOf(s, 0),
		IndexT:     s.IndexOf(t, 0),
		Concat:     s.Concat(t).UTF16(),
		CmpT:       s.Compare(t),
		EqT:        s.Equal(t),
		IsASCII:    s.IsASCII(),
		UpperASCII: s.ToUpperASCII().UTF16(),
		Hash:       s.Hash(),
	}
	for k := 0; k < n; k++ {
		o.CharCodes = append(o.CharCodes, s.At(k))
		cp, _ := s.CodePointAt(k)
		o.CodePoints = append(o.CodePoints, int32(cp))
	}
	if n >= 2 {
		o.SliceMid = s.Slice(1, n-1).UTF16()
	} else {
		o.SliceMid = []uint16{}
	}
	if o.Concat == nil {
		o.Concat = []uint16{}
	}
	if o.UpperASCII == nil {
		o.UpperASCII = []uint16{}
	}
	return o
}

// TestT2_2_MetamorphicRepresentation est l'instrument central du jalon.
//
// Une variable d'environnement force toute chaîne en UTF-16. La totalité des
// observations doit être identique dans les deux modes. Un écart n'est pas une
// tolérance : c'est un défaut de forme, et cet unique instrument attrape toute
// la classe.
func TestT2_2_MetamorphicRepresentation(t *testing.T) {
	cs := corpus()

	saved := ForceUTF16()
	defer SetForceUTF16(saved)

	collect := func(force bool) []observation {
		SetForceUTF16(force)
		out := make([]observation, len(cs))
		for i, u := range cs {
			s := fromUnits(u)
			next := fromUnits(cs[(i+1)%len(cs)])
			out[i] = observe(s, next)
		}
		return out
	}

	SetForceUTF16(false)
	narrow := collect(false)
	wide := collect(true)

	// Le mode force doit effectivement changer la forme, sans quoi le test ne
	// mesure rien. Témoin sur un cas purement ASCII.
	SetForceUTF16(false)
	if k := fromUnits([]uint16{0x41}).Kind(); k != Latin1 {
		t.Fatalf("témoin : forme %v hors mode force, Latin1 attendu", k)
	}
	SetForceUTF16(true)
	if k := fromUnits([]uint16{0x41}).Kind(); k != UTF16 {
		t.Fatalf("témoin : forme %v en mode force, UTF16 attendu — le mode ne fait rien", k)
	}
	SetForceUTF16(false)

	for i := range narrow {
		if !reflect.DeepEqual(narrow[i], wide[i]) {
			t.Errorf("cas %d %v : observation différente selon la forme\n  étroite : %+v\n  large   : %+v",
				i, cs[i], narrow[i], wide[i])
		}
	}
}

// ─── T2.1 : parité contre Node ──────────────────────────────────────────────

// TestT2_1_ParityVsNode compare les opérations à l'oracle Node, sur le corpus
// adversarial. Le JavaScript est LU DEPUIS LE DISQUE (interdit I2) : il n'est
// pas dupliqué dans ce fichier.
func TestT2_1_ParityVsNode(t *testing.T) {
	const script = "testdata/string_ops.js"
	if _, err := os.Stat(script); err != nil {
		t.Skipf("oracle absent (%s)", script)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node absent : la parité ne peut pas être mesurée")
	}

	cs := corpus()

	in, err := json.Marshal(map[string]any{"cases": cs})
	if err != nil {
		t.Fatalf("encodage du corpus : %v", err)
	}
	cmd := exec.Command("node", script)
	cmd.Stdin = bytes.NewReader(in)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("exécution de l'oracle Node : %v\n%s", err, errBuf.String())
	}

	var got struct {
		Results []observation `json:"results"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("décodage de la sortie Node : %v", err)
	}
	if len(got.Results) != len(cs) {
		t.Fatalf("Node rend %d résultats pour %d cas", len(got.Results), len(cs))
	}

	saved := ForceUTF16()
	defer SetForceUTF16(saved)

	// La parité est exigée dans LES DEUX formes : c'est la conjonction de T2.1
	// et T2.2 qui fait la preuve.
	for _, force := range []bool{false, true} {
		SetForceUTF16(force)
		mode := "forme minimale"
		if force {
			mode = "forme large forcée"
		}
		for i, u := range cs {
			s := fromUnits(u)
			next := fromUnits(cs[(i+1)%len(cs)])
			mine := observe(s, next)
			theirs := got.Results[i]
			theirs.Hash = mine.Hash // Node n'a pas d'empreinte à comparer

			if !reflect.DeepEqual(mine, theirs) {
				t.Errorf("%s, cas %d %v : divergence contre Node\n  js55 : %+v\n  node : %+v",
					mode, i, u, mine, theirs)
			}
		}
	}
}

// ─── T2.3 : banc quadratique, arbitre du sort des cordes ────────────────────

// TestT2_3_QuadraticConcat mesure le coût de la construction d'une chaîne par
// concaténations successives, la forme plate ne portant pas de corde.
//
// Ce test ne juge PAS : il publie un chiffre et n'échoue que si le coût dépasse
// une borne franche. Tant qu'il passe, les cordes sont inutiles ; le jour où il
// échoue, leur besoin est PROUVÉ et non supposé (plan §4).
func TestT2_3_QuadraticConcat(t *testing.T) {
	if testing.Short() {
		t.Skip("banc long")
	}
	const target = 1 << 20 // un mégaoctet d'unités de code
	const chunk = 16
	const budget = 20 * time.Second

	piece := FromGo("0123456789abcdef")
	if piece.Len() != chunk {
		t.Fatalf("morceau de %d unités, %d attendu", piece.Len(), chunk)
	}

	start := time.Now()
	acc := Empty
	for acc.Len() < target {
		acc = acc.Concat(piece)
		if time.Since(start) > budget {
			t.Fatalf("BANC QUADRATIQUE FRANCHI : %d unités seulement en %v (cible %d).\n"+
				"La forme Rope doit être implémentée — le besoin est désormais mesuré, "+
				"non supposé (plan §J6).", acc.Len(), budget, target)
		}
	}
	elapsed := time.Since(start)

	// Témoin linéaire : la MÊME chaîne construite en un seul tampon. Sans lui,
	// le chiffre ci-dessus n'est pas décidable — il ne dit pas si le coût vient
	// de la quadratique ou de la machine.
	linStart := time.Now()
	buf := make([]byte, 0, target)
	for len(buf) < target {
		buf = append(buf, piece.latin1...)
	}
	linear := FromLatin1(buf)
	linElapsed := time.Since(linStart)

	if !linear.Equal(acc) {
		t.Fatal("le témoin linéaire et la construction par concaténation diffèrent")
	}

	ratio := float64(elapsed) / float64(linElapsed)
	t.Logf("construction de %d unités par concaténations de %d", acc.Len(), chunk)
	t.Logf("  par concaténations successives : %v", elapsed)
	t.Logf("  en un seul tampon (témoin)     : %v", linElapsed)
	t.Logf("  rapport mesuré                 : x%.0f", ratio)
	// Le rapport est la grandeur décidable, pas le temps absolu : un budget en
	// secondes ne dit pas si le coût vient de la quadratique ou de la machine.
	// Avant l'implantation de la corde, ce rapport valait 14 069.
	const maxRatio = 50
	if ratio > maxRatio {
		t.Fatalf("RAPPORT QUADRATIQUE FRANCHI : x%.0f contre le témoin linéaire "+
			"(plafond x%d). La concaténation est redevenue quadratique — la forme "+
			"corde ne joue plus son rôle.", ratio, maxRatio)
	}
	t.Logf("verdict : rapport x%.0f, sous le plafond de x%d — la forme corde tient "+
		"la concaténation itérative au linéaire.", ratio, maxRatio)
	_ = budget

	if acc.Kind() != Latin1 {
		t.Errorf("forme %v, Latin1 attendu pour un contenu ASCII", acc.Kind())
	}
}

// ─── Internement ────────────────────────────────────────────────────────────

func TestInterning(t *testing.T) {
	saved := ForceUTF16()
	defer SetForceUTF16(saved)

	tab := NewTable()

	SetForceUTF16(false)
	narrow := tab.Intern(fromUnits([]uint16{0x41, 0x42}))
	SetForceUTF16(true)
	wide := tab.Intern(fromUnits([]uint16{0x41, 0x42}))
	SetForceUTF16(false)

	// Le point décisif : deux chaînes de même contenu mais de FORME différente
	// doivent s'interner sur le même pointeur. Sans cela, la comparaison de clés
	// par identité du modèle à formes serait fausse.
	if narrow != wide {
		t.Errorf("deux formes du même contenu s'internent sur des pointeurs distincts : "+
			"%v et %v — la comparaison de clés par identité serait fausse",
			narrow.Kind(), wide.Kind())
	}
	if tab.Len() != 1 {
		t.Errorf("table de %d entrées, 1 attendue", tab.Len())
	}

	// Deux contenus distincts ne doivent pas collisionner sous la même clé. Le
	// cas piège : une chaîne Latin-1 de deux octets contre une chaîne large
	// d'une unité formée des mêmes octets.
	a := tab.Intern(fromUnits([]uint16{0x42, 0x41}))
	b := tab.Intern(fromUnits([]uint16{0x4142}))
	if a == b {
		t.Error("« BA » en Latin-1 et U+4142 en UTF-16 s'internent ensemble : clé non injective")
	}

	// Un demi-codet isolé ne doit pas se confondre avec le caractère de
	// remplacement, ce qu'une clé passant par GoString provoquerait.
	lone := tab.Intern(fromUnits([]uint16{0xD83D}))
	repl := tab.Intern(fromUnits([]uint16{0xFFFD}))
	if lone == repl {
		t.Error("demi-codet isolé et U+FFFD s'internent ensemble : la clé passe par une conversion UTF-8")
	}
}

func TestHashIsFormIndependent(t *testing.T) {
	saved := ForceUTF16()
	defer SetForceUTF16(saved)

	for _, u := range corpus() {
		SetForceUTF16(false)
		h1 := fromUnits(u).Hash()
		SetForceUTF16(true)
		h2 := fromUnits(u).Hash()
		if h1 != h2 {
			t.Errorf("%v : empreinte %#x en forme minimale, %#x en forme large", u, h1, h2)
		}
	}
	SetForceUTF16(false)
}

// ─── Formes réservées ───────────────────────────────────────────────────────

// TestReservedKindsPanic vérifie que la forme encore réservée échoue bruyamment
// plutôt que d'être ignorée en silence. Un tag réservé qui traverse un
// commutateur sans branche est la manière dont un défaut de forme s'installe.
// Rope n'y figure plus : elle est implémentée depuis que le banc T2.3 a mesuré
// le rapport de 14 069 contre le témoin linéaire.
func TestReservedKindsPanic(t *testing.T) {
	for _, k := range []Kind{Sliced} {
		s := &String{kind: k}
		for name, op := range map[string]func(){
			// Len n'y figure pas : la longueur est un champ valide pour toute
			// forme, corde comprise. C'est ce qui rend « (a+b).length » gratuit.
			"UTF16":    func() { s.UTF16() },
			"GoString": func() { s.GoString() },
			"IsASCII":  func() { s.IsASCII() },
			"Hash":     func() { s.Hash() },
		} {
			func() {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("%s sur la forme %v n'a pas paniqué : le tag réservé "+
							"traverse le commutateur en silence", name, k)
					}
				}()
				op()
			}()
		}
	}
}

func TestKindEnumIsDense(t *testing.T) {
	for k := Kind(0); int(k) < NumKinds; k++ {
		if k.String() == "Kind(?)" {
			t.Errorf("Kind %d sans nom : l'énumération a un trou", k)
		}
	}
}

// ─── T2.5 : fuzz ────────────────────────────────────────────────────────────

func FuzzString(f *testing.F) {
	for _, u := range corpus() {
		b := make([]byte, 0, len(u)*2)
		for _, c := range u {
			b = append(b, byte(c), byte(c>>8))
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		u := make([]uint16, len(data)/2)
		for i := range u {
			u[i] = uint16(data[i*2]) | uint16(data[i*2+1])<<8
		}

		saved := ForceUTF16()
		defer SetForceUTF16(saved)

		// Invariant : aucune opération ne panique, et l'observation ne dépend
		// pas de la forme.
		SetForceUTF16(false)
		a := observe(fromUnits(u), fromUnits(u))
		SetForceUTF16(true)
		b := observe(fromUnits(u), fromUnits(u))
		SetForceUTF16(false)

		if !reflect.DeepEqual(a, b) {
			t.Fatalf("observation dépendante de la forme sur %d unités\n  étroite : %+v\n  large   : %+v",
				len(u), a, b)
		}
		// Aller-retour : la matérialisation en unités de code est fidèle.
		s := fromUnits(u)
		back := s.UTF16()
		if len(back) != len(u) {
			t.Fatalf("aller-retour : %d unités rendues pour %d", len(back), len(u))
		}
		for i := range u {
			if back[i] != u[i] {
				t.Fatalf("aller-retour : unité %d vaut %#04x, %#04x attendu", i, back[i], u[i])
			}
		}
	})
}

// ─── Chemin rapide de casse ─────────────────────────────────────────────────

// TestToUpperASCIIRefusesNonASCII garde la sémantique déclarée : le noyau émis
// ne connaît que l'ASCII, et la fonction rend la chaîne inchangée plutôt qu'un
// résultat faux sur les mappings Latin-1.
func TestToUpperASCIIRefusesNonASCII(t *testing.T) {
	// « é » en U+00E9 : sa majuscule est U+00C9, que le noyau ASCII ne sait pas
	// produire. La fonction doit donc rendre la chaîne inchangée.
	s := fromUnits([]uint16{0xE9})
	if got := s.ToUpperASCII(); !got.Equal(s) {
		t.Errorf("é mis en majuscule à tort : %v", got.UTF16())
	}
	// Sur de l'ASCII pur, le noyau émis agit.
	a := FromGo("abcZ{`")
	want := FromGo("ABCZ{`")
	if got := a.ToUpperASCII(); !got.Equal(want) {
		t.Errorf("casse ASCII : %q, %q attendu", got.GoString(), want.GoString())
	}
}

func TestRandomizedRepresentationAgreement(t *testing.T) {
	saved := ForceUTF16()
	defer SetForceUTF16(saved)

	rnd := rand.New(rand.NewSource(20260829))
	for iter := 0; iter < 2000; iter++ {
		n := rnd.Intn(64)
		u := make([]uint16, n)
		for i := range u {
			switch rnd.Intn(4) {
			case 0:
				u[i] = uint16(rnd.Intn(0x80)) // ASCII
			case 1:
				u[i] = uint16(rnd.Intn(0x100)) // Latin-1
			case 2:
				u[i] = uint16(0xD800 + rnd.Intn(0x800)) // demi-codets
			default:
				u[i] = uint16(rnd.Intn(0x10000))
			}
		}
		SetForceUTF16(false)
		a := observe(fromUnits(u), fromUnits(u))
		SetForceUTF16(true)
		b := observe(fromUnits(u), fromUnits(u))
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("itération %d, %v : observation dépendante de la forme", iter, u)
		}
	}
	SetForceUTF16(false)
}

func ExampleString_Len() {
	// La longueur se compte en unités de code UTF-16, ce que le langage observe.
	fmt.Println(FromGo("😀").Len())
	// Output: 2
}

func TestCharAtAndCodePointAt_HorsBMP(t *testing.T) {
	saved := ForceUTF16()
	defer SetForceUTF16(saved)

	for _, force := range []bool{false, true} {
		SetForceUTF16(force)
		// "a😀b" -> 'a' (0x61), 0xD83D, 0xDE00, 'b' (0x62)
		s := FromGo("a😀b")
		if s.Len() != 4 {
			t.Fatalf("Longueur attendue 4, obtenu %d", s.Len())
		}

		// CharAt
		if c0 := s.CharAt(0); c0.GoString() != "a" || c0.Len() != 1 {
			t.Errorf("CharAt(0) = %q (len %d), attendu 'a' (len 1)", c0.GoString(), c0.Len())
		}
		if c1 := s.CharAt(1); c1.At(0) != 0xD83D || c1.Len() != 1 {
			t.Errorf("CharAt(1) unit = %#x, attendu 0xD83D", c1.At(0))
		}
		if c2 := s.CharAt(2); c2.At(0) != 0xDE00 || c2.Len() != 1 {
			t.Errorf("CharAt(2) unit = %#x, attendu 0xDE00", c2.At(0))
		}
		if c3 := s.CharAt(3); c3.GoString() != "b" || c3.Len() != 1 {
			t.Errorf("CharAt(3) = %q, attendu 'b'", c3.GoString())
		}
		if cNeg := s.CharAt(-1); cNeg != Empty {
			t.Errorf("CharAt(-1) = %v, attendu Empty", cNeg)
		}
		if cOut := s.CharAt(4); cOut != Empty {
			t.Errorf("CharAt(4) = %v, attendu Empty", cOut)
		}

		// CodePointAt hors BMP
		cp0, w0 := s.CodePointAt(0)
		if cp0 != 'a' || w0 != 1 {
			t.Errorf("CodePointAt(0) = %d (w=%d), attendu %d (w=1)", cp0, w0, rune('a'))
		}
		cp1, w1 := s.CodePointAt(1)
		if cp1 != 0x1F600 || w1 != 2 { // 😀 = 0x1F600 (128512)
			t.Errorf("CodePointAt(1) = %#x (%d) w=%d, attendu 0x1F600 (w=2)", cp1, cp1, w1)
		}
		cp2, w2 := s.CodePointAt(2)
		if cp2 != 0xDE00 || w2 != 1 {
			t.Errorf("CodePointAt(2) = %#x w=%d, attendu 0xDE00 (w=1)", cp2, w2)
		}
		cp3, w3 := s.CodePointAt(3)
		if cp3 != 'b' || w3 != 1 {
			t.Errorf("CodePointAt(3) = %d (w=%d), attendu %d (w=1)", cp3, w3, rune('b'))
		}
		cpOut, wOut := s.CodePointAt(4)
		if cpOut != -1 || wOut != 0 {
			t.Errorf("CodePointAt(4) = %d (w=%d), attendu -1 (w=0)", cpOut, wOut)
		}
		cpNeg, wNeg := s.CodePointAt(-1)
		if cpNeg != -1 || wNeg != 0 {
			t.Errorf("CodePointAt(-1) = %d (w=%d), attendu -1 (w=0)", cpNeg, wNeg)
		}

		// Demi-codet isolé
		loneHigh := FromUTF16([]uint16{0xD83D, 0x0041})
		cpLone, wLone := loneHigh.CodePointAt(0)
		if cpLone != 0xD83D || wLone != 1 {
			t.Errorf("CodePointAt(0) sur demi-codet haut isolé = %#x (w=%d), attendu 0xD83D (w=1)", cpLone, wLone)
		}
	}
}

func TestFromCodePoints(t *testing.T) {
	// Unicode points dont hors BMP (0x1F600)
	cps := []int{0x61, 0x1F600, 0x62}
	s, ok := FromCodePoints(cps)
	if !ok {
		t.Fatalf("FromCodePoints a échoué pour des codepoints valides")
	}
	if s.Len() != 4 {
		t.Fatalf("Longueur attendue 4, obtenu %d", s.Len())
	}
	cp1, w1 := s.CodePointAt(1)
	if cp1 != 0x1F600 || w1 != 2 {
		t.Fatalf("Codepoint hors BMP attendu 0x1F600, obtenu %#x", cp1)
	}

	// Invalides
	if _, ok := FromCodePoints([]int{-1}); ok {
		t.Errorf("FromCodePoints(-1) aurait dû échouer")
	}
	if _, ok := FromCodePoints([]int{0x110000}); ok {
		t.Errorf("FromCodePoints(0x110000) aurait dû échouer")
	}

	// FromCodePoint unitaire
	s1, ok1 := FromCodePoint(0x1F4A9)
	if !ok1 || s1.Len() != 2 {
		t.Fatalf("FromCodePoint(0x1F4A9) échoué ou longueur %d != 2", s1.Len())
	}
	cpA, _ := s1.CodePointAt(0)
	if cpA != 0x1F4A9 {
		t.Fatalf("Codepoint attendu 0x1F4A9, obtenu %#x", cpA)
	}
}

func TestStringSearchAndTransformMethods(t *testing.T) {
	saved := ForceUTF16()
	defer SetForceUTF16(saved)

	for _, force := range []bool{false, true} {
		SetForceUTF16(force)
		s := FromGo("hello world hello")
		h := FromGo("hello")
		w := FromGo("world")

		// LastIndexOf
		if idx := s.LastIndexOf(h, 20); idx != 12 {
			t.Errorf("LastIndexOf(hello, 20) = %d, attendu 12", idx)
		}
		if idx := s.LastIndexOf(h, 11); idx != 0 {
			t.Errorf("LastIndexOf(hello, 11) = %d, attendu 0", idx)
		}
		if idx := s.LastIndexOf(w, 4); idx != -1 {
			t.Errorf("LastIndexOf(world, 4) = %d, attendu -1", idx)
		}

		// StartsWith / EndsWith / Includes
		if !s.StartsWith(h, 0) {
			t.Errorf("StartsWith(hello, 0) faux")
		}
		if s.StartsWith(w, 0) {
			t.Errorf("StartsWith(world, 0) vrai à tort")
		}
		if !s.EndsWith(h, s.Len()) {
			t.Errorf("EndsWith(hello, Len) faux")
		}
		if !s.Includes(w, 0) {
			t.Errorf("Includes(world, 0) faux")
		}

		// Repeat
		rep := FromGo("ab").Repeat(3)
		if rep.GoString() != "ababab" {
			t.Errorf("Repeat(3) = %q, attendu ababab", rep.GoString())
		}

		// ToLowerASCII
		upper := FromGo("HeLLo WoRLd")
		lower := upper.ToLowerASCII()
		if lower.GoString() != "hello world" {
			t.Errorf("ToLowerASCII = %q, attendu 'hello world'", lower.GoString())
		}

		// Trim / TrimStart / TrimEnd
		ws := FromGo(" \t\r\n hello \u2000\n ")
		if tr := ws.Trim(); tr.GoString() != "hello" {
			t.Errorf("Trim = %q, attendu 'hello'", tr.GoString())
		}
		if trs := ws.TrimStart(); trs.GoString() != "hello \u2000\n " {
			t.Errorf("TrimStart err: %q", trs.GoString())
		}
		if tre := ws.TrimEnd(); tre.GoString() != " \t\r\n hello" {
			t.Errorf("TrimEnd err: %q", tre.GoString())
		}
	}
}

// TestConcurrentHash vérifie que Hash() est strictement thread-safe et
// lock-free lors d'accès concurrents massifs sur une même instance de chaîne.
func TestConcurrentHash(t *testing.T) {
	s := FromGo("concurrent_test_string_for_hash_verification")
	const n = 128
	var wg sync.WaitGroup
	wg.Add(n)
	hashes := make([]uint32, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			hashes[idx] = s.Hash()
		}(i)
	}
	wg.Wait()
	first := hashes[0]
	for idx, h := range hashes {
		if h != first {
			t.Fatalf("goroutine %d: empreintes divergentes: %d != %d", idx, h, first)
		}
	}
}
