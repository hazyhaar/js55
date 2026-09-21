// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"fmt"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

func (vm *VM) requireReflectTarget(v Value, name string) error {
	if !v.IsObject() || vm.IsString(v) {
		return fmt.Errorf("TypeError: %s called on non-object", name)
	}
	return nil
}

func (vm *VM) arrayValues(v Value) []Value {
	if !v.IsObject() {
		return nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil {
		return nil
	}
	if o.kind == KindArray {
		out := make([]Value, len(o.elements))
		copy(out, o.elements)
		return out
	}
	lenKey := vm.heap.Intern().InternGo("length")
	lv, ok := vm.heap.GetProperty(v.Handle(), lenKey)
	if !ok {
		return nil
	}
	n := int(lv.ToInt())
	if n < 0 {
		n = 0
	}
	if n > 10000 {
		n = 10000
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		el := vm.heap.GetElement(v.Handle(), i)
		if el.IsUndefined() {
			if pv, ok := vm.heap.GetProperty(v.Handle(), vm.heap.Intern().InternGo(fmt.Sprintf("%d", i))); ok {
				el = pv
			}
		}
		out[i] = el
	}
	return out
}

func (vm *VM) descriptorObject(v Value, key *str.String) Value {
	return vm.dataDescriptorObject(v, key)
}

// InstallReflectBuiltins registers standard Reflect global methods.
func (vm *VM) InstallReflectBuiltins() {
	reflectObj := vm.heap.NewObject()
	objV := ObjectValue(reflectObj)
	vm.heap.AddRoot(&objV)
	defer vm.heap.RemoveRoot(&objV)

	vm.defineNative(reflectObj, "get", 2, func(vm *VM, args []Value) (Value, error) {
		var target, key, recv Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			key = args[1]
		}
		if err := vm.requireReflectTarget(target, "Reflect.get"); err != nil {
			return Undefined, err
		}
		recv = target
		if len(args) > 2 && args[2].IsObject() {
			recv = args[2]
		}
		name := vm.keyString(key)
		if acc, ok := vm.heap.GetOwnProperty(target.Handle(), name); ok && acc.IsObject() {
			if o := vm.heap.Get(acc.Handle()); o != nil && o.kind == KindAccessor {
				if len(o.elements) > 0 && vm.isFunction(o.elements[0]) {
					return vm.invoke(recv, o.elements[0], nil)
				}
			}
		}
		return vm.getProp(target, name), nil
	})
	vm.defineNative(reflectObj, "has", 2, func(vm *VM, args []Value) (Value, error) {
		var target, key Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			key = args[1]
		}
		if err := vm.requireReflectTarget(target, "Reflect.has"); err != nil {
			return Undefined, err
		}
		ok, err := vm.hasIn(&frame{}, key, target)
		if err != nil {
			return Undefined, err
		}
		return Bool(ok), nil
	})
	vm.defineNative(reflectObj, "set", 3, func(vm *VM, args []Value) (Value, error) {
		var target, key, val Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			key = args[1]
		}
		if len(args) > 2 {
			val = args[2]
		}
		if err := vm.requireReflectTarget(target, "Reflect.set"); err != nil {
			return Undefined, err
		}
		recv := target
		if len(args) > 3 && args[3].IsObject() {
			recv = args[3]
		}
		if err := vm.setProp(&frame{}, recv, vm.keyString(key), val); err != nil {
			return False, nil
		}
		return True, nil
	})
	vm.defineNative(reflectObj, "deleteProperty", 2, func(vm *VM, args []Value) (Value, error) {
		var target, key Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			key = args[1]
		}
		if err := vm.requireReflectTarget(target, "Reflect.deleteProperty"); err != nil {
			return Undefined, err
		}
		return Bool(vm.deleteKey(target, vm.keyString(key))), nil
	})
	vm.defineNative(reflectObj, "ownKeys", 1, func(vm *VM, args []Value) (Value, error) {
		var target Value
		if len(args) > 0 {
			target = args[0]
		}
		if err := vm.requireReflectTarget(target, "Reflect.ownKeys"); err != nil {
			return Undefined, err
		}
		names := vm.BuiltinObjectKeys(target)
		h := vm.heap.NewArray(len(names))
		hv := ObjectValue(h)
		vm.heap.AddRoot(&hv)
		defer vm.heap.RemoveRoot(&hv)
		for i, k := range names {
			vm.heap.SetElement(h, i, ObjectValue(vm.heap.NewString(k)))
		}
		return hv, nil
	})
	vm.defineNative(reflectObj, "getPrototypeOf", 1, func(vm *VM, args []Value) (Value, error) {
		var target Value
		if len(args) > 0 {
			target = args[0]
		}
		if err := vm.requireReflectTarget(target, "Reflect.getPrototypeOf"); err != nil {
			return Undefined, err
		}
		o := vm.heap.Get(target.Handle())
		if o == nil || o.proto == NoHandle {
			return Null, nil
		}
		return ObjectValue(o.proto), nil
	})
	vm.defineNative(reflectObj, "setPrototypeOf", 2, func(vm *VM, args []Value) (Value, error) {
		var target, proto Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			proto = args[1]
		}
		if err := vm.requireReflectTarget(target, "Reflect.setPrototypeOf"); err != nil {
			return Undefined, err
		}
		o := vm.heap.Get(target.Handle())
		if o == nil {
			return False, nil
		}
		if o.frozen {
			return False, nil
		}
		if proto.IsNull() {
			o.proto = NoHandle
			return True, nil
		}
		if !proto.IsObject() {
			return Undefined, fmt.Errorf("TypeError: Object prototype may only be an Object or null")
		}
		o.proto = proto.Handle()
		return True, nil
	})
	vm.defineNative(reflectObj, "isExtensible", 1, func(vm *VM, args []Value) (Value, error) {
		var target Value
		if len(args) > 0 {
			target = args[0]
		}
		if err := vm.requireReflectTarget(target, "Reflect.isExtensible"); err != nil {
			return Undefined, err
		}
		o := vm.heap.Get(target.Handle())
		if o == nil {
			return False, nil
		}
		return Bool(!o.frozen), nil
	})
	vm.defineNative(reflectObj, "preventExtensions", 1, func(vm *VM, args []Value) (Value, error) {
		var target Value
		if len(args) > 0 {
			target = args[0]
		}
		if err := vm.requireReflectTarget(target, "Reflect.preventExtensions"); err != nil {
			return Undefined, err
		}
		if o := vm.heap.Get(target.Handle()); o != nil {
			o.frozen = true
		}
		return True, nil
	})
	vm.defineNative(reflectObj, "getOwnPropertyDescriptor", 2, func(vm *VM, args []Value) (Value, error) {
		var target, key Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			key = args[1]
		}
		if err := vm.requireReflectTarget(target, "Reflect.getOwnPropertyDescriptor"); err != nil {
			return Undefined, err
		}
		return vm.descriptorObject(target, vm.keyString(key)), nil
	})
	vm.defineNative(reflectObj, "defineProperty", 3, func(vm *VM, args []Value) (Value, error) {
		var target, key, desc Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			key = args[1]
		}
		if len(args) > 2 {
			desc = args[2]
		}
		if err := vm.requireReflectTarget(target, "Reflect.defineProperty"); err != nil {
			return Undefined, err
		}
		if !desc.IsObject() {
			return False, nil
		}
		if o := vm.heap.Get(target.Handle()); o != nil && o.frozen {
			return False, nil
		}
		vm.defineDataFromDesc(target.Handle(), vm.keyString(key), desc)
		return True, nil
	})
	vm.defineNative(reflectObj, "apply", 3, func(vm *VM, args []Value) (Value, error) {
		var target, thisArg, argList Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			thisArg = args[1]
		}
		if len(args) > 2 {
			argList = args[2]
		}
		if !vm.isFunction(target) {
			return Undefined, fmt.Errorf("TypeError: Reflect.apply target is not a function")
		}
		callArgs := vm.arrayValues(argList)
		return vm.CallFunction(target, thisArg, callArgs)
	})
	vm.defineNative(reflectObj, "construct", 2, func(vm *VM, args []Value) (Value, error) {
		var target, argList, newTarget Value
		if len(args) > 0 {
			target = args[0]
		}
		if len(args) > 1 {
			argList = args[1]
		}
		if len(args) > 2 {
			newTarget = args[2]
		} else {
			newTarget = target
		}
		if !target.IsObject() {
			return Undefined, fmt.Errorf("TypeError: Reflect.construct target is not a constructor")
		}
		callArgs := vm.arrayValues(argList)
		vm.push(target)
		for _, a := range callArgs {
			vm.push(a)
		}
		dummy := frame{}
		if err := vm.instantiateAs(&dummy, len(callArgs), newTarget); err != nil {
			return Undefined, err
		}
		return vm.pop(), nil
	})

	vm.SetGlobal("Reflect", objV)
}

// BuiltinReflectGet implements Reflect.get(target, propertyKey).
func (vm *VM) BuiltinReflectGet(target Value, key *str.String) Value {
	return vm.getProp(target, key)
}

// BuiltinReflectHas implements Reflect.has(target, propertyKey).
func (vm *VM) BuiltinReflectHas(target Value, key *str.String) bool {
	if !target.IsObject() {
		return false
	}
	_, ok := vm.heap.GetProperty(target.Handle(), key)
	return ok
}
