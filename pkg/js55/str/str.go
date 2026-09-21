// Package str porte la représentation des chaînes du moteur.
//
// Le fait qui commande toute la conception : une chaîne JavaScript est une
// SUITE D'UNITÉS DE CODE UTF-16, et cela est observable depuis le langage.
// « "😀".length » vaut 2, « charCodeAt » rend des demi-codets isolés, et une
// chaîne peut légalement contenir un demi-codet non apparié. Une représentation
// interne en UTF-8 rendrait donc « length » et l'indexation linéaires, ou
// imposerait un index parallèle. C'est la raison pour laquelle aucun moteur de
// production ne stocke ses chaînes en UTF-8, et pourquoi ce paquet n'en stocke
// pas non plus.
//
// Trois formes sont implémentées, une reste réservée (plan §J2.2) :
//
//	Latin1  []byte    une unité de code par octet, valeurs 0..255
//	UTF16   []uint16  plat
//	Rope              concaténation différée, aplatie à la demande
//	Sliced            réservé, non implémenté — vue sur une chaîne parente
//
// La forme corde n'était initialement qu'un tag réservé. Le banc T2.3 a mesuré
// la construction d'une chaîne d'un mégaoctet par concaténations successives à
// 14 069 fois le coût du témoin linéaire : le besoin est devenu MESURÉ et non
// supposé, et la forme a été implantée. La forme Sliced conserve son statut de
// tag réservé, avec une branche qui panique dans tous les commutateurs — un tag
// qui traverse un commutateur en silence est la manière dont un défaut de forme
// s'installe.
//
// Contrainte C1 : aucune interface sur le chemin de calcul. Kind est un octet,
// et les commutateurs portent sur lui.
package str

import (
	"os"
	"sync/atomic"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/hazyhaar/js55/pkg/c2strclass"
)

// Kind identifie la forme de stockage. Dense et contiguë (contrainte C2).
type Kind uint8

const (
	Latin1 Kind = iota
	UTF16
	Rope   // concaténation différée
	Sliced // réservé, non implémenté
	numKinds
)

func (k Kind) String() string {
	switch k {
	case Latin1:
		return "Latin1"
	case UTF16:
		return "UTF16"
	case Rope:
		return "Rope"
	case Sliced:
		return "Sliced"
	}
	return "Kind(?)"
}

// NumKinds borne l'énumération, pour les gardes.
const NumKinds = int(numKinds)

// forceUTF16 impose la forme UTF-16 à toute chaîne construite. C'est le levier
// de l'invariant métamorphique T2.2 : la totalité de la suite de conformité doit
// rendre un résultat identique avec et sans. Un écart entre les deux modes est
// un défaut de forme, jamais une tolérance.
var forceUTF16 = os.Getenv("JS55_FORCE_UTF16") == "1"

// ForceUTF16 indique si le mode métamorphique est actif.
func ForceUTF16() bool { return forceUTF16 }

// SetForceUTF16 bascule le mode. Réservé aux tests ; le mode normal se règle par
// la variable d'environnement JS55_FORCE_UTF16.
func SetForceUTF16(v bool) { forceUTF16 = v }

const hashComputedBit = uint64(1) << 32

// String est une chaîne du moteur. Les tranches latin1 et utf16 sont
// mutuellement exclusives : celle qui ne correspond pas à kind est nil.
type String struct {
	hashState uint64 // bit 32 : calculé, bits 0..31 : empreinte FNV-1a
	kind      Kind
	narrow    bool // corde : les deux côtés tiennent en Latin-1
	length    int  // valide pour TOUTES les formes, corde comprise
	latin1    []byte
	utf16     []uint16
	// left et right ne valent que pour la forme Rope. Aplatir les remet à nil.
	left, right *String
}

// ropeThreshold est la longueur au-delà de laquelle une concaténation devient
// une corde au lieu de recopier. En deçà, la recopie est moins chère que le
// nœud et son aplatissement ultérieur.
const ropeThreshold = 32

// Empty est la chaîne vide, partagée.
var Empty = &String{kind: Latin1, latin1: []byte{}, length: 0}

func init() {
	Empty.Hash()
}

// unsupported centralise le refus des formes réservées. Toute branche de
// déréférencement passe par ici, ce qui garantit qu'aucune ne les ignore en
// silence.
func unsupported(k Kind, op string) {
	panic("js55/str: forme " + k.String() + " non implémentée (" + op + ")")
}

// ─── Construction ───────────────────────────────────────────────────────────

// FromUTF16 construit une chaîne depuis des unités de code. La forme est
// décidée par un scan, dont le noyau est la feuille émise c2strclass : celle-ci
// rend 1 si toutes les unités tiennent dans un octet.
func FromUTF16(u []uint16) *String {
	if len(u) == 0 {
		return Empty
	}
	if !forceUTF16 && c2strclass.C2strclass_is_latin1_u16(u, uint64(len(u))) == 1 {
		b := make([]byte, len(u))
		for i, c := range u {
			b[i] = byte(c)
		}
		return &String{kind: Latin1, latin1: b, length: len(b)}
	}
	cp := make([]uint16, len(u))
	copy(cp, u)
	return &String{kind: UTF16, utf16: cp, length: len(cp)}
}

// FromLatin1 construit une chaîne depuis des octets déjà en Latin-1. Les octets
// sont recopiés : une chaîne du moteur est immuable.
func FromLatin1(b []byte) *String {
	if len(b) == 0 {
		return Empty
	}
	if forceUTF16 {
		u := make([]uint16, len(b))
		for i, c := range b {
			u[i] = uint16(c)
		}
		return &String{kind: UTF16, utf16: u, length: len(u)}
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return &String{kind: Latin1, latin1: cp, length: len(cp)}
}

// FromGo construit une chaîne depuis du texte Go, supposé UTF-8 valide. Les
// octets invalides deviennent U+FFFD, conformément au décodage Go.
func FromGo(s string) *String {
	if s == "" {
		return Empty
	}
	// Chemin rapide : une source purement ASCII est déjà du Latin-1.
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return FromLatin1([]byte(s))
	}
	return FromUTF16(utf16.Encode([]rune(s)))
}

// ─── Accès ──────────────────────────────────────────────────────────────────

// Kind rend la forme de stockage.
func (s *String) Kind() Kind { return s.kind }

// Len rend la longueur en unités de code UTF-16, ce que le langage observe.
// La corde la connaît sans être aplatie : c'est ce qui rend « (a+b).length »
// gratuit.
func (s *String) Len() int { return s.length }

// At rend l'unité de code d'indice i. Hors borne, rend 0 : c'est à l'appelant
// de distinguer, comme le fait charCodeAt en rendant NaN.
func (s *String) At(i int) uint16 {
	s.flatten()
	if i < 0 || i >= s.Len() {
		return 0
	}
	switch s.kind {
	case Latin1:
		return uint16(s.latin1[i])
	case UTF16:
		return s.utf16[i]
	default:
		unsupported(s.kind, "At")
		return 0
	}
}

// CodePointAt rend le point de code à l'indice i et sa largeur en unités de
// code. Un demi-codet isolé est rendu tel quel sur une largeur de 1, comme le
// prescrit la sémantique du langage.
func (s *String) CodePointAt(i int) (rune, int) {
	n := s.Len()
	if i < 0 || i >= n {
		return -1, 0
	}
	c := s.At(i)
	if c < 0xD800 || c > 0xDBFF || i+1 >= n {
		return rune(c), 1
	}
	d := s.At(i + 1)
	if d < 0xDC00 || d > 0xDFFF {
		return rune(c), 1
	}
	return utf16.DecodeRune(rune(c), rune(d)), 2
}

// CharAt rend la sous-chaîne d'une seule unité de code à l'indice i, ou Empty si hors borne.
func (s *String) CharAt(i int) *String {
	if i < 0 || i >= s.Len() {
		return Empty
	}
	return s.Slice(i, i+1)
}

// FromCodePoints construit une chaîne depuis des points de code Unicode (0..0x10FFFF).
// Rend (nil, false) si un point de code est hors de l'intervalle licite [0, 0x10FFFF].
func FromCodePoints(cps []int) (*String, bool) {
	if len(cps) == 0 {
		return Empty, true
	}
	u16 := make([]uint16, 0, len(cps))
	for _, cp := range cps {
		if cp < 0 || cp > 0x10FFFF {
			return nil, false
		}
		if cp <= 0xFFFF {
			u16 = append(u16, uint16(cp))
		} else {
			r1, r2 := utf16.EncodeRune(rune(cp))
			u16 = append(u16, uint16(r1), uint16(r2))
		}
	}
	return FromUTF16(u16), true
}

// FromCodePoint construit une chaîne depuis un unique point de code Unicode (0..0x10FFFF).
// Rend (nil, false) si le point de code est hors de l'intervalle licite [0, 0x10FFFF].
func FromCodePoint(cp int) (*String, bool) {
	if cp < 0 || cp > 0x10FFFF {
		return nil, false
	}
	if cp <= 0xFF && !forceUTF16 {
		return FromLatin1([]byte{byte(cp)}), true
	}
	if cp <= 0xFFFF {
		return FromUTF16([]uint16{uint16(cp)}), true
	}
	r1, r2 := utf16.EncodeRune(rune(cp))
	return FromUTF16([]uint16{uint16(r1), uint16(r2)}), true
}

// UTF16 matérialise une copie des unités de code. Modifier la tranche rendue
// ne modifie jamais la chaîne ni son empreinte mémorisée.
func (s *String) UTF16() []uint16 {
	s.flatten()
	switch s.kind {
	case Latin1:
		u := make([]uint16, len(s.latin1))
		for i, c := range s.latin1 {
			u[i] = uint16(c)
		}
		return u
	case UTF16:
		cp := make([]uint16, len(s.utf16))
		copy(cp, s.utf16)
		return cp
	default:
		unsupported(s.kind, "UTF16")
		return nil
	}
}

func UTF16LenOfUTF8Prefix(s string, byteIdx int) int {
	if byteIdx <= 0 {
		return 0
	}
	if byteIdx > len(s) {
		byteIdx = len(s)
	}
	return len(utf16.Encode([]rune(s[:byteIdx])))
}

// GoString rend le texte en UTF-8. Les demi-codets isolés, licites dans une
// chaîne JavaScript mais inexprimables en UTF-8, deviennent U+FFFD. Cette
// fonction sert l'affichage et le diagnostic ; elle n'est PAS une conversion
// réversible et ne doit jamais servir de clé ni de support de comparaison.
func (s *String) GoString() string {
	s.flatten()
	switch s.kind {
	case Latin1:
		return string(latin1Runes(s.latin1))
	case UTF16:
		return string(utf16.Decode(s.utf16))
	default:
		unsupported(s.kind, "GoString")
		return ""
	}
}

func latin1Runes(b []byte) []rune {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return r
}

// ─── Opérations ─────────────────────────────────────────────────────────────

// Equal compare deux chaînes unité de code par unité de code. La comparaison ne
// dépend PAS de la forme de stockage : c'est ce que garantit l'invariant
// métamorphique T2.2.
func (s *String) Equal(t *String) bool {
	if s == t {
		return true
	}
	s.flatten()
	t.flatten()
	if s.Len() != t.Len() {
		return false
	}
	if s.kind == Latin1 && t.kind == Latin1 {
		return string(s.latin1) == string(t.latin1)
	}
	for i, n := 0, s.Len(); i < n; i++ {
		if s.At(i) != t.At(i) {
			return false
		}
	}
	return true
}

// EqualASCII compare aux unités d'un littéral ASCII sans allouer. Les octets
// hors ASCII rendent faux : le chemin FromGo reste requis pour l'UTF-8 général.
func (s *String) EqualASCII(lit string) bool {
	if s == nil || s.length != len(lit) {
		return false
	}
	for i := 0; i < len(lit); i++ {
		if lit[i] >= utf8.RuneSelf {
			return false
		}
	}
	s.flatten()
	switch s.kind {
	case Latin1:
		for i := 0; i < len(lit); i++ {
			if s.latin1[i] != lit[i] {
				return false
			}
		}
		return true
	case UTF16:
		for i := 0; i < len(lit); i++ {
			if s.utf16[i] != uint16(lit[i]) {
				return false
			}
		}
		return true
	default:
		unsupported(s.kind, "EqualASCII")
		return false
	}
}

// Compare ordonne deux chaînes par unité de code, comme l'opérateur « < » du
// langage. Rend -1, 0 ou 1.
func (s *String) Compare(t *String) int {
	s.flatten()
	t.flatten()
	n := s.Len()
	if m := t.Len(); m < n {
		n = m
	}
	for i := 0; i < n; i++ {
		a, b := s.At(i), t.At(i)
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	switch {
	case s.Len() < t.Len():
		return -1
	case s.Len() > t.Len():
		return 1
	}
	return 0
}

// Concat concatène deux chaînes. La forme du résultat est décidée par le scan :
// deux chaînes Latin-1 donnent du Latin-1, toute présence d'UTF-16 propage la
// forme large.
//
// Cette concaténation est LINÉAIRE en la longueur du résultat. Construire une
// chaîne par concaténations successives est donc quadratique — c'est
// exactement ce que le banc T2.3 mesure, et son verdict décidera si la forme
// Rope doit être implémentée.
func (s *String) Concat(t *String) *String {
	if s.Len() == 0 {
		return t
	}
	if t.Len() == 0 {
		return s
	}
	n := s.length + t.length

	// Au-delà du seuil, la concaténation devient une corde : le contenu n'est
	// pas recopié, seul un nœud est créé. C'est ce qui ramène la construction
	// itérative d'une chaîne du quadratique au linéaire — le banc T2.3 a mesuré
	// un rapport de 14 069 contre le témoin linéaire avant cette forme.
	if n >= ropeThreshold {
		return &String{
			kind:   Rope,
			length: n,
			narrow: !forceUTF16 && s.isNarrow() && t.isNarrow(),
			left:   s,
			right:  t,
		}
	}

	if !forceUTF16 && s.kind == Latin1 && t.kind == Latin1 {
		b := make([]byte, 0, n)
		b = append(b, s.latin1...)
		b = append(b, t.latin1...)
		return &String{kind: Latin1, latin1: b, length: n}
	}
	u := make([]uint16, 0, n)
	u = append(u, s.UTF16()...)
	u = append(u, t.UTF16()...)
	return &String{kind: UTF16, utf16: u, length: n}
}

// isNarrow indique que la chaîne tient en Latin-1, sans l'aplatir si c'est une
// corde : le drapeau est propagé à la construction du nœud.
func (s *String) isNarrow() bool {
	switch s.kind {
	case Latin1:
		return true
	case UTF16:
		return false
	case Rope:
		return s.narrow
	default:
		unsupported(s.kind, "isNarrow")
		return false
	}
}

// flatten matérialise une corde en forme plate, EN PLACE. Le parcours est
// ITÉRATIF : une corde construite par un million de concaténations successives
// a une profondeur d'un million, et un parcours récursif ferait déborder la
// pile — ce serait remplacer un défaut de performance par un défaut de
// robustesse.
//
// L'aplatissement est mémorisé : le contenu d'une chaîne étant immuable, il
// n'est jamais refait.
func (s *String) flatten() {
	if s.kind != Rope {
		return
	}

	narrow := s.narrow
	total := s.length

	// Parcours itératif en profondeur, feuilles collectées de gauche à droite.
	stack := make([]*String, 0, 64)
	stack = append(stack, s)

	if narrow {
		out := make([]byte, 0, total)
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n.kind == Rope {
				stack = append(stack, n.right, n.left) // gauche dépilée en premier
				continue
			}
			out = append(out, n.latin1...)
		}
		s.kind, s.latin1, s.utf16 = Latin1, out, nil
	} else {
		out := make([]uint16, 0, total)
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n.kind == Rope {
				stack = append(stack, n.right, n.left)
				continue
			}
			out = append(out, n.UTF16()...)
		}
		s.kind, s.utf16, s.latin1 = UTF16, out, nil
	}
	s.left, s.right = nil, nil
}

// Slice rend la sous-chaîne [a, b). Les bornes sont supposées déjà normalisées
// par l'appelant, qui seul connaît la sémantique de la méthode invoquée.
func (s *String) Slice(a, b int) *String {
	s.flatten()
	n := s.Len()
	if a < 0 {
		a = 0
	}
	if b > n {
		b = n
	}
	if a >= b {
		return Empty
	}
	switch s.kind {
	case Latin1:
		if forceUTF16 {
			return FromLatin1(s.latin1[a:b])
		}
		cp := make([]byte, b-a)
		copy(cp, s.latin1[a:b])
		return &String{kind: Latin1, latin1: cp, length: b - a}
	case UTF16:
		// Une tranche d'une chaîne large peut redevenir étroite : « "é1".slice(1) »
		// tient dans un octet. Repasser par FromUTF16 garde la forme minimale et
		// rend Equal indépendant du chemin de construction.
		return FromUTF16(s.utf16[a:b])
	default:
		unsupported(s.kind, "Slice")
		return nil
	}
}

// IndexOf rend l'indice de la première occurrence de t à partir de from, ou -1.
func (s *String) IndexOf(t *String, from int) int {
	s.flatten()
	t.flatten()
	n, m := s.Len(), t.Len()
	if from < 0 {
		from = 0
	}
	if m == 0 {
		if from > n {
			return n
		}
		return from
	}
	if from+m > n {
		return -1
	}
	if s.kind == Latin1 && t.kind == Latin1 {
		i := indexBytes(s.latin1[from:], t.latin1)
		if i < 0 {
			return -1
		}
		return from + i
	}
	first := t.At(0)
	for i := from; i+m <= n; i++ {
		if s.At(i) != first {
			continue
		}
		match := true
		for j := 1; j < m; j++ {
			if s.At(i+j) != t.At(j) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func indexBytes(h, n []byte) int {
	if len(n) > len(h) {
		return -1
	}
	for i := 0; i+len(n) <= len(h); i++ {
		if string(h[i:i+len(n)]) == string(n) {
			return i
		}
	}
	return -1
}

// LastIndexOf rend l'indice de la dernière occurrence de t avant ou à l'indice from, ou -1.
func (s *String) LastIndexOf(t *String, from int) int {
	s.flatten()
	t.flatten()
	n, m := s.Len(), t.Len()
	if from < 0 {
		from = 0
	}
	if from > n {
		from = n
	}
	if m == 0 {
		return from
	}
	if n < m {
		return -1
	}
	maxStart := from
	if maxStart+m > n {
		maxStart = n - m
	}
	if s.kind == Latin1 && t.kind == Latin1 {
		for i := maxStart; i >= 0; i-- {
			if string(s.latin1[i:i+m]) == string(t.latin1) {
				return i
			}
		}
		return -1
	}
	first := t.At(0)
	for i := maxStart; i >= 0; i-- {
		if s.At(i) != first {
			continue
		}
		match := true
		for j := 1; j < m; j++ {
			if s.At(i+j) != t.At(j) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// StartsWith indique si s commence par t à partir de l'indice pos.
func (s *String) StartsWith(t *String, pos int) bool {
	s.flatten()
	t.flatten()
	n, m := s.Len(), t.Len()
	if pos < 0 {
		pos = 0
	}
	if pos+m > n {
		return false
	}
	if s.kind == Latin1 && t.kind == Latin1 {
		return string(s.latin1[pos:pos+m]) == string(t.latin1)
	}
	for i := 0; i < m; i++ {
		if s.At(pos+i) != t.At(i) {
			return false
		}
	}
	return true
}

// EndsWith indique si s se termine par t à l'indice de fin endPos.
func (s *String) EndsWith(t *String, endPos int) bool {
	s.flatten()
	t.flatten()
	n, m := s.Len(), t.Len()
	if endPos < 0 {
		return false
	}
	if endPos > n {
		endPos = n
	}
	if endPos < m {
		return false
	}
	start := endPos - m
	if s.kind == Latin1 && t.kind == Latin1 {
		return string(s.latin1[start:start+m]) == string(t.latin1)
	}
	for i := 0; i < m; i++ {
		if s.At(start+i) != t.At(i) {
			return false
		}
	}
	return true
}

// Includes indique si s contient t à partir de l'indice from.
func (s *String) Includes(t *String, from int) bool {
	return s.IndexOf(t, from) != -1
}

// Repeat répète la chaîne count fois.
func (s *String) Repeat(count int) *String {
	if count <= 0 || s.Len() == 0 {
		return Empty
	}
	if count == 1 {
		return s
	}
	s.flatten()
	n := s.Len() * count
	switch s.kind {
	case Latin1:
		if forceUTF16 {
			u := make([]uint16, 0, n)
			unit := s.UTF16()
			for i := 0; i < count; i++ {
				u = append(u, unit...)
			}
			return &String{kind: UTF16, utf16: u, length: n}
		}
		b := make([]byte, 0, n)
		for i := 0; i < count; i++ {
			b = append(b, s.latin1...)
		}
		return &String{kind: Latin1, latin1: b, length: n}
	case UTF16:
		u := make([]uint16, 0, n)
		for i := 0; i < count; i++ {
			u = append(u, s.utf16...)
		}
		return &String{kind: UTF16, utf16: u, length: n}
	default:
		unsupported(s.kind, "Repeat")
		return nil
	}
}

// ToLowerASCII met en minuscules les seules lettres ASCII.
func (s *String) ToLowerASCII() *String {
	s.flatten()
	if !s.IsASCII() {
		return s
	}
	switch s.kind {
	case Latin1:
		b := make([]byte, len(s.latin1))
		for i, c := range s.latin1 {
			if c >= 'A' && c <= 'Z' {
				b[i] = c + ('a' - 'A')
			} else {
				b[i] = c
			}
		}
		return &String{kind: Latin1, latin1: b, length: len(b)}
	case UTF16:
		u := make([]uint16, len(s.utf16))
		for i, c := range s.utf16 {
			if c >= 'A' && c <= 'Z' {
				u[i] = c + ('a' - 'A')
			} else {
				u[i] = c
			}
		}
		return &String{kind: UTF16, utf16: u, length: len(u)}
	default:
		unsupported(s.kind, "ToLowerASCII")
		return nil
	}
}

func isECMAWhitespace(u uint16) bool {
	switch u {
	case 0x0009, 0x000A, 0x000B, 0x000C, 0x000D, 0x0020, 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	if u >= 0x2000 && u <= 0x200A {
		return true
	}
	return false
}

// Trim retire les espaces blancs et sauts de ligne ECMAScript aux deux extrémités.
func (s *String) Trim() *String {
	s.flatten()
	n := s.Len()
	start := 0
	for start < n && isECMAWhitespace(s.At(start)) {
		start++
	}
	end := n
	for end > start && isECMAWhitespace(s.At(end-1)) {
		end--
	}
	return s.Slice(start, end)
}

// TrimStart retire les espaces blancs et sauts de ligne ECMAScript au début.
func (s *String) TrimStart() *String {
	s.flatten()
	n := s.Len()
	start := 0
	for start < n && isECMAWhitespace(s.At(start)) {
		start++
	}
	return s.Slice(start, n)
}

// TrimEnd retire les espaces blancs et sauts de ligne ECMAScript à la fin.
func (s *String) TrimEnd() *String {
	s.flatten()
	n := s.Len()
	end := n
	for end > 0 && isECMAWhitespace(s.At(end-1)) {
		end--
	}
	return s.Slice(0, end)
}

// IsASCII indique que toutes les unités de code sont inférieures à 0x80. C'est
// la garde du chemin rapide de conversion de casse : au-delà, les mappings
// Latin-1 (é vers É) ne se réduisent pas à un décalage de bit.
func (s *String) IsASCII() bool {
	s.flatten()
	switch s.kind {
	case Latin1:
		for _, c := range s.latin1 {
			if c >= 0x80 {
				return false
			}
		}
		return true
	case UTF16:
		for _, c := range s.utf16 {
			if c >= 0x80 {
				return false
			}
		}
		return true
	default:
		unsupported(s.kind, "IsASCII")
		return false
	}
}

// ToUpperASCII met en majuscules les seules lettres ASCII. Le noyau est la
// feuille émise c2strclass. La fonction n'est PAS toUpperCase : elle ne
// s'applique qu'aux chaînes purement ASCII, ce que l'appelant doit vérifier par
// IsASCII. Sur une chaîne non ASCII, elle rend la chaîne inchangée plutôt que
// de produire un résultat faux.
func (s *String) ToUpperASCII() *String {
	s.flatten()
	if !s.IsASCII() {
		return s
	}
	switch s.kind {
	case Latin1:
		b := make([]byte, len(s.latin1))
		copy(b, s.latin1)
		c2strclass.C2strclass_ascii_upper(b, uint64(len(b)))
		return &String{kind: Latin1, latin1: b, length: len(b)}
	case UTF16:
		b := make([]byte, len(s.utf16))
		for i, c := range s.utf16 {
			b[i] = byte(c)
		}
		c2strclass.C2strclass_ascii_upper(b, uint64(len(b)))
		u := make([]uint16, len(b))
		for i, c := range b {
			u[i] = uint16(c)
		}
		return &String{kind: UTF16, utf16: u, length: len(u)}
	default:
		unsupported(s.kind, "ToUpperASCII")
		return nil
	}
}

// ─── Empreinte ──────────────────────────────────────────────────────────────

// Hash rend une empreinte des unités de code, mémorisée après le premier calcul.
// Elle ne dépend PAS de la forme de stockage : deux chaînes égales ont la même
// empreinte, quelle que soit leur représentation. C'est le prérequis du modèle
// à formes du jalon 3, où une forme compare ses clés par identité de pointeur
// après internement.
func (s *String) Hash() uint32 {
	if state := atomic.LoadUint64(&s.hashState); state&hashComputedBit != 0 {
		return uint32(state)
	}
	s.flatten()
	if state := atomic.LoadUint64(&s.hashState); state&hashComputedBit != 0 {
		return uint32(state)
	}
	// FNV-1a sur les unités de code, en petit-boutiste, pour que le résultat
	// soit indépendant de la forme.
	const (
		offset = 2166136261
		prime  = 16777619
	)
	h := uint32(offset)
	switch s.kind {
	case Latin1:
		for _, c := range s.latin1 {
			h = (h ^ uint32(c)) * prime
			h = (h ^ 0) * prime
		}
	case UTF16:
		for _, c := range s.utf16 {
			h = (h ^ uint32(c&0xFF)) * prime
			h = (h ^ uint32(c>>8)) * prime
		}
	default:
		unsupported(s.kind, "Hash")
	}
	atomic.StoreUint64(&s.hashState, hashComputedBit|uint64(h))
	return h
}
