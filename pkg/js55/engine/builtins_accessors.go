// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"github.com/hazyhaar/js55/pkg/js55/str"
)

func (vm *VM) defineAccessor(obj Value, name *str.String, fn Value, setter bool) error {
	if !obj.IsObject() {
		return nil
	}
	vm.heap.AddRoot(&obj)
	vm.heap.AddRoot(&fn)
	defer vm.heap.RemoveRoot(&obj)
	defer vm.heap.RemoveRoot(&fn)
	accH := Handle(0)
	if cur, ok := vm.heap.GetOwnProperty(obj.Handle(), name); ok && cur.IsObject() {
		if o := vm.heap.Get(cur.Handle()); o != nil && o.kind == KindAccessor {
			accH = cur.Handle()
		}
	}
	if accH == NoHandle {
		accH = vm.heap.NewObject()
		if o := vm.heap.Get(accH); o != nil {
			o.kind = KindAccessor
			o.elements = []Value{Undefined, Undefined}
		}
		av := ObjectValue(accH)
		vm.heap.AddRoot(&av)
		vm.heap.SetProperty(obj.Handle(), name, av)
		vm.heap.RemoveRoot(&av)
	}
	o := vm.heap.Get(accH)
	if o == nil {
		return nil
	}
	if len(o.elements) < 2 {
		o.elements = []Value{Undefined, Undefined}
	}
	if setter {
		o.elements[1] = fn
	} else {
		o.elements[0] = fn
	}
	return nil
}

func (vm *VM) getPropInvoke(obj Value, name *str.String) (Value, error) {
	if p := vm.proxyOf(obj); p != nil {
		get := vm.proxyTrap(p.handler, "get")
		if vm.isFunction(get) {
			key := vm.NewStringValue(name)
			return vm.CallFunction(get, p.handler, []Value{p.target, key, obj})
		}
		return vm.getPropInvoke(p.target, name)
	}
	v := vm.getProp(obj, name)
	if !v.IsObject() {
		return v, nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind != KindAccessor {
		return v, nil
	}
	if len(o.elements) == 0 || !vm.isFunction(o.elements[0]) {
		return Undefined, nil
	}
	return vm.invoke(obj, o.elements[0], nil)
}

func (vm *VM) superGet(fr *frame, name *str.String) (Value, error) {
	if fr == nil || !fr.callee.IsObject() {
		return Undefined, nil
	}
	fn := vm.heap.Get(fr.callee.Handle())
	if fn == nil {
		return Undefined, nil
	}
	homeID := fn.homeObject
	if homeID == NoHandle {
		if proto, ok := vm.heap.GetOwnProperty(fr.callee.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
			homeID = proto.Handle()
		}
	}
	home := vm.heap.Get(homeID)
	if home == nil || home.proto == NoHandle {
		return Undefined, nil
	}
	v := vm.getProp(ObjectValue(home.proto), name)
	if v.IsObject() {
		if o := vm.heap.Get(v.Handle()); o != nil && o.kind == KindAccessor {
			if len(o.elements) == 0 || !vm.isFunction(o.elements[0]) {
				return Undefined, nil
			}
			return vm.invoke(vm.CurrentThis(), o.elements[0], nil)
		}
	}
	return v, nil
}

func (vm *VM) deleteKey(obj Value, name *str.String) bool {
	if !obj.IsObject() {
		return true
	}
	if ok, handled := vm.proxyDelete(obj, name); handled {
		return ok
	}
	return vm.heap.DeleteProperty(obj.Handle(), name)
}

func (vm *VM) deleteGlobal(name *str.String) bool {
	k := vm.heap.Intern().Intern(name)
	delete(vm.globals, k)
	if vm.globalObj != NoHandle {
		vm.heap.DeleteProperty(vm.globalObj, k)
	}
	return true
}

func (vm *VM) superCall(fr *frame, argc int) error {
	this := vm.CurrentThis()
	ctor := Undefined
	thisRoot := this
	vm.heap.AddRoot(&thisRoot)
	defer vm.heap.RemoveRoot(&thisRoot)
	// super() resolves from the currently executing constructor, not the
	// most-derived instance prototype (which recurses at the second level).
	if fr != nil && fr.callee.IsObject() {
		if o := vm.heap.Get(fr.callee.Handle()); o != nil && o.proto != NoHandle {
			ctor = ObjectValue(o.proto)
		}
	}
	args := make([]Value, argc)
	for i := argc - 1; i >= 0; i-- {
		args[i] = vm.pop()
	}
	vm.heap.AddRoot(&ctor)
	defer vm.heap.RemoveRoot(&ctor)
	vm.push(ctor)
	for _, a := range args {
		vm.push(a)
	}
	nt := Undefined
	if fr != nil {
		nt = fr.newTarget
	}
	return vm.callInternal(fr, argc, this, this, nt)
}

func (vm *VM) startAsync(ph Handle, gen, sent Value) {
	vm.heap.AddRoot(&gen)
	defer vm.heap.RemoveRoot(&gen)
	_ = vm.stepAsync(ph, gen, sent)
}

func (vm *VM) stepAsync(ph Handle, gen, sent Value) error {
	r, err := vm.generatorResume(gen, sent, genResumeNext)
	if err != nil {
		msg := err.Error()
		if th, ok := err.(*Throw); ok {
			vm.settlePromise(ph, promiseRejected, th.Value)
			return nil
		}
		vm.settlePromise(ph, promiseRejected, vm.NewStringValue(str.FromGo(msg)))
		return nil
	}
	done := vm.truthy(vm.getProp(r, str.FromGo("done")))
	val := vm.getProp(r, str.FromGo("value"))
	if done {
		vm.settlePromise(ph, promiseFulfilled, val)
		return nil
	}
	if vm.promiseOf(val) != nil {
		thenFn := vm.getProp(val, str.FromGo("then"))
		onF := ObjectValue(vm.heap.NewFunction(&Chunk{
			Name:   "asyncThen",
			Params: 1,
			Native: func(vm *VM, args []Value) (Value, error) {
				v := Undefined
				if len(args) > 0 {
					v = args[0]
				}
				return Undefined, vm.stepAsync(ph, gen, v)
			},
		}, NoHandle))
		onR := ObjectValue(vm.heap.NewFunction(&Chunk{
			Name:   "asyncCatch",
			Params: 1,
			Native: func(vm *VM, args []Value) (Value, error) {
				v := Undefined
				if len(args) > 0 {
					v = args[0]
				}
				vm.settlePromise(ph, promiseRejected, v)
				return Undefined, nil
			},
		}, NoHandle))
		_, err := vm.CallFunction(thenFn, val, []Value{onF, onR})
		return err
	}
	gv := gen
	pv := ObjectValue(ph)
	sv := val
	vm.EnqueueMicrotask(func() error {
		return vm.stepAsync(ph, gv, sv)
	}, gv, pv, sv)
	return nil
}
