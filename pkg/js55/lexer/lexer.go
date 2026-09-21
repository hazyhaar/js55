package lexer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lexer découpe une source ECMAScript. Il n'alloue pas : les valeurs de lexèmes
// sont des tranches de la source d'origine.
//
// La distinction entre division et littéral d'expression régulière n'est pas
// décidable lexicalement en ECMAScript. Le parser la tranche en passant
// allowRegexp à Next.
type Lexer struct {
	src  string
	off  int // décalage en octets du prochain caractère à lire
	line int
	col  int // en unités de code UTF-16, à partir de 1

	newlineBefore bool
}

// New crée un lexer positionné au début de src.
//
// Un commentaire hashbang « #! » n'est reconnu qu'en tout début de source, ce
// qui est exactement ce que la grammaire prescrit : ailleurs, « # » ouvre un
// nom privé.
func New(src string) *Lexer {
	l := &Lexer{src: src, off: 0, line: 1, col: 1}
	if strings.HasPrefix(src, "#!") {
		for {
			r, _ := l.peek()
			if r == eof || isLineTerminator(r) {
				break
			}
			l.advance()
		}
	}
	return l
}

// Pos rend la position courante.
func (l *Lexer) Pos() Position {
	return Position{Offset: l.off, Line: l.line, Col: l.col}
}

const eof = -1

// peek rend la rune courante sans avancer, et sa largeur en octets.
func (l *Lexer) peek() (rune, int) {
	if l.off >= len(l.src) {
		return eof, 0
	}
	c := l.src[l.off]
	if c < utf8.RuneSelf {
		return rune(c), 1
	}
	r, w := utf8.DecodeRuneInString(l.src[l.off:])
	return r, w
}

// peekAt rend l'octet à la distance n, ou 0 hors borne. Sert au munch maximal,
// qui ne raisonne que sur de l'ASCII.
func (l *Lexer) peekAt(n int) byte {
	if l.off+n >= len(l.src) {
		return 0
	}
	return l.src[l.off+n]
}

// advance consomme une rune et met à jour la position. Toute progression du
// lexer passe par ici, ce qui garantit que l'offset croît strictement.
func (l *Lexer) advance() rune {
	r, w := l.peek()
	if r == eof {
		return eof
	}
	l.off += w
	if isLineTerminator(r) {
		l.line++
		l.col = 1
	} else if r > 0xFFFF {
		l.col += 2 // une paire de demi-codets en UTF-16
	} else {
		l.col++
	}
	return r
}

func isLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}

func isWhitespace(r rune) bool {
	switch r {
	case '\t', '\v', '\f', ' ', 0x00A0, 0xFEFF:
		return true
	}
	return r > 0x7F && unicode.Is(unicode.Zs, r)
}

func isIdentStart(r rune) bool {
	if r < utf8.RuneSelf {
		return r == '$' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.In(r, unicode.L, unicode.Nl, unicode.Other_ID_Start)
}

func isIdentPart(r rune) bool {
	if r < utf8.RuneSelf {
		return r == '$' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')
	}
	if r == 0x200C || r == 0x200D { // ZWNJ, ZWJ
		return true
	}
	return unicode.In(r, unicode.L, unicode.Nl, unicode.Mn, unicode.Mc,
		unicode.Nd, unicode.Pc, unicode.Other_ID_Start, unicode.Other_ID_Continue)
}

func isDecimal(c byte) bool { return c >= '0' && c <= '9' }
func isHex(c byte) bool {
	return isDecimal(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// skipSpace consomme espaces, terminateurs de ligne et commentaires. Il
// mémorise le franchissement d'un terminateur de ligne pour l'insertion
// automatique de points-virgules. Il rend un lexème Illegal non nul si un
// commentaire de bloc n'est pas refermé.
func (l *Lexer) skipSpace() *Token {
	for {
		r, _ := l.peek()
		switch {
		case r == eof:
			return nil

		case isLineTerminator(r):
			l.newlineBefore = true
			l.advance()

		case isWhitespace(r):
			l.advance()

		case r == '/' && l.peekAt(1) == '/':
			l.advance()
			l.advance()
			for {
				c, _ := l.peek()
				if c == eof || isLineTerminator(c) {
					break
				}
				l.advance()
			}

		case r == '/' && l.peekAt(1) == '*':
			start := l.Pos()
			l.advance()
			l.advance()
			closed := false
			for {
				c, _ := l.peek()
				if c == eof {
					break
				}
				if isLineTerminator(c) {
					// Un commentaire de bloc contenant un terminateur de ligne
					// compte comme tel pour l'insertion de points-virgules.
					l.newlineBefore = true
				}
				if c == '*' && l.peekAt(1) == '/' {
					l.advance()
					l.advance()
					closed = true
					break
				}
				l.advance()
			}
			if !closed {
				return &Token{Kind: Illegal, Pos: start,
					Value: l.src[start.Offset:l.off], Message: "commentaire de bloc non refermé"}
			}

		default:
			return nil
		}
	}
}

// Next rend le lexème suivant. allowRegexp indique que la position courante
// admet un littéral d'expression régulière : le parser seul le sait.
//
// Next progresse toujours, ou rend EOF. Aucune entrée ne peut la faire boucler.
func (l *Lexer) Next(allowRegexp bool) Token {
	l.newlineBefore = false
	if bad := l.skipSpace(); bad != nil {
		bad.NewlineBefore = l.newlineBefore
		return *bad
	}

	nl := l.newlineBefore
	start := l.Pos()

	r, _ := l.peek()
	if r == eof {
		return Token{Kind: EOF, Pos: start, NewlineBefore: nl}
	}

	before := l.off

	var tok Token
	switch {
	case isIdentStart(r) || r == '\\':
		tok = l.scanIdent(start)
	case r == '#':
		tok = l.scanPrivateIdent(start)
	case (r < utf8.RuneSelf && isDecimal(byte(r))) || (r == '.' && isDecimal(l.peekAt(1))):
		tok = l.scanNumber(start)
	case r == '"' || r == '\'':
		tok = l.scanString(start, byte(r))
	case r == '`':
		tok = l.scanTemplate(start, true)
	case r == '/' && allowRegexp:
		tok = l.scanRegexp(start)
	default:
		tok = l.scanPunct(start)
	}

	// Invariant dur T1.3 : tout lexème autre que EOF consomme au moins un
	// octet. Si un scanner manque à la règle, on force la progression et on le
	// signale, plutôt que de boucler indéfiniment. Ce filet a déjà rattrapé une
	// troncature de rune en octet dans le dispatch des littéraux numériques.
	if l.off == before {
		l.advance()
		tok = Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
			Message: "lexème sans progression : défaut du scanner"}
	}

	tok.NewlineBefore = nl
	return tok
}

// NextTemplateContinuation reprend un gabarit après la substitution, à partir de
// l'accolade fermante. Le parser l'appelle lorsqu'il a fini l'expression.
func (l *Lexer) NextTemplateContinuation() Token {
	start := l.Pos()
	if r, _ := l.peek(); r != '}' {
		return Token{Kind: Illegal, Pos: start, Message: "'}' attendu pour reprendre le gabarit"}
	}
	return l.scanTemplate(start, false)
}

func (l *Lexer) scanIdent(start Position) Token {
	for {
		r, _ := l.peek()
		if r == '\\' {
			// Séquence d'échappement Unicode dans un identifiant : consommée
			// telle quelle, le parser la validera et la déséchappera.
			l.advance()
			if c, _ := l.peek(); c == 'u' {
				l.advance()
				if !l.scanUnicodeEscapeTail() {
					return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
						Message: "échappement Unicode mal formé dans un identifiant"}
				}
				continue
			}
			return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
				Message: "échappement invalide dans un identifiant"}
		}
		if r == eof || !isIdentPart(r) {
			break
		}
		l.advance()
	}
	v := l.src[start.Offset:l.off]
	k := Ident
	if !strings.Contains(v, "\\") && IsKeyword(v) {
		k = Keyword
	}
	return Token{Kind: k, Pos: start, Value: v}
}

// scanUnicodeEscapeTail consomme ce qui suit "\u" : soit exactement quatre
// chiffres hexadécimaux, soit "{" suivi d'au moins un chiffre et de "}".
// Rend faux si l'échappement est mal formé — un identifiant qui en contient un
// n'est pas un identifiant, et l'accepter produirait un arbre irréimprimable.
// Ne panique jamais sur une entrée tronquée.
func (l *Lexer) scanUnicodeEscapeTail() bool {
	if c, _ := l.peek(); c == '{' {
		l.advance()
		n := 0
		var cp uint32
		for {
			c, _ := l.peek()
			if c == eof || c > 0x7F || !isHex(byte(c)) {
				break
			}
			cp = cp*16 + uint32(hexVal(byte(c)))
			if cp > 0x10FFFF {
				cp = 0x110000 // saturation : la valeur exacte n'importe plus
			}
			n++
			l.advance()
		}
		if n == 0 || cp > 0x10FFFF {
			return false
		}
		if c, _ := l.peek(); c != '}' {
			return false
		}
		l.advance()
		return true
	}
	for i := 0; i < 4; i++ {
		c, _ := l.peek()
		if c == eof || c > 0x7F || !isHex(byte(c)) {
			return false
		}
		l.advance()
	}
	return true
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

func (l *Lexer) scanPrivateIdent(start Position) Token {
	l.advance() // '#'
	r, _ := l.peek()
	if r == eof || (!isIdentStart(r) && r != '\\') {
		return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
			Message: "nom privé sans identifiant"}
	}
	for {
		r, _ := l.peek()
		if r == '\\' {
			l.advance()
			if c, _ := l.peek(); c == 'u' {
				l.advance()
				if !l.scanUnicodeEscapeTail() {
					return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
						Message: "échappement Unicode mal formé dans un nom privé"}
				}
				continue
			}
			return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
				Message: "échappement invalide dans un nom privé"}
		}
		if r == eof || !isIdentPart(r) {
			break
		}
		l.advance()
	}
	return Token{Kind: PrivateIdent, Pos: start, Value: l.src[start.Offset:l.off]}
}

// scanNumber reconnaît les littéraux numériques d'ECMAScript 2020 : décimal,
// hexadécimal, octal, binaire, octal hérité, séparateurs et suffixe BigInt.
func (l *Lexer) scanNumber(start Position) Token {
	isBig := false

	digits := func(pred func(byte) bool) {
		for {
			c, _ := l.peek()
			if c == eof {
				return
			}
			b := byte(c)
			if b == '_' {
				l.advance()
				continue
			}
			if c > 0x7F || !pred(b) {
				return
			}
			l.advance()
		}
	}

	if r, _ := l.peek(); r == '0' {
		l.advance()
		switch c := l.peekAt(0); c {
		case 'x', 'X':
			l.advance()
			digits(isHex)
			goto suffix
		case 'o', 'O':
			l.advance()
			digits(func(b byte) bool { return b >= '0' && b <= '7' })
			goto suffix
		case 'b', 'B':
			l.advance()
			digits(func(b byte) bool { return b == '0' || b == '1' })
			goto suffix
		default:
			// Octal hérité ou décimal commençant par zéro : la validation en
			// mode strict est une erreur précoce du parser, pas du lexer.
			digits(isDecimal)
		}
	} else {
		digits(isDecimal)
	}

	if c, _ := l.peek(); c == '.' {
		l.advance()
		digits(isDecimal)
	}
	if c, _ := l.peek(); c == 'e' || c == 'E' {
		save := l.off
		saveLine, saveCol := l.line, l.col
		l.advance()
		if c, _ := l.peek(); c == '+' || c == '-' {
			l.advance()
		}
		if c, _ := l.peek(); c != eof && c < 0x80 && isDecimal(byte(c)) {
			digits(isDecimal)
		} else {
			// Exposant vide : ce n'est pas un exposant, on rend le terrain.
			l.off, l.line, l.col = save, saveLine, saveCol
		}
	}

suffix:
	if c, _ := l.peek(); c == 'n' {
		l.advance()
		isBig = true
	}

	// Un identifiant collé au littéral est une erreur précoce.
	if c, _ := l.peek(); c != eof && (isIdentStart(c) || (c < 0x80 && isDecimal(byte(c)))) {
		for {
			c, _ := l.peek()
			if c == eof || !isIdentPart(c) {
				break
			}
			l.advance()
		}
		return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
			Message: "identifiant accolé à un littéral numérique"}
	}

	k := Number
	if isBig {
		k = BigInt
	}
	return Token{Kind: k, Pos: start, Value: l.src[start.Offset:l.off]}
}

func (l *Lexer) scanString(start Position, quote byte) Token {
	l.advance() // guillemet ouvrant
	for {
		r, _ := l.peek()
		switch {
		case r == eof:
			return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
				Message: "chaîne non refermée"}
		case r == rune(quote):
			l.advance()
			return Token{Kind: String, Pos: start, Value: l.src[start.Offset:l.off]}
		case r == '\\':
			l.advance()
			c, _ := l.peek()
			if c == eof {
				return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
					Message: "chaîne non refermée après un échappement"}
			}
			// Une continuation de ligne \<CRLF> compte pour un seul saut.
			if c == '\r' && l.peekAt(1) == '\n' {
				l.advance()
			}
			l.advance()
		case r == '\n' || r == '\r':
			// U+2028 et U+2029 sont licites dans une chaîne depuis ES2019 ;
			// seuls LF et CR terminent prématurément.
			return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
				Message: "terminateur de ligne dans une chaîne"}
		default:
			l.advance()
		}
	}
}

// scanTemplate lit un gabarit. head vaut vrai à l'ouverture par un accent grave,
// faux lors d'une reprise après substitution, qui commence par une accolade.
func (l *Lexer) scanTemplate(start Position, head bool) Token {
	l.advance() // '`' ou '}'
	for {
		r, _ := l.peek()
		switch {
		case r == eof:
			return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
				Message: "gabarit non refermé"}
		case r == '`':
			l.advance()
			k := TemplateTail
			if head {
				k = NoSubTemplate
			}
			return Token{Kind: k, Pos: start, Value: l.src[start.Offset:l.off]}
		case r == '$' && l.peekAt(1) == '{':
			l.advance()
			l.advance()
			k := TemplateMiddle
			if head {
				k = TemplateHead
			}
			return Token{Kind: k, Pos: start, Value: l.src[start.Offset:l.off]}
		case r == '\\':
			l.advance()
			if c, _ := l.peek(); c != eof {
				l.advance()
			}
		default:
			l.advance()
		}
	}
}

// scanRegexp lit un littéral d'expression régulière. Le lexer n'en valide pas
// la grammaire interne : il en délimite le corps et les fanions, la compilation
// étant le travail du moteur d'expressions régulières.
func (l *Lexer) scanRegexp(start Position) Token {
	l.advance() // '/'
	inClass := false
	for {
		r, _ := l.peek()
		switch {
		case r == eof || isLineTerminator(r):
			return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
				Message: "littéral d'expression régulière non refermé"}
		case r == '\\':
			l.advance()
			if c, _ := l.peek(); c != eof && !isLineTerminator(c) {
				l.advance()
			}
			continue
		case r == '[':
			inClass = true
		case r == ']':
			inClass = false
		case r == '/' && !inClass:
			l.advance()
			for {
				c, _ := l.peek()
				if c == eof || !isIdentPart(c) {
					break
				}
				l.advance()
			}
			return Token{Kind: Regexp, Pos: start, Value: l.src[start.Offset:l.off]}
		}
		l.advance()
	}
}

// punct décrit un ponctuateur : son texte et son genre. La table est ordonnée
// du plus long au plus court, ce qui réalise le munch maximal par simple
// parcours.
type punct struct {
	text string
	kind Kind
}

var puncts = []punct{
	{">>>=", UShrAssign},
	{"...", Ellipsis}, {"===", EqEqEq}, {"!==", NotEqEq}, {"**=", StarStarAssign},
	{"<<=", ShlAssign}, {">>=", ShrAssign}, {">>>", UShr}, {"&&=", AndAndAssign},
	{"||=", OrOrAssign}, {"??=", QuestionQuestionAssign},
	{"=>", Arrow}, {"==", EqEq}, {"!=", NotEq}, {"<=", Le}, {">=", Ge},
	{"**", StarStar}, {"++", PlusPlus}, {"--", MinusMinus}, {"<<", Shl},
	{">>", Shr}, {"&&", AndAnd}, {"||", OrOr}, {"??", QuestionQuestion},
	{"?.", QuestionDot}, {"+=", PlusAssign}, {"-=", MinusAssign},
	{"*=", StarAssign}, {"/=", SlashAssign}, {"%=", PercentAssign},
	{"&=", AndAssign}, {"|=", OrAssign}, {"^=", XorAssign},
	{"{", LBrace}, {"}", RBrace}, {"(", LParen}, {")", RParen},
	{"[", LBracket}, {"]", RBracket}, {".", Dot}, {";", Semicolon},
	{",", Comma}, {":", Colon}, {"?", Question}, {"<", Lt}, {">", Gt},
	{"+", Plus}, {"-", Minus}, {"*", Star}, {"/", Slash}, {"%", Percent},
	{"&", And}, {"|", Or}, {"^", Xor}, {"!", Not}, {"~", Tilde}, {"=", Assign},
}

func (l *Lexer) scanPunct(start Position) Token {
	rest := l.src[l.off:]
	for _, p := range puncts {
		if strings.HasPrefix(rest, p.text) {
			// "?." suivi d'un chiffre est un point d'interrogation puis un
			// accès, pas un chaînage optionnel : a?.5:b reste un ternaire.
			if p.kind == QuestionDot && isDecimal(l.peekAt(2)) {
				continue
			}
			for i := 0; i < len(p.text); i++ {
				l.advance()
			}
			return Token{Kind: p.kind, Pos: start, Value: p.text}
		}
	}
	l.advance() // progression garantie : aucun caractère ne peut bloquer le lexer
	return Token{Kind: Illegal, Pos: start, Value: l.src[start.Offset:l.off],
		Message: "caractère inattendu"}
}

// ─── Sauvegarde et reprise ──────────────────────────────────────────────────

// State fige la position du lexer. Il sert au parser pour relire un lexème dans
// un autre mode : la distinction entre division et expression régulière n'étant
// pas décidable lexicalement, le parser lit d'abord en mode division, puis
// relit en mode expression régulière lorsqu'il constate être en position de
// préfixe. Cette relecture est exacte, là où une heuristique sur le lexème
// précédent se trompe notamment après une parenthèse fermante — « if (x) /re/ »
// est une expression régulière, « (a) / b » une division.
type State struct {
	off, line, col int
	newlineBefore  bool
}

// Save fige l'état courant.
func (l *Lexer) Save() State {
	return State{off: l.off, line: l.line, col: l.col, newlineBefore: l.newlineBefore}
}

// Restore replace le lexer dans un état antérieur. Toute autre valeur que celle
// rendue par Save sur le même lexer est ignorée si elle sort de la source.
func (l *Lexer) Restore(s State) {
	if s.off < 0 || s.off > len(l.src) {
		return
	}
	l.off, l.line, l.col = s.off, s.line, s.col
	l.newlineBefore = s.newlineBefore
}

// SeekTo replace le lexer sur une position rendue par un lexème, pour le relire.
func (l *Lexer) SeekTo(p Position) {
	if p.Offset < 0 || p.Offset > len(l.src) {
		return
	}
	l.off, l.line, l.col = p.Offset, p.Line, p.Col
}
