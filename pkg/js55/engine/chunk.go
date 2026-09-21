package engine

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// ConstKind identifie le type d'une constante. Dense et contiguë.
type ConstKind uint8

const (
	ConstNumber ConstKind = iota
	ConstString
	ConstFunction
	ConstBigInt
	numConstKinds
)

// NumConstKinds borne l'énumération.
const NumConstKinds = int(numConstKinds)

func (k ConstKind) String() string {
	switch k {
	case ConstNumber:
		return "number"
	case ConstString:
		return "string"
	case ConstFunction:
		return "function"
	case ConstBigInt:
		return "bigint"
	}
	return "const(?)"
}

// Const est une entrée de la table de constantes.
type Const struct {
	Kind ConstKind
	Num  float64
	Text *str.String
	Fn   *Chunk
	Big  int64
}

// ExceptionHandler définit une plage d'instructions protégée par try/catch/finally.
type ExceptionHandler struct {
	Start      int // Décalage IP de début (inclus)
	End        int // Décalage IP de fin (exclus)
	CatchIP    int // Décalage IP du gestionnaire catch (-1 si absent)
	FinallyIP  int // Décalage IP du bloc finally (-1 si absent)
	FinallyEnd int
	CatchSlot  int // Emplacement de variable locale pour l'erreur (-1 si anonyme)
	Depth      int // Profondeur d'environnement
}

// Chunk est une unité de code compilée : une fonction, ou le corps d'un script.
type Chunk struct {
	Name     string
	Code     []byte
	Consts   []Const
	Handlers []ExceptionHandler
	// Locals est le nombre d'emplacements locaux, paramètres compris.
	Locals int
	// Params est le nombre de paramètres déclarés.
	Params int
	// SelfSlot est l'emplacement où l'appel doit ranger la fonction elle-même,
	// ou -1.
	SelfSlot int
	// ArgumentsSlot est l'emplacement de l'objet arguments, ou -1 (fléchées).
	ArgumentsSlot int
	RestSlot      int
	// Lines associe à chaque décalage d'instruction la ligne source.
	Lines map[int]int

	// Native est une fonction Go exécutée directement lors de l'appel sans passer par le bytecode.
	Native    func(vm *VM, args []Value) (Value, error)
	Construct bool
	// archtimeTag est un tag interne pour marquer les fonctions reconnues.
	archtimeTag  archtimeGeometryTag
	archtimeHash string
	// Generator : l'appel crée un objet générateur, le corps s'exécute à next().
	Generator bool
	Async     bool
	Strict    bool
	Arrow     bool
	// ExportDefault indique si l'unité exporte une valeur par défaut.
	ExportDefault bool
	// ExportNames recense les noms de symboles exportés par l'unité.
	ExportNames []string
	// SafeEnv is an opt-in compiler proof over the completed bytecode and
	// constants of an ordinary function. False always selects NewEnv. Manual
	// and deserialized chunks default to false; consumers must not forge this
	// flag or mutate a compiled unit while it is executing.
	SafeEnv         bool `json:"-"`
	safeEnvVerified bool // non-serializable compiler provenance; manual opt-in is insufficient
}

// HasExport vérifie si l'unité déclare un export pour le nom donné.
func (c *Chunk) HasExport(name string) bool {
	for _, n := range c.ExportNames {
		if n == name {
			return true
		}
	}
	return false
}

// NewChunk crée une unité vide.
func NewChunk(name string) *Chunk {
	return &Chunk{Name: name, Lines: map[int]int{}, SelfSlot: -1, ArgumentsSlot: -1, RestSlot: -1, SafeEnv: false}
}

// emit écrit un opcode sans opérande et rend son décalage.
func (c *Chunk) emit(op Op, line int) int {
	at := len(c.Code)
	c.Lines[at] = line
	c.Code = append(c.Code, byte(op))
	return at
}

// emit16 écrit un opcode et son opérande sur deux octets.
func (c *Chunk) emit16(op Op, arg uint16, line int) int {
	at := c.emit(op, line)
	c.Code = binary.LittleEndian.AppendUint16(c.Code, arg)
	return at
}

// emit32 écrit un opcode et son opérande sur quatre octets.
func (c *Chunk) emit32(op Op, arg uint32, line int) int {
	at := c.emit(op, line)
	c.Code = binary.LittleEndian.AppendUint32(c.Code, arg)
	return at
}

// patch32 réécrit l'opérande 32 bits de l'instruction commençant à at. Sert aux
// sauts en avant, dont la cible n'est connue qu'après coup.
func (c *Chunk) patch32(at int, arg uint32) {
	binary.LittleEndian.PutUint32(c.Code[at+1:at+5], arg)
}

// addConst ajoute une constante et rend son indice. Les constantes identiques
// sont fusionnées : deux occurrences d'une même chaîne partagent une entrée, ce
// qui rend la table de constantes comparable d'une compilation à l'autre.
func (c *Chunk) addConst(k Const) uint32 {
	for i, e := range c.Consts {
		if e.Kind != k.Kind {
			continue
		}
		switch k.Kind {
		case ConstNumber:
			// La comparaison passe par les bits : NaN ne s'égale pas à lui-même,
			// et -0 s'égale à +0 alors que ce sont deux constantes distinctes.
			if sameFloatBits(e.Num, k.Num) {
				return uint32(i)
			}
		case ConstString:
			if e.Text.Equal(k.Text) {
				return uint32(i)
			}
		case ConstFunction:
			if e.Fn == k.Fn {
				return uint32(i)
			}
		case ConstBigInt:
			if e.Big == k.Big {
				return uint32(i)
			}
		}
	}
	c.Consts = append(c.Consts, k)
	return uint32(len(c.Consts) - 1)
}

// Disassemble rend une représentation textuelle stable de l'unité. C'est le
// support des goldens de l'instrument T4.3 : un changement de génération de code
// se lit dans le diff, il ne se découvre pas en production.
func (c *Chunk) Disassemble() string {
	var b strings.Builder
	fmt.Fprintf(&b, "== %s == %d locaux, %d paramètres, %d constantes, self=%d\n",
		c.Name, c.Locals, c.Params, len(c.Consts), c.SelfSlot)

	for i, k := range c.Consts {
		switch k.Kind {
		case ConstNumber:
			fmt.Fprintf(&b, "  k%-3d number   %v\n", i, k.Num)
		case ConstString:
			fmt.Fprintf(&b, "  k%-3d string   %q\n", i, k.Text.GoString())
		case ConstFunction:
			fmt.Fprintf(&b, "  k%-3d function %s\n", i, k.Fn.Name)
		case ConstBigInt:
			fmt.Fprintf(&b, "  k%-3d bigint   %d\n", i, k.Big)
		}
	}

	for ip := 0; ip < len(c.Code); {
		op := Op(c.Code[ip])
		fmt.Fprintf(&b, "  %04d %-12s", ip, op.Name())
		switch op.Width() {
		case 2:
			if ip+3 > len(c.Code) {
				b.WriteString(" <tronqué>\n")
				return b.String()
			}
			fmt.Fprintf(&b, " %d", binary.LittleEndian.Uint16(c.Code[ip+1:ip+3]))
			ip += 3
		case 4:
			if ip+5 > len(c.Code) {
				b.WriteString(" <tronqué>\n")
				return b.String()
			}
			v := binary.LittleEndian.Uint32(c.Code[ip+1 : ip+5])
			switch op {
			case OpJump, OpJumpIfFalse, OpJumpIfTrue, OpInt32:
				fmt.Fprintf(&b, " %d", int32(v))
			default:
				fmt.Fprintf(&b, " %d", v)
			}
			ip += 5
		default:
			ip++
		}
		b.WriteByte('\n')
	}

	// Les unités imbriquées suivent, ce qui rend le golden récursif et complet.
	for _, k := range c.Consts {
		if k.Kind == ConstFunction {
			b.WriteByte('\n')
			b.WriteString(k.Fn.Disassemble())
		}
	}
	return b.String()
}

func sameFloatBits(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b)
}
