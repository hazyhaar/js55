// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	jsreMaxInputLen = 1 << 20

	jsreGasNode      int64 = 1
	jsreGasBacktrack int64 = 2
)

type regexpFinder interface {
	FindStringSubmatchIndex(s string) []int
	FindStringSubmatchIndexWithGas(s string, gas *int64) ([]int, bool)
	FindStringSubmatchIndexInterruptible(s string, gas *int64, isInterrupted func() bool) ([]int, bool)
	SubexpNames() []string
}

type re2Finder struct {
	re interface {
		FindStringSubmatchIndex(string) []int
		SubexpNames() []string
	}
}

func (f re2Finder) FindStringSubmatchIndex(s string) []int {
	if f.re == nil {
		return nil
	}
	return f.re.FindStringSubmatchIndex(s)
}

func (f re2Finder) FindStringSubmatchIndexWithGas(s string, gas *int64) ([]int, bool) {
	if len(s) > jsreMaxInputLen {
		return nil, false
	}
	if gas != nil {
		if *gas <= 0 {
			return nil, false
		}
		*gas -= int64(len(s))
		if *gas <= 0 {
			return nil, false
		}
	}
	return f.FindStringSubmatchIndex(s), true
}

// FindStringSubmatchIndexInterruptible honore l'annulation de l'isolat avant de
// déléguer au moteur RE2 ; la chaîne qui dépasse le plafond admis est refusée
// plutôt que débitée au plafond.
func (f re2Finder) FindStringSubmatchIndexInterruptible(s string, gas *int64, isInterrupted func() bool) ([]int, bool) {
	if isInterrupted != nil && isInterrupted() {
		return nil, false
	}
	return f.FindStringSubmatchIndexWithGas(s, gas)
}

func (f re2Finder) SubexpNames() []string {
	if f.re == nil {
		return nil
	}
	return f.re.SubexpNames()
}

func (p *jsreProg) SubexpNames() []string { return p.names }

const (
	jsreEmpty byte = iota
	jsreLit
	jsreDot
	jsreCls
	jsreConcat
	jsreAlt
	jsreRepeat
	jsreCap
	jsreLook
	jsreBack
	jsreAnchor
)

type jsreNode struct {
	op       byte
	r        rune
	kids     []*jsreNode
	min, max int
	lazy     bool
	cap      int
	name     string
	neg      bool
	behind   bool
	cls      *jsreCharClass
	anchor   byte
}

type jsreCharClass struct {
	neg    bool
	latin  [256]bool
	ranges [][2]rune
}

type jsreProg struct {
	root      *jsreNode
	ncap      int
	icase     bool
	multiline bool
	dotAll    bool
	names     []string
	nameIdx   map[string]int
}

type jsreParser struct {
	pat       string
	i         int
	icase     bool
	multiline bool
	dotAll    bool
	ncap      int
	names     []string
	nameIdx   map[string]int
}

func compileJSRE(pattern, flags string) (*jsreProg, error) {
	p := &jsreParser{
		pat:     pattern,
		names:   []string{""},
		nameIdx: map[string]int{},
	}
	for _, f := range flags {
		switch f {
		case 'i':
			p.icase = true
		case 'm':
			p.multiline = true
		case 's':
			p.dotAll = true
		}
	}
	n, err := p.disjunction()
	if err != nil {
		return nil, err
	}
	if p.i < len(p.pat) {
		return nil, fmt.Errorf("trailing %q", p.pat[p.i:])
	}
	return &jsreProg{
		root:      n,
		ncap:      p.ncap,
		icase:     p.icase,
		multiline: p.multiline,
		dotAll:    p.dotAll,
		names:     p.names,
		nameIdx:   p.nameIdx,
	}, nil
}

func (p *jsreParser) peek() byte {
	if p.i >= len(p.pat) {
		return 0
	}
	return p.pat[p.i]
}

func (p *jsreParser) rest() string {
	if p.i >= len(p.pat) {
		return ""
	}
	return p.pat[p.i:]
}

func (p *jsreParser) disjunction() (*jsreNode, error) {
	left, err := p.alternative()
	if err != nil {
		return nil, err
	}
	if p.peek() != '|' {
		return left, nil
	}
	alts := []*jsreNode{left}
	for p.peek() == '|' {
		p.i++
		n, err := p.alternative()
		if err != nil {
			return nil, err
		}
		alts = append(alts, n)
	}
	return &jsreNode{op: jsreAlt, kids: alts}, nil
}

func (p *jsreParser) alternative() (*jsreNode, error) {
	var terms []*jsreNode
	for {
		c := p.peek()
		if c == 0 || c == '|' || c == ')' {
			break
		}
		n, err := p.term()
		if err != nil {
			return nil, err
		}
		if n != nil {
			terms = append(terms, n)
		}
	}
	if len(terms) == 0 {
		return &jsreNode{op: jsreEmpty}, nil
	}
	if len(terms) == 1 {
		return terms[0], nil
	}
	return &jsreNode{op: jsreConcat, kids: terms}, nil
}

func (p *jsreParser) term() (*jsreNode, error) {
	n, assertion, err := p.atom()
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}
	min, max, lazy, ok, err := p.quantifier()
	if err != nil {
		return nil, err
	}
	if !ok {
		return n, nil
	}
	if assertion {
		return nil, fmt.Errorf("quantifier on assertion")
	}
	return &jsreNode{op: jsreRepeat, kids: []*jsreNode{n}, min: min, max: max, lazy: lazy}, nil
}

func (p *jsreParser) quantifier() (min, max int, lazy, ok bool, err error) {
	c := p.peek()
	switch c {
	case '*':
		p.i++
		min, max, ok = 0, -1, true
	case '+':
		p.i++
		min, max, ok = 1, -1, true
	case '?':
		p.i++
		min, max, ok = 0, 1, true
	case '{':
		min, max, ok, err = p.braceQuant()
		if err != nil || !ok {
			return 0, 0, false, ok, err
		}
	default:
		return 0, 0, false, false, nil
	}
	if p.peek() == '?' {
		p.i++
		lazy = true
	}
	return min, max, lazy, true, nil
}

func (p *jsreParser) braceQuant() (min, max int, ok bool, err error) {
	start := p.i
	p.i++
	if p.i >= len(p.pat) || p.pat[p.i] < '0' || p.pat[p.i] > '9' {
		p.i = start
		return 0, 0, false, nil
	}
	min, p.i = p.readInt(p.i)
	max = min
	if p.peek() == ',' {
		p.i++
		if p.peek() >= '0' && p.peek() <= '9' {
			max, p.i = p.readInt(p.i)
		} else {
			max = -1
		}
	}
	if p.peek() != '}' {
		p.i = start
		return 0, 0, false, nil
	}
	p.i++
	if max >= 0 && max < min {
		return 0, 0, false, fmt.Errorf("out of order quantifier")
	}
	return min, max, true, nil
}

func (p *jsreParser) readInt(i int) (int, int) {
	n := 0
	for i < len(p.pat) && p.pat[i] >= '0' && p.pat[i] <= '9' {
		n = n*10 + int(p.pat[i]-'0')
		i++
	}
	return n, i
}

func (p *jsreParser) atom() (*jsreNode, bool, error) {
	if p.i >= len(p.pat) {
		return nil, false, nil
	}
	c := p.pat[p.i]
	switch c {
	case '^':
		p.i++
		return &jsreNode{op: jsreAnchor, anchor: '^'}, true, nil
	case '$':
		p.i++
		return &jsreNode{op: jsreAnchor, anchor: '$'}, true, nil
	case '.':
		p.i++
		return &jsreNode{op: jsreDot}, false, nil
	case '[':
		cls, err := p.parseClass()
		if err != nil {
			return nil, false, err
		}
		return &jsreNode{op: jsreCls, cls: cls}, false, nil
	case '(':
		return p.parseGroup()
	case '\\':
		return p.parseEscape(false)
	case '|', ')':
		return nil, false, nil
	case '*', '+', '?':
		return nil, false, fmt.Errorf("nothing to repeat")
	case '{':
		save := p.i
		_, _, ok, err := p.braceQuant()
		if err != nil {
			return nil, false, err
		}
		if ok {
			return nil, false, fmt.Errorf("nothing to repeat")
		}
		p.i = save + 1
		return &jsreNode{op: jsreLit, r: '{'}, false, nil
	default:
		r, sz := utf8.DecodeRuneInString(p.pat[p.i:])
		p.i += sz
		return &jsreNode{op: jsreLit, r: r}, false, nil
	}
}

func (p *jsreParser) parseGroup() (*jsreNode, bool, error) {
	p.i++
	if p.peek() != '?' {
		p.ncap++
		idx := p.ncap
		p.names = append(p.names, "")
		inner, err := p.disjunction()
		if err != nil {
			return nil, false, err
		}
		if p.peek() != ')' {
			return nil, false, fmt.Errorf("unclosed group")
		}
		p.i++
		return &jsreNode{op: jsreCap, cap: idx, kids: []*jsreNode{inner}}, false, nil
	}
	p.i++
	rest := p.rest()
	switch {
	case len(rest) == 0:
		return nil, false, fmt.Errorf("empty group")
	case rest[0] == ':':
		p.i++
		inner, err := p.disjunction()
		if err != nil {
			return nil, false, err
		}
		if p.peek() != ')' {
			return nil, false, fmt.Errorf("unclosed group")
		}
		p.i++
		return inner, false, nil
	case rest[0] == '=':
		p.i++
		inner, err := p.disjunction()
		if err != nil {
			return nil, false, err
		}
		if p.peek() != ')' {
			return nil, false, fmt.Errorf("unclosed group")
		}
		p.i++
		return &jsreNode{op: jsreLook, kids: []*jsreNode{inner}}, true, nil
	case rest[0] == '!':
		p.i++
		inner, err := p.disjunction()
		if err != nil {
			return nil, false, err
		}
		if p.peek() != ')' {
			return nil, false, fmt.Errorf("unclosed group")
		}
		p.i++
		return &jsreNode{op: jsreLook, neg: true, kids: []*jsreNode{inner}}, true, nil
	case len(rest) >= 2 && rest[0] == '<' && rest[1] == '=':
		p.i += 2
		inner, err := p.disjunction()
		if err != nil {
			return nil, false, err
		}
		if p.peek() != ')' {
			return nil, false, fmt.Errorf("unclosed group")
		}
		p.i++
		return &jsreNode{op: jsreLook, behind: true, kids: []*jsreNode{inner}}, true, nil
	case len(rest) >= 2 && rest[0] == '<' && rest[1] == '!':
		p.i += 2
		inner, err := p.disjunction()
		if err != nil {
			return nil, false, err
		}
		if p.peek() != ')' {
			return nil, false, fmt.Errorf("unclosed group")
		}
		p.i++
		return &jsreNode{op: jsreLook, behind: true, neg: true, kids: []*jsreNode{inner}}, true, nil
	case rest[0] == '<':
		p.i++
		name, err := p.readName()
		if err != nil {
			return nil, false, err
		}
		p.ncap++
		idx := p.ncap
		p.names = append(p.names, name)
		p.nameIdx[name] = idx
		inner, err := p.disjunction()
		if err != nil {
			return nil, false, err
		}
		if p.peek() != ')' {
			return nil, false, fmt.Errorf("unclosed group")
		}
		p.i++
		return &jsreNode{op: jsreCap, cap: idx, name: name, kids: []*jsreNode{inner}}, false, nil
	default:
		return nil, false, fmt.Errorf("unknown group")
	}
}

func (p *jsreParser) readName() (string, error) {
	start := p.i
	for p.i < len(p.pat) {
		c := p.pat[p.i]
		if c == '>' {
			if p.i == start {
				return "", fmt.Errorf("empty name")
			}
			name := p.pat[start:p.i]
			p.i++
			return name, nil
		}
		if !(c == '_' || c == '$' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return "", fmt.Errorf("bad name")
		}
		p.i++
	}
	return "", fmt.Errorf("unclosed name")
}

func (p *jsreParser) parseEscape(inClass bool) (*jsreNode, bool, error) {
	p.i++
	if p.i >= len(p.pat) {
		return nil, false, fmt.Errorf("dangling escape")
	}
	c := p.pat[p.i]
	p.i++
	switch c {
	case 'd', 'D', 'w', 'W', 's', 'S':
		cls := unicodeClass(c)
		return &jsreNode{op: jsreCls, cls: cls}, false, nil
	case 'b':
		if inClass {
			return &jsreNode{op: jsreLit, r: '\b'}, false, nil
		}
		return &jsreNode{op: jsreAnchor, anchor: 'b'}, true, nil
	case 'B':
		if inClass {
			return &jsreNode{op: jsreLit, r: 'B'}, false, nil
		}
		return &jsreNode{op: jsreAnchor, anchor: 'B'}, true, nil
	case 'n':
		return &jsreNode{op: jsreLit, r: '\n'}, false, nil
	case 'r':
		return &jsreNode{op: jsreLit, r: '\r'}, false, nil
	case 't':
		return &jsreNode{op: jsreLit, r: '\t'}, false, nil
	case 'f':
		return &jsreNode{op: jsreLit, r: '\f'}, false, nil
	case 'v':
		return &jsreNode{op: jsreLit, r: '\v'}, false, nil
	case '0':
		return &jsreNode{op: jsreLit, r: 0}, false, nil
	case 'x':
		r, err := p.readHex(2)
		if err != nil {
			return &jsreNode{op: jsreLit, r: 'x'}, false, nil
		}
		return &jsreNode{op: jsreLit, r: r}, false, nil
	case 'u':
		r, err := p.readHex(4)
		if err != nil {
			return &jsreNode{op: jsreLit, r: 'u'}, false, nil
		}
		return &jsreNode{op: jsreLit, r: r}, false, nil
	case 'k':
		if p.peek() == '<' {
			p.i++
			name, err := p.readName()
			if err != nil {
				return nil, false, err
			}
			idx, ok := p.nameIdx[name]
			if !ok {
				return nil, false, fmt.Errorf("unknown named backreference")
			}
			return &jsreNode{op: jsreBack, cap: idx}, false, nil
		}
		return &jsreNode{op: jsreLit, r: 'k'}, false, nil
	default:
		if c >= '1' && c <= '9' && !inClass {
			idx := int(c - '0')
			for p.i < len(p.pat) && p.pat[p.i] >= '0' && p.pat[p.i] <= '9' {
				n := idx*10 + int(p.pat[p.i]-'0')
				if n > 99 {
					break
				}
				idx = n
				p.i++
			}
			return &jsreNode{op: jsreBack, cap: idx}, false, nil
		}
		r, sz := utf8.DecodeRuneInString(string(c) + p.pat[p.i:])
		if sz > 1 {
			p.i += sz - 1
		}
		return &jsreNode{op: jsreLit, r: r}, false, nil
	}
}

func (p *jsreParser) readHex(n int) (rune, error) {
	if p.i+n > len(p.pat) {
		return 0, fmt.Errorf("short hex")
	}
	v := 0
	for k := 0; k < n; k++ {
		c := p.pat[p.i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			d = int(c - 'A' + 10)
		default:
			return 0, fmt.Errorf("bad hex")
		}
		v = v*16 + d
		p.i++
	}
	return rune(v), nil
}

func (p *jsreParser) parseClass() (*jsreCharClass, error) {
	p.i++
	cls := &jsreCharClass{}
	if p.peek() == '^' {
		cls.neg = true
		p.i++
	}
	first := true
	var pendingDash bool
	var last rune
	var hasLast bool
	flush := func(r rune) {
		if pendingDash && hasLast {
			if last > r {
				cls.addRune(last)
				cls.addRune('-')
				cls.addRune(r)
			} else {
				cls.addRange(last, r)
			}
			pendingDash = false
			hasLast = false
			return
		}
		if hasLast {
			cls.addRune(last)
		}
		last = r
		hasLast = true
	}
	for p.i < len(p.pat) && (p.pat[p.i] != ']' || first) {
		first = false
		if p.pat[p.i] == '\\' {
			n, _, err := p.parseEscape(true)
			if err != nil {
				return nil, err
			}
			if n.op == jsreCls && n.cls != nil {
				if hasLast {
					cls.addRune(last)
					hasLast = false
				}
				pendingDash = false
				cls.merge(n.cls)
				continue
			}
			if n.op == jsreLit {
				if pendingDash && hasLast {
					flush(n.r)
				} else if p.peek() == '-' {
					if hasLast {
						cls.addRune(last)
					}
					last = n.r
					hasLast = true
					pendingDash = false
				} else {
					flush(n.r)
				}
			}
			continue
		}
		if p.pat[p.i] == '-' && hasLast && p.i+1 < len(p.pat) && p.pat[p.i+1] != ']' {
			p.i++
			pendingDash = true
			continue
		}
		r, sz := utf8.DecodeRuneInString(p.pat[p.i:])
		p.i += sz
		flush(r)
	}
	if hasLast {
		cls.addRune(last)
	}
	if pendingDash {
		cls.addRune('-')
	}
	if p.peek() != ']' {
		return nil, fmt.Errorf("unclosed class")
	}
	p.i++
	return cls, nil
}

func (c *jsreCharClass) addRune(r rune) { c.addRange(r, r) }

func (c *jsreCharClass) addRange(a, b rune) {
	if a > b {
		a, b = b, a
	}
	if b < 256 {
		for r := a; r <= b; r++ {
			c.latin[r] = true
		}
		return
	}
	if a < 256 {
		for r := a; r < 256; r++ {
			c.latin[r] = true
		}
		a = 256
	}
	c.ranges = append(c.ranges, [2]rune{a, b})
}

func (c *jsreCharClass) merge(o *jsreCharClass) {
	for i := 0; i < 256; i++ {
		if o.latin[i] {
			c.latin[i] = true
		}
	}
	c.ranges = append(c.ranges, o.ranges...)
}

func (c *jsreCharClass) has(r rune, icase bool) bool {
	ok := c.hasExact(r)
	if !ok && icase {
		ok = c.hasExact(unicode.ToLower(r)) || c.hasExact(unicode.ToUpper(r))
	}
	if c.neg {
		return !ok
	}
	return ok
}

func (c *jsreCharClass) hasExact(r rune) bool {
	if r < 256 {
		return c.latin[r]
	}
	for _, rg := range c.ranges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

func unicodeClass(kind byte) *jsreCharClass {
	cls := &jsreCharClass{}
	switch kind {
	case 'd':
		cls.addRange('0', '9')
	case 'D':
		cls.neg = true
		cls.addRange('0', '9')
	case 'w':
		cls.addRange('0', '9')
		cls.addRange('A', 'Z')
		cls.addRange('a', 'z')
		cls.addRune('_')
	case 'W':
		cls.neg = true
		cls.addRange('0', '9')
		cls.addRange('A', 'Z')
		cls.addRange('a', 'z')
		cls.addRune('_')
	case 's':
		for _, r := range []rune{'\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff} {
			cls.addRune(r)
		}
		cls.addRange(0x2000, 0x200a)
	case 'S':
		u := unicodeClass('s')
		u.neg = true
		return u
	}
	return cls
}

// jsreRun porte l'état d'exécution partagé par la récursion du moteur : le
// quota de gas de l'isolat, le drapeau d'abandon terminal et le contrôle
// périodique d'annulation. Le drapeau aborted distingue un échec ordinaire
// (aucune correspondance à cette position) d'une rupture de quota, distinction
// que l'unique booléen de retour ne peut exprimer.
type jsreRun struct {
	gas       *int64
	aborted   bool
	interrupt func() bool
	steps     uint
}

// enter franchit le seuil d'un nœud : contrôle périodique d'annulation puis
// débit d'un pas. Le passage au nœud suivant n'est autorisé que tant que le
// drapeau d'abandon n'est pas posé.
func (r *jsreRun) enter() bool {
	if r == nil || r.aborted {
		return false
	}
	r.steps++
	if r.interrupt != nil && r.steps&0xff == 0 && r.interrupt() {
		r.aborted = true
		return false
	}
	return r.charge(jsreGasNode)
}

// charge débite n unités de gas. Toute insuffisance pose le drapeau d'abandon
// terminal et met le compteur à zéro, de sorte qu'aucun appelant ne puisse
// confondre une rupture de quota avec une fin normale sans correspondance.
func (r *jsreRun) charge(n int64) bool {
	if r.aborted {
		return false
	}
	if r.gas == nil {
		return true
	}
	if *r.gas <= n {
		*r.gas = 0
		r.aborted = true
		return false
	}
	*r.gas -= n
	return true
}

func (p *jsreProg) FindStringSubmatchIndex(s string) []int {
	gas := int64(8_000_000)
	loc, _ := p.findSubmatchIndex(s, &gas, nil)
	return loc
}

// FindStringSubmatchIndexWithGas exécute la recherche en décomptant le quota
// fourni. Le booléen vaut false dès que le gas est épuisé : l'appelant peut
// alors interrompre l'isolat au lieu de relancer une recherche sur la position
// suivante.
func (p *jsreProg) FindStringSubmatchIndexWithGas(s string, gas *int64) ([]int, bool) {
	return p.findSubmatchIndex(s, gas, nil)
}

// FindStringSubmatchIndexInterruptible raccorde l'annulation externe de
// l'isolat au moteur : le rappel isInterrupted est consulté périodiquement
// pendant la marche, ce qui permet d'abandonner une recherche coûteuse avant
// l'épuisement complet du quota.
func (p *jsreProg) FindStringSubmatchIndexInterruptible(s string, gas *int64, isInterrupted func() bool) ([]int, bool) {
	return p.findSubmatchIndex(s, gas, isInterrupted)
}

func (p *jsreProg) findSubmatchIndex(s string, gas *int64, isInterrupted func() bool) ([]int, bool) {
	if isInterrupted != nil && isInterrupted() {
		return nil, false
	}
	if len(s) > jsreMaxInputLen {
		return nil, false
	}
	if gas == nil {
		g := int64(8_000_000)
		gas = &g
	}
	caps := make([]int, (p.ncap+1)*2)
	run := &jsreRun{gas: gas, interrupt: isInterrupted}
	for i := 0; i <= len(s); {
		for j := range caps {
			caps[j] = -1
		}
		ok := p.exec(p.root, s, i, caps, run, func(end int) bool {
			caps[0] = i
			caps[1] = end
			return true
		})
		if ok {
			return caps, true
		}
		if run.aborted {
			return nil, false
		}
		if i == len(s) {
			break
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		if sz < 1 {
			sz = 1
		}
		i += sz
	}
	return nil, true
}

func (p *jsreProg) exec(n *jsreNode, s string, pos int, caps []int, run *jsreRun, k func(int) bool) bool {
	if n == nil {
		return k(pos)
	}
	if !run.enter() {
		return false
	}
	switch n.op {
	case jsreEmpty:
		return k(pos)
	case jsreLit:
		if pos >= len(s) {
			return false
		}
		r, sz := utf8.DecodeRuneInString(s[pos:])
		if !p.eqRune(r, n.r) {
			return false
		}
		return k(pos + sz)
	case jsreDot:
		if pos >= len(s) {
			return false
		}
		r, sz := utf8.DecodeRuneInString(s[pos:])
		if !p.dotAll && (r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029) {
			return false
		}
		return k(pos + sz)
	case jsreCls:
		if pos >= len(s) {
			return false
		}
		r, sz := utf8.DecodeRuneInString(s[pos:])
		if n.cls == nil || !n.cls.has(r, p.icase) {
			return false
		}
		return k(pos + sz)
	case jsreConcat:
		var walk func(int, int) bool
		walk = func(i, pos int) bool {
			if i == len(n.kids) {
				return k(pos)
			}
			return p.exec(n.kids[i], s, pos, caps, run, func(np int) bool {
				return walk(i+1, np)
			})
		}
		return walk(0, pos)
	case jsreAlt:
		for idx, kid := range n.kids {
			if idx > 0 {
				if !run.charge(jsreGasBacktrack) {
					return false
				}
			}
			if p.exec(kid, s, pos, caps, run, k) {
				return true
			}
			if run.aborted {
				return false
			}
		}
		return false
	case jsreRepeat:
		child := n.kids[0]
		max := n.max
		if max < 0 {
			max = len(s) - pos + 1
			if max < n.min {
				max = n.min
			}
		}
		var take func(count, pos int) bool
		take = func(count, pos int) bool {
			if !run.charge(jsreGasBacktrack) {
				return false
			}
			if n.lazy {
				if count >= n.min && k(pos) {
					return true
				}
				if count == max {
					return false
				}
				return p.exec(child, s, pos, caps, run, func(np int) bool {
					if np == pos && count >= n.min {
						return false
					}
					return take(count+1, np)
				})
			}
			if count == max {
				return k(pos)
			}
			matched := false
			if count < max {
				matched = p.exec(child, s, pos, caps, run, func(np int) bool {
					if np == pos && count >= n.min {
						return k(pos)
					}
					return take(count+1, np)
				})
			}
			if matched {
				return true
			}
			if run.aborted {
				return false
			}
			if count >= n.min {
				return k(pos)
			}
			return false
		}
		return take(0, pos)
	case jsreCap:
		oldS, oldE := -1, -1
		if n.cap*2+1 < len(caps) {
			oldS, oldE = caps[n.cap*2], caps[n.cap*2+1]
		}
		start := pos
		inner := n.kids[0]
		return p.exec(inner, s, pos, caps, run, func(end int) bool {
			if n.cap*2+1 < len(caps) {
				caps[n.cap*2] = start
				caps[n.cap*2+1] = end
			}
			if k(end) {
				return true
			}
			if n.cap*2+1 < len(caps) {
				caps[n.cap*2], caps[n.cap*2+1] = oldS, oldE
			}
			return false
		})
	case jsreLook:
		inner := n.kids[0]
		if n.behind {
			found := false
			for start := 0; start <= pos; {
				if p.exec(inner, s, start, caps, run, func(end int) bool {
					return end == pos
				}) {
					found = true
					break
				}
				if run.aborted {
					return false
				}
				if start == pos {
					break
				}
				_, sz := utf8.DecodeRuneInString(s[start:])
				if sz < 1 {
					sz = 1
				}
				start += sz
			}
			if run.aborted {
				return false
			}
			if n.neg {
				found = !found
			}
			if found {
				return k(pos)
			}
			return false
		}
		found := p.exec(inner, s, pos, caps, run, func(int) bool { return true })
		if run.aborted {
			return false
		}
		if n.neg {
			found = !found
		}
		if found {
			return k(pos)
		}
		return false
	case jsreBack:
		if n.cap*2+1 >= len(caps) {
			return false
		}
		a, b := caps[n.cap*2], caps[n.cap*2+1]
		if a < 0 || b < 0 || b < a {
			return false
		}
		frag := s[a:b]
		if pos+len(frag) > len(s) {
			return false
		}
		got := s[pos : pos+len(frag)]
		if p.icase {
			if !eqFold(got, frag) {
				return false
			}
		} else if got != frag {
			return false
		}
		return k(pos + len(frag))
	case jsreAnchor:
		switch n.anchor {
		case '^':
			if pos == 0 || (p.multiline && pos > 0 && isNL(prevRune(s, pos))) {
				return k(pos)
			}
		case '$':
			if pos == len(s) {
				return k(pos)
			}
			if p.multiline && pos < len(s) {
				r, _ := utf8.DecodeRuneInString(s[pos:])
				if isNL(r) {
					return k(pos)
				}
			}
		case 'b':
			if isWordBound(s, pos) {
				return k(pos)
			}
		case 'B':
			if !isWordBound(s, pos) {
				return k(pos)
			}
		}
		return false
	default:
		return false
	}
}

func (p *jsreProg) eqRune(a, b rune) bool {
	if a == b {
		return true
	}
	if !p.icase {
		return false
	}
	return unicode.ToLower(a) == unicode.ToLower(b)
}

func eqFold(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	if len(ar) != len(br) {
		return false
	}
	for i := range ar {
		if unicode.ToLower(ar[i]) != unicode.ToLower(br[i]) {
			return false
		}
	}
	return true
}

func isNL(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}

func isWordBound(s string, pos int) bool {
	prev := wordRune(prevRune(s, pos))
	var next bool
	if pos < len(s) {
		r, _ := utf8.DecodeRuneInString(s[pos:])
		next = wordRune(r)
	}
	return prev != next
}

func prevRune(s string, pos int) rune {
	if pos <= 0 {
		return 0
	}
	r, _ := utf8.DecodeLastRuneInString(s[:pos])
	return r
}

func wordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
