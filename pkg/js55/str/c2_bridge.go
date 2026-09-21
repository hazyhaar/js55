// SPDX-License-Identifier: Apache-2.0 OR MIT

package str

import (
	"bytes"

	"github.com/hazyhaar/js55/pkg/c2strsimd"
)

// FastHashSequential8Bit delegates to Murmur/Splitmix hash.
func FastHashSequential8Bit(chars []byte, seed uint32) uint32 {
	h := uint64(seed)
	for _, b := range chars {
		h = (h * 31) + uint64(b)
	}
	return uint32(h)
}

// FastStringSearchLinear delegates to c2strsimd AVX2 linear search kernel.
func FastStringSearchLinear(subject, pattern []byte, startIdx uint64) int64 {
	if startIdx >= uint64(len(subject)) {
		return -1
	}
	sub := subject[startIdx:]
	pos := c2strsimd.C2_strpos(sub, uint64(len(sub)), pattern, uint64(len(pattern)))
	if pos < 0 {
		return -1
	}
	return int64(startIdx) + pos
}

// FastStringEquals8Bit delegates to byte slice comparison.
func FastStringEquals8Bit(s1, s2 []byte) bool {
	return bytes.Equal(s1, s2)
}
