// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"math"
	"math/rand"
)

// InstallMathBuiltins registers standard Math object methods and constants in VM.
func (vm *VM) InstallMathBuiltins() {
	mathObj := vm.heap.NewObject()
	hMath := ObjectValue(mathObj)
	vm.SetGlobal("Math", hMath)

	setProp := func(name string, v Value) {
		k := vm.heap.Intern().InternGo(name)
		vm.heap.SetProperty(mathObj, k, v)
	}

	setProp("E", Number(math.E))
	setProp("LN10", Number(math.Ln10))
	setProp("LN2", Number(math.Ln2))
	setProp("LOG10E", Number(math.Log10E))
	setProp("LOG2E", Number(math.Log2E))
	setProp("PI", Number(math.Pi))
	setProp("SQRT1_2", Number(math.Sqrt(0.5)))
	setProp("SQRT2", Number(math.Sqrt(2)))

	regUnary := func(name string, fn func(float64) float64) {
		chunk := &Chunk{
			Name:   name,
			Params: 1,
			Native: func(vm *VM, args []Value) (Value, error) {
				if len(args) == 0 {
					return Number(fn(math.NaN())), nil
				}
				x, err := vm.toNumberErr(args[0])
				if err != nil {
					return Undefined, err
				}
				return Number(fn(x)), nil
			},
		}
		fnHandle := vm.heap.NewFunction(chunk, NoHandle)
		setProp(name, ObjectValue(fnHandle))
	}

	regUnary("abs", math.Abs)
	regUnary("floor", math.Floor)
	regUnary("ceil", math.Ceil)
	regUnary("round", func(x float64) float64 {
		return math.Floor(x + 0.5)
	})
	regUnary("sqrt", math.Sqrt)
	regUnary("sin", math.Sin)
	regUnary("cos", math.Cos)
	regUnary("tan", math.Tan)
	regUnary("asin", math.Asin)
	regUnary("acos", math.Acos)
	regUnary("atan", math.Atan)
	regUnary("fround", BuiltinMathFround)
	regUnary("log", math.Log)
	regUnary("exp", math.Exp)

	// Variadic min/max. With no arguments Math.min returns +Inf and
	// Math.max returns -Inf, matching ECMAScript semantics. If any
	// argument converts to NaN the result is NaN.
	regVariadic := func(name string, combine func(a, b float64) float64, init float64) {
		chunk := &Chunk{
			Name:   name,
			Params: 0,
			Native: func(vm *VM, args []Value) (Value, error) {
				if len(args) == 0 {
					return Number(init), nil
				}
				acc := init
				for _, a := range args {
					f, err := vm.toNumberErr(a)
					if err != nil {
						return Undefined, err
					}
					if math.IsNaN(f) {
						return Number(math.NaN()), nil
					}
					acc = combine(acc, f)
				}
				return Number(acc), nil
			},
		}
		fnHandle := vm.heap.NewFunction(chunk, NoHandle)
		setProp(name, ObjectValue(fnHandle))
	}
	regVariadic("min", math.Min, math.Inf(1))
	regVariadic("max", math.Max, math.Inf(-1))

	// Binary pow.
	regBinary := func(name string, fn func(float64, float64) float64) {
		chunk := &Chunk{
			Name:   name,
			Params: 2,
			Native: func(vm *VM, args []Value) (Value, error) {
				if len(args) < 2 {
					return Number(math.NaN()), nil
				}
				x, err := vm.toNumberErr(args[0])
				if err != nil {
					return Undefined, err
				}
				y, err := vm.toNumberErr(args[1])
				if err != nil {
					return Undefined, err
				}
				return Number(fn(x, y)), nil
			},
		}
		fnHandle := vm.heap.NewFunction(chunk, NoHandle)
		setProp(name, ObjectValue(fnHandle))
	}
	regBinary("pow", math.Pow)
	regBinary("atan2", math.Atan2)
	vm.defineNative(mathObj, "hypot", 2, func(vm *VM, args []Value) (Value, error) {
		acc := 0.0
		infinite := false
		for _, v := range args {
			x, err := vm.toNumberErr(v)
			if err != nil {
				return Undefined, err
			}
			infinite = infinite || math.IsInf(x, 0)
			acc = math.Hypot(acc, x)
		}
		if infinite {
			acc = math.Inf(1)
		}
		return Number(acc), nil
	})

	randChunk := &Chunk{
		Name:   "random",
		Params: 0,
		Native: func(vm *VM, args []Value) (Value, error) {
			return Number(rand.Float64()), nil
		},
	}
	setProp("random", ObjectValue(vm.heap.NewFunction(randChunk, NoHandle)))

	vm.SetGlobal("Math", hMath)
}
