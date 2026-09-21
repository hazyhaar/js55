// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"fmt"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

const (
	promisePending promiseState = iota
	promiseFulfilled
	promiseRejected
)

type promiseState uint8

type promiseReaction struct {
	onFulfilled Value
	onRejected  Value
	child       Value
}

type PromiseData struct {
	state     promiseState
	result    Value
	reactions []promiseReaction
	settled   bool
}

func (vm *VM) InstallPromiseBuiltins() {
	proto := vm.heap.NewObject()
	protoV := ObjectValue(proto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)
	vm.promiseProto = proto

	ctor := vm.heap.NewFunction(&Chunk{
		Name:      "Promise",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.ctorThis()
			ph := this.Handle()
			if o := vm.heap.Get(ph); o != nil {
				o.kind = KindPromise
				o.promise = &PromiseData{state: promisePending}
				if o.proto == NoHandle {
					o.proto = vm.promiseProto
				}
			}
			if len(args) == 0 || !vm.isFunction(args[0]) {
				return Undefined, fmt.Errorf("TypeError: Promise resolver is not a function")
			}
			resolve := vm.promiseSettleFn(ph, promiseFulfilled)
			reject := vm.promiseSettleFn(ph, promiseRejected)
			if _, err := vm.CallFunction(args[0], Undefined, []Value{resolve, reject}); err != nil {
				vm.settlePromise(ph, promiseRejected, vm.NewStringValue(str.FromGo(err.Error())))
			}
			return this, nil
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), protoV)

	vm.defineNative(ctor, "resolve", 1, func(vm *VM, args []Value) (Value, error) {
		v := Undefined
		if len(args) > 0 {
			v = args[0]
		}
		if vm.promiseOf(v) != nil {
			return v, nil
		}
		h := vm.newPromise()
		vm.settlePromise(h, promiseFulfilled, v)
		return ObjectValue(h), nil
	})
	vm.defineNative(ctor, "reject", 1, func(vm *VM, args []Value) (Value, error) {
		v := Undefined
		if len(args) > 0 {
			v = args[0]
		}
		h := vm.newPromise()
		vm.settlePromise(h, promiseRejected, v)
		return ObjectValue(h), nil
	})
	vm.defineNative(ctor, "all", 1, func(vm *VM, args []Value) (Value, error) {
		return vm.promiseAll(args, false, false)
	})
	vm.defineNative(ctor, "race", 1, func(vm *VM, args []Value) (Value, error) {
		return vm.promiseAll(args, true, false)
	})
	vm.defineNative(ctor, "allSettled", 1, func(vm *VM, args []Value) (Value, error) {
		return vm.promiseAll(args, false, true)
	})

	vm.defineNative(proto, "then", 2, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		pd := vm.promiseOf(this)
		if pd == nil {
			return Undefined, fmt.Errorf("TypeError: Method Promise.prototype.then called on incompatible receiver")
		}
		var onFulfilled, onRejected Value
		if len(args) > 0 {
			onFulfilled = args[0]
		}
		if len(args) > 1 {
			onRejected = args[1]
		}
		child := vm.newPromise()
		childV := ObjectValue(child)
		switch pd.state {
		case promiseFulfilled:
			vm.enqueueReaction(onFulfilled, pd.result, childV, false)
		case promiseRejected:
			vm.enqueueReaction(onRejected, pd.result, childV, true)
		default:
			pd.reactions = append(pd.reactions, promiseReaction{
				onFulfilled: onFulfilled,
				onRejected:  onRejected,
				child:       childV,
			})
		}
		return childV, nil
	})
	vm.defineNative(proto, "catch", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		thenFn := vm.getProp(this, vm.heap.Intern().InternGo("then"))
		var onRejected Value
		if len(args) > 0 {
			onRejected = args[0]
		}
		return vm.CallFunction(thenFn, this, []Value{Undefined, onRejected})
	})

	vm.SetGlobal("Promise", ctorV)
}

func (vm *VM) ctorThis() Value {
	this := vm.CurrentThis()
	if !this.IsObject() || vm.IsString(this) || this.Handle() == vm.globalObj {
		return ObjectValue(vm.heap.NewObject())
	}
	return this
}

func (vm *VM) newPromise() Handle {
	h := vm.heap.NewObject()
	if o := vm.heap.Get(h); o != nil {
		o.kind = KindPromise
		o.promise = &PromiseData{state: promisePending}
		o.proto = vm.promiseProto
	}
	return h
}

func (vm *VM) promiseOf(v Value) *PromiseData {
	if !v.IsObject() {
		return nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind != KindPromise {
		return nil
	}
	return o.promise
}

func (vm *VM) promiseSettleFn(ph Handle, state promiseState) Value {
	ch := &Chunk{
		Name:   "settle",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			v := Undefined
			if len(args) > 0 {
				v = args[0]
			}
			vm.settlePromise(ph, state, v)
			return Undefined, nil
		},
	}
	return ObjectValue(vm.heap.NewFunction(ch, NoHandle))
}

func (vm *VM) settlePromise(ph Handle, state promiseState, result Value) {
	o := vm.heap.Get(ph)
	if o == nil || o.promise == nil || o.promise.settled {
		return
	}
	pd := o.promise
	pd.settled = true
	pd.state = state
	pd.result = result
	reactions := pd.reactions
	pd.reactions = nil
	self := ObjectValue(ph)
	for _, r := range reactions {
		if state == promiseFulfilled {
			vm.enqueueReaction(r.onFulfilled, result, r.child, false)
		} else {
			vm.enqueueReaction(r.onRejected, result, r.child, true)
		}
	}
	_ = self
}

func (vm *VM) promiseAll(args []Value, race, settled bool) (Value, error) {
	out := vm.newPromise()
	outV := ObjectValue(out)
	if len(args) == 0 {
		vm.settlePromise(out, promiseFulfilled, ObjectValue(vm.heap.NewArray(0)))
		return outV, nil
	}
	n := vm.arrayLikeLength(args[0])
	if n <= 0 {
		vm.settlePromise(out, promiseFulfilled, ObjectValue(vm.heap.NewArray(0)))
		return outV, nil
	}
	results := vm.heap.NewArray(n)
	remaining := n
	var finished bool
	thenKey := vm.heap.Intern().InternGo("then")
	statusKey := vm.heap.Intern().InternGo("status")
	valueKey := vm.heap.Intern().InternGo("value")
	reasonKey := vm.heap.Intern().InternGo("reason")
	fulfilledS := vm.NewStringValue(str.FromGo("fulfilled"))
	rejectedS := vm.NewStringValue(str.FromGo("rejected"))
	for i := 0; i < n; i++ {
		idx := i
		el, err := vm.arrayIterIndex(args[0], i)
		if err != nil {
			return outV, err
		}
		p := el
		if vm.promiseOf(p) == nil {
			h := vm.newPromise()
			vm.settlePromise(h, promiseFulfilled, el)
			p = ObjectValue(h)
		}
		onF := ObjectValue(vm.heap.NewFunction(&Chunk{
			Name:   "allOnF",
			Params: 1,
			Native: func(vm *VM, a []Value) (Value, error) {
				if finished {
					return Undefined, nil
				}
				v := Undefined
				if len(a) > 0 {
					v = a[0]
				}
				if race {
					finished = true
					vm.settlePromise(out, promiseFulfilled, v)
					return Undefined, nil
				}
				if settled {
					rec := vm.heap.NewObject()
					vm.heap.SetProperty(rec, statusKey, fulfilledS)
					vm.heap.SetProperty(rec, valueKey, v)
					vm.heap.SetElement(results, idx, ObjectValue(rec))
				} else {
					vm.heap.SetElement(results, idx, v)
				}
				remaining--
				if remaining == 0 {
					finished = true
					vm.settlePromise(out, promiseFulfilled, ObjectValue(results))
				}
				return Undefined, nil
			},
		}, NoHandle))
		onR := ObjectValue(vm.heap.NewFunction(&Chunk{
			Name:   "allOnR",
			Params: 1,
			Native: func(vm *VM, a []Value) (Value, error) {
				if finished {
					return Undefined, nil
				}
				v := Undefined
				if len(a) > 0 {
					v = a[0]
				}
				if settled {
					rec := vm.heap.NewObject()
					vm.heap.SetProperty(rec, statusKey, rejectedS)
					vm.heap.SetProperty(rec, reasonKey, v)
					vm.heap.SetElement(results, idx, ObjectValue(rec))
					remaining--
					if remaining == 0 {
						finished = true
						vm.settlePromise(out, promiseFulfilled, ObjectValue(results))
					}
					return Undefined, nil
				}
				finished = true
				vm.settlePromise(out, promiseRejected, v)
				return Undefined, nil
			},
		}, NoHandle))
		thenFn := vm.getProp(p, thenKey)
		if _, err := vm.CallFunction(thenFn, p, []Value{onF, onR}); err != nil {
			return outV, err
		}
	}
	return outV, nil
}

func (vm *VM) enqueueReaction(cb, value, child Value, isReject bool) {
	vm.EnqueueMicrotask(func() error {
		if !vm.isFunction(cb) {
			st := promiseFulfilled
			if isReject {
				st = promiseRejected
			}
			if child.IsObject() {
				vm.settlePromise(child.Handle(), st, value)
			}
			return nil
		}
		res, err := vm.CallFunction(cb, Undefined, []Value{value})
		if !child.IsObject() {
			return nil
		}
		if err != nil {
			vm.settlePromise(child.Handle(), promiseRejected, vm.NewStringValue(str.FromGo(err.Error())))
			return nil
		}
		vm.settlePromise(child.Handle(), promiseFulfilled, res)
		return nil
	}, cb, value, child)
}
