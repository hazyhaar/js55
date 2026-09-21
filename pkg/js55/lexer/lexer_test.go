// SPDX-License-Identifier: BUSL-1.1
package lexer

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// lex découpe entièrement src, en admettant une expression régulière après tout
// ponctuateur ou mot réservé — approximation suffisante pour les tests unitaires,
// le parser tranchant réellement.
func lex(t *testing.T, src string) []Token {
	t.Helper()
	l := New(src)
	var out []Token
	allowRe := true
	for i := 0; ; i++ {
		if i > 1<<20 {
			t.Fatalf("lexage non terminant sur %q", src)
		}
		tok := l.Next(allowRe)
		out = append(out, tok)
		if tok.Kind == EOF {
			return out
		}
		switch tok.Kind {
		case Ident, Number, BigInt, String, Regexp, RParen, RBracket,
			NoSubTemplate, TemplateTail, PrivateIdent:
			allowRe = false
		default:
			allowRe = true
		}
	}
}

func kinds(toks []Token) []Kind {
	out := make([]Kind, 0, len(toks))
	for _, t := range toks {
		out = append(out, t.Kind)
	}
	return out
}

func eqKinds(got []Kind, want ...Kind) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestKindEnumIsDense(t *testing.T) {
	// Contrainte C2 : l'énumération doit rester dense pour qu'un commutateur
	// sur Kind reçoive une table de saut. Un trou dans les noms signale une
	// valeur sautée.
	for k := Kind(0); int(k) < NumKinds; k++ {
		if kindNames[k] == "" {
			t.Errorf("Kind %d sans nom : l'énumération a un trou, la densité C2 est perdue", k)
		}
	}
	if NumKinds > 255 {
		t.Errorf("NumKinds = %d dépasse la capacité d'un uint8", NumKinds)
	}
}

func TestPunctuatorsMaximalMunch(t *testing.T) {
	cases := []struct {
		src  string
		want []Kind
	}{
		{">>>=", []Kind{UShrAssign, EOF}},
		{">>>", []Kind{UShr, EOF}},
		{">>=", []Kind{ShrAssign, EOF}},
		{">>", []Kind{Shr, EOF}},
		{">", []Kind{Gt, EOF}},
		{"===", []Kind{EqEqEq, EOF}},
		{"==", []Kind{EqEq, EOF}},
		{"=", []Kind{Assign, EOF}},
		{"...", []Kind{Ellipsis, EOF}},
		{"..", []Kind{Dot, Dot, EOF}},
		{"?.", []Kind{QuestionDot, EOF}},
		{"??=", []Kind{QuestionQuestionAssign, EOF}},
		{"&&=", []Kind{AndAndAssign, EOF}},
		{"=>", []Kind{Arrow, EOF}},
		{"**=", []Kind{StarStarAssign, EOF}},
	}
	for _, c := range cases {
		if got := kinds(lex(t, c.src)); !eqKinds(got, c.want...) {
			t.Errorf("%q : %v, attendu %v", c.src, got, c.want)
		}
	}
}

func TestOptionalChainingVersusTernary(t *testing.T) {
	// a?.5:b est un ternaire, pas un chaînage optionnel : la règle est que "?."
	// suivi d'un chiffre ne se munche pas.
	got := kinds(lex(t, "a?.5:b"))
	want := []Kind{Ident, Question, Number, Colon, Ident, EOF}
	if !eqKinds(got, want...) {
		t.Errorf("a?.5:b : %v, attendu %v", got, want)
	}
	got = kinds(lex(t, "a?.b"))
	want = []Kind{Ident, QuestionDot, Ident, EOF}
	if !eqKinds(got, want...) {
		t.Errorf("a?.b : %v, attendu %v", got, want)
	}
}

func TestNumbers(t *testing.T) {
	cases := []struct {
		src  string
		kind Kind
	}{
		{"0", Number}, {"42", Number}, {"3.14", Number}, {".5", Number},
		{"1e10", Number}, {"1E+10", Number}, {"1e-10", Number},
		{"0x1F", Number}, {"0XdeadBEEF", Number},
		{"0o17", Number}, {"0b1010", Number},
		{"0777", Number},  // octal hérité : licite lexicalement
		{"1_000", Number}, // séparateurs
		{"0n", BigInt}, {"123n", BigInt}, {"0x1Fn", BigInt},
	}
	for _, c := range cases {
		toks := lex(t, c.src)
		if toks[0].Kind != c.kind {
			t.Errorf("%q : %v (%s), attendu %v", c.src, toks[0].Kind, toks[0].Message, c.kind)
		}
		if toks[0].Value != c.src {
			t.Errorf("%q : valeur %q, attendue entière", c.src, toks[0].Value)
		}
	}

	// Exposant vide : "1e" n'a pas d'exposant, le "e" devient un identifiant
	// accolé, donc une erreur précoce signalée par le lexer.
	if k := lex(t, "1e")[0].Kind; k != Illegal {
		t.Errorf(`"1e" : %v, attendu Illegal`, k)
	}
	if k := lex(t, "3in")[0].Kind; k != Illegal {
		t.Errorf(`"3in" : %v, attendu Illegal (identifiant accolé)`, k)
	}
}

func TestStringsAndEscapes(t *testing.T) {
	ok := []string{
		`"abc"`, `'abc'`, `"a\"b"`, `'a\'b'`, `"\n\t\\"`,
		`"A"`, `"\u{1F600}"`, `"\x41"`,
		"\"a\\\nb\"",    // continuation de ligne
		"\"a\\\r\nb\"",  // continuation CRLF
		"\"\u2028\"",    // licite dans une chaîne depuis ES2019
	}
	for _, s := range ok {
		toks := lex(t, s)
		if toks[0].Kind != String {
			t.Errorf("%q : %v (%s), attendu String", s, toks[0].Kind, toks[0].Message)
		}
	}

	bad := []string{`"abc`, `'abc`, "\"a\nb\"", `"\`}
	for _, s := range bad {
		if k := lex(t, s)[0].Kind; k != Illegal {
			t.Errorf("%q : %v, attendu Illegal", s, k)
		}
	}
}

func TestTemplates(t *testing.T) {
	if k := lex(t, "`abc`")[0].Kind; k != NoSubTemplate {
		t.Errorf("`abc` : %v, attendu NoSubTemplate", k)
	}
	l := New("`a${b}c`")
	head := l.Next(true)
	if head.Kind != TemplateHead {
		t.Fatalf("tête : %v, attendu TemplateHead", head.Kind)
	}
	if id := l.Next(true); id.Kind != Ident || id.Value != "b" {
		t.Fatalf("substitution : %v %q", id.Kind, id.Value)
	}
	tail := l.NextTemplateContinuation()
	if tail.Kind != TemplateTail {
		t.Fatalf("queue : %v, attendu TemplateTail", tail.Kind)
	}
	if k := lex(t, "`abc")[0].Kind; k != Illegal {
		t.Error("gabarit non refermé : Illegal attendu")
	}
}

func TestIdentifiersAndKeywords(t *testing.T) {
	if k := lex(t, "foo")[0].Kind; k != Ident {
		t.Errorf("foo : %v, attendu Ident", k)
	}
	if k := lex(t, "function")[0].Kind; k != Keyword {
		t.Errorf("function : %v, attendu Keyword", k)
	}
	if k := lex(t, "$_a1")[0].Kind; k != Ident {
		t.Errorf("$_a1 : %v, attendu Ident", k)
	}
	if k := lex(t, "café")[0].Kind; k != Ident {
		t.Errorf("café : %v, attendu Ident", k)
	}
	if k := lex(t, "#priv")[0].Kind; k != PrivateIdent {
		t.Errorf("#priv : %v, attendu PrivateIdent", k)
	}
	if k := lex(t, "#")[0].Kind; k != Illegal {
		t.Errorf("# seul : %v, attendu Illegal", k)
	}
	// Un identifiant écrit avec un échappement Unicode n'est pas classé mot
	// réservé par le lexer : \u0069f s'écrit « if » mais reste un Ident, et
	// c'est au parser d'appliquer la règle d'erreur précoce.
	if k := lex(t, `\u0069f`)[0].Kind; k != Ident {
		t.Errorf(`\u0069f : %v, attendu Ident`, k)
	}
}

func TestRegexpVersusDivision(t *testing.T) {
	l := New("/ab[/]c/gi")
	if tok := l.Next(true); tok.Kind != Regexp || tok.Value != "/ab[/]c/gi" {
		t.Errorf("expression régulière : %v %q", tok.Kind, tok.Value)
	}
	l = New("/ab/")
	if tok := l.Next(false); tok.Kind != Slash {
		t.Errorf("division attendue quand allowRegexp est faux, obtenu %v", tok.Kind)
	}
	if k := lex(t, "/unterminated")[0].Kind; k != Illegal {
		t.Error("expression régulière non refermée : Illegal attendu")
	}
}

func TestCommentsAndNewlineBefore(t *testing.T) {
	toks := lex(t, "a // ligne\nb")
	if len(toks) != 3 || toks[1].Value != "b" {
		t.Fatalf("découpage inattendu : %v", kinds(toks))
	}
	if !toks[1].NewlineBefore {
		t.Error("b devrait porter NewlineBefore après un commentaire de ligne")
	}

	toks = lex(t, "a /* x */ b")
	if toks[1].NewlineBefore {
		t.Error("un commentaire de bloc sans saut ne doit pas poser NewlineBefore")
	}
	toks = lex(t, "a /* x\ny */ b")
	if !toks[1].NewlineBefore {
		t.Error("un commentaire de bloc contenant un saut doit poser NewlineBefore")
	}
	if k := lex(t, "a /* non refermé")[1].Kind; k != Illegal {
		t.Error("commentaire de bloc non refermé : Illegal attendu")
	}
}

func TestPositions(t *testing.T) {
	// Plan T1.4 : la position rapportée doit pointer le bon caractère.
	src := "var x = 1;\nlet\ty = 2;"
	toks := lex(t, src)
	check := func(i, line, col int, val string) {
		t.Helper()
		if toks[i].Value != val {
			t.Fatalf("lexème %d = %q, attendu %q", i, toks[i].Value, val)
		}
		if toks[i].Pos.Line != line || toks[i].Pos.Col != col {
			t.Errorf("%q : ligne %d colonne %d, attendu %d:%d",
				val, toks[i].Pos.Line, toks[i].Pos.Col, line, col)
		}
	}
	check(0, 1, 1, "var")
	check(1, 1, 5, "x")
	check(5, 2, 1, "let")
	check(6, 2, 5, "y") // la tabulation compte pour une unité de code

	// Colonne en unités UTF-16 : un caractère hors plan de base en vaut deux.
	toks = lex(t, "'😀' x")
	if toks[1].Pos.Col != 6 {
		t.Errorf("après '😀' : colonne %d, attendu 6 (paire de demi-codets)", toks[1].Pos.Col)
	}
}

// TestNeverPanicsOnAdversarialInput garde l'invariant dur T1.3 sur un corpus
// nommé, en complément du fuzz.
func TestNeverPanicsOnAdversarialInput(t *testing.T) {
	corpus := []string{
		"", "\x00", "\xff", "\xff\xfe", "/*", "/*/", "//", "`", "${", "}",
		`"`, `'`, `\`, `\u`, `\u{`, `\u{ffffffffffff}`, "#", "0x", "0b", "0o",
		"1e", "1e+", ".", "..", "?.", "??", ">>>>>>>", "\u2028", "\u2029",
		strings.Repeat("(", 4096), strings.Repeat("`${", 512),
		strings.Repeat("\\", 1024), "\ufeff", "\x00\x00\x00",
	}
	for _, src := range corpus {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panique sur %q : %v", src, r)
				}
			}()
			l := New(src)
			for i := 0; i < 1<<16; i++ {
				if l.Next(true).Kind == EOF {
					return
				}
			}
			t.Errorf("lexage non terminant sur %q", src)
		}()
	}
}

// TestAlwaysTerminates vérifie que le lexer progresse toujours : le nombre de
// lexèmes est borné par la taille de la source, ce qui exclut toute boucle.
func TestAlwaysTerminates(t *testing.T) {
	srcs := []string{"a b c", "\x00\x01\x02", "@@@@", "€€€", "\\\\\\"}
	for _, src := range srcs {
		deadline := time.Now().Add(2 * time.Second)
		l := New(src)
		n := 0
		for {
			if time.Now().After(deadline) {
				t.Fatalf("lexage de %q dépasse deux secondes", src)
			}
			if l.Next(true).Kind == EOF {
				break
			}
			n++
			if n > utf8.RuneCountInString(src)+1 {
				t.Fatalf("lexage de %q produit plus de lexèmes que de runes : progression rompue", src)
			}
		}
	}
}

func FuzzLexer(f *testing.F) {
	seeds := []string{
		"var x = 1;", "function f(a,b){return a+b}", "`a${b}c`",
		"/re[/]g/gi", "0x1Fn", "class A { #p = 1 }", "a?.b??c",
		"'\\u{1F600}'", "/* c */ // l\n", "x => x ** 2",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		// Invariant T1.3 : ni panique, ni boucle. La borne sur le nombre de
		// lexèmes est le témoin de progression.
		l := New(src)
		max := len(src) + 2
		for i := 0; ; i++ {
			if i > max {
				t.Fatalf("plus de %d lexèmes pour %d octets : progression rompue", i, len(src))
			}
			if l.Next(i%2 == 0).Kind == EOF {
				return
			}
		}
	})
}
