// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"math"
	"math/bits"
)

// BuiltinMathImul computes 32-bit integer multiplication (C/C++ overflow semantics).
func BuiltinMathImul(a, b int32) int32 {
	return a * b
}

// BuiltinMathClz32 counts leading zeros in a 32-bit integer.
func BuiltinMathClz32(a uint32) int32 {
	return int32(bits.LeadingZeros32(a))
}

// BuiltinMathFround rounds a float64 to nearest 32-bit single precision float.
func BuiltinMathFround(a float64) float64 {
	return float64(float32(a))
}

// BuiltinMathHypot computes sqrt(a*a + b*b) without intermediate overflow.
func BuiltinMathHypot(a, b float64) float64 {
	return math.Hypot(a, b)
}

// BuiltinMathCbrt computes the cube root of a number.
func BuiltinMathCbrt(a float64) float64 {
	return math.Cbrt(a)
}

// BuiltinMathLog1p computes natural logarithm of 1 + x.
func BuiltinMathLog1p(a float64) float64 {
	return math.Log1p(a)
}

// BuiltinMathExpm1 computes e^x - 1.
func BuiltinMathExpm1(a float64) float64 {
	return math.Expm1(a)
}
