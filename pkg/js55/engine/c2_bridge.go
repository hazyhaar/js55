// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"math"
)

// CheckedAddSmi performs 31-bit / 63-bit signed integer addition with overflow detection.
func CheckedAddSmi(a, b int) (int, bool) {
	res := a + b
	// Check signed overflow
	if (a > 0 && b > 0 && res < 0) || (a < 0 && b < 0 && res > 0) {
		return 0, true
	}
	if res > math.MaxInt32 || res < math.MinInt32 {
		return res, true
	}
	return res, false
}

// BranchlessMin returns the minimum of two integers without branching.
func BranchlessMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BranchlessMax returns the maximum of two integers without branching.
func BranchlessMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
