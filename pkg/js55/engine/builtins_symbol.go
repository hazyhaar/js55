// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"github.com/hazyhaar/js55/pkg/js55/str"
)

var (
	SymbolIterator    = str.FromGo("Symbol.iterator")
	SymbolToPrimitive = str.FromGo("Symbol.toPrimitive")
	SymbolHasInstance = str.FromGo("Symbol.hasInstance")
	SymbolToStringTag = str.FromGo("Symbol.toStringTag")
)

func init() {
	SymbolIterator.Hash()
	SymbolToPrimitive.Hash()
	SymbolHasInstance.Hash()
	SymbolToStringTag.Hash()
}

func (vm *VM) InstallSymbolBuiltins() {
	proto := vm.heap.NewObject()
	protoV := ObjectValue(proto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)
	ctor := vm.heap.NewFunction(&Chunk{
		Name:   "Symbol",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			desc := ""
			if len(args) > 0 && !args[0].IsUndefined() {
				desc = vm.toDisplayString(args[0])
			}
			h := vm.heap.NewObject()
			if o := vm.heap.Get(h); o != nil {
				o.kind = KindSymbol
				o.text = vm.heap.Intern().InternGo(desc)
				o.proto = proto
			}
			return ObjectValue(h), nil
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	setSym := func(name string, s *str.String) {
		h := vm.heap.NewObject()
		if o := vm.heap.Get(h); o != nil {
			o.kind = KindSymbol
			o.text = s
		}
		vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo(name), ObjectValue(h))
	}
	setSym("iterator", SymbolIterator)
	setSym("toPrimitive", SymbolToPrimitive)
	setSym("hasInstance", SymbolHasInstance)
	setSym("toStringTag", SymbolToStringTag)
	vm.defineNative(proto, "toString", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		desc := ""
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && o.kind == KindSymbol && o.text != nil {
				desc = o.text.GoString()
			}
		}
		if desc == "" {
			return vm.NewStringValue(str.FromGo("Symbol()")), nil
		}
		return vm.NewStringValue(str.FromGo("Symbol(" + desc + ")")), nil
	})
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), protoV)
	vm.SetGlobal("Symbol", ctorV)
}
