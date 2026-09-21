// SPDX-License-Identifier: BUSL-1.1

package engine

// IteratorResult represents an ECMAScript { value: ..., done: ... } object.
type IteratorResult struct {
	Value Value
	Done  bool
}

// ArrayIterator implements ECMAScript Array.prototype.values() iterator.
type ArrayIterator struct {
	Array Handle
	Index int
}

// Next advances the ArrayIterator.
func (it *ArrayIterator) Next(vm *VM) (Value, bool) {
	o := vm.heap.Get(it.Array)
	if o == nil || o.kind != KindArray || it.Index >= len(o.elements) {
		return Undefined, true
	}
	val := o.elements[it.Index]
	it.Index++
	return val, false
}
