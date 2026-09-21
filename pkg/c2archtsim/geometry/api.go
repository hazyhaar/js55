// SPDX-License-Identifier: Apache-2.0 OR MIT
// Package geometry computes scalar ARCHTIME envelopes of finite interleaved
// XYZ float32 buffers. The compute functions are generated from custom C;
// these public adapters validate all positions before publishing any output.
package geometry

import "math"

// Fixed admission threshold from the small-size scalar/SIMD campaign.
// Only dispatch lives here; both compute paths are generated from C.
const simdMinFloats = 64 * 3

func boundsKernel(positions []float32, out *[6]float64) {
	if len(positions) < simdMinFloats {
		Archtime_small_bounds(positions, uint64(len(positions)), out[:])
		return
	}
	Archtime_bounds_xyz(positions, uint64(len(positions)), out[:])
}

func sphereKernel(positions []float32, out *[4]float64) {
	if len(positions) < simdMinFloats {
		Archtime_small_sphere(positions, uint64(len(positions)), out[:])
		return
	}
	Archtime_sphere_xyz(positions, uint64(len(positions)), out[:])
}

// BoundsXYZ writes minX,minY,minZ,maxX,maxY,maxZ. It rejects nil output,
// empty or incomplete triplets and nonfinite values without modifying out.
func BoundsXYZ(positions []float32, out *[6]float64) bool {
	if out == nil || !positionsOK(positions) {
		return false
	}
	boundsKernel(positions, out)
	return true
}

// SphereXYZ writes centerX,centerY,centerZ,radius using the box midpoint and
// the farthest position. Its admission and rejection contract matches BoundsXYZ.
func SphereXYZ(positions []float32, out *[4]float64) bool {
	if out == nil || !positionsOK(positions) {
		return false
	}
	sphereKernel(positions, out)
	return true
}

func positionsOK(positions []float32) bool {
	n := len(positions)
	if n == 0 || n%3 != 0 {
		return false
	}
	for i := 0; i < n; i++ {
		if !finite32(positions[i]) {
			return false
		}
	}
	return true
}

func finite32(x float32) bool {
	bits := math.Float32bits(x) & 0x7fffffff
	return bits < 0x7f800000
}
