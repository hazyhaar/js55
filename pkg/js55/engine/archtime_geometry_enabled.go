//go:build archtime_geometry

package engine

import "github.com/hazyhaar/c2pkg/c2archtsim/geometry"

const archtimeGeometryAvailable = true

func archtimeBoundsXYZ(positions []float32, out *[6]float64) bool {
	return geometry.BoundsXYZ(positions, out)
}
