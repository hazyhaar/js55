//go:build !archtime_geometry

package engine

const archtimeGeometryAvailable = false

func archtimeBoundsXYZ(positions []float32, out *[6]float64) bool {
	panic("archtime geometry is not linked: unreachable behind availability gate")
}
