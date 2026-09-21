// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"fmt"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

type ProxyData struct {
	target  Value
	handler Value
}

func (vm *VM) InstallProxyBuiltins() {
	ctor := vm.heap.NewFunction(&Chunk{
		Name:      "Proxy",
		Params:    2,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.ctorThis()
			var target, handler Value
			if len(args) > 0 {
				target = args[0]
			}
			if len(args) > 1 {
				handler = args[1]
			}
			if !target.IsObject() || vm.IsString(target) {
				return Undefined, fmt.Errorf("TypeError: Cannot create proxy with a non-object as target")
			}
			if !handler.IsObject() || vm.IsString(handler) {
				return Undefined, fmt.Errorf("TypeError: Cannot create proxy with a non-object as handler")
			}
			if o := vm.heap.Get(this.Handle()); o != nil {
				o.kind = KindProxy
				o.proxy = &ProxyData{target: target, handler: handler}
			}
			return this, nil
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	vm.SetGlobal("Proxy", ctorV)
}

func (vm *VM) proxyOf(obj Value) *ProxyData {
	if !obj.IsObject() {
		return nil
	}
	o := vm.heap.Get(obj.Handle())
	if o == nil || o.kind != KindProxy {
		return nil
	}
	return o.proxy
}

func (vm *VM) proxyTrap(handler Value, name string) Value {
	return vm.getProp(handler, vm.heap.Intern().InternGo(name))
}

func (vm *VM) proxyGet(obj Value, name *str.String) (Value, bool) {
	p := vm.proxyOf(obj)
	if p == nil {
		return Undefined, false
	}
	getFn := vm.proxyTrap(p.handler, "get")
	if vm.isFunction(getFn) {
		key := vm.NewStringValue(name)
		res, err := vm.CallFunction(getFn, p.handler, []Value{p.target, key, obj})
		if err != nil {
			return Undefined, true
		}
		return res, true
	}
	return vm.getProp(p.target, name), true
}

func (vm *VM) proxySet(obj Value, name *str.String, v Value) (bool, error) {
	p := vm.proxyOf(obj)
	if p == nil {
		return false, nil
	}
	setFn := vm.proxyTrap(p.handler, "set")
	if vm.isFunction(setFn) {
		key := vm.NewStringValue(name)
		_, err := vm.CallFunction(setFn, p.handler, []Value{p.target, key, v, obj})
		return true, err
	}
	if err := vm.setProp(nil, p.target, name, v); err != nil {
		return true, err
	}
	return true, nil
}

func (vm *VM) proxyHas(obj Value, name *str.String) (bool, bool, error) {
	p := vm.proxyOf(obj)
	if p == nil {
		return false, false, nil
	}
	hasFn := vm.proxyTrap(p.handler, "has")
	if vm.isFunction(hasFn) {
		key := vm.NewStringValue(name)
		res, err := vm.CallFunction(hasFn, p.handler, []Value{p.target, key})
		if err != nil {
			return false, true, err
		}
		return vm.truthy(res), true, nil
	}
	_, ok := vm.heap.GetProperty(p.target.Handle(), name)
	return ok, true, nil
}

func (vm *VM) proxyDelete(obj Value, name *str.String) (bool, bool) {
	p := vm.proxyOf(obj)
	if p == nil {
		return false, false
	}
	delFn := vm.proxyTrap(p.handler, "deleteProperty")
	if vm.isFunction(delFn) {
		key := vm.NewStringValue(name)
		res, err := vm.CallFunction(delFn, p.handler, []Value{p.target, key})
		if err != nil {
			return false, true
		}
		return vm.truthy(res), true
	}
	return vm.heap.DeleteProperty(p.target.Handle(), name), true
}

func (vm *VM) packArgs(argc int) Value {
	h := vm.heap.NewArray(argc)
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	for i := 0; i < argc; i++ {
		vm.heap.SetElement(h, i, vm.peek(argc-1-i))
	}
	vm.heap.RemoveRoot(&hv)
	return hv
}

func (vm *VM) proxyDispatch(fr *frame, p *ProxyData, argc int, thisVal, newObj, newTarget Value) (bool, error) {
	constructing := newObj.IsObject()
	trapName := "apply"
	if constructing {
		trapName = "construct"
	}
	trap := vm.proxyTrap(p.handler, trapName)
	if !vm.isFunction(trap) {
		return false, nil
	}
	args := vm.packArgs(argc)
	vm.heap.AddRoot(&args)
	defer vm.heap.RemoveRoot(&args)
	var res Value
	var err error
	if constructing {
		nt := newTarget
		if !nt.IsObject() {
			nt = vm.peek(argc)
		}
		res, err = vm.CallFunction(trap, p.handler, []Value{p.target, args, nt})
	} else {
		res, err = vm.CallFunction(trap, p.handler, []Value{p.target, thisVal, args})
	}
	if err != nil {
		return true, err
	}
	vm.heap.TruncateStack(vm.heap.StackLen() - argc - 1)
	if constructing && !res.IsObject() {
		res = newObj
	}
	vm.push(res)
	return true, nil
}
