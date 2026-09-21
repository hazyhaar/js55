// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"fmt"
	"math"
	"strconv"
)

// Value represents an ECMAScript value encoded via 64-bit NaN-tagging.
// Under NaN-tagging:
// - Standard IEEE-754 double floats have normal bit patterns.
// - Quiet NaNs (exponent all 1s, highest mantissa bit 1) are tagged to carry:
//   - Undefined
//   - Null
//   - Boolean (true / false)
//   - Int32
//   - Object references, sous forme de HANDLE 48 bits — un indice dans le tas
//     du moteur, jamais un pointeur brut.
//
// Le choix du handle plutôt que du pointeur n'est pas cosmétique. Un pointeur Go
// rangé dans un uint64 est INVISIBLE du ramasse-miettes de Go : l'objet pointé
// peut être collecté alors qu'il est encore référencé depuis une Value. Le tas
// du moteur étant une tranche Go ordinaire, ranger un indice garde tout objet
// visible du ramasse-miettes de la plateforme, et rend le marquage-balayage du
// moteur précis par construction.
type Value uint64

const (
	tagNaN       uint64 = 0x7FF8000000000000
	tagUndefined uint64 = tagNaN | 0x0001000000000000
	tagNull      uint64 = tagNaN | 0x0002000000000000
	tagBoolean   uint64 = tagNaN | 0x0003000000000000
	tagInt32     uint64 = tagNaN | 0x0004000000000000
	tagPointer   uint64 = tagNaN | 0x8000000000000000
	ptrMask      uint64 = 0x0000FFFFFFFFFFFF
)

var (
	Undefined = Value(tagUndefined)
	Null      = Value(tagNull)
	True      = Value(tagBoolean | 1)
	False     = Value(tagBoolean | 0)
)

func Number(f float64) Value {
	if math.IsNaN(f) {
		return Value(tagNaN)
	}
	return Value(math.Float64bits(f))
}

func Int(i int32) Value {
	return Value(tagInt32 | uint64(uint32(i)))
}

func Bool(b bool) Value {
	if b {
		return True
	}
	return False
}

// Handle désigne un objet dans le tas du moteur. Il porte DEUX champs dans les
// 48 bits utiles du mot NaN-tagué :
//
//	bits  0..31  indice dans la tranche du tas
//	bits 32..47  génération de l'emplacement
//
// La génération n'est pas un ornement. Sans elle, un emplacement libéré puis
// réattribué ferait qu'une Value périmée désignerait silencieusement un AUTRE
// objet — un défaut d'usage après libération dont le symptôme serait un
// résultat faux plutôt qu'une panne. La génération transforme ce cas en handle
// invalide, donc en échec situé.
type Handle uint64

// NoHandle est le handle vide.
const NoHandle Handle = 0

const handleIndexBits = 32

// makeHandle assemble un handle depuis un indice et une génération.
func makeHandle(index uint32, gen uint16) Handle {
	return Handle(uint64(index) | uint64(gen)<<handleIndexBits)
}

// Index rend l'indice porté par le handle.
func (h Handle) Index() uint32 { return uint32(h) }

// Gen rend la génération portée par le handle.
func (h Handle) Gen() uint16 { return uint16(h >> handleIndexBits) }

// ObjectValue encode une référence d'objet. Un handle nul rend Null.
func ObjectValue(h Handle) Value {
	if h == NoHandle {
		return Null
	}
	return Value(tagPointer | (uint64(h) & ptrMask))
}

// Handle rend le handle porté par une référence d'objet, ou NoHandle.
func (v Value) Handle() Handle {
	if !v.IsPointer() {
		return NoHandle
	}
	return Handle(uint64(v) & ptrMask)
}

// IsObject est le nom métier de IsPointer.
func (v Value) IsObject() bool { return v.IsPointer() }

func (v Value) IsUndefined() bool { return v == Undefined }
func (v Value) IsNull() bool      { return v == Null }
func (v Value) IsBool() bool      { return (uint64(v) & 0xFFFF000000000000) == tagBoolean }
func (v Value) IsInt() bool       { return (uint64(v) & 0xFFFF000000000000) == tagInt32 }
func (v Value) IsPointer() bool   { return (uint64(v) & 0xFFFF000000000000) == tagPointer }
func (v Value) IsNumber() bool {
	u := uint64(v)
	if v.IsInt() {
		return true
	}
	return (u&tagNaN) != tagNaN || u == tagNaN
}

func (v Value) ToInt() int32 {
	if v.IsInt() {
		return int32(uint32(v))
	}
	if v.IsNumber() {
		return int32(v.ToFloat())
	}
	return 0
}

func (v Value) ToFloat() float64 {
	if v.IsInt() {
		return float64(v.ToInt())
	}
	if (uint64(v)&tagNaN) == tagNaN && uint64(v) != tagNaN {
		return math.NaN()
	}
	return math.Float64frombits(uint64(v))
}

func (v Value) ToBool() bool {
	if v.IsBool() {
		return (uint64(v) & 1) == 1
	}
	if v.IsInt() {
		return v.ToInt() != 0
	}
	if v.IsNumber() {
		f := v.ToFloat()
		return f != 0 && !math.IsNaN(f)
	}
	if v.IsUndefined() || v.IsNull() {
		return false
	}
	return true
}

func (v Value) String() string {
	if v.IsUndefined() {
		return "undefined"
	}
	if v.IsNull() {
		return "null"
	}
	if v.IsBool() {
		if v.ToBool() {
			return "true"
		}
		return "false"
	}
	if v.IsInt() {
		return strconv.Itoa(int(v.ToInt()))
	}
	if v.IsNumber() {
		return fmt.Sprintf("%g", v.ToFloat())
	}
	return fmt.Sprintf("[Object #%d]", v.Handle())
}
