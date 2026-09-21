// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"fmt"
	"strconv"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// InstallObjectBuiltins registers Object static methods and Prototype.
func (vm *VM) InstallObjectBuiltins() {
	objProto := vm.heap.NewObject()
	if o := vm.heap.Get(objProto); o != nil {
		o.proto = NoHandle
	}
	vm.heap.objectProto = objProto
	objProtoV := ObjectValue(objProto)
	vm.heap.AddRoot(&objProtoV)
	defer vm.heap.RemoveRoot(&objProtoV)

	defineStrictNative := func(obj Handle, name string, params int, fn func(*VM, []Value) (Value, error)) {
		ch := &Chunk{Name: name, Params: params, Strict: true, Native: fn}
		fv := ObjectValue(vm.heap.NewFunction(ch, NoHandle))
		vm.heap.AddRoot(&fv)
		vm.heap.SetProperty(obj, vm.heap.Intern().InternGo(name), fv)
		vm.heap.RemoveRoot(&fv)
	}

	defineStrictNative(objProto, "valueOf", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsNull() || this.IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		if this.IsObject() {
			return this, nil
		}
		return vm.toObject(this), nil
	})
	defineStrictNative(objProto, "toString", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsUndefined() {
			return vm.NewStringValue(str.FromGo("[object Undefined]")), nil
		}
		if this.IsNull() {
			return vm.NewStringValue(str.FromGo("[object Null]")), nil
		}
		tag := vm.builtinToStringTag(this)
		obj := vm.toObject(this)
		if obj.IsObject() {
			tagVal, err := vm.getPropInvoke(obj, SymbolToStringTag)
			if err != nil {
				return Undefined, err
			}
			if s := vm.StringOf(tagVal); s != nil {
				tag = s.GoString()
			}
		}
		return vm.NewStringValue(str.FromGo("[object " + tag + "]")), nil
	})
	defineStrictNative(objProto, "toLocaleString", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsNull() || this.IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		toStr := vm.getProp(this, vm.heap.Intern().InternGo("toString"))
		if vm.isFunction(toStr) {
			return vm.invoke(this, toStr, nil)
		}
		return Undefined, fmt.Errorf("TypeError: toString is not a function")
	})
	defineStrictNative(objProto, "hasOwnProperty", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsNull() || this.IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		var key Value = Undefined
		if len(args) > 0 {
			key = args[0]
		}
		name := vm.keyString(key)
		if s := vm.stringData(this); s != nil {
			if name.Equal(str.FromGo("length")) {
				return True, nil
			}
			if i, ok := arrayIndexKey(name); ok && i >= 0 && i < s.Len() {
				return True, nil
			}
			if vm.StringOf(this) != nil {
				return False, nil
			}
		}
		if !this.IsObject() {
			return False, nil
		}
		if o := vm.heap.Get(this.Handle()); o != nil && o.kind == KindArray && name.Equal(str.FromGo("length")) {
			return True, nil
		}
		_, ok := vm.heap.GetOwnProperty(this.Handle(), name)
		return Bool(ok), nil
	})
	defineStrictNative(objProto, "propertyIsEnumerable", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsNull() || this.IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		var key Value = Undefined
		if len(args) > 0 {
			key = args[0]
		}
		name := vm.keyString(key)
		if s := vm.stringData(this); s != nil {
			if i, ok := arrayIndexKey(name); ok && i >= 0 && i < s.Len() {
				return True, nil
			}
			if vm.StringOf(this) != nil {
				return False, nil
			}
		}
		if !this.IsObject() {
			return False, nil
		}
		o := vm.heap.Get(this.Handle())
		if o == nil {
			return False, nil
		}
		if o.kind == KindArray && name.Equal(str.FromGo("length")) {
			return False, nil
		}
		if _, ok := vm.heap.GetOwnProperty(this.Handle(), name); !ok {
			return False, nil
		}
		if o.shape == nil {
			return True, nil
		}
		slot := o.shape.Lookup(name)
		if slot < 0 {
			return True, nil
		}
		return Bool(o.slotAttr(slot)&attrEnumerable != 0), nil
	})
	defineStrictNative(objProto, "isPrototypeOf", 1, func(vm *VM, args []Value) (Value, error) {
		if len(args) == 0 || !vm.isJSObject(args[0]) {
			return False, nil
		}
		this := vm.CurrentThis()
		if this.IsNull() || this.IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Object.prototype.isPrototypeOf called on null or undefined")
		}
		targetH := this.Handle()
		if !this.IsObject() {
			obj := vm.toObject(this)
			if !obj.IsObject() {
				return False, nil
			}
			targetH = obj.Handle()
		}
		curr := args[0].Handle()
		depth := 0
		for curr != NoHandle && depth < 256 {
			o := vm.heap.Get(curr)
			if o == nil {
				break
			}
			if o.proto == targetH {
				return True, nil
			}
			curr = o.proto
			depth++
		}
		return False, nil
	})

	lookupAccessor := func(vm *VM, args []Value, setter bool) (Value, error) {
		this := vm.CurrentThis()
		if this.IsNull() || this.IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		obj := this
		if !obj.IsObject() {
			obj = vm.toObject(this)
		}
		if !obj.IsObject() {
			return Undefined, nil
		}
		var key Value = Undefined
		if len(args) > 0 {
			key = args[0]
		}
		name, err := vm.propertyKeyOf(key)
		if err != nil {
			return Undefined, err
		}
		idx := 0
		if setter {
			idx = 1
		}
		h := obj.Handle()
		for depth := 0; h != NoHandle && depth < 256; depth++ {
			o := vm.heap.Get(h)
			if o == nil {
				break
			}
			if cur, ok := vm.heap.GetOwnProperty(h, name); ok {
				if cur.IsObject() {
					if acc := vm.heap.Get(cur.Handle()); acc != nil && acc.kind == KindAccessor && len(acc.elements) > idx {
						return acc.elements[idx], nil
					}
				}
				return Undefined, nil
			}
			h = o.proto
		}
		return Undefined, nil
	}
	defineAccessorFromArgs := func(vm *VM, args []Value, setter bool, who string) (Value, error) {
		this := vm.CurrentThis()
		if this.IsNull() || this.IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		if len(args) < 2 || !vm.isFunction(args[1]) {
			return Undefined, fmt.Errorf("TypeError: Object.prototype.%s: Expecting function", who)
		}
		obj := this
		if !obj.IsObject() {
			obj = vm.toObject(this)
		}
		if !obj.IsObject() {
			return Undefined, nil
		}
		name, err := vm.propertyKeyOf(args[0])
		if err != nil {
			return Undefined, err
		}
		descObj := vm.heap.NewObject()
		if setter {
			vm.heap.SetProperty(descObj, vm.heap.Intern().InternGo("set"), args[1])
		} else {
			vm.heap.SetProperty(descObj, vm.heap.Intern().InternGo("get"), args[1])
		}
		vm.heap.SetProperty(descObj, vm.heap.Intern().InternGo("enumerable"), True)
		vm.heap.SetProperty(descObj, vm.heap.Intern().InternGo("configurable"), True)
		return Undefined, vm.defineDataFromDesc(obj.Handle(), name, ObjectValue(descObj))
	}
	defineStrictNative(objProto, "__lookupGetter__", 1, func(vm *VM, args []Value) (Value, error) {
		return lookupAccessor(vm, args, false)
	})
	defineStrictNative(objProto, "__lookupSetter__", 1, func(vm *VM, args []Value) (Value, error) {
		return lookupAccessor(vm, args, true)
	})
	defineStrictNative(objProto, "__defineGetter__", 2, func(vm *VM, args []Value) (Value, error) {
		return defineAccessorFromArgs(vm, args, false, "__defineGetter__")
	})
	defineStrictNative(objProto, "__defineSetter__", 2, func(vm *VM, args []Value) (Value, error) {
		return defineAccessorFromArgs(vm, args, true, "__defineSetter__")
	})

	protoKey := vm.heap.Intern().InternGo("__proto__")
	protoGetter := vm.heap.NewFunction(&Chunk{
		Name:   "get __proto__",
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			if this.IsNull() || this.IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			if !this.IsObject() {
				return Undefined, nil
			}
			o := vm.heap.Get(this.Handle())
			if o == nil || o.proto == NoHandle {
				return Null, nil
			}
			return ObjectValue(o.proto), nil
		},
	}, NoHandle)
	protoSetter := vm.heap.NewFunction(&Chunk{
		Name:   "set __proto__",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			if this.IsNull() || this.IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			if !this.IsObject() || len(args) == 0 {
				return Undefined, nil
			}
			protoArg := args[0]
			if protoArg != Null && !protoArg.IsObject() {
				return Undefined, nil
			}
			o := vm.heap.Get(this.Handle())
			if o == nil {
				return Undefined, nil
			}
			newProto := NoHandle
			if protoArg != Null {
				newProto = protoArg.Handle()
			}
			if newProto != NoHandle {
				targetH := this.Handle()
				curr := newProto
				depth := 0
				for curr != NoHandle && depth < 256 {
					if curr == targetH {
						return Undefined, fmt.Errorf("TypeError: Cyclic __proto__ value")
					}
					oProto := vm.heap.Get(curr)
					if oProto == nil {
						break
					}
					curr = oProto.proto
					depth++
				}
			}
			if o.frozen {
				if o.proto != newProto {
					return Undefined, fmt.Errorf("TypeError: Cannot set prototype of non-extensible object")
				}
				return Undefined, nil
			}
			o = vm.heap.Mutable(this.Handle())
			if o == nil {
				return Undefined, nil
			}
			o.proto = newProto
			return Undefined, nil
		},
	}, NoHandle)

	accHandle := vm.heap.NewObject()
	if acc := vm.heap.Get(accHandle); acc != nil {
		acc.kind = KindAccessor
		acc.elements = []Value{ObjectValue(protoGetter), ObjectValue(protoSetter)}
	}
	accV := ObjectValue(accHandle)
	vm.heap.AddRoot(&accV)
	vm.heap.SetProperty(objProto, protoKey, accV)
	vm.heap.SetPropertyAttrs(objProto, protoKey, attrConfigurable)
	vm.heap.RemoveRoot(&accV)

	objConstructor := vm.heap.NewFunction(&Chunk{
		Name:      "Object",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			// Construction par une SOUS-CLASSE : la spécification rend l'objet
			// déjà dérivé de NewTarget et ignore l'argument. La sous-classe se
			// reconnaît à ce que le prototype de this n'est pas Object.prototype ;
			// une construction directe par Object, elle, retombe sur ToObject.
			underived := this.IsObject() && this.Handle() != vm.globalObj
			if underived {
				if o := vm.heap.Get(this.Handle()); o == nil || o.proto == objProto {
					underived = false
				}
			}
			if underived {
				return this, nil
			}
			if len(args) > 0 && !args[0].IsNull() && !args[0].IsUndefined() {
				// Une chaîne primitive est portée par le tas : IsObject la
				// reconnaîtrait à tort comme un objet et la rendrait telle
				// quelle, alors que ToObject doit l'envelopper.
				if args[0].IsObject() && vm.StringOf(args[0]) == nil {
					if o := vm.heap.Get(args[0].Handle()); o != nil && o.kind == KindSymbol {
						return vm.toObject(args[0]), nil
					}
					return args[0], nil
				}
				return vm.toObject(args[0]), nil
			}
			if len(vm.newStack) > 0 && vm.newStack[len(vm.newStack)-1].IsObject() && this.IsObject() && this.Handle() != vm.globalObj {
				return this, nil
			}
			return ObjectValue(vm.heap.NewObject()), nil
		},
	}, NoHandle)
	hObj := ObjectValue(objConstructor)
	vm.heap.AddRoot(&hObj)
	defer vm.heap.RemoveRoot(&hObj)
	vm.heap.SetProperty(objConstructor, vm.heap.Intern().InternGo("prototype"), objProtoV)
	vm.heap.SetProperty(objProto, vm.heap.Intern().InternGo("constructor"), hObj)
	vm.SetGlobal("Object", hObj)
	if vm.globalObj != NoHandle && vm.heap.Get(vm.globalObj) != nil {
		vm.heap.SetProperty(vm.globalObj, vm.heap.Intern().InternGo("Object"), hObj)
	}
	if vm.heap.arrayProto != NoHandle {
		if o := vm.heap.Get(vm.heap.arrayProto); o != nil && o.proto == NoHandle {
			o.proto = objProto
		}
	}
	if vm.heap.functionProto != NoHandle {
		if o := vm.heap.Get(vm.heap.functionProto); o != nil && o.proto == NoHandle {
			o.proto = objProto
		}
	}

	setObjProp := func(name string, v Value) {
		k := vm.heap.Intern().InternGo(name)
		vm.heap.SetProperty(objConstructor, k, v)
	}

	defineStrictNative(objConstructor, "hasOwn", 2, func(vm *VM, args []Value) (Value, error) {
		if err := vm.rejectIfConstructor("Object.hasOwn"); err != nil {
			return Undefined, err
		}
		if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
			return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
		}
		obj := args[0]
		if !obj.IsObject() {
			obj = vm.toObject(obj)
		}
		var key Value
		if len(args) > 1 {
			key = args[1]
		}
		name, err := vm.propertyKeyOf(key)
		if err != nil {
			return Undefined, err
		}
		if s := vm.stringData(obj); s != nil {
			if name.Equal(str.FromGo("length")) {
				return True, nil
			}
			if i, ok := arrayIndexKey(name); ok && i >= 0 && i < s.Len() {
				return True, nil
			}
		}
		if o := vm.heap.Get(obj.Handle()); o != nil && o.kind == KindArray && name.Equal(str.FromGo("length")) {
			return True, nil
		}
		_, ok := vm.heap.GetOwnProperty(obj.Handle(), name)
		return Bool(ok), nil
	})

	keysChunk := &Chunk{
		Name:   "keys",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.keys"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			obj := args[0]
			if !obj.IsObject() {
				obj = vm.toObject(obj)
			}
			names := vm.BuiltinObjectKeys(obj)
			arrHandle := vm.heap.NewArray(len(names))
			arrVal := ObjectValue(arrHandle)
			vm.heap.AddRoot(&arrVal)
			defer vm.heap.RemoveRoot(&arrVal)
			for i, k := range names {
				vm.heap.SetElement(arrHandle, i, ObjectValue(vm.heap.NewString(k)))
			}
			return arrVal, nil
		},
	}
	keysFn := ObjectValue(vm.heap.NewFunction(keysChunk, NoHandle))
	setObjProp("keys", keysFn)

	namesChunk := &Chunk{
		Name:   "getOwnPropertyNames",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.getOwnPropertyNames"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			obj := args[0]
			if !obj.IsObject() {
				obj = vm.toObject(obj)
			}
			raw := vm.ownPropertyNames(obj, false)
			names := make([]*str.String, 0, len(raw))
			for _, k := range raw {
				if !isSymbolKey(k) {
					names = append(names, k)
				}
			}
			arrHandle := vm.heap.NewArray(len(names))
			arrVal := ObjectValue(arrHandle)
			vm.heap.AddRoot(&arrVal)
			defer vm.heap.RemoveRoot(&arrVal)
			for i, k := range names {
				vm.heap.SetElement(arrHandle, i, ObjectValue(vm.heap.NewString(k)))
			}
			return arrVal, nil
		},
	}
	setObjProp("getOwnPropertyNames", ObjectValue(vm.heap.NewFunction(namesChunk, NoHandle)))

	symbolsChunk := &Chunk{
		Name:   "getOwnPropertySymbols",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.getOwnPropertySymbols"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			obj := args[0]
			if !obj.IsObject() {
				obj = vm.toObject(obj)
			}
			raw := vm.ownPropertyNames(obj, false)
			var symVals []Value
			for _, k := range raw {
				if isSymbolKey(k) {
					ks := k.GoString()
					if len(ks) > 2 && ks[0] == 1 && ks[1] == 's' {
						if id, err := strconv.ParseUint(ks[2:], 10, 64); err == nil {
							if symObj := vm.heap.Get(Handle(id)); symObj != nil {
								symVals = append(symVals, ObjectValue(Handle(id)))
							}
						}
					}
				}
			}
			arrHandle := vm.heap.NewArray(len(symVals))
			arrVal := ObjectValue(arrHandle)
			vm.heap.AddRoot(&arrVal)
			defer vm.heap.RemoveRoot(&arrVal)
			for i, sv := range symVals {
				vm.heap.SetElement(arrHandle, i, sv)
			}
			return arrVal, nil
		},
	}
	setObjProp("getOwnPropertySymbols", ObjectValue(vm.heap.NewFunction(symbolsChunk, NoHandle)))

	valuesChunk := &Chunk{
		Name:   "values",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.values"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			obj := args[0]
			if !vm.isJSObject(obj) {
				obj = vm.toObject(obj)
			}
			names := vm.BuiltinObjectKeys(obj)
			arrHandle := vm.heap.NewArray(len(names))
			arrVal := ObjectValue(arrHandle)
			vm.heap.AddRoot(&arrVal)
			defer vm.heap.RemoveRoot(&arrVal)
			for i, k := range names {
				val, err := vm.enumerableOwnValue(obj, k)
				if err != nil {
					return Undefined, err
				}
				vm.heap.SetElement(arrHandle, i, val)
			}
			return arrVal, nil
		},
	}
	setObjProp("values", ObjectValue(vm.heap.NewFunction(valuesChunk, NoHandle)))

	entriesChunk := &Chunk{
		Name:   "entries",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.entries"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			obj := args[0]
			if !vm.isJSObject(obj) {
				obj = vm.toObject(obj)
			}
			names := vm.BuiltinObjectKeys(obj)
			n := len(names)
			arrHandle := vm.heap.NewArray(n)
			arrVal := ObjectValue(arrHandle)
			vm.heap.AddRoot(&arrVal)
			defer vm.heap.RemoveRoot(&arrVal)
			for i := 0; i < n; i++ {
				val, err := vm.enumerableOwnValue(obj, names[i])
				if err != nil {
					return Undefined, err
				}
				pair := vm.heap.NewArray(2)
				pairVal := ObjectValue(pair)
				vm.heap.AddRoot(&pairVal)
				vm.heap.SetElement(pair, 0, ObjectValue(vm.heap.NewString(names[i])))
				vm.heap.SetElement(pair, 1, val)
				vm.heap.SetElement(arrHandle, i, pairVal)
				vm.heap.RemoveRoot(&pairVal)
			}
			return arrVal, nil
		},
	}
	setObjProp("entries", ObjectValue(vm.heap.NewFunction(entriesChunk, NoHandle)))

	assignChunk := &Chunk{
		Name:   "assign",
		Params: 2,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.assign"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			target := args[0]
			if target.IsNull() || target.IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
			}
			to := target
			if !vm.isJSObject(target) {
				to = vm.toObject(target)
			}
			for i := 1; i < len(args); i++ {
				src := args[i]
				if src.IsNull() || src.IsUndefined() {
					continue
				}
				srcObj := src
				if !vm.isJSObject(src) {
					srcObj = vm.toObject(src)
				}
				names := vm.ownPropertyNames(srcObj, true)
				for _, k := range names {
					val, err := vm.enumerableOwnValue(srcObj, k)
					if err != nil {
						return Undefined, err
					}
					if to.IsObject() {
						if toObj := vm.heap.Get(to.Handle()); toObj != nil {
							if toObj.kind == KindArray {
								if idx, ok := arrayIndexKey(k); ok {
									vm.heap.SetElement(to.Handle(), idx, val)
								}
							}
							if toObj.kind == KindStringObject && toObj.text != nil {
								if k.Equal(str.FromGo("length")) {
									return Undefined, fmt.Errorf("TypeError: Cannot assign to read only property 'length' of String")
								}
								if idx, ok := arrayIndexKey(k); ok && idx >= 0 && idx < toObj.text.Len() {
									return Undefined, fmt.Errorf("TypeError: Cannot assign to read only property of String")
								}
							}
						}
					}
					if err := vm.setProp(&frame{}, to, k, val); err != nil {
						return Undefined, err
					}
				}
			}
			return to, nil
		},
	}
	setObjProp("assign", ObjectValue(vm.heap.NewFunction(assignChunk, NoHandle)))

	createChunk := &Chunk{
		Name:   "create",
		Params: 2,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.create"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 {
				return Undefined, fmt.Errorf("TypeError: Object prototype may only be an Object or null")
			}
			protoVal := args[0]
			var protoHandle Handle = NoHandle
			if protoVal == Null {
				protoHandle = NoHandle
			} else if vm.isJSObject(protoVal) {
				protoHandle = protoVal.Handle()
			} else {
				return Undefined, fmt.Errorf("TypeError: Object prototype may only be an Object or null")
			}
			newObj := vm.heap.NewObject()
			if o := vm.heap.Get(newObj); o != nil {
				o.proto = protoHandle
			}
			if len(args) > 1 && !args[1].IsUndefined() {
				entries, err := vm.readPropertiesDescriptors(args[1])
				if err != nil {
					return Undefined, err
				}
				for _, e := range entries {
					if err := vm.defineDataFromDesc(newObj, e.key, e.desc); err != nil {
						return Undefined, err
					}
				}
			}
			return ObjectValue(newObj), nil
		},
	}
	setObjProp("create", ObjectValue(vm.heap.NewFunction(createChunk, NoHandle)))

	getProtoChunk := &Chunk{
		Name:   "getPrototypeOf",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.getPrototypeOf"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Object.getPrototypeOf called on non-object")
			}
			obj := args[0]
			if !obj.IsObject() {
				obj = vm.toObject(obj)
			}
			o := vm.heap.Get(obj.Handle())
			if o == nil || o.proto == NoHandle {
				return Null, nil
			}
			return ObjectValue(o.proto), nil
		},
	}
	setObjProp("getPrototypeOf", ObjectValue(vm.heap.NewFunction(getProtoChunk, NoHandle)))

	setProtoChunk := &Chunk{
		Name:   "setPrototypeOf",
		Params: 2,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.setPrototypeOf"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Object.setPrototypeOf called on non-object")
			}
			if !vm.isJSObject(args[0]) {
				return args[0], nil
			}
			if len(args) < 2 || (args[1] != Null && !vm.isJSObject(args[1])) {
				return Undefined, fmt.Errorf("TypeError: Object prototype may only be an Object or null")
			}
			o := vm.heap.Get(args[0].Handle())
			if o == nil {
				return args[0], nil
			}
			if args[1].IsObject() {
				targetH := args[0].Handle()
				curr := args[1].Handle()
				depth := 0
				for curr != NoHandle && depth < 256 {
					if curr == targetH {
						return Undefined, fmt.Errorf("TypeError: Cyclic __proto__ value")
					}
					oProto := vm.heap.Get(curr)
					if oProto == nil {
						break
					}
					curr = oProto.proto
					depth++
				}
			}
			if o.frozen {
				curProto := o.proto
				newProto := NoHandle
				if args[1].IsObject() {
					newProto = args[1].Handle()
				}
				if curProto != newProto {
					return Undefined, fmt.Errorf("TypeError: Cannot set prototype of non-extensible object")
				}
				return args[0], nil
			}
			o = vm.heap.Mutable(args[0].Handle())
			if o == nil {
				return args[0], nil
			}
			if args[1] == Null {
				o.proto = NoHandle
			} else {
				o.proto = args[1].Handle()
			}
			return args[0], nil
		},
	}
	setObjProp("setPrototypeOf", ObjectValue(vm.heap.NewFunction(setProtoChunk, NoHandle)))

	defProp := &Chunk{
		Name:   "defineProperty",
		Params: 3,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.defineProperty"); err != nil {
				return Undefined, err
			}
			if len(args) < 1 || !vm.isJSObject(args[0]) {
				return Undefined, fmt.Errorf("TypeError: Object.defineProperty called on non-object")
			}
			if len(args) < 2 {
				return Undefined, fmt.Errorf("TypeError: Object.defineProperty requires property key")
			}
			name, err := vm.propertyKeyOf(args[1])
			if err != nil {
				return Undefined, err
			}
			var desc Value
			if len(args) > 2 {
				desc = args[2]
			}
			if !vm.isJSObject(desc) {
				return Undefined, fmt.Errorf("TypeError: Property description must be an object")
			}
			if err := vm.defineDataFromDesc(args[0].Handle(), name, desc); err != nil {
				return Undefined, err
			}
			return args[0], nil
		},
	}
	setObjProp("defineProperty", ObjectValue(vm.heap.NewFunction(defProp, NoHandle)))

	getOwn := &Chunk{
		Name:   "getOwnPropertyDescriptor",
		Params: 2,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.getOwnPropertyDescriptor"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Object.getOwnPropertyDescriptor called on non-object")
			}
			if len(args) < 2 {
				return Undefined, nil
			}
			obj := args[0]
			if !obj.IsObject() {
				obj = vm.toObject(obj)
			}
			name, err := vm.propertyKeyOf(args[1])
			if err != nil {
				return Undefined, err
			}
			return vm.dataDescriptorObject(obj, name), nil
		},
	}
	setObjProp("getOwnPropertyDescriptor", ObjectValue(vm.heap.NewFunction(getOwn, NoHandle)))

	getOwns := &Chunk{
		Name:   "getOwnPropertyDescriptors",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.getOwnPropertyDescriptors"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Object.getOwnPropertyDescriptors called on non-object")
			}
			obj := args[0]
			if !obj.IsObject() {
				obj = vm.toObject(obj)
			}
			return vm.ownDescriptorsObject(obj)
		},
	}
	setObjProp("getOwnPropertyDescriptors", ObjectValue(vm.heap.NewFunction(getOwns, NoHandle)))

	defProps := &Chunk{
		Name:   "defineProperties",
		Params: 2,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.defineProperties"); err != nil {
				return Undefined, err
			}
			if len(args) < 1 || !vm.isJSObject(args[0]) {
				return Undefined, fmt.Errorf("TypeError: Object.defineProperties called on non-object")
			}
			if len(args) < 2 {
				return Undefined, fmt.Errorf("TypeError: Object.defineProperties requires properties argument")
			}
			entries, err := vm.readPropertiesDescriptors(args[1])
			if err != nil {
				return Undefined, err
			}
			for _, e := range entries {
				if err := vm.defineDataFromDesc(args[0].Handle(), e.key, e.desc); err != nil {
					return Undefined, err
				}
			}
			return args[0], nil
		},
	}
	setObjProp("defineProperties", ObjectValue(vm.heap.NewFunction(defProps, NoHandle)))

	fromEnt := &Chunk{
		Name:   "fromEntries",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.fromEntries"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, fmt.Errorf("TypeError: Object.fromEntries requires an iterable")
			}
			iter := args[0]
			h := vm.heap.NewObject()
			hv := ObjectValue(h)
			vm.heap.AddRoot(&hv)
			defer vm.heap.RemoveRoot(&hv)
			addEntry := func(el Value, closer func()) error {
				if !vm.isJSObject(el) {
					if closer != nil {
						closer()
					}
					return fmt.Errorf("TypeError: Iterator value is not an entry object")
				}
				kVal, err := vm.entryIndex(el, 0)
				if err != nil {
					if closer != nil {
						closer()
					}
					return err
				}
				vVal, err := vm.entryIndex(el, 1)
				if err != nil {
					if closer != nil {
						closer()
					}
					return err
				}
				k, err := vm.propertyKeyOf(kVal)
				if err != nil {
					if closer != nil {
						closer()
					}
					return err
				}
				vm.heap.SetProperty(h, k, vVal)
				return nil
			}
			iterFn := vm.getProp(iter, SymbolIterator)
			if vm.isFunction(iterFn) {
				itObj, err := vm.invoke(iter, iterFn, nil)
				if err != nil {
					return Undefined, err
				}
				closer := func() { _ = vm.closeIterator(itObj) }
				nextFn := vm.getProp(itObj, vm.heap.Intern().InternGo("next"))
				if !vm.isFunction(nextFn) {
					return Undefined, fmt.Errorf("TypeError: iterator.next is not a function")
				}
				doneKey := vm.heap.Intern().InternGo("done")
				valueKey := vm.heap.Intern().InternGo("value")
				for step := 0; step < 100000; step++ {
					res, err := vm.invoke(itObj, nextFn, nil)
					if err != nil {
						return Undefined, err
					}
					if !vm.isJSObject(res) {
						return Undefined, fmt.Errorf("TypeError: iterator result is not an object")
					}
					doneV, err := vm.getPropInvoke(res, doneKey)
					if err != nil {
						return Undefined, err
					}
					if vm.truthy(doneV) {
						break
					}
					valV, err := vm.getPropInvoke(res, valueKey)
					if err != nil {
						closer()
						return Undefined, err
					}
					if err := addEntry(valV, closer); err != nil {
						return Undefined, err
					}
				}
				return hv, nil
			}
			n := vm.arrayLikeLength(iter)
			for i := 0; i < n; i++ {
				el, err := vm.arrayIterIndex(iter, i)
				if err != nil {
					return Undefined, err
				}
				if err := addEntry(el, nil); err != nil {
					return Undefined, err
				}
			}
			return hv, nil
		},
	}
	setObjProp("fromEntries", ObjectValue(vm.heap.NewFunction(fromEnt, NoHandle)))

	freezeChunk := &Chunk{
		Name:   "freeze",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.freeze"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 {
				return Undefined, nil
			}
			v := args[0]
			if v.IsObject() {
				if o := vm.heap.Mutable(v.Handle()); o != nil {
					o.frozen = true
					o.ensureAttrs()
					for i := range o.attrs {
						val := o.slots[i]
						isAccessor := val.IsObject() && vm.heap.Get(val.Handle()) != nil && vm.heap.Get(val.Handle()).kind == KindAccessor
						if isAccessor {
							o.attrs[i] &^= attrConfigurable
						} else {
							o.attrs[i] &^= (attrWritable | attrConfigurable)
						}
					}
					if o.kind == KindArray {
						lenKey := vm.heap.Intern().InternGo("length")
						lenSlot := o.shape.Lookup(lenKey)
						if lenSlot >= 0 {
							o.attrs[lenSlot] &^= attrWritable
						}
					}
				}
			}
			return v, nil
		},
	}
	setObjProp("freeze", ObjectValue(vm.heap.NewFunction(freezeChunk, NoHandle)))

	sealChunk := &Chunk{
		Name:   "seal",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.seal"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 {
				return Undefined, nil
			}
			v := args[0]
			if v.IsObject() {
				if o := vm.heap.Mutable(v.Handle()); o != nil {
					o.frozen = true
					o.ensureAttrs()
					for i := range o.attrs {
						o.attrs[i] &^= attrConfigurable
					}
					if o.kind == KindArray {
						lenKey := vm.heap.Intern().InternGo("length")
						lenSlot := o.shape.Lookup(lenKey)
						if lenSlot >= 0 {
							o.attrs[lenSlot] &^= attrConfigurable
						}
					}
				}
			}
			return v, nil
		},
	}
	setObjProp("seal", ObjectValue(vm.heap.NewFunction(sealChunk, NoHandle)))

	preventExtChunk := &Chunk{
		Name:   "preventExtensions",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.preventExtensions"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 {
				return Undefined, nil
			}
			v := args[0]
			if v.IsObject() {
				if o := vm.heap.Mutable(v.Handle()); o != nil {
					o.frozen = true
				}
			}
			return v, nil
		},
	}
	setObjProp("preventExtensions", ObjectValue(vm.heap.NewFunction(preventExtChunk, NoHandle)))

	isExtensibleChunk := &Chunk{
		Name:   "isExtensible",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.isExtensible"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || !args[0].IsObject() {
				return False, nil
			}
			o := vm.heap.Get(args[0].Handle())
			return Bool(o != nil && !o.frozen), nil
		},
	}
	setObjProp("isExtensible", ObjectValue(vm.heap.NewFunction(isExtensibleChunk, NoHandle)))

	isSealedChunk := &Chunk{
		Name:   "isSealed",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.isSealed"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || !args[0].IsObject() {
				return True, nil
			}
			o := vm.heap.Get(args[0].Handle())
			if o == nil {
				return True, nil
			}
			if !o.frozen {
				return False, nil
			}
			for i := 0; i < len(o.slots); i++ {
				if o.slotAttr(i)&attrConfigurable != 0 {
					return False, nil
				}
			}
			return True, nil
		},
	}
	setObjProp("isSealed", ObjectValue(vm.heap.NewFunction(isSealedChunk, NoHandle)))

	isChunk := &Chunk{
		Name:   "is",
		Params: 2,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.is"); err != nil {
				return Undefined, err
			}
			var a, b Value = Undefined, Undefined
			if len(args) > 0 {
				a = args[0]
			}
			if len(args) > 1 {
				b = args[1]
			}
			return Bool(vm.sameValue(a, b)), nil
		},
	}
	setObjProp("is", ObjectValue(vm.heap.NewFunction(isChunk, NoHandle)))

	isFrozenChunk := &Chunk{
		Name:   "isFrozen",
		Params: 1,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.isFrozen"); err != nil {
				return Undefined, err
			}
			if len(args) == 0 || !args[0].IsObject() {
				return True, nil
			}
			o := vm.heap.Get(args[0].Handle())
			if o == nil {
				return True, nil
			}
			if !o.frozen {
				return False, nil
			}
			for i := 0; i < len(o.slots); i++ {
				if o.slotAttr(i)&attrConfigurable != 0 {
					return False, nil
				}
				val := o.slots[i]
				isAccessor := val.IsObject() && vm.heap.Get(val.Handle()) != nil && vm.heap.Get(val.Handle()).kind == KindAccessor
				if !isAccessor && o.slotAttr(i)&attrWritable != 0 {
					return False, nil
				}
			}
			return True, nil
		},
	}
	setObjProp("isFrozen", ObjectValue(vm.heap.NewFunction(isFrozenChunk, NoHandle)))

	groupByChunk := &Chunk{
		Name:   "groupBy",
		Params: 2,
		Strict: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if err := vm.rejectIfConstructor("Object.groupBy"); err != nil {
				return Undefined, err
			}
			if len(args) < 2 {
				return Undefined, fmt.Errorf("TypeError: Object.groupBy requires at least 2 arguments")
			}
			items := args[0]
			cb := args[1]
			if !vm.isFunction(cb) {
				return Undefined, fmt.Errorf("TypeError: callback is not a function")
			}
			outObj := vm.heap.NewObject()
			if o := vm.heap.Get(outObj); o != nil {
				o.proto = NoHandle
			}
			n := vm.arrayLikeLength(items)
			for i := 0; i < n; i++ {
				val, err := vm.arrayIterIndex(items, i)
				if err != nil {
					return Undefined, err
				}
				keyVal, err := vm.invoke(Undefined, cb, []Value{val, Int(int32(i))})
				if err != nil {
					return Undefined, err
				}
				keyStr, err := vm.propertyKeyOf(keyVal)
				if err != nil {
					return Undefined, err
				}
				var groupArr Handle
				if cur, ok := vm.heap.GetOwnProperty(outObj, keyStr); ok && cur.IsObject() {
					groupArr = cur.Handle()
				} else {
					groupArr = vm.heap.NewArray(0)
					av := ObjectValue(groupArr)
					vm.heap.SetProperty(outObj, keyStr, av)
				}
				vm.BuiltinArrayPush(ObjectValue(groupArr), val)
			}
			return ObjectValue(outObj), nil
		},
	}
	setObjProp("groupBy", ObjectValue(vm.heap.NewFunction(groupByChunk, NoHandle)))

	// Les propriétés d'un objet intrinsèque sont { writable: true,
	// enumerable: false, configurable: true }. Les installer par SetProperty
	// leur donne l'attribut par défaut, énumérable, ce qui fait aussi
	// apparaître les méthodes natives dans un for-in ou un Object.keys.
	vm.markBuiltinAttrs(objProto)
	vm.markBuiltinAttrs(objConstructor)
	vm.heap.SetPropertyAttrs(objConstructor, vm.heap.Intern().InternGo("prototype"), 0)
	vm.stampFunctionNameLength(objConstructor)
	vm.stampNativeNameLength(objProto)
	vm.stampNativeNameLength(objConstructor)

	vm.linkEarlyIntrinsics(objProto)
}

// linkEarlyIntrinsics répare la chaîne de prototypes des intrinsèques posés
// AVANT Object.
//
// Object.prototype n'existe qu'au moment de l'installation d'Object ; les
// objets créés plus tôt — Math, String.prototype, Number.prototype,
// Boolean.prototype, Error.prototype — sont donc racines de leur propre
// chaîne. Sans ce rattrapage, hasOwnProperty, propertyIsEnumerable, valueOf et
// toString y sont introuvables, et l'enveloppe d'une primitive n'expose pas de
// constructor : Object(true).constructor.prototype rend alors une lecture sur
// undefined là où la spécification rend Boolean.prototype.
//
// Le rattachement ne touche qu'un objet dont le prototype est encore vide : un
// objet déjà chaîné (RangeError.prototype vers Error.prototype) est laissé en
// place.
func (vm *VM) linkEarlyIntrinsics(objProto Handle) {
	link := func(h Handle) {
		if h == NoHandle || h == objProto {
			return
		}
		o := vm.heap.Get(h)
		if o == nil || o.proto != NoHandle {
			return
		}
		o.proto = objProto
	}

	ctorKey := vm.heap.Intern().InternGo("constructor")
	protoKey := vm.heap.Intern().InternGo("prototype")

	// Objets intrinsèques sans constructeur : leur prototype est Object.prototype.
	// Un nom encore absent des globales — un intrinsèque installé APRÈS Object —
	// est ignoré sans effet plutôt que de figer ici l'ordre d'installation.
	for _, name := range []string{"Math", "JSON", "Reflect"} {
		g, ok := vm.GetGlobal(name)
		if !ok || !g.IsObject() {
			continue
		}
		link(g.Handle())
		vm.markBuiltinAttrs(g.Handle())
	}

	// Constructeurs : la fonction elle-même et son prototype descendent
	// d'Object.prototype, et le prototype porte un constructor propre.
	for _, name := range []string{
		"Function", "Array", "String", "Number", "Boolean", "Symbol", "BigInt",
		"Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError",
		"TypeError", "URIError", "AggregateError",
	} {
		g, ok := vm.GetGlobal(name)
		if !ok || !g.IsObject() {
			continue
		}
		link(g.Handle())
		vm.markBuiltinAttrs(g.Handle())
		protoVal, ok := vm.heap.GetOwnProperty(g.Handle(), protoKey)
		if !ok || !protoVal.IsObject() {
			continue
		}
		link(protoVal.Handle())
		vm.markBuiltinAttrs(protoVal.Handle())
		if _, has := vm.heap.GetOwnProperty(protoVal.Handle(), ctorKey); !has {
			vm.heap.DefineDataProperty(protoVal.Handle(), ctorKey, g, attrWritable|attrConfigurable)
		}
		// La liaison C.prototype est scellée : ni réécrivable, ni supprimable.
		vm.heap.SetPropertyAttrs(g.Handle(), protoKey, 0)
	}

	vm.sealIntrinsicConstants()
	vm.completeWrapperToString()
}

// completeWrapperToString pose le toString manquant des prototypes d'enveloppe.
//
// Tant que Boolean.prototype ne descendait de rien, l'absence d'un toString
// propre restait sans conséquence. Une fois la chaîne raccordée à
// Object.prototype, la conversion d'une enveloppe en clé de propriété trouve le
// toString générique et rend "[object Boolean]" là où la spécification rend
// "true" : Object.defineProperty(o, new Boolean(false), {}) définirait alors la
// mauvaise clé. Le toString propre rétablit ToString(primitive).
func (vm *VM) completeWrapperToString() {
	toStringKey := vm.heap.Intern().InternGo("toString")
	for _, name := range []string{"Boolean", "Number", "String"} {
		g, ok := vm.GetGlobal(name)
		if !ok || !g.IsObject() {
			continue
		}
		protoVal, ok := vm.heap.GetOwnProperty(g.Handle(), vm.heap.Intern().InternGo("prototype"))
		if !ok || !protoVal.IsObject() {
			continue
		}
		if _, has := vm.heap.GetOwnProperty(protoVal.Handle(), toStringKey); has {
			continue
		}
		vm.defineNative(protoVal.Handle(), "toString", 0, func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			if this.IsObject() {
				if o := vm.heap.Get(this.Handle()); o != nil && o.prim != Undefined {
					return vm.NewStringValue(str.FromGo(vm.toDisplayString(o.prim))), nil
				}
			}
			return vm.NewStringValue(str.FromGo(vm.toDisplayString(this))), nil
		})
		vm.heap.SetPropertyAttrs(protoVal.Handle(), toStringKey, attrWritable|attrConfigurable)
	}
}

// intrinsicConstants énumère les valeurs constantes des intrinsèques. La
// spécification les pose { writable: false, enumerable: false,
// configurable: false } ; markBuiltinAttrs, qui vise les méthodes, les rendrait
// à tort réécrivables.
var intrinsicConstants = map[string][]string{
	"Math":   {"E", "LN10", "LN2", "LOG10E", "LOG2E", "PI", "SQRT1_2", "SQRT2"},
	"Number": {"EPSILON", "MAX_SAFE_INTEGER", "MAX_VALUE", "MIN_SAFE_INTEGER", "MIN_VALUE", "NaN", "NEGATIVE_INFINITY", "POSITIVE_INFINITY"},
}

// sealIntrinsicConstants scelle les constantes numériques des intrinsèques.
func (vm *VM) sealIntrinsicConstants() {
	for name, keys := range intrinsicConstants {
		g, ok := vm.GetGlobal(name)
		if !ok || !g.IsObject() {
			continue
		}
		for _, k := range keys {
			key := vm.heap.Intern().InternGo(k)
			if _, has := vm.heap.GetOwnProperty(g.Handle(), key); !has {
				continue
			}
			vm.heap.SetPropertyAttrs(g.Handle(), key, 0)
		}
	}
}

// markBuiltinAttrs rend non énumérables les propriétés de données propres d'un
// objet intrinsèque. Les propriétés accesseur sont laissées intactes : leur
// attribut d'écriture n'a pas de sens et a déjà été posé à l'installation.
func (vm *VM) markBuiltinAttrs(h Handle) {
	o := vm.heap.Get(h)
	if o == nil || o.shape == nil {
		return
	}
	for _, k := range vm.ownPropertyNames(ObjectValue(h), false) {
		cur, ok := vm.heap.GetOwnProperty(h, k)
		if !ok {
			continue
		}
		if cur.IsObject() {
			if acc := vm.heap.Get(cur.Handle()); acc != nil && acc.kind == KindAccessor {
				continue
			}
		}
		vm.heap.SetPropertyAttrs(h, k, attrWritable|attrConfigurable)
	}
}

func (vm *VM) rejectIfConstructor(who string) error {
	if len(vm.newStack) > 0 {
		top := vm.newStack[len(vm.newStack)-1]
		if top.IsObject() {
			if g, ok := vm.GetGlobal("Object"); ok && top == g {
				return nil
			}
			return fmt.Errorf("TypeError: %s is not a constructor", who)
		}
	}
	return nil
}

func (vm *VM) isJSObject(v Value) bool {
	if !v.IsObject() {
		return false
	}
	if vm.StringOf(v) != nil {
		return false
	}
	o := vm.heap.Get(v.Handle())
	if o == nil {
		return false
	}
	if o.kind == KindSymbol || o.kind == KindBigInt {
		return false
	}
	return true
}

func (vm *VM) stampFunctionNameLength(fnH Handle) {
	o := vm.heap.Get(fnH)
	if o == nil || o.kind != KindFunction || o.fn == nil {
		return
	}
	name := o.fn.Name
	if name == "" {
		return
	}
	lenKey := vm.heap.Intern().InternGo("length")
	nameKey := vm.heap.Intern().InternGo("name")
	vm.heap.DefineDataProperty(fnH, lenKey, Int(int32(o.fn.Params)), attrConfigurable)
	vm.heap.DefineDataProperty(fnH, nameKey, vm.NewStringValue(str.FromGo(name)), attrConfigurable)
}

func (vm *VM) stampNativeNameLength(h Handle) {
	for _, k := range vm.ownPropertyNames(ObjectValue(h), false) {
		cur, ok := vm.heap.GetOwnProperty(h, k)
		if !ok || !cur.IsObject() {
			continue
		}
		o := vm.heap.Get(cur.Handle())
		if o == nil {
			continue
		}
		if o.kind == KindFunction {
			vm.stampFunctionNameLength(cur.Handle())
			continue
		}
		if o.kind == KindAccessor {
			for _, el := range o.elements {
				if el.IsObject() {
					vm.stampFunctionNameLength(el.Handle())
				}
			}
		}
	}
}

// propertyKeyOf applique ToPropertyKey. Un objet enveloppe — String, Number,
// Boolean — doit d'abord être ramené à sa primitive : sans cette étape la clé
// calculée est la représentation de l'objet et non celle de sa valeur, et
// Object.getOwnPropertyDescriptor(o, new String("x")) rend undefined là où la
// spécification rend le descripteur de "x".
func (vm *VM) propertyKeyOf(v Value) (*str.String, error) {
	if !v.IsObject() {
		return vm.keyString(v), nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil {
		return vm.keyString(v), nil
	}
	switch o.kind {
	case KindSymbol, KindString:
		return vm.keyString(v), nil
	case KindStringObject:
		if o.text != nil {
			return vm.heap.Intern().InternGo(o.text.GoString()), nil
		}
	}
	// La conversion se fait ici plutôt que par toPrimitive : ce dernier
	// n'accepte un résultat que s'il n'est pas un objet, or une chaîne du
	// moteur EST un objet du tas, si bien qu'un toString utilisateur rendant
	// une chaîne serait rejeté et la conversion échouerait.
	for _, n := range []string{"toString", "valueOf"} {
		m := vm.getProp(v, vm.heap.Intern().InternGo(n))
		if !vm.isFunction(m) {
			continue
		}
		res, err := vm.invoke(v, m, nil)
		if err != nil {
			return nil, err
		}
		if !res.IsObject() || vm.StringOf(res) != nil {
			return vm.keyString(res), nil
		}
		if o := vm.heap.Get(res.Handle()); o != nil && o.kind == KindSymbol {
			return vm.keyString(res), nil
		}
	}
	return nil, fmt.Errorf("TypeError: Cannot convert object to primitive value")
}

func (vm *VM) toObject(v Value) Value {
	// Une chaîne primitive est portée par le tas : le test d'objet la
	// laisserait passer telle quelle et ToObject("") rendrait une chaîne, si
	// bien que Object("") === Object("") serait vrai. L'enveloppe se construit
	// donc avant ce test.
	if s := vm.StringOf(v); s != nil {
		h := vm.heap.NewStringObject(s)
		if strCtor, ok := vm.GetGlobal("String"); ok && strCtor.IsObject() {
			if proto, ok := vm.heap.GetProperty(strCtor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
				if o := vm.heap.Get(h); o != nil {
					o.proto = proto.Handle()
				}
			}
		}
		return ObjectValue(h)
	}
	if v.IsObject() {
		if o := vm.heap.Get(v.Handle()); o != nil {
			if o.kind == KindSymbol {
				h := vm.heap.NewObject()
				if envObj := vm.heap.Get(h); envObj != nil {
					envObj.prim = v
					if symCtor, ok := vm.GetGlobal("Symbol"); ok && symCtor.IsObject() {
						if proto, ok := vm.heap.GetProperty(symCtor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
							envObj.proto = proto.Handle()
						}
					}
				}
				return ObjectValue(h)
			}
			if o.kind == KindBigInt {
				h := vm.heap.NewObject()
				if envObj := vm.heap.Get(h); envObj != nil {
					envObj.prim = v
					if biCtor, ok := vm.GetGlobal("BigInt"); ok && biCtor.IsObject() {
						if proto, ok := vm.heap.GetProperty(biCtor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
							envObj.proto = proto.Handle()
						}
					}
				}
				return ObjectValue(h)
			}
		}
		return v
	}
	if s := vm.StringOf(v); s != nil {
		h := vm.heap.NewStringObject(s)
		if strCtor, ok := vm.GetGlobal("String"); ok && strCtor.IsObject() {
			if proto, ok := vm.heap.GetProperty(strCtor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
				if o := vm.heap.Get(h); o != nil {
					o.proto = proto.Handle()
				}
			}
		}
		return ObjectValue(h)
	}
	if v.IsInt() || v.IsNumber() {
		h := vm.heap.NewObject()
		if o := vm.heap.Get(h); o != nil {
			o.prim = v
			if numCtor, ok := vm.GetGlobal("Number"); ok && numCtor.IsObject() {
				if proto, ok := vm.heap.GetProperty(numCtor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
					o.proto = proto.Handle()
				}
			}
		}
		return ObjectValue(h)
	}
	if v.IsBool() {
		h := vm.heap.NewObject()
		if o := vm.heap.Get(h); o != nil {
			o.prim = v
			if boolCtor, ok := vm.GetGlobal("Boolean"); ok && boolCtor.IsObject() {
				if proto, ok := vm.heap.GetProperty(boolCtor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
					o.proto = proto.Handle()
				}
			}
		}
		return ObjectValue(h)
	}
	if v.IsNull() || v.IsUndefined() {
		return ObjectValue(vm.heap.NewObject())
	}
	return v
}

func (vm *VM) builtinToStringTag(this Value) string {
	if this.IsUndefined() {
		return "Undefined"
	}
	if this.IsNull() {
		return "Null"
	}
	if s := vm.StringOf(this); s != nil {
		return "String"
	}
	if this.IsInt() || this.IsNumber() {
		return "Number"
	}
	if this.IsBool() {
		return "Boolean"
	}
	if !this.IsObject() {
		return "Object"
	}
	o := vm.heap.Get(this.Handle())
	if o == nil {
		return "Object"
	}
	switch o.kind {
	case KindArray:
		return "Array"
	case KindFunction:
		return "Function"
	case KindError:
		return "Error"
	case KindRegExp:
		return "RegExp"
	case KindMap:
		return "Map"
	case KindSet:
		return "Set"
	case KindPromise:
		return "Promise"
	case KindArguments:
		return "Arguments"
	case KindStringObject:
		return "String"
	case KindBigInt:
		return "BigInt"
	case KindSymbol:
		return "Symbol"
	}
	if tag := vm.wrapperToStringTag(o); tag != "" {
		return tag
	}
	return "Object"
}

func (vm *VM) wrapperToStringTag(o *Object) string {
	pairs := []struct{ ctor, tag string }{
		{"Boolean", "Boolean"},
		{"Number", "Number"},
		{"Date", "Date"},
		{"Symbol", "Symbol"},
		{"BigInt", "BigInt"},
	}
	protoKey := vm.heap.Intern().InternGo("prototype")
	for _, p := range pairs {
		g, ok := vm.GetGlobal(p.ctor)
		if !ok || !g.IsObject() {
			continue
		}
		proto, ok := vm.heap.GetOwnProperty(g.Handle(), protoKey)
		if !ok || !proto.IsObject() {
			continue
		}
		if o.proto == proto.Handle() {
			return p.tag
		}
	}
	return ""
}

func (vm *VM) stringData(v Value) *str.String {
	if s := vm.StringOf(v); s != nil {
		return s
	}
	if v.IsObject() {
		if o := vm.heap.Get(v.Handle()); o != nil && o.kind == KindStringObject {
			return o.text
		}
	}
	return nil
}

func (vm *VM) stringIndexValue(s *str.String, i int) Value {
	if s == nil || i < 0 || i >= s.Len() {
		return Undefined
	}
	return vm.NewStringValue(s.Slice(i, i+1))
}

func (vm *VM) enumerableOwnValue(obj Value, k *str.String) (Value, error) {
	if s := vm.stringData(obj); s != nil {
		if i, ok := arrayIndexKey(k); ok {
			return vm.stringIndexValue(s, i), nil
		}
	}
	return vm.getPropInvoke(obj, k)
}

func (vm *VM) entryIndex(el Value, i int) (Value, error) {
	if el.IsObject() {
		if o := vm.heap.Get(el.Handle()); o != nil && o.kind == KindStringObject && o.text != nil {
			if i >= 0 && i < o.text.Len() {
				return vm.NewStringValue(o.text.Slice(i, i+1)), nil
			}
			return Undefined, nil
		}
	}
	return vm.arrayIterIndex(el, i)
}

func (vm *VM) closeIterator(it Value) error {
	if !it.IsObject() {
		return nil
	}
	retFn := vm.getProp(it, vm.heap.Intern().InternGo("return"))
	if !vm.isFunction(retFn) {
		return nil
	}
	_, err := vm.invoke(it, retFn, nil)
	return err
}

// BuiltinObjectKeys extracts all own property keys from an Object using its Shape.
func (vm *VM) BuiltinObjectKeys(v Value) []*str.String {
	if vm.StringOf(v) != nil {
		return vm.ownPropertyNames(v, true)
	}
	if !v.IsObject() {
		return nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil {
		return nil
	}
	names := vm.ownPropertyNames(v, true)
	out := make([]*str.String, 0, len(names))
	for _, k := range names {
		if isSymbolKey(k) {
			continue
		}
		out = append(out, k)
	}
	return out
}

// BuiltinObjectValues extracts all property values from an Object.
func (vm *VM) BuiltinObjectValues(v Value) []Value {
	names := vm.BuiltinObjectKeys(v)
	res := make([]Value, 0, len(names))
	for _, k := range names {
		val, err := vm.enumerableOwnValue(v, k)
		if err != nil {
			continue
		}
		res = append(res, val)
	}
	return res
}
