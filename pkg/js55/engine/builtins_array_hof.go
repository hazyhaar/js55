// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

// BuiltinArrayMap executes callback on each element and returns a new JSArray.
func (vm *VM) BuiltinArrayMap(arr Value, callback func(val Value, index int) Value) Value {
	if !arr.IsObject() {
		return Undefined
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return Undefined
	}
	resHandle := vm.heap.NewArray(len(o.elements))
	resObj := vm.heap.MustGet(resHandle)
	for i, el := range o.elements {
		resObj.elements[i] = callback(el, i)
	}
	return ObjectValue(resHandle)
}

// BuiltinArrayFilter returns a new JSArray containing elements that pass predicate.
func (vm *VM) BuiltinArrayFilter(arr Value, predicate func(val Value, index int) bool) Value {
	if !arr.IsObject() {
		return Undefined
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return Undefined
	}
	filtered := make([]Value, 0, len(o.elements))
	for i, el := range o.elements {
		if predicate(el, i) {
			filtered = append(filtered, el)
		}
	}
	resHandle := vm.heap.NewArray(len(filtered))
	resObj := vm.heap.MustGet(resHandle)
	copy(resObj.elements, filtered)
	return ObjectValue(resHandle)
}

// BuiltinArrayForEach executes callback on each element.
func (vm *VM) BuiltinArrayForEach(arr Value, callback func(val Value, index int)) {
	if !arr.IsObject() {
		return
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return
	}
	for i, el := range o.elements {
		callback(el, i)
	}
}
