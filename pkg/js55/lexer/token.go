// SPDX-License-Identifier: BUSL-1.1
// Package lexer découpe une source ECMAScript en lexèmes.
//
// Contraintes du plan (/devhoros/pkg/js55/PLAN_MOTEUR.md §1) appliquées ici :
//   - C1 : aucun interface{} sur le chemin de lexage ; Kind est un octet.
//   - C2 : l'énumération Kind est DENSE et contiguë, pour qu'un commutateur sur
//     elle reçoive une table de saut (minCases=8, minDensity=4).
//
// Invariant dur (plan T1.3) : aucune entrée, valide ou non, ne fait paniquer le
// lexer, ne provoque de dépassement de pile ni de boucle infinie. Une source
// invalide produit un lexème Illegal porteur d'un message, jamais une panique.
package lexer

// Kind identifie la nature d'un lexème. L'énumération est dense et contiguë :
// aucune valeur n'est sautée, aucune n'est assignée explicitement.
type Kind uint8

const (
	EOF Kind = iota
	Illegal

	// Littéraux et identifiants
	Ident
	PrivateIdent // #x
	Keyword
	Number
	BigInt
	String
	TemplateHead   // `...${
	TemplateMiddle // }...${
	TemplateTail   // }...`
	NoSubTemplate  // `...`
	Regexp

	// Ponctuateurs — un par lexème, ordre sans signification sémantique
	LBrace
	RBrace
	LParen
	RParen
	LBracket
	RBracket
	Dot
	Ellipsis
	Semicolon
	Comma
	Colon
	Question
	QuestionDot
	QuestionQuestion
	QuestionQuestionAssign
	Arrow

	Lt
	Gt
	Le
	Ge
	EqEq
	NotEq
	EqEqEq
	NotEqEq

	Plus
	Minus
	Star
	Slash
	Percent
	StarStar
	PlusPlus
	MinusMinus

	Shl
	Shr
	UShr

	And
	Or
	Xor
	Not
	Tilde
	AndAnd
	OrOr

	Assign
	PlusAssign
	MinusAssign
	StarAssign
	SlashAssign
	PercentAssign
	StarStarAssign
	ShlAssign
	ShrAssign
	UShrAssign
	AndAssign
	OrAssign
	XorAssign
	AndAndAssign
	OrOrAssign

	// Sentinelle : borne haute exclusive de l'énumération. Toute valeur de Kind
	// vérifie k < numKinds ; la garde de densité s'appuie dessus.
	numKinds
)

// NumKinds expose la borne haute de l'énumération pour les gardes.
const NumKinds = int(numKinds)

var kindNames = [numKinds]string{
	EOF:                    "EOF",
	Illegal:                "Illegal",
	Ident:                  "Ident",
	PrivateIdent:           "PrivateIdent",
	Keyword:                "Keyword",
	Number:                 "Number",
	BigInt:                 "BigInt",
	String:                 "String",
	TemplateHead:           "TemplateHead",
	TemplateMiddle:         "TemplateMiddle",
	TemplateTail:           "TemplateTail",
	NoSubTemplate:          "NoSubTemplate",
	Regexp:                 "Regexp",
	LBrace:                 "{",
	RBrace:                 "}",
	LParen:                 "(",
	RParen:                 ")",
	LBracket:               "[",
	RBracket:               "]",
	Dot:                    ".",
	Ellipsis:               "...",
	Semicolon:              ";",
	Comma:                  ",",
	Colon:                  ":",
	Question:               "?",
	QuestionDot:            "?.",
	QuestionQuestion:       "??",
	QuestionQuestionAssign: "??=",
	Arrow:                  "=>",
	Lt:                     "<",
	Gt:                     ">",
	Le:                     "<=",
	Ge:                     ">=",
	EqEq:                   "==",
	NotEq:                  "!=",
	EqEqEq:                 "===",
	NotEqEq:                "!==",
	Plus:                   "+",
	Minus:                  "-",
	Star:                   "*",
	Slash:                  "/",
	Percent:                "%",
	StarStar:               "**",
	PlusPlus:               "++",
	MinusMinus:             "--",
	Shl:                    "<<",
	Shr:                    ">>",
	UShr:                   ">>>",
	And:                    "&",
	Or:                     "|",
	Xor:                    "^",
	Not:                    "!",
	Tilde:                  "~",
	AndAnd:                 "&&",
	OrOr:                   "||",
	Assign:                 "=",
	PlusAssign:             "+=",
	MinusAssign:            "-=",
	StarAssign:             "*=",
	SlashAssign:            "/=",
	PercentAssign:          "%=",
	StarStarAssign:         "**=",
	ShlAssign:              "<<=",
	ShrAssign:              ">>=",
	UShrAssign:             ">>>=",
	AndAssign:              "&=",
	OrAssign:               "|=",
	XorAssign:              "^=",
	AndAndAssign:           "&&=",
	OrOrAssign:             "||=",
}

func (k Kind) String() string {
	if int(k) >= NumKinds {
		return "Kind(?)"
	}
	return kindNames[k]
}

// Position repère un lexème dans la source. Line et Col comptent à partir de 1,
// Col en unités de code UTF-16 pour coïncider avec ce que le langage observe
// (plan §J2.1).
type Position struct {
	Offset int // décalage en octets dans la source
	Line   int
	Col    int
}

// Token est un lexème. La structure reste petite et sans pointeur autre que la
// tranche de valeur, qui pointe dans la source d'origine sans la recopier.
type Token struct {
	Kind Kind
	Pos  Position
	// Value porte le texte brut du lexème, non déséchappé. Le déséchappement
	// des chaînes et des identifiants est un travail du parser, qui seul sait
	// si l'erreur doit être précoce ou tardive.
	Value string
	// NewlineBefore indique qu'un terminateur de ligne sépare ce lexème du
	// précédent. C'est l'unique information dont l'insertion automatique de
	// points-virgules a besoin.
	NewlineBefore bool
	// Message renseigne le motif lorsque Kind vaut Illegal ; vide sinon.
	Message string
}

// keywords recense les mots réservés d'ECMAScript 2020, réservés stricts inclus.
// Le lexer les classe en Keyword ; c'est au parser de décider lesquels sont
// contextuels dans la position où ils apparaissent.
var keywords = map[string]bool{
	"await": true, "break": true, "case": true, "catch": true, "class": true,
	"const": true, "continue": true, "debugger": true, "default": true,
	"delete": true, "do": true, "else": true, "enum": true, "export": true,
	"extends": true, "false": true, "finally": true, "for": true,
	"function": true, "if": true, "import": true, "in": true, "instanceof": true,
	"new": true, "null": true, "return": true, "super": true, "switch": true,
	"this": true, "throw": true, "true": true, "try": true, "typeof": true,
	"var": true, "void": true, "while": true, "with": true, "yield": true,
	// Réservés en mode strict uniquement ; le parser tranche.
	"let": true, "static": true, "implements": true, "interface": true,
	"package": true, "private": true, "protected": true, "public": true,
}

// IsKeyword indique si s est un mot réservé d'ECMAScript 2020.
func IsKeyword(s string) bool { return keywords[s] }
