// SPDX-License-Identifier: BUSL-1.1
package engine

import "math"

func checkFinite32All(arr []float32) bool {
	for _, x := range arr {
		bits := math.Float32bits(x) & 0x7fffffff
		if bits >= 0x7f800000 {
			return false
		}
	}
	return true
}

func checkIndicesOK(idx []uint32, nverts uint64) bool {
	for _, x := range idx {
		if uint64(x) >= nverts {
			return false
		}
	}
	return true
}
