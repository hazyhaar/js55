// SPDX-License-Identifier: BUSL-1.1
package engine

import "fmt"

// NewProxy exposes the existing proxy machinery to an embedding host.
// The caller must root the returned value for as long as the host retains it.
func (vm *VM) NewProxy(target, handler Value) Value {
	vm.heap.AddRoot(&target)
	vm.heap.AddRoot(&handler)
	defer vm.heap.RemoveRoot(&target)
	defer vm.heap.RemoveRoot(&handler)
	h := vm.heap.NewObject()
	o := vm.heap.MustGet(h)
	o.kind = KindProxy
	o.proxy = &ProxyData{target: target, handler: handler}
	return ObjectValue(h)
}

func (vm *VM) GetProperty(v Value, key string) (Value, error) {
	return vm.getPropInvoke(v, vm.heap.Intern().InternGo(key))
}

func (vm *VM) IsCallable(v Value) bool { return vm.isFunction(v) }

// TypedArrayInfo provides host access to TypedArray metadata.
func (vm *VM) TypedArrayInfo(v Value) (name string, byteLength, byteOffset int, ok bool) {
	if vm.taOutOfBounds(v) {
		return "", 0, 0, false
	}
	buf, offset, bpe, _, _, ok := vm.typedArrayView(v)
	if !ok || buf == nil {
		return "", 0, 0, false
	}
	n := vm.arrayLikeLength(v)
	o := vm.heap.Get(v.Handle())
	if o == nil {
		return "", 0, 0, false
	}
	return o.typedName, n * bpe, offset, true
}

// Sequence returns a detached snapshot. Shared buffer views are rejected until
// their byte-level storage semantics can be guaranteed by the engine.
func (vm *VM) Sequence(v Value) (name string, values []Value, ok bool, err error) {
	if !v.IsObject() {
		return
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind != KindArray {
		return
	}
	ok = true
	name = o.typedName
	if name != "" && o.env != NoHandle {
		err = fmt.Errorf("host transfer: shared ArrayBuffer views are not supported")
		return
	}
	values = append([]Value(nil), o.elements...)
	return
}

// ReplaceSequence copies host output back only after checking the entire shape.
func (vm *VM) ReplaceSequence(v Value, values []Value) error {
	name, old, ok, err := vm.Sequence(v)
	if err != nil {
		return err
	}
	if !ok || name == "" || len(old) != len(values) {
		return fmt.Errorf("host transfer: incompatible output buffer")
	}
	copyValues := make([]Value, len(values))
	for i, x := range values {
		if !x.IsNumber() && !x.IsInt() {
			return fmt.Errorf("host transfer: nonnumeric output")
		}
		copyValues[i], err = vm.coerceTypedElement(x, name)
		if err != nil {
			return err
		}
	}
	copy(vm.heap.MustGet(v.Handle()).elements, copyValues)
	return nil
}
