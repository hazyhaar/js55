// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"sync"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// RootRealm est le royaume racine gelé du processus. Les intrinsèques ECMAScript
// y sont installés une seule fois. Chaque isolat en hérite par référence et ne
// les modifie qu'en déclenchant une copie locale.
type RootRealm struct {
	Heap           *Heap
	Globals        map[*str.String]Value
	GlobalObj      Handle
	ObjectProto    Handle
	ArrayProto     Handle
	FunctionProto  Handle
	PromiseProto   Handle
	RegexpProto    Handle
	GeneratorProto Handle
}

var (
	rootOnce         sync.Once
	DefaultRootRealm *RootRealm
)

func ensureRootRealm() {
	rootOnce.Do(buildRootRealm)
}

func buildRootRealm() {
	h := NewHeap()
	h.buildingRoot = true
	vm := NewVM(h)
	sealRoot(h, vm)
	DefaultRootRealm = &RootRealm{
		Heap:           h,
		Globals:        vm.globals,
		GlobalObj:      vm.globalObj,
		ObjectProto:    h.objectProto,
		ArrayProto:     h.arrayProto,
		FunctionProto:  h.functionProto,
		PromiseProto:   vm.promiseProto,
		RegexpProto:    vm.regexpProto,
		GeneratorProto: vm.generatorProto,
	}
}

func sealRoot(h *Heap, vm *VM) {
	seen := map[*Shape]bool{}
	for _, o := range h.objs {
		if o == nil {
			continue
		}
		o.isRoot = true
		o.proto = tagRoot(o.proto)
		o.env = tagRoot(o.env)
		o.homeObject = tagRoot(o.homeObject)
		for i := range o.slots {
			o.slots[i] = tagRootValue(o.slots[i])
		}
		for i := range o.elements {
			o.elements[i] = tagRootValue(o.elements[i])
		}
		o.prim = tagRootValue(o.prim)
		if o.promise != nil {
			o.promise.result = tagRootValue(o.promise.result)
			for i := range o.promise.reactions {
				o.promise.reactions[i].onFulfilled = tagRootValue(o.promise.reactions[i].onFulfilled)
				o.promise.reactions[i].onRejected = tagRootValue(o.promise.reactions[i].onRejected)
				o.promise.reactions[i].child = tagRootValue(o.promise.reactions[i].child)
			}
		}
		if o.proxy != nil {
			o.proxy.target = tagRootValue(o.proxy.target)
			o.proxy.handler = tagRootValue(o.proxy.handler)
		}
		if o.mapData != nil {
			for i := range o.mapData.Keys {
				o.mapData.Keys[i] = tagRootValue(o.mapData.Keys[i])
			}
			for i := range o.mapData.Values {
				o.mapData.Values[i] = tagRootValue(o.mapData.Values[i])
			}
		}
		if o.setData != nil {
			for i := range o.setData.Elements {
				o.setData.Elements[i] = tagRootValue(o.setData.Elements[i])
			}
		}
		markShapeShared(o.shape, seen)
	}
	markShapeShared(h.rootShape, seen)
	h.objectProto = tagRoot(h.objectProto)
	h.arrayProto = tagRoot(h.arrayProto)
	h.functionProto = tagRoot(h.functionProto)
	vm.globalObj = tagRoot(vm.globalObj)
	vm.promiseProto = tagRoot(vm.promiseProto)
	vm.regexpProto = tagRoot(vm.regexpProto)
	vm.generatorProto = tagRoot(vm.generatorProto)
	for k, v := range vm.globals {
		vm.globals[k] = tagRootValue(v)
	}
	h.sealedRoot = true
}

func markShapeShared(s *Shape, seen map[*Shape]bool) {
	if s == nil || seen[s] {
		return
	}
	seen[s] = true
	s.shared = true
	markShapeShared(s.parent, seen)
	for _, next := range s.transitions {
		markShapeShared(next, seen)
	}
}

func (r *RootRealm) lookup(key *str.String) (Value, bool) {
	if r == nil || key == nil || r.Globals == nil {
		return Undefined, false
	}
	if rk := r.Heap.Intern().Find(key); rk != nil {
		if v, ok := r.Globals[rk]; ok {
			return v, true
		}
	}
	for k, v := range r.Globals {
		if k != nil && k.Equal(key) {
			return v, true
		}
	}
	return Undefined, false
}

func objectCharge(o *Object) int64 {
	n := int64(96)
	n += int64(len(o.slots)) * 8
	n += int64(len(o.elements)) * 8
	n += int64(len(o.bytes))
	n += int64(len(o.attrs))
	if o.mapData != nil {
		n += int64(len(o.mapData.Keys)+len(o.mapData.Values)) * 8
	}
	if o.setData != nil {
		n += int64(len(o.setData.Elements)) * 8
	}
	return n
}

func cloneObject(src *Object) *Object {
	c := *src
	c.isRoot = false
	c.marked = false
	if src.slots != nil {
		c.slots = append([]Value(nil), src.slots...)
	}
	if src.elements != nil {
		c.elements = append([]Value(nil), src.elements...)
	}
	if src.attrs != nil {
		c.attrs = append([]uint8(nil), src.attrs...)
	}
	if src.deleted != nil {
		c.deleted = append([]*str.String(nil), src.deleted...)
	}
	if src.bytes != nil {
		c.bytes = append([]byte(nil), src.bytes...)
	}
	if src.mapData != nil {
		md := *src.mapData
		md.Keys = append([]Value(nil), src.mapData.Keys...)
		md.Values = append([]Value(nil), src.mapData.Values...)
		c.mapData = &md
	}
	if src.setData != nil {
		sd := *src.setData
		sd.Elements = append([]Value(nil), src.setData.Elements...)
		c.setData = &sd
	}
	if src.promise != nil {
		pd := *src.promise
		pd.reactions = append([]promiseReaction(nil), src.promise.reactions...)
		c.promise = &pd
	}
	if src.proxy != nil {
		px := *src.proxy
		c.proxy = &px
	}
	if src.gen != nil {
		gs := *src.gen
		gs.stack = append([]Value(nil), src.gen.stack...)
		c.gen = &gs
	}
	return &c
}
