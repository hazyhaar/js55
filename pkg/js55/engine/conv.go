// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"math"
	"strconv"
	"strings"

	"github.com/hazyhaar/js55/pkg/c2d2s"
	"github.com/hazyhaar/js55/pkg/js55/str"
)

// Conversions numériques et textuelles.
// Number::toString délègue à c2d2s (noyau C transpilé, oracle Node).

func numberToString(f float64) string {
	return c2d2s.Format(f)
}

// parseNumber applique ToNumber sur une chaîne. Une chaîne vide ou faite de
// blancs vaut zéro ; une chaîne non numérique vaut NaN.
func parseNumber(s string) float64 {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0
	}
	switch t {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if strings.HasPrefix(t, "0x") || strings.HasPrefix(t, "0X") {
		if v, err := strconv.ParseUint(t[2:], 16, 64); err == nil {
			return float64(v)
		}
		return math.NaN()
	}
	if strings.HasPrefix(t, "0b") || strings.HasPrefix(t, "0B") {
		if v, err := strconv.ParseUint(t[2:], 2, 64); err == nil {
			return float64(v)
		}
		return math.NaN()
	}
	if strings.HasPrefix(t, "0o") || strings.HasPrefix(t, "0O") {
		if v, err := strconv.ParseUint(t[2:], 8, 64); err == nil {
			return float64(v)
		}
		return math.NaN()
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func parseBigInt64(raw string) (int64, error) {
	s := strings.TrimSuffix(raw, "n")
	s = strings.ReplaceAll(s, "_", "")
	base := 10
	switch {
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		base = 16
		s = s[2:]
	case strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B"):
		base = 2
		s = s[2:]
	case strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O"):
		base = 8
		s = s[2:]
	}
	return strconv.ParseInt(s, base, 64)
}

// toDisplayString applique ToString. Le résultat sert autant à la concaténation
// qu'aux messages d'erreur.
func (vm *VM) toDisplayString(v Value) string {
	if s := vm.StringOf(v); s != nil {
		return s.GoString()
	}
	switch {
	case v.IsUndefined():
		return "undefined"
	case v.IsNull():
		return "null"
	case v.IsBool():
		if v.ToBool() {
			return "true"
		}
		return "false"
	case v.IsObject():
		o := vm.heap.Get(v.Handle())
		if o == nil {
			return "[object Released]"
		}
		switch o.kind {
		case KindBigInt:
			return strconv.FormatInt(o.bigInt, 10)
		case KindSymbol:
			desc := ""
			if o.text != nil {
				desc = o.text.GoString()
			}
			if desc == "" {
				return "Symbol()"
			}
			return "Symbol(" + desc + ")"
		case KindFunction:
			name := "anonymous"
			if o.fn != nil && o.fn.Name != "" {
				name = o.fn.Name
			}
			return "function " + name + "() { [bytecode] }"
		case KindGenerator:
			return "[object Generator]"
		case KindArray:
			parts := make([]string, 0, len(o.elements))
			for _, e := range o.elements {
				if e.IsUndefined() || e.IsNull() {
					parts = append(parts, "")
					continue
				}
				parts = append(parts, vm.toDisplayString(e))
			}
			return strings.Join(parts, ",")
		default:
			return "[object Object]"
		}
	default:
		return numberToString(v.ToFloat())
	}
}

// ToStringValue rend la forme textuelle d'une valeur, sous forme de chaîne du
// moteur.
func (vm *VM) ToStringValue(v Value) *str.String {
	if s := vm.StringOf(v); s != nil {
		return s
	}
	return str.FromGo(vm.toDisplayString(v))
}

// ToNumber convertit une valeur JavaScript en flottant selon les règles de coercion ECMAScript.
func (vm *VM) ToNumber(v Value) float64 {
	switch {
	case v.IsUndefined():
		return math.NaN()
	case v.IsNull():
		return 0
	case v.IsBool():
		if v.ToBool() {
			return 1
		}
		return 0
	case v.IsNumber():
		return v.ToFloat()
	default:
		if n, ok := vm.bigIntOf(v); ok {
			return float64(n)
		}
		if s := vm.StringOf(v); s != nil {
			strVal := strings.TrimSpace(s.GoString())
			if strVal == "" {
				return 0
			}
			if strVal == "Infinity" || strVal == "+Infinity" {
				return math.Inf(1)
			}
			if strVal == "-Infinity" {
				return math.Inf(-1)
			}
			f, err := strconv.ParseFloat(strVal, 64)
			if err != nil {
				return math.NaN()
			}
			return f
		}
		return v.ToFloat()
	}
}
