// SPDX-License-Identifier: Apache-2.0 OR MIT
// Source C : /devhoros/c2simd/sources/c2d2s/d2s.c
// sha256(d2s.c)=9ec4daf4be63f9b628da9be5575063ab38d9f0eaa84dc4f2f7141ebfc76cb6ef

package c2d2s

import (
	"math"
	"strconv"
)

func finiteNeg(f float64) bool {
	return math.Signbit(f) && f != 0 && !math.IsNaN(f) && !math.IsInf(f, 0)
}

func parseECMA(b []byte) (s uint64, k, nform int, ok bool) {
	if len(b) == 0 {
		return 0, 0, 0, false
	}
	epos := -1
	for i, c := range b {
		if c == 'e' || c == 'E' {
			epos = i
			break
		}
	}
	if epos >= 0 {
		mant := b[:epos]
		es := 1
		expv := 0
		rest := b[epos+1:]
		if len(rest) == 0 {
			return 0, 0, 0, false
		}
		if rest[0] == '+' {
			rest = rest[1:]
		} else if rest[0] == '-' {
			es = -1
			rest = rest[1:]
		}
		for _, c := range rest {
			if c < '0' || c > '9' {
				return 0, 0, 0, false
			}
			expv = expv*10 + int(c-'0')
		}
		expv *= es
		nd := 0
		for _, c := range mant {
			if c == '.' {
				continue
			}
			if c < '0' || c > '9' {
				return 0, 0, 0, false
			}
			s = s*10 + uint64(c-'0')
			nd++
		}
		if nd == 0 {
			return 0, 0, 0, false
		}
		return s, nd, expv + 1, true
	}
	if len(b) >= 2 && b[0] == '0' && b[1] == '.' {
		frac := b[2:]
		z := 0
		for z < len(frac) && frac[z] == '0' {
			z++
		}
		rest := frac[z:]
		if len(rest) == 0 {
			return 0, 1, 1, true
		}
		for _, c := range rest {
			if c < '0' || c > '9' {
				return 0, 0, 0, false
			}
			s = s*10 + uint64(c-'0')
		}
		return s, len(rest), -z, true
	}
	dot := -1
	for i, c := range b {
		if c == '.' {
			dot = i
			break
		}
	}
	if dot >= 0 {
		nd := 0
		for i, c := range b {
			if i == dot {
				continue
			}
			if c < '0' || c > '9' {
				return 0, 0, 0, false
			}
			d := uint64(c - '0')
			if s > (^uint64(0)-d)/10 {
				return 0, 0, 0, false
			}
			s = s*10 + d
			nd++
		}
		return s, nd, dot, true
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, 0, 0, false
		}
		d := uint64(c - '0')
		if s > (^uint64(0)-d)/10 {
			return 0, 0, 0, false
		}
		s = s*10 + d
	}
	return s, len(b), len(b), true
}

func writeAbs(buf []byte, f float64) int {
	mag := f
	if finiteNeg(f) {
		mag = -f
	}
	switch {
	case math.IsNaN(f):
		return copy(buf, "NaN")
	case mag == 0:
		buf[0] = '0'
		return 1
	case math.IsInf(f, 0):
		return copy(buf, "Infinity")
	}
	var tmp [32]byte
	sci := strconv.AppendFloat(tmp[:0], mag, 'e', -1, 64)
	s, k, nform, ok := parseECMA(sci)
	if !ok {
		return copy(buf, sci)
	}
	return int(emit_ecma(buf, 0, s, k, nform))
}

// Format rend Number::toString ECMAScript en base dix.
func Format(f float64) string {
	var buf [64]byte
	n := writeAbs(buf[:], f)
	if finiteNeg(f) || math.IsInf(f, -1) {
		return "-" + string(buf[:n])
	}
	return string(buf[:n])
}

// Append ajoute Number::toString ECMAScript en base dix à dst.
// Aucune allocation si cap(dst)-len(dst) >= 64 (plus un octet si signe).
func Append(dst []byte, f float64) []byte {
	if finiteNeg(f) || math.IsInf(f, -1) {
		dst = append(dst, '-')
	}
	p := len(dst)
	if cap(dst)-p < 64 {
		nbuf := make([]byte, p, p+64)
		copy(nbuf, dst)
		dst = nbuf
	}
	n := writeAbs(dst[p:p+64], f)
	return dst[:p+n]
}
