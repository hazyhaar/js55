// SPDX-License-Identifier: BUSL-1.1
package engine

// archtimeGeometryAvailable reports that the closed Box3 bounds kernel is
// linked into the pure-Go build. The scalar implementation below is
// self-contained: no external module nor build tag is required.
const archtimeGeometryAvailable = true

// archtimeBoundsXYZ writes minX, minY, minZ, maxX, maxY, maxZ into out for
// finite interleaved XYZ float32 positions. Malformed or non-finite inputs are
// refused without modifying out, preserving the rejection contract of the
// generic admit path.
func archtimeBoundsXYZ(positions []float32, out *[6]float64) bool {
	if len(positions) < 3 || len(positions)%3 != 0 || !checkFinite32All(positions) {
		return false
	}
	minX, maxX := float64(positions[0]), float64(positions[0])
	minY, maxY := float64(positions[1]), float64(positions[1])
	minZ, maxZ := float64(positions[2]), float64(positions[2])
	for i := 3; i < len(positions); i += 3 {
		x := float64(positions[i])
		y := float64(positions[i+1])
		z := float64(positions[i+2])
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
		if z < minZ {
			minZ = z
		}
		if z > maxZ {
			maxZ = z
		}
	}
	out[0], out[1], out[2] = minX, minY, minZ
	out[3], out[4], out[5] = maxX, maxY, maxZ
	return true
}
