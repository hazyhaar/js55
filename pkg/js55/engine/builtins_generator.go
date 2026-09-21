// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"strconv"
	"unicode/utf16"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

type GenState struct {
	ip        int
	stack     []Value
	this      Value
	delegate  Value
	sent      Value
	started   bool
	done      bool
	delegated bool
	throwing  bool
	returning bool
}

type yieldSuspend struct {
	value Value
	star  bool
	await bool
}

func (*yieldSuspend) Error() string { return "yield" }

func (vm *VM) InstallGeneratorBuiltins() {
	proto := vm.heap.NewObject()
	vm.generatorProto = proto
	protoV := ObjectValue(proto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)

	vm.defineNative(proto, "next", 1, func(vm *VM, args []Value) (Value, error) {
		sent := Undefined
		if len(args) > 0 {
			sent = args[0]
		}
		return vm.generatorResume(vm.CurrentThis(), sent, genResumeNext)
	})
	vm.defineNative(proto, "return", 1, func(vm *VM, args []Value) (Value, error) {
		sent := Undefined
		if len(args) > 0 {
			sent = args[0]
		}
		return vm.generatorResume(vm.CurrentThis(), sent, genResumeReturn)
	})
	vm.defineNative(proto, "throw", 1, func(vm *VM, args []Value) (Value, error) {
		sent := Undefined
		if len(args) > 0 {
			sent = args[0]
		}
		return vm.generatorResume(vm.CurrentThis(), sent, genResumeThrow)
	})
	itCh := &Chunk{Name: "[Symbol.iterator]", Params: 0, Native: func(vm *VM, args []Value) (Value, error) {
		return vm.CurrentThis(), nil
	}}
	itFn := ObjectValue(vm.heap.NewFunction(itCh, NoHandle))
	vm.heap.AddRoot(&itFn)
	vm.heap.SetProperty(proto, SymbolIterator, itFn)
	vm.heap.RemoveRoot(&itFn)
}

const (
	genResumeNext = iota
	genResumeReturn
	genResumeThrow
)

func (vm *VM) newGenerator(fn *Chunk, env Handle, thisVal Value) Value {
	h := vm.heap.NewObject()
	if o := vm.heap.Get(h); o != nil {
		o.kind = KindGenerator
		o.fn = fn
		o.env = env
		o.proto = vm.generatorProto
		o.gen = &GenState{this: thisVal}
	}
	return ObjectValue(h)
}

func (vm *VM) iterResult(value Value, done bool) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("value"), value)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("done"), Bool(done))
	return hv
}

func (vm *VM) generatorResume(gen Value, sent Value, mode int) (Value, error) {
	if !gen.IsObject() {
		return Undefined, &Throw{Value: vm.NewStringValue(str.FromGo("TypeError: not a generator")), Text: "TypeError: not a generator"}
	}
	o := vm.heap.Get(gen.Handle())
	if o == nil || o.kind != KindGenerator || o.gen == nil {
		return Undefined, &Throw{Value: vm.NewStringValue(str.FromGo("TypeError: not a generator")), Text: "TypeError: not a generator"}
	}
	gs := o.gen
	if gs.done {
		if mode == genResumeThrow {
			return Undefined, &Throw{Value: sent, Text: vm.toDisplayString(sent)}
		}
		return vm.iterResult(sent, true), nil
	}
	if mode == genResumeReturn {
		vm.genReturning = true
		defer func() { vm.genReturning = false }()
		if gs.delegate.IsObject() {
			if retFn := vm.getProp(gs.delegate, str.FromGo("return")); vm.isFunction(retFn) {
				if _, err := vm.invoke(gs.delegate, retFn, []Value{sent}); err != nil {
					gs.done = true
					gs.delegate = Undefined
					return Undefined, err
				}
			}
		}
		if gs.started {
			gs.returning = true
			gs.sent = sent
			return vm.generatorRunReturn(o, sent)
		}
		gs.done = true
		gs.delegate = Undefined
		return vm.iterResult(sent, true), nil
	}
	if mode == genResumeThrow {
		if gs.delegate.IsObject() {
			if thFn := vm.getProp(gs.delegate, str.FromGo("throw")); vm.isFunction(thFn) {
				r, err := vm.invoke(gs.delegate, thFn, []Value{sent})
				if err != nil {
					gs.done = true
					gs.delegate = Undefined
					return Undefined, err
				}
				return vm.yieldStarHandle(o, r)
			}
		}
		if !gs.started {
			gs.done = true
			return Undefined, &Throw{Value: sent, Text: vm.toDisplayString(sent)}
		}
		gs.throwing = true
		gs.sent = sent
		return vm.generatorRun(o, sent)
	}
	if gs.delegate.IsObject() {
		return vm.yieldStarNext(o, sent)
	}
	return vm.generatorRun(o, sent)
}

func (vm *VM) generatorRunReturn(o *Object, sent Value) (Value, error) {
	gs := o.gen
	vm.heap.AddRoot(&sent)
	defer vm.heap.RemoveRoot(&sent)
	min := len(vm.frames)
	vm.thisStack = append(vm.thisStack, gs.this)
	vm.newStack = append(vm.newStack, Undefined)
	base := vm.heap.StackLen()
	for _, v := range gs.stack {
		vm.push(v)
	}
	vm.frames = append(vm.frames, frame{chunk: o.fn, env: o.env, ip: gs.ip, base: base})
	th := &Throw{Value: vm.returnTok, Text: "Return"}
	if vm.unwindToCatch(th, min) {
		_, err := vm.interpret(min)
		if err != nil {
			if th2, ok := err.(*Throw); ok && th2.Value == vm.returnTok {
				err = nil
			} else if th2, ok := err.(*Throw); ok {
				vm.generatorUnwindFrame(min, base)
				gs.done = true
				return Undefined, th2
			} else {
				vm.generatorUnwindFrame(min, base)
				gs.done = true
				return Undefined, err
			}
		}
		if vm.jumpToFinally(min) {
			_, ferr := vm.interpret(min)
			vm.generatorUnwindFrame(min, base)
			gs.done = true
			if ferr != nil {
				if th3, ok := ferr.(*Throw); ok && th3.Value == vm.returnTok {
					return vm.iterResult(sent, true), nil
				}
				return Undefined, ferr
			}
			return vm.iterResult(sent, true), nil
		}
		vm.generatorUnwindFrame(min, base)
		gs.done = true
		return vm.iterResult(sent, true), nil
	}
	if vm.jumpToFinally(min) {
		_, err := vm.interpret(min)
		vm.generatorUnwindFrame(min, base)
		gs.done = true
		if err != nil {
			if _, ok := err.(*finished); ok {
				return vm.iterResult(sent, true), nil
			}
			if _, ok := err.(*yieldSuspend); ok {
				return vm.iterResult(sent, true), nil
			}
			return Undefined, err
		}
		return vm.iterResult(sent, true), nil
	}
	vm.generatorUnwindFrame(min, base)
	gs.done = true
	return vm.iterResult(sent, true), nil
}

func (vm *VM) generatorRun(o *Object, sent Value) (Value, error) {
	gs := o.gen
	vm.heap.AddRoot(&sent)
	defer vm.heap.RemoveRoot(&sent)
	min := len(vm.frames)
	vm.thisStack = append(vm.thisStack, gs.this)
	vm.newStack = append(vm.newStack, Undefined)
	base := vm.heap.StackLen()
	for _, v := range gs.stack {
		vm.push(v)
	}
	if gs.returning {
		gs.returning = false
		retv := gs.sent
		vm.frames = append(vm.frames, frame{chunk: o.fn, env: o.env, ip: gs.ip, base: base})
		if vm.jumpToFinally(min) {
			_, err := vm.interpret(min)
			vm.generatorUnwindFrame(min, base)
			gs.done = true
			if err != nil {
				return Undefined, err
			}
			return vm.iterResult(retv, true), nil
		}
		vm.generatorUnwindFrame(min, base)
		gs.done = true
		return vm.iterResult(retv, true), nil
	}
	if gs.throwing {
		gs.throwing = false
		th := &Throw{Value: gs.sent, Text: vm.toDisplayString(gs.sent)}
		gs.sent = Undefined
		vm.frames = append(vm.frames, frame{chunk: o.fn, env: o.env, ip: gs.ip, base: base})
		if vm.unwindTo(th, min) {
			return vm.generatorFinishInterpret(o, min, base)
		}
		vm.generatorUnwindFrame(min, base)
		gs.done = true
		return Undefined, th
	}
	if gs.started {
		vm.push(sent)
	}
	gs.started = true
	vm.frames = append(vm.frames, frame{chunk: o.fn, env: o.env, ip: gs.ip, base: base})
	return vm.generatorFinishInterpret(o, min, base)
}

func (vm *VM) generatorFinishInterpret(o *Object, min, base int) (Value, error) {
	gs := o.gen
	v, err := vm.interpret(min)
	if ys, ok := err.(*yieldSuspend); ok {
		vm.generatorSave(o, min, base)
		yielded := ys.value
		vm.heap.AddRoot(&yielded)
		defer vm.heap.RemoveRoot(&yielded)
		if ys.star {
			it, ierr := vm.getIterator(yielded)
			if ierr != nil {
				gs.done = true
				return Undefined, ierr
			}
			gs.delegate = it
			gs.delegated = false
			return vm.yieldStarNext(o, Undefined)
		}
		return vm.iterResult(yielded, false), nil
	}
	vm.generatorUnwindFrame(min, base)
	gs.done = true
	gs.stack = nil
	if err != nil {
		return Undefined, err
	}
	return vm.iterResult(v, true), nil
}

func (vm *VM) generatorSave(o *Object, min, base int) {
	gs := o.gen
	if len(vm.frames) > min {
		fr := vm.frames[len(vm.frames)-1]
		gs.ip = fr.ip
		st := vm.heap.stack
		if fr.base < len(st) {
			gs.stack = append([]Value{}, st[fr.base:]...)
		} else {
			gs.stack = nil
		}
	}
	vm.generatorUnwindFrame(min, base)
}

func (vm *VM) generatorUnwindFrame(min, base int) {
	if len(vm.frames) > min {
		vm.frames = vm.frames[:min]
	}
	if len(vm.thisStack) > 1 {
		vm.thisStack = vm.thisStack[:len(vm.thisStack)-1]
	}
	if len(vm.newStack) > 1 {
		vm.newStack = vm.newStack[:len(vm.newStack)-1]
	}
	vm.heap.TruncateStack(base)
}

func (vm *VM) yieldStarNext(o *Object, sent Value) (Value, error) {
	gs := o.gen
	it := gs.delegate
	nextFn := vm.getProp(it, str.FromGo("next"))
	var args []Value
	if gs.delegated {
		args = []Value{sent}
	}
	gs.delegated = true
	r, err := vm.invoke(it, nextFn, args)
	if err != nil {
		gs.done = true
		gs.delegate = Undefined
		return Undefined, err
	}
	return vm.yieldStarHandle(o, r)
}

func (vm *VM) yieldStarHandle(o *Object, r Value) (Value, error) {
	gs := o.gen
	done := vm.truthy(vm.getProp(r, str.FromGo("done")))
	val := vm.getProp(r, str.FromGo("value"))
	if done {
		gs.delegate = Undefined
		gs.delegated = false
		return vm.generatorRun(o, val)
	}
	return vm.iterResult(val, false), nil
}

func (vm *VM) jumpToFinally(min int) bool {
	if len(vm.frames) <= min {
		return false
	}
	fr := &vm.frames[len(vm.frames)-1]
	bestIdx := -1
	bestLen := int(1 << 30)
	for i, h := range fr.chunk.Handlers {
		if fr.ip >= h.Start && fr.ip <= h.End && h.FinallyIP >= 0 {
			curLen := h.End - h.Start
			if curLen < bestLen {
				bestLen = curLen
				bestIdx = i
			}
		}
	}
	if bestIdx < 0 {
		return false
	}
	h := fr.chunk.Handlers[bestIdx]
	vm.heap.TruncateStack(fr.base)
	fr.ip = h.FinallyIP
	if h.FinallyEnd > h.FinallyIP {
		vm.stopIP = h.FinallyEnd
	}
	return true
}

func (vm *VM) retain(v Value) Value {
	vm.keep = v
	return v
}

func (vm *VM) getIterator(v Value) (Value, error) {
	vm.heap.AddRoot(&v)
	defer vm.heap.RemoveRoot(&v)
	if s := vm.StringOf(v); s != nil {
		arr := vm.heap.NewArray(0)
		av := ObjectValue(arr)
		vm.heap.AddRoot(&av)
		defer vm.heap.RemoveRoot(&av)
		for i := 0; i < s.Len(); {
			r, w := s.CodePointAt(i)
			var ch Value
			if w == 2 {
				u1, u2 := utf16.EncodeRune(r)
				ch = vm.NewStringValue(str.FromUTF16([]uint16{uint16(u1), uint16(u2)}))
			} else {
				ch = vm.NewStringValue(s.Slice(i, i+1))
			}
			vm.heap.AddRoot(&ch)
			vm.heap.SetElement(arr, len(vm.heap.Get(arr).elements), ch)
			vm.heap.RemoveRoot(&ch)
			i += w
		}
		return vm.retain(vm.newArrayIterator(av)), nil
	}
	if v.IsNull() || v.IsUndefined() {
		return Undefined, vm.throwNamedError(&frame{}, "TypeError", "TypeError: not iterable")
	}
	if v.IsObject() {
		meth := vm.getProp(v, SymbolIterator)
		if vm.isFunction(meth) {
			it, err := vm.CallFunction(meth, v, nil)
			if err != nil {
				return Undefined, err
			}
			return vm.retain(it), nil
		}
		if !meth.IsUndefined() && !meth.IsNull() {
			return Undefined, vm.throwNamedError(&frame{}, "TypeError", "TypeError: not iterable")
		}
		if o := vm.heap.Get(v.Handle()); o != nil && o.kind == KindGenerator {
			return vm.retain(v), nil
		}
		if next := vm.getProp(v, str.FromGo("next")); vm.isFunction(next) {
			return vm.retain(v), nil
		}
	}
	return Undefined, vm.throwNamedError(&frame{}, "TypeError", "TypeError: not iterable")
}

func (vm *VM) wrapIterator(it Value) Value {
	if !it.IsObject() {
		return it
	}
	vm.heap.AddRoot(&it)
	defer vm.heap.RemoveRoot(&it)
	next := vm.getProp(it, str.FromGo("next"))
	vm.heap.AddRoot(&next)
	defer vm.heap.RemoveRoot(&next)
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	if o := vm.heap.Get(h); o != nil {
		o.elements = []Value{it, next}
	}
	vm.defineNative(h, "next", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		o := vm.heap.Get(this.Handle())
		if o == nil || len(o.elements) < 2 {
			return vm.iterResult(Undefined, true), nil
		}
		return vm.CallFunction(o.elements[1], o.elements[0], args)
	})
	vm.defineNative(h, "return", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		o := vm.heap.Get(this.Handle())
		if o == nil || len(o.elements) == 0 {
			return vm.iterResult(Undefined, true), nil
		}
		inner := o.elements[0]
		ret := vm.getProp(inner, str.FromGo("return"))
		if !vm.isFunction(ret) {
			return vm.iterResult(Undefined, true), nil
		}
		r, err := vm.CallFunction(ret, inner, args)
		if err != nil {
			return Undefined, err
		}
		if vm.genReturning && (!r.IsObject() || vm.IsString(r)) {
			return Undefined, vm.throwNamedError(&frame{}, "TypeError", "TypeError: iterator return() did not return an object")
		}
		return r, nil
	})
	return hv
}

func (vm *VM) newStringIterator(s *str.String) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	held := vm.NewStringValue(s)
	vm.heap.AddRoot(&held)
	defer vm.heap.RemoveRoot(&held)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("s"), held)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("i"), Int(0))
	vm.defineNative(h, "next", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		sv := vm.StringOf(vm.getProp(this, str.FromGo("s")))
		i := int(vm.toNumber(vm.getProp(this, str.FromGo("i"))))
		if sv == nil || i >= sv.Len() {
			return vm.iterResult(Undefined, true), nil
		}
		r, w := sv.CodePointAt(i)
		var ch Value
		if w == 2 {
			u1, u2 := utf16.EncodeRune(r)
			ch = vm.NewStringValue(str.FromUTF16([]uint16{uint16(u1), uint16(u2)}))
		} else {
			ch = vm.NewStringValue(sv.Slice(i, i+1))
		}
		vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("i"), Int(int32(i+w)))
		return vm.iterResult(ch, false), nil
	})
	return hv
}

func (vm *VM) newArrayIterator(arr Value) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("arr"), arr)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("i"), Int(0))
	vm.defineNative(h, "next", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		arr := vm.getProp(this, str.FromGo("arr"))
		i := int(vm.toNumber(vm.getProp(this, str.FromGo("i"))))
		if !arr.IsObject() {
			return vm.iterResult(Undefined, true), nil
		}
		if vm.taOutOfBounds(arr) {
			return Undefined, vm.throwText(&frame{}, "TypeError: Cannot perform TypedArray index get on a typed array that is out of bounds")
		}
		n := vm.arrayLikeLength(arr)
		if i >= n {
			return vm.iterResult(Undefined, true), nil
		}
		el, err := vm.arrayIterIndex(arr, i)
		if err != nil {
			return Undefined, err
		}
		vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("i"), Int(int32(i+1)))
		return vm.iterResult(el, false), nil
	})
	return hv
}

func (vm *VM) arrayIterIndex(arr Value, i int) (Value, error) {
	if arr.IsObject() {
		if o := vm.heap.Get(arr.Handle()); o != nil && o.kind == KindArray && o.env != NoHandle {
			return vm.typedArrayIndex(arr, i)
		}
	}
	name := vm.heap.Intern().InternGo(strconv.Itoa(i))
	if own, ok := vm.heap.GetOwnProperty(arr.Handle(), name); ok {
		if own.IsObject() {
			if acc := vm.heap.Get(own.Handle()); acc != nil && acc.kind == KindAccessor {
				return vm.getPropInvoke(arr, name)
			}
		}
		return own, nil
	}
	if o := vm.heap.Get(arr.Handle()); o != nil && o.kind == KindArray && i >= 0 && i < len(o.elements) {
		return o.elements[i], nil
	}
	return vm.getPropInvoke(arr, name)
}

func (vm *VM) newArrayKeyIterator(arr Value) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("arr"), arr)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("i"), Int(0))
	vm.defineNative(h, "next", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		arr := vm.getProp(this, str.FromGo("arr"))
		i := int(vm.toNumber(vm.getProp(this, str.FromGo("i"))))
		if !arr.IsObject() {
			return vm.iterResult(Undefined, true), nil
		}
		o := vm.heap.Get(arr.Handle())
		if o == nil || o.kind != KindArray || i >= len(o.elements) {
			return vm.iterResult(Undefined, true), nil
		}
		vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("i"), Int(int32(i+1)))
		return vm.iterResult(Int(int32(i)), false), nil
	})
	return hv
}

func (vm *VM) newArrayEntryIterator(arr Value) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("arr"), arr)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("i"), Int(0))
	vm.defineNative(h, "next", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		arr := vm.getProp(this, str.FromGo("arr"))
		i := int(vm.toNumber(vm.getProp(this, str.FromGo("i"))))
		if !arr.IsObject() {
			return vm.iterResult(Undefined, true), nil
		}
		o := vm.heap.Get(arr.Handle())
		if o == nil || o.kind != KindArray || i >= len(o.elements) {
			return vm.iterResult(Undefined, true), nil
		}
		el := o.elements[i]
		pair := vm.heap.NewArray(2)
		pv := ObjectValue(pair)
		vm.heap.AddRoot(&pv)
		defer vm.heap.RemoveRoot(&pv)
		vm.heap.SetElement(pair, 0, Int(int32(i)))
		vm.heap.SetElement(pair, 1, el)
		vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("i"), Int(int32(i+1)))
		return vm.iterResult(pv, false), nil
	})
	return hv
}
