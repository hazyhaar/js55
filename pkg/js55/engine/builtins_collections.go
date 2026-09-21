// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import "fmt"

// MapData stores Key-Value pairs for JS Map.
type MapData struct {
	Keys   []Value
	Values []Value
}

// SetData stores unique elements for JS Set.
type SetData struct {
	Elements []Value
}

func (vm *VM) InstallCollectionsBuiltins() {
	mapProto := vm.heap.NewObject()
	mapProtoV := ObjectValue(mapProto)
	vm.heap.AddRoot(&mapProtoV)
	defer vm.heap.RemoveRoot(&mapProtoV)
	mapCtor := vm.heap.NewFunction(&Chunk{
		Name:      "Map",
		Params:    0,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			if !this.IsObject() {
				this = ObjectValue(vm.heap.NewObject())
			}
			if o := vm.heap.Get(this.Handle()); o != nil {
				o.kind = KindMap
				o.mapData = &MapData{}
			}
			return this, nil
		},
	}, NoHandle)
	mapCtorV := ObjectValue(mapCtor)
	vm.heap.AddRoot(&mapCtorV)
	defer vm.heap.RemoveRoot(&mapCtorV)
	vm.heap.SetProperty(mapCtor, vm.heap.Intern().InternGo("prototype"), mapProtoV)
	vm.SetGlobal("Map", mapCtorV)
	vm.installMapMethods(mapProto)

	setProto := vm.heap.NewObject()
	setProtoV := ObjectValue(setProto)
	vm.heap.AddRoot(&setProtoV)
	defer vm.heap.RemoveRoot(&setProtoV)
	setCtor := vm.heap.NewFunction(&Chunk{
		Name:      "Set",
		Construct: true,
		Params:    0,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			if !this.IsObject() {
				this = ObjectValue(vm.heap.NewObject())
			}
			if o := vm.heap.Get(this.Handle()); o != nil {
				o.kind = KindSet
				o.setData = &SetData{}
			}
			return this, nil
		},
	}, NoHandle)
	setCtorV := ObjectValue(setCtor)
	vm.heap.AddRoot(&setCtorV)
	defer vm.heap.RemoveRoot(&setCtorV)
	vm.heap.SetProperty(setCtor, vm.heap.Intern().InternGo("prototype"), setProtoV)
	vm.SetGlobal("Set", setCtorV)
	vm.installSetMethods(setProto)

	wmProto := vm.heap.NewObject()
	wmProtoV := ObjectValue(wmProto)
	vm.heap.AddRoot(&wmProtoV)
	defer vm.heap.RemoveRoot(&wmProtoV)
	wmCtor := vm.heap.NewFunction(&Chunk{
		Name:      "WeakMap",
		Params:    0,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			if !this.IsObject() {
				this = ObjectValue(vm.heap.NewObject())
			}
			if o := vm.heap.Get(this.Handle()); o != nil {
				o.kind = KindWeakMap
				o.mapData = &MapData{}
			}
			return this, nil
		},
	}, NoHandle)
	wmCtorV := ObjectValue(wmCtor)
	vm.heap.AddRoot(&wmCtorV)
	defer vm.heap.RemoveRoot(&wmCtorV)
	vm.heap.SetProperty(wmCtor, vm.heap.Intern().InternGo("prototype"), wmProtoV)
	vm.SetGlobal("WeakMap", wmCtorV)
	vm.installMapMethods(wmProto)
}

func (vm *VM) installMapMethods(proto Handle) {
	setMeth := func(name string, n int, fn func(*VM, []Value) (Value, error)) {
		ch := &Chunk{Name: name, Params: n, Native: fn}
		fv := ObjectValue(vm.heap.NewFunction(ch, NoHandle))
		vm.heap.AddRoot(&fv)
		vm.heap.SetProperty(proto, vm.heap.Intern().InternGo(name), fv)
		vm.heap.RemoveRoot(&fv)
	}
	setMeth("set", 2, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		o := vm.mapOf(this)
		if o == nil {
			return Undefined, nil
		}
		var k, v Value
		if len(args) > 0 {
			k = args[0]
		}
		if len(args) > 1 {
			v = args[1]
		}
		if obj := vm.heap.Get(this.Handle()); obj != nil && obj.kind == KindWeakMap && !k.IsObject() {
			return this, nil
		}
		o.MapSet(k, v)
		return this, nil
	})
	setMeth("get", 1, func(vm *VM, args []Value) (Value, error) {
		o := vm.mapOf(vm.CurrentThis())
		if o == nil {
			return Undefined, nil
		}
		var k Value
		if len(args) > 0 {
			k = args[0]
		}
		v, _ := o.MapGet(k)
		return v, nil
	})
	setMeth("has", 1, func(vm *VM, args []Value) (Value, error) {
		o := vm.mapOf(vm.CurrentThis())
		if o == nil {
			return False, nil
		}
		var k Value
		if len(args) > 0 {
			k = args[0]
		}
		_, ok := o.MapGet(k)
		return Bool(ok), nil
	})
	setMeth("delete", 1, func(vm *VM, args []Value) (Value, error) {
		o := vm.mapOf(vm.CurrentThis())
		if o == nil {
			return False, nil
		}
		var k Value
		if len(args) > 0 {
			k = args[0]
		}
		return Bool(o.MapDelete(k)), nil
	})
	setMeth("forEach", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		o := vm.mapOf(this)
		if o == nil || len(args) == 0 || !vm.isFunction(args[0]) {
			return Undefined, fmt.Errorf("TypeError: Map.prototype.forEach callback is not a function")
		}
		thisArg := Undefined
		if len(args) > 1 {
			thisArg = args[1]
		}
		cb := args[0]
		for i := range o.Keys {
			if _, err := vm.invoke(thisArg, cb, []Value{o.Values[i], o.Keys[i], this}); err != nil {
				return Undefined, err
			}
		}
		return Undefined, nil
	})
	setMeth("entries", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.newMapIterator(vm.CurrentThis()), nil
	})
	if ent, ok := vm.heap.GetProperty(proto, vm.heap.Intern().InternGo("entries")); ok {
		vm.heap.SetProperty(proto, SymbolIterator, ent)
	}
}

func (vm *VM) installSetMethods(proto Handle) {
	setMeth := func(name string, n int, fn func(*VM, []Value) (Value, error)) {
		ch := &Chunk{Name: name, Params: n, Native: fn}
		fv := ObjectValue(vm.heap.NewFunction(ch, NoHandle))
		vm.heap.AddRoot(&fv)
		vm.heap.SetProperty(proto, vm.heap.Intern().InternGo(name), fv)
		vm.heap.RemoveRoot(&fv)
	}
	setMeth("add", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		o := vm.setOf(this)
		if o == nil {
			return this, nil
		}
		var v Value
		if len(args) > 0 {
			v = args[0]
		}
		o.SetAdd(v)
		return this, nil
	})
	setMeth("has", 1, func(vm *VM, args []Value) (Value, error) {
		o := vm.setOf(vm.CurrentThis())
		if o == nil {
			return False, nil
		}
		var v Value
		if len(args) > 0 {
			v = args[0]
		}
		return Bool(o.SetHas(v)), nil
	})
	setMeth("delete", 1, func(vm *VM, args []Value) (Value, error) {
		o := vm.setOf(vm.CurrentThis())
		if o == nil {
			return False, nil
		}
		var v Value
		if len(args) > 0 {
			v = args[0]
		}
		return Bool(o.SetDelete(v)), nil
	})
	setMeth("forEach", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		o := vm.setOf(this)
		if o == nil || len(args) == 0 || !vm.isFunction(args[0]) {
			return Undefined, fmt.Errorf("TypeError: Set.prototype.forEach callback is not a function")
		}
		thisArg := Undefined
		if len(args) > 1 {
			thisArg = args[1]
		}
		cb := args[0]
		for _, el := range o.Elements {
			if _, err := vm.invoke(thisArg, cb, []Value{el, el, this}); err != nil {
				return Undefined, err
			}
		}
		return Undefined, nil
	})
	setMeth("values", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.newSetIterator(vm.CurrentThis()), nil
	})
	if valFn, ok := vm.heap.GetProperty(proto, vm.heap.Intern().InternGo("values")); ok {
		vm.heap.SetProperty(proto, SymbolIterator, valFn)
	}
}

func (vm *VM) mapOf(v Value) *MapData {
	if !v.IsObject() {
		return nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.mapData == nil {
		return nil
	}
	return o.mapData
}

func (vm *VM) setOf(v Value) *SetData {
	if !v.IsObject() {
		return nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.setData == nil {
		return nil
	}
	return o.setData
}

func (m *MapData) MapSet(k, v Value) {
	for i, key := range m.Keys {
		if key == k {
			m.Values[i] = v
			return
		}
	}
	m.Keys = append(m.Keys, k)
	m.Values = append(m.Values, v)
}

func (m *MapData) MapGet(k Value) (Value, bool) {
	for i, key := range m.Keys {
		if key == k {
			return m.Values[i], true
		}
	}
	return Undefined, false
}

func (m *MapData) MapDelete(k Value) bool {
	for i, key := range m.Keys {
		if key == k {
			m.Keys = append(m.Keys[:i], m.Keys[i+1:]...)
			m.Values = append(m.Values[:i], m.Values[i+1:]...)
			return true
		}
	}
	return false
}

func (s *SetData) SetAdd(v Value) {
	for _, el := range s.Elements {
		if el == v {
			return
		}
	}
	s.Elements = append(s.Elements, v)
}

func (s *SetData) SetHas(v Value) bool {
	for _, el := range s.Elements {
		if el == v {
			return true
		}
	}
	return false
}

func (s *SetData) SetDelete(v Value) bool {
	for i, el := range s.Elements {
		if el == v {
			s.Elements = append(s.Elements[:i], s.Elements[i+1:]...)
			return true
		}
	}
	return false
}

func collectionSize(v Value, h *Heap) (Value, bool) {
	if !v.IsObject() {
		return Undefined, false
	}
	o := h.Get(v.Handle())
	if o == nil {
		return Undefined, false
	}
	if o.mapData != nil {
		return Int(int32(len(o.mapData.Keys))), true
	}
	if o.setData != nil {
		return Int(int32(len(o.setData.Elements))), true
	}
	return Undefined, false
}

func (vm *VM) newMapIterator(m Value) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("m"), m)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("i"), Int(0))
	vm.defineNative(h, "next", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		mp := vm.getProp(this, vm.heap.Intern().InternGo("m"))
		i := int(vm.toNumber(vm.getProp(this, vm.heap.Intern().InternGo("i"))))
		md := vm.mapOf(mp)
		if md == nil || i >= len(md.Keys) {
			return vm.iterResult(Undefined, true), nil
		}
		pair := vm.heap.NewArray(2)
		pv := ObjectValue(pair)
		vm.heap.AddRoot(&pv)
		defer vm.heap.RemoveRoot(&pv)
		vm.heap.SetElement(pair, 0, md.Keys[i])
		vm.heap.SetElement(pair, 1, md.Values[i])
		vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("i"), Int(int32(i+1)))
		return vm.iterResult(pv, false), nil
	})
	return hv
}

func (vm *VM) newSetIterator(s Value) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("s"), s)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("i"), Int(0))
	vm.defineNative(h, "next", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		st := vm.getProp(this, vm.heap.Intern().InternGo("s"))
		i := int(vm.toNumber(vm.getProp(this, vm.heap.Intern().InternGo("i"))))
		sd := vm.setOf(st)
		if sd == nil || i >= len(sd.Elements) {
			return vm.iterResult(Undefined, true), nil
		}
		el := sd.Elements[i]
		vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("i"), Int(int32(i+1)))
		return vm.iterResult(el, false), nil
	})
	return hv
}
