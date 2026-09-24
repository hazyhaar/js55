// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// InstallStringNumberBuiltins registers String and Number prototypes and constructors.
func (vm *VM) InstallStringNumberBuiltins() {
	numProto := vm.heap.NewObject()
	numProtoV := ObjectValue(numProto)
	vm.heap.AddRoot(&numProtoV)
	defer vm.heap.RemoveRoot(&numProtoV)
	vm.defineNative(numProto, "valueOf", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.prim.IsNumber() {
				return o.prim, nil
			}
		}
		return Number(vm.ToNumber(this)), nil
	})
	vm.defineNative(numProto, "toString", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		n := vm.ToNumber(this)
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.prim.IsNumber() {
				n = o.prim.ToFloat()
			}
		}
		return vm.NewStringValue(str.FromGo(numberToString(n))), nil
	})
	vm.defineNative(numProto, "toFixed", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		n := vm.ToNumber(this)
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.prim.IsNumber() {
				n = o.prim.ToFloat()
			}
		}
		digits := 0
		if len(args) > 0 && !args[0].IsUndefined() {
			d := vm.ToNumber(args[0])
			if math.IsNaN(d) {
				digits = 0
			} else {
				digits = int(d)
			}
		}
		if digits < 0 || digits > 100 {
			return Undefined, fmt.Errorf("RangeError: toFixed() digits out of range")
		}
		if math.IsNaN(n) {
			return vm.NewStringValue(str.FromGo("NaN")), nil
		}
		if math.IsInf(n, 1) {
			return vm.NewStringValue(str.FromGo("Infinity")), nil
		}
		if math.IsInf(n, -1) {
			return vm.NewStringValue(str.FromGo("-Infinity")), nil
		}
		return vm.NewStringValue(str.FromGo(strconv.FormatFloat(n, 'f', digits, 64))), nil
	})
	vm.defineNative(numProto, "toPrecision", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		n := vm.ToNumber(this)
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.prim.IsNumber() {
				n = o.prim.ToFloat()
			}
		}
		if len(args) == 0 || args[0].IsUndefined() {
			return vm.NewStringValue(str.FromGo(numberToString(n))), nil
		}
		prec := int(vm.ToNumber(args[0]))
		if prec < 1 || prec > 100 {
			return Undefined, fmt.Errorf("RangeError: toPrecision() precision out of range")
		}
		if math.IsNaN(n) {
			return vm.NewStringValue(str.FromGo("NaN")), nil
		}
		if math.IsInf(n, 1) {
			return vm.NewStringValue(str.FromGo("Infinity")), nil
		}
		if math.IsInf(n, -1) {
			return vm.NewStringValue(str.FromGo("-Infinity")), nil
		}
		return vm.NewStringValue(str.FromGo(strconv.FormatFloat(n, 'g', prec, 64))), nil
	})
	vm.defineNative(numProto, "toExponential", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		n := vm.ToNumber(this)
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.prim.IsNumber() {
				n = o.prim.ToFloat()
			}
		}
		digits := -1
		if len(args) > 0 && !args[0].IsUndefined() {
			d := vm.ToNumber(args[0])
			if math.IsNaN(d) {
				digits = 0
			} else {
				digits = int(d)
			}
			if digits < 0 || digits > 100 {
				return Undefined, fmt.Errorf("RangeError: toExponential() fractionDigits out of range")
			}
		}
		if math.IsNaN(n) {
			return vm.NewStringValue(str.FromGo("NaN")), nil
		}
		if math.IsInf(n, 1) {
			return vm.NewStringValue(str.FromGo("Infinity")), nil
		}
		if math.IsInf(n, -1) {
			return vm.NewStringValue(str.FromGo("-Infinity")), nil
		}
		return vm.NewStringValue(str.FromGo(strconv.FormatFloat(n, 'e', digits, 64))), nil
	})
	numObj := vm.heap.NewFunction(&Chunk{
		Name:      "Number",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			n := float64(0)
			if len(args) > 0 {
				n = vm.ToNumber(args[0])
			}
			this := vm.CurrentThis()
			if len(vm.newStack) > 0 && vm.newStack[len(vm.newStack)-1].IsObject() && this.IsObject() && this.Handle() != vm.globalObj {
				if o := vm.heap.Get(this.Handle()); o != nil {
					o.prim = Number(n)
					if vm.heap.objectProto != NoHandle {
						o.proto = numProto
					}
				}
				return this, nil
			}
			return Number(n), nil
		},
	}, NoHandle)
	hNum := ObjectValue(numObj)
	vm.heap.AddRoot(&hNum)
	defer vm.heap.RemoveRoot(&hNum)
	vm.heap.SetProperty(numObj, vm.heap.Intern().InternGo("prototype"), numProtoV)
	vm.SetGlobal("Number", hNum)

	setNumProp := func(name string, v Value) {
		k := vm.heap.Intern().InternGo(name)
		vm.heap.SetProperty(numObj, k, v)
	}

	setNumProp("MAX_VALUE", Number(math.MaxFloat64))
	setNumProp("MIN_VALUE", Number(math.SmallestNonzeroFloat64))
	setNumProp("NaN", Number(math.NaN()))
	setNumProp("NEGATIVE_INFINITY", Number(math.Inf(-1)))
	setNumProp("POSITIVE_INFINITY", Number(math.Inf(1)))
	setNumProp("MAX_SAFE_INTEGER", Number(9007199254740991))
	setNumProp("MIN_SAFE_INTEGER", Number(-9007199254740991))
	setNumProp("EPSILON", Number(2.220446049250313e-16))

	// Global parseInt & parseFloat, isNaN, isFinite
	parseIntChunk := &Chunk{
		Name:   "parseInt",
		Params: 2,
		Native: func(vm *VM, args []Value) (Value, error) {
			if len(args) == 0 {
				return Number(math.NaN()), nil
			}
			strVal := vm.toDisplayString(args[0])
			radix := 0
			if len(args) > 1 {
				radix = int(args[1].ToInt())
			}
			return Number(BuiltinParseInt(strVal, radix)), nil
		},
	}
	hParseInt := ObjectValue(vm.heap.NewFunction(parseIntChunk, NoHandle))
	vm.SetGlobal("parseInt", hParseInt)
	setNumProp("parseInt", hParseInt)

	parseFloatChunk := &Chunk{
		Name:   "parseFloat",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			if len(args) == 0 {
				return Number(math.NaN()), nil
			}
			strVal := vm.toDisplayString(args[0])
			return Number(BuiltinParseFloat(strVal)), nil
		},
	}
	hParseFloat := ObjectValue(vm.heap.NewFunction(parseFloatChunk, NoHandle))
	vm.SetGlobal("parseFloat", hParseFloat)
	setNumProp("parseFloat", hParseFloat)

	isNaNChunk := &Chunk{
		Name:   "isNaN",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			if len(args) == 0 {
				return True, nil
			}
			f := vm.ToNumber(args[0])
			return Bool(math.IsNaN(f)), nil
		},
	}
	hIsNaN := ObjectValue(vm.heap.NewFunction(isNaNChunk, NoHandle))
	vm.SetGlobal("isNaN", hIsNaN)
	setNumProp("isNaN", hIsNaN)

	isFiniteChunk := &Chunk{
		Name:   "isFinite",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			if len(args) == 0 {
				return False, nil
			}
			f := vm.ToNumber(args[0])
			return Bool(!math.IsNaN(f) && !math.IsInf(f, 0)), nil
		},
	}
	hIsFinite := ObjectValue(vm.heap.NewFunction(isFiniteChunk, NoHandle))
	vm.SetGlobal("isFinite", hIsFinite)
	setNumProp("isFinite", hIsFinite)

	strProto := vm.heap.NewObject()
	strProtoV := ObjectValue(strProto)
	vm.heap.AddRoot(&strProtoV)
	defer vm.heap.RemoveRoot(&strProtoV)

	thisToString := func(vm *VM, this Value) (*str.String, error) {
		if this.IsNull() || this.IsUndefined() {
			return nil, fmt.Errorf("TypeError: String prototype method called on null or undefined")
		}
		if s := vm.StringOf(this); s != nil {
			return s, nil
		}
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.kind == KindStringObject && o.text != nil {
				return o.text, nil
			}
		}
		return vm.ToStringValue(this), nil
	}

	toInteger := func(vm *VM, v Value, def int) int {
		if v.IsUndefined() {
			return def
		}
		n := vm.ToNumber(v)
		if math.IsNaN(n) {
			return 0
		}
		if n <= -math.MaxFloat64 {
			return -1 << 30
		}
		if n >= math.MaxFloat64 {
			return 1 << 30
		}
		return int(n)
	}

	stringPrimitive := func(vm *VM, this Value) Value {
		if s := vm.StringOf(this); s != nil {
			return this
		}
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.kind == KindStringObject && o.text != nil {
				return vm.NewStringValue(o.text)
			}
		}
		return vm.NewStringValue(str.FromGo(vm.toDisplayString(this)))
	}
	vm.defineNative(strProto, "toString", 0, func(vm *VM, args []Value) (Value, error) {
		return stringPrimitive(vm, vm.CurrentThis()), nil
	})
	vm.defineNative(strProto, "valueOf", 0, func(vm *VM, args []Value) (Value, error) {
		return stringPrimitive(vm, vm.CurrentThis()), nil
	})

	vm.defineNative(strProto, "charAt", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		pos := 0
		if len(args) > 0 {
			pos = toInteger(vm, args[0], 0)
		}
		if pos < 0 || pos >= s.Len() {
			return vm.NewStringValue(str.Empty), nil
		}
		return vm.NewStringValue(s.CharAt(pos)), nil
	})

	vm.defineNative(strProto, "charCodeAt", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		pos := 0
		if len(args) > 0 {
			pos = toInteger(vm, args[0], 0)
		}
		if pos < 0 || pos >= s.Len() {
			return Number(math.NaN()), nil
		}
		return Int(int32(s.At(pos))), nil
	})

	vm.defineNative(strProto, "codePointAt", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		pos := 0
		if len(args) > 0 {
			pos = toInteger(vm, args[0], 0)
		}
		if pos < 0 || pos >= s.Len() {
			return Undefined, nil
		}
		cp, _ := s.CodePointAt(pos)
		if cp < 0 {
			return Undefined, nil
		}
		return Int(int32(cp)), nil
	})

	vm.defineNative(strProto, "concat", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		res := s
		for _, a := range args {
			res = res.Concat(vm.ToStringValue(a))
		}
		return vm.NewStringValue(res), nil
	})

	vm.defineNative(strProto, "indexOf", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		search := str.FromGo("undefined")
		if len(args) > 0 {
			search = vm.ToStringValue(args[0])
		}
		pos := 0
		if len(args) > 1 {
			pos = toInteger(vm, args[1], 0)
		}
		return Int(int32(s.IndexOf(search, pos))), nil
	})

	vm.defineNative(strProto, "lastIndexOf", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		search := str.FromGo("undefined")
		if len(args) > 0 {
			search = vm.ToStringValue(args[0])
		}
		pos := s.Len()
		if len(args) > 1 && !args[1].IsUndefined() {
			n := vm.ToNumber(args[1])
			if math.IsNaN(n) {
				pos = s.Len()
			} else {
				pos = int(n)
			}
		}
		return Int(int32(s.LastIndexOf(search, pos))), nil
	})

	vm.defineNative(strProto, "includes", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		search := str.FromGo("undefined")
		if len(args) > 0 {
			search = vm.ToStringValue(args[0])
		}
		pos := 0
		if len(args) > 1 {
			pos = toInteger(vm, args[1], 0)
		}
		return Bool(s.Includes(search, pos)), nil
	})

	vm.defineNative(strProto, "startsWith", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		search := str.FromGo("undefined")
		if len(args) > 0 {
			search = vm.ToStringValue(args[0])
		}
		pos := 0
		if len(args) > 1 {
			pos = toInteger(vm, args[1], 0)
		}
		return Bool(s.StartsWith(search, pos)), nil
	})

	vm.defineNative(strProto, "endsWith", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		search := str.FromGo("undefined")
		if len(args) > 0 {
			search = vm.ToStringValue(args[0])
		}
		endPos := s.Len()
		if len(args) > 1 && !args[1].IsUndefined() {
			endPos = toInteger(vm, args[1], s.Len())
		}
		return Bool(s.EndsWith(search, endPos)), nil
	})

	vm.defineNative(strProto, "slice", 2, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		n := s.Len()
		intStart := 0
		if len(args) > 0 {
			intStart = toInteger(vm, args[0], 0)
		}
		from := 0
		if intStart < 0 {
			from = n + intStart
			if from < 0 {
				from = 0
			}
		} else {
			from = intStart
			if from > n {
				from = n
			}
		}
		intEnd := n
		if len(args) > 1 && !args[1].IsUndefined() {
			intEnd = toInteger(vm, args[1], n)
		}
		to := 0
		if intEnd < 0 {
			to = n + intEnd
			if to < 0 {
				to = 0
			}
		} else {
			to = intEnd
			if to > n {
				to = n
			}
		}
		if from >= to {
			return vm.NewStringValue(str.Empty), nil
		}
		return vm.NewStringValue(s.Slice(from, to)), nil
	})

	vm.defineNative(strProto, "substring", 2, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		n := s.Len()
		intStart := 0
		if len(args) > 0 {
			intStart = toInteger(vm, args[0], 0)
		}
		intEnd := n
		if len(args) > 1 && !args[1].IsUndefined() {
			intEnd = toInteger(vm, args[1], n)
		}
		if intStart < 0 {
			intStart = 0
		} else if intStart > n {
			intStart = n
		}
		if intEnd < 0 {
			intEnd = 0
		} else if intEnd > n {
			intEnd = n
		}
		from, to := intStart, intEnd
		if from > to {
			from, to = to, from
		}
		return vm.NewStringValue(s.Slice(from, to)), nil
	})

	vm.defineNative(strProto, "substr", 2, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		n := s.Len()
		intStart := 0
		if len(args) > 0 {
			intStart = toInteger(vm, args[0], 0)
		}
		if intStart < 0 {
			intStart = n + intStart
			if intStart < 0 {
				intStart = 0
			}
		} else if intStart > n {
			intStart = n
		}
		intLen := n - intStart
		if len(args) > 1 && !args[1].IsUndefined() {
			intLen = toInteger(vm, args[1], 0)
		}
		if intLen <= 0 {
			return vm.NewStringValue(str.Empty), nil
		}
		to := intStart + intLen
		if to > n {
			to = n
		}
		return vm.NewStringValue(s.Slice(intStart, to)), nil
	})

	vm.defineNative(strProto, "repeat", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		if len(args) == 0 {
			return vm.NewStringValue(str.Empty), nil
		}
		cnt := vm.ToNumber(args[0])
		if math.IsNaN(cnt) || cnt < 0 || math.IsInf(cnt, 1) {
			return Undefined, fmt.Errorf("RangeError: Invalid count value")
		}
		icnt := int(cnt)
		return vm.NewStringValue(s.Repeat(icnt)), nil
	})

	vm.defineNative(strProto, "padStart", 2, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		targetLen := 0
		if len(args) > 0 {
			targetLen = toInteger(vm, args[0], 0)
		}
		if targetLen <= s.Len() {
			return vm.NewStringValue(s), nil
		}
		pad := str.FromGo(" ")
		if len(args) > 1 && !args[1].IsUndefined() {
			pad = vm.ToStringValue(args[1])
		}
		if pad.Len() == 0 {
			return vm.NewStringValue(s), nil
		}
		fillLen := targetLen - s.Len()
		reps := (fillLen + pad.Len() - 1) / pad.Len()
		padded := pad.Repeat(reps).Slice(0, fillLen)
		return vm.NewStringValue(padded.Concat(s)), nil
	})

	vm.defineNative(strProto, "padEnd", 2, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		targetLen := 0
		if len(args) > 0 {
			targetLen = toInteger(vm, args[0], 0)
		}
		if targetLen <= s.Len() {
			return vm.NewStringValue(s), nil
		}
		pad := str.FromGo(" ")
		if len(args) > 1 && !args[1].IsUndefined() {
			pad = vm.ToStringValue(args[1])
		}
		if pad.Len() == 0 {
			return vm.NewStringValue(s), nil
		}
		fillLen := targetLen - s.Len()
		reps := (fillLen + pad.Len() - 1) / pad.Len()
		padded := pad.Repeat(reps).Slice(0, fillLen)
		return vm.NewStringValue(s.Concat(padded)), nil
	})

	vm.defineNative(strProto, "trim", 0, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		return vm.NewStringValue(s.Trim()), nil
	})

	trimStartFn := func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		return vm.NewStringValue(s.TrimStart()), nil
	}
	vm.defineNative(strProto, "trimStart", 0, trimStartFn)
	vm.defineNative(strProto, "trimLeft", 0, trimStartFn)

	trimEndFn := func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		return vm.NewStringValue(s.TrimEnd()), nil
	}
	vm.defineNative(strProto, "trimEnd", 0, trimEndFn)
	vm.defineNative(strProto, "trimRight", 0, trimEndFn)

	vm.defineNative(strProto, "toLowerCase", 0, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		if s.IsASCII() {
			return vm.NewStringValue(s.ToLowerASCII()), nil
		}
		return vm.NewStringValue(str.FromGo(strings.ToLower(s.GoString()))), nil
	})

	vm.defineNative(strProto, "toUpperCase", 0, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		if s.IsASCII() {
			return vm.NewStringValue(s.ToUpperASCII()), nil
		}
		return vm.NewStringValue(str.FromGo(strings.ToUpper(s.GoString()))), nil
	})

	vm.defineNative(strProto, "match", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		var reVal Value
		if len(args) > 0 {
			reVal = args[0]
		}
		return vm.stringMatch(s.GoString(), reVal)
	})

	vm.defineNative(strProto, "search", 1, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		var reVal Value
		if len(args) > 0 {
			reVal = args[0]
		}
		return vm.stringSearch(s.GoString(), reVal)
	})

	vm.defineNative(strProto, "split", 2, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		lim := 1 << 30
		if len(args) > 1 && !args[1].IsUndefined() {
			lim = int(uint32(vm.ToNumber(args[1])))
		}
		if lim == 0 {
			return ObjectValue(vm.heap.NewArray(0)), nil
		}
		if len(args) == 0 || args[0].IsUndefined() {
			arr := vm.heap.NewArray(1)
			vm.heap.SetElement(arr, 0, vm.NewStringValue(s))
			return ObjectValue(arr), nil
		}
		if rd := vm.regexpOf(args[0]); rd != nil {
			return vm.stringSplitRegexp(s.GoString(), rd, lim)
		}
		sep := vm.ToStringValue(args[0])
		if sep.Len() == 0 {
			count := s.Len()
			if count > lim {
				count = lim
			}
			arr := vm.heap.NewArray(count)
			for i := 0; i < count; i++ {
				vm.heap.SetElement(arr, i, vm.NewStringValue(s.CharAt(i)))
			}
			return ObjectValue(arr), nil
		}
		var parts []*str.String
		pos := 0
		for pos <= s.Len() && len(parts) < lim {
			idx := s.IndexOf(sep, pos)
			if idx == -1 {
				parts = append(parts, s.Slice(pos, s.Len()))
				break
			}
			parts = append(parts, s.Slice(pos, idx))
			pos = idx + sep.Len()
		}
		arr := vm.heap.NewArray(len(parts))
		for i, p := range parts {
			vm.heap.SetElement(arr, i, vm.NewStringValue(p))
		}
		return ObjectValue(arr), nil
	})

	vm.defineNative(strProto, "replace", 2, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		if len(args) == 0 {
			return vm.NewStringValue(s), nil
		}
		if rd := vm.regexpOf(args[0]); rd != nil {
			rep := Undefined
			if len(args) > 1 {
				rep = args[1]
			}
			return vm.stringReplaceRegexp(s.GoString(), args[0], rd, rep)
		}
		searchStr := vm.ToStringValue(args[0])
		idx := s.IndexOf(searchStr, 0)
		if idx == -1 {
			return vm.NewStringValue(s), nil
		}
		repVal := Undefined
		if len(args) > 1 {
			repVal = args[1]
		}
		repStr := str.FromGo("undefined")
		if vm.isFunction(repVal) {
			callRes, err := vm.invoke(Undefined, repVal, []Value{vm.NewStringValue(searchStr), Int(int32(idx)), vm.NewStringValue(s)})
			if err != nil {
				return Undefined, err
			}
			repStr = vm.ToStringValue(callRes)
		} else if !repVal.IsUndefined() {
			repStr = vm.ToStringValue(repVal)
		}
		res := s.Slice(0, idx).Concat(repStr).Concat(s.Slice(idx+searchStr.Len(), s.Len()))
		return vm.NewStringValue(res), nil
	})

	vm.defineNative(strProto, "replaceAll", 2, func(vm *VM, args []Value) (Value, error) {
		s, err := thisToString(vm, vm.CurrentThis())
		if err != nil {
			return Undefined, err
		}
		if len(args) == 0 {
			return vm.NewStringValue(s), nil
		}
		searchStr := vm.ToStringValue(args[0])
		repVal := Undefined
		if len(args) > 1 {
			repVal = args[1]
		}
		isFn := vm.isFunction(repVal)
		repStr := str.FromGo("undefined")
		if !isFn && !repVal.IsUndefined() {
			repStr = vm.ToStringValue(repVal)
		}
		if searchStr.Len() == 0 {
			var res *str.String = str.Empty
			for i := 0; i < s.Len(); i++ {
				rep := repStr
				if isFn {
					callRes, err := vm.invoke(Undefined, repVal, []Value{vm.NewStringValue(str.Empty), Int(int32(i)), vm.NewStringValue(s)})
					if err != nil {
						return Undefined, err
					}
					rep = vm.ToStringValue(callRes)
				}
				res = res.Concat(rep).Concat(s.CharAt(i))
			}
			rep := repStr
			if isFn {
				callRes, err := vm.invoke(Undefined, repVal, []Value{vm.NewStringValue(str.Empty), Int(int32(s.Len())), vm.NewStringValue(s)})
				if err != nil {
					return Undefined, err
				}
				rep = vm.ToStringValue(callRes)
			}
			res = res.Concat(rep)
			return vm.NewStringValue(res), nil
		}
		var res *str.String = str.Empty
		pos := 0
		for pos <= s.Len() {
			idx := s.IndexOf(searchStr, pos)
			if idx == -1 {
				res = res.Concat(s.Slice(pos, s.Len()))
				break
			}
			res = res.Concat(s.Slice(pos, idx))
			rep := repStr
			if isFn {
				callRes, err := vm.invoke(Undefined, repVal, []Value{vm.NewStringValue(searchStr), Int(int32(idx)), vm.NewStringValue(s)})
				if err != nil {
					return Undefined, err
				}
				rep = vm.ToStringValue(callRes)
			}
			res = res.Concat(rep)
			pos = idx + searchStr.Len()
		}
		return vm.NewStringValue(res), nil
	})

	strCtor := vm.heap.NewFunction(&Chunk{
		Name:      "String",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			s := ""
			if len(args) > 0 {
				p, err := vm.toPrimitive(args[0], "string")
				if err != nil {
					return Undefined, err
				}
				s = vm.toDisplayString(p)
			}
			this := vm.CurrentThis()
			if len(vm.newStack) > 0 && vm.newStack[len(vm.newStack)-1].IsObject() && this.IsObject() && this.Handle() != vm.globalObj {
				if o := vm.heap.Get(this.Handle()); o != nil {
					o.kind = KindStringObject
					o.text = vm.heap.Intern().InternGo(s)
					o.proto = strProto
				}
				return this, nil
			}
			return vm.NewStringValue(str.FromGo(s)), nil
		},
	}, NoHandle)
	hStr := ObjectValue(strCtor)
	vm.heap.AddRoot(&hStr)
	defer vm.heap.RemoveRoot(&hStr)
	vm.heap.SetProperty(strCtor, vm.heap.Intern().InternGo("prototype"), strProtoV)
	vm.heap.SetProperty(strProto, vm.heap.Intern().InternGo("constructor"), hStr)
	vm.SetGlobal("String", hStr)

	vm.defineNative(strCtor, "fromCharCode", 1, func(vm *VM, args []Value) (Value, error) {
		u16 := make([]uint16, len(args))
		for i, a := range args {
			u16[i] = uint16(vm.ToNumber(a))
		}
		return vm.NewStringValue(str.FromUTF16(u16)), nil
	})

	vm.defineNative(strCtor, "fromCodePoint", 1, func(vm *VM, args []Value) (Value, error) {
		u16 := make([]uint16, 0, len(args))
		for _, a := range args {
			n := vm.ToNumber(a)
			if math.IsNaN(n) || math.IsInf(n, 0) || math.Floor(n) != n || n < 0 || n > 0x10FFFF {
				return Undefined, fmt.Errorf("RangeError: %v is not a valid code point", n)
			}
			cp := int(n)
			if cp <= 0xFFFF {
				u16 = append(u16, uint16(cp))
			} else {
				r1, r2 := utf16.EncodeRune(rune(cp))
				u16 = append(u16, uint16(r1), uint16(r2))
			}
		}
		return vm.NewStringValue(str.FromUTF16(u16)), nil
	})

	vm.defineNative(strCtor, "raw", 1, func(vm *VM, args []Value) (Value, error) {
		if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		cooked := args[0]
		rawProp := vm.getProp(cooked, str.FromGo("raw"))
		if rawProp.IsNull() || rawProp.IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		lenProp := vm.getProp(rawProp, str.FromGo("length"))
		rawLen := toInteger(vm, lenProp, 0)
		if rawLen <= 0 {
			return vm.NewStringValue(str.Empty), nil
		}
		var res *str.String = str.Empty
		for i := 0; i < rawLen; i++ {
			if i > 0 && i-1 < len(args)-1 {
				res = res.Concat(vm.ToStringValue(args[i]))
			}
			chunk := vm.getElem(rawProp, Int(int32(i)))
			res = res.Concat(vm.ToStringValue(chunk))
		}
		return vm.NewStringValue(res), nil
	})
}

// BuiltinParseInt parses an integer with radix and leading prefix support.
func BuiltinParseInt(s string, radix int) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return math.NaN()
	}
	b := []byte(s)
	off := 0
	sign := 1.0
	if b[0] == '+' {
		off = 1
	} else if b[0] == '-' {
		sign = -1.0
		off = 1
	}
	if off >= len(b) {
		return math.NaN()
	}
	rest := b[off:]
	if radix == 0 {
		if len(rest) >= 2 && rest[0] == '0' && (rest[1] == 'x' || rest[1] == 'X') {
			radix = 16
			rest = rest[2:]
		} else {
			radix = 10
		}
	}
	if C2js_parseint_radix_ok(int32(radix)) == 0 {
		return math.NaN()
	}
	if radix == 16 && len(rest) >= 2 && rest[0] == '0' && (rest[1] == 'x' || rest[1] == 'X') {
		rest = rest[2:]
	}
	if len(rest) == 0 {
		return math.NaN()
	}
	z := int(C2js_parseint_skip_zeros(rest, int32(len(rest))))
	if z == len(rest) {
		if sign < 0 {
			return math.Copysign(0, -1)
		}
		return 0
	}
	rest = rest[z:]
	acc := 0.0
	got := false
	for i := 0; i < len(rest); i++ {
		d := C2js_parseint_digit(int32(rest[i]), int32(radix))
		if d < 0 {
			break
		}
		acc = acc*float64(radix) + float64(d)
		got = true
	}
	if !got {
		return math.NaN()
	}
	return sign * acc
}

// BuiltinParseFloat parses a float64 with leading prefix support.
func BuiltinParseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return math.NaN()
	}
	for end := len(s); end > 0; end-- {
		v, err := strconv.ParseFloat(s[:end], 64)
		if err == nil {
			return v
		}
	}
	return math.NaN()
}

func (vm *VM) InstallBooleanAndErrorBuiltins() {
	boolProto := vm.heap.NewObject()
	bpv := ObjectValue(boolProto)
	vm.heap.AddRoot(&bpv)
	defer vm.heap.RemoveRoot(&bpv)
	vm.defineNative(boolProto, "valueOf", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.prim.IsBool() {
				return o.prim, nil
			}
		}
		return Bool(vm.truthy(this)), nil
	})
	boolCtor := vm.heap.NewFunction(&Chunk{
		Name:      "Boolean",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			b := false
			if len(args) > 0 {
				b = vm.truthy(args[0])
			}
			this := vm.CurrentThis()
			if len(vm.newStack) > 0 && vm.newStack[len(vm.newStack)-1].IsObject() && this.IsObject() && this.Handle() != vm.globalObj {
				if o := vm.heap.Get(this.Handle()); o != nil {
					o.prim = Bool(b)
				}
				return this, nil
			}
			return Bool(b), nil
		},
	}, NoHandle)
	bcv := ObjectValue(boolCtor)
	vm.heap.AddRoot(&bcv)
	vm.heap.SetProperty(boolCtor, vm.heap.Intern().InternGo("prototype"), bpv)
	vm.SetGlobal("Boolean", bcv)
	vm.heap.RemoveRoot(&bcv)

	errProto := vm.heap.NewObject()
	epv := ObjectValue(errProto)
	vm.heap.AddRoot(&epv)
	defer vm.heap.RemoveRoot(&epv)
	vm.heap.SetProperty(errProto, vm.heap.Intern().InternGo("name"), vm.NewStringValue(str.FromGo("Error")))
	vm.installErrorCtor("Error", errProto)
	for _, n := range []string{"TypeError", "ReferenceError", "RangeError", "SyntaxError"} {
		p := vm.heap.NewObject()
		pv := ObjectValue(p)
		vm.heap.AddRoot(&pv)
		vm.heap.SetProto(p, errProto)
		vm.installErrorCtor(n, p)
		vm.heap.RemoveRoot(&pv)
	}
}

func (vm *VM) installErrorCtor(name string, proto Handle) {
	n := name
	ph := proto
	hold := ObjectValue(ph)
	vm.heap.AddRoot(&hold)
	defer vm.heap.RemoveRoot(&hold)
	ctor := vm.heap.NewFunction(&Chunk{
		Name:      n,
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			if !this.IsObject() || this.Handle() == vm.globalObj {
				this = ObjectValue(vm.heap.NewObject())
				vm.heap.SetProto(this.Handle(), ph)
			}
			msg := ""
			if len(args) > 0 {
				msg = vm.toDisplayString(args[0])
			}
			vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("message"), vm.NewStringValue(str.FromGo(msg)))
			vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("name"), vm.NewStringValue(str.FromGo(n)))
			return this, nil
		},
	}, NoHandle)
	cv := ObjectValue(ctor)
	pv := ObjectValue(ph)
	vm.heap.AddRoot(&cv)
	vm.heap.AddRoot(&pv)
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), pv)
	vm.heap.SetProperty(ph, vm.heap.Intern().InternGo("constructor"), cv)
	vm.heap.SetProperty(ph, vm.heap.Intern().InternGo("name"), vm.NewStringValue(str.FromGo(n)))
	vm.SetGlobal(n, cv)
	vm.heap.RemoveRoot(&cv)
	vm.heap.RemoveRoot(&pv)
}
