//go:build !archtime_geometry

package engine

const archtimeNormalsAvailable = false

func (vm *VM) tryArchtimeNormals(fr *frame, fn *Object, argc int, thisVal Value) (bool, Value, error) {
	return false, Undefined, nil
}
