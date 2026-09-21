// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

func (vm *VM) InstallArrayBuiltins() {
	arrayProto := vm.heap.NewObject()
	vm.heap.arrayProto = arrayProto
	protoV := ObjectValue(arrayProto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)

	ctor := vm.heap.NewFunction(&Chunk{
		Name:      "Array",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			if len(args) == 1 && (args[0].IsInt() || args[0].IsNumber()) {
				n, err := vm.arrayAllocLength(args[0])
				if err != nil {
					return Undefined, err
				}
				return ObjectValue(vm.heap.NewArray(n)), nil
			}
			h := vm.heap.NewArray(len(args))
			for i, a := range args {
				vm.heap.SetElement(h, i, a)
			}
			return ObjectValue(h), nil
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), protoV)
	vm.SetGlobal("Array", ctorV)
	vm.installArraySort(arrayProto)

	vm.defineNative(ctor, "isArray", 1, func(vm *VM, args []Value) (Value, error) {
		if len(args) == 0 {
			return False, nil
		}
		v := args[0]
		if !v.IsObject() {
			return False, nil
		}
		o := vm.heap.Get(v.Handle())
		return Bool(o != nil && o.kind == KindArray && o.typedName == ""), nil
	})
	vm.defineNative(ctor, "from", 1, func(vm *VM, args []Value) (Value, error) {
		src := Undefined
		if len(args) > 0 {
			src = args[0]
		}
		mapfn := Undefined
		mapping := false
		if len(args) > 1 && !args[1].IsUndefined() {
			mapfn = args[1]
			mapping = true
		}
		thisArg := Undefined
		if len(args) > 2 {
			thisArg = args[2]
		}
		return vm.arrayFrom(src, mapfn, thisArg, mapping)
	})
	vm.defineNative(ctor, "of", 0, func(vm *VM, args []Value) (Value, error) {
		h := vm.heap.NewArray(len(args))
		for i, a := range args {
			vm.heap.SetElement(h, i, a)
		}
		return ObjectValue(h), nil
	})

	vm.defineNative(arrayProto, "push", 1, func(vm *VM, args []Value) (Value, error) {
		n := vm.BuiltinArrayPush(vm.CurrentThis(), args...)
		return Int(int32(n)), nil
	})
	vm.defineNative(arrayProto, "pop", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.BuiltinArrayPop(vm.CurrentThis()), nil
	})
	vm.defineNative(arrayProto, "slice", 2, func(vm *VM, args []Value) (Value, error) {
		start, end := 0, 1<<30
		if len(args) > 0 {
			start = int(vm.toNumber(args[0]))
		}
		if len(args) > 1 {
			end = int(vm.toNumber(args[1]))
		}
		return vm.BuiltinArraySlice(vm.CurrentThis(), start, end), nil
	})
	vm.defineNative(arrayProto, "reverse", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.arrayReverse(vm.CurrentThis()), nil
	})
	vm.defineNative(arrayProto, "join", 1, func(vm *VM, args []Value) (Value, error) {
		sep := ","
		if len(args) > 0 && !args[0].IsUndefined() {
			sep = vm.toDisplayString(args[0])
		}
		return vm.NewStringValue(str.FromGo(vm.arrayJoin(vm.CurrentThis(), sep))), nil
	})
	vm.defineNative(arrayProto, "toString", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.NewStringValue(str.FromGo(vm.arrayJoin(vm.CurrentThis(), ","))), nil
	})
	vm.defineNative(arrayProto, "concat", 1, func(vm *VM, args []Value) (Value, error) {
		return vm.arrayConcat(vm.CurrentThis(), args), nil
	})
	vm.defineNative(arrayProto, "shift", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.arrayShift(vm.CurrentThis()), nil
	})
	vm.defineNative(arrayProto, "unshift", 1, func(vm *VM, args []Value) (Value, error) {
		return Int(int32(vm.arrayUnshift(vm.CurrentThis(), args))), nil
	})
	vm.defineNative(arrayProto, "indexOf", 1, func(vm *VM, args []Value) (Value, error) {
		needle := Undefined
		if len(args) > 0 {
			needle = args[0]
		}
		return Int(int32(vm.arrayIndexOf(vm.CurrentThis(), needle))), nil
	})
	vm.defineNative(arrayProto, "includes", 1, func(vm *VM, args []Value) (Value, error) {
		needle := Undefined
		if len(args) > 0 {
			needle = args[0]
		}
		return Bool(vm.arrayIndexOf(vm.CurrentThis(), needle) >= 0), nil
	})
	vm.defineNative(arrayProto, "forEach", 1, func(vm *VM, args []Value) (Value, error) {
		if len(args) == 0 {
			return Undefined, nil
		}
		return Undefined, vm.arrayEach(vm.CurrentThis(), args[0], false)
	})
	vm.defineNative(arrayProto, "map", 1, func(vm *VM, args []Value) (Value, error) {
		cb := Undefined
		if len(args) > 0 {
			cb = args[0]
		}
		thisArg := Undefined
		if len(args) > 1 {
			thisArg = args[1]
		}
		return vm.arrayMap(vm.CurrentThis(), cb, thisArg)
	})
	vm.defineNative(arrayProto, "filter", 1, func(vm *VM, args []Value) (Value, error) {
		cb := Undefined
		if len(args) > 0 {
			cb = args[0]
		}
		thisArg := Undefined
		if len(args) > 1 {
			thisArg = args[1]
		}
		return vm.arrayFilter(vm.CurrentThis(), cb, thisArg)
	})
	vm.defineNative(arrayProto, "reduce", 1, func(vm *VM, args []Value) (Value, error) {
		cb := Undefined
		if len(args) > 0 {
			cb = args[0]
		}
		hasInit := len(args) > 1
		init := Undefined
		if hasInit {
			init = args[1]
		}
		return vm.arrayReduce(vm.CurrentThis(), cb, hasInit, init)
	})
	vm.defineNative(arrayProto, "find", 1, func(vm *VM, args []Value) (Value, error) {
		if len(args) == 0 {
			return Undefined, nil
		}
		return vm.arrayFind(vm.CurrentThis(), args[0])
	})
	vm.defineNative(arrayProto, "values", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.newArrayIterator(vm.CurrentThis()), nil
	})
	vm.defineNative(arrayProto, "keys", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.newArrayKeyIterator(vm.CurrentThis()), nil
	})
	vm.defineNative(arrayProto, "entries", 0, func(vm *VM, args []Value) (Value, error) {
		return vm.newArrayEntryIterator(vm.CurrentThis()), nil
	})
	valFn := vm.getProp(protoV, str.FromGo("values"))
	vm.heap.SetProperty(arrayProto, SymbolIterator, valFn)

	vm.markBuiltinAttrs(arrayProto)
	vm.markBuiltinAttrs(ctor)
	vm.heap.SetPropertyAttrs(ctor, vm.heap.Intern().InternGo("prototype"), 0)
	vm.stampFunctionNameLength(ctor)
	vm.stampNativeNameLength(arrayProto)
	vm.stampNativeNameLength(ctor)
}

func (vm *VM) arrayJoin(arr Value, sep string) string {
	if !arr.IsObject() {
		return ""
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return ""
	}
	parts := make([]string, 0, len(o.elements))
	for _, e := range o.elements {
		if e.IsUndefined() || e.IsNull() {
			parts = append(parts, "")
			continue
		}
		parts = append(parts, vm.toDisplayString(e))
	}
	return strings.Join(parts, sep)
}

func (vm *VM) arrayFrom(src, mapfn, thisArg Value, mapping bool) (Value, error) {
	if mapping && !vm.isFunction(mapfn) {
		return Undefined, fmt.Errorf("TypeError: Array.from map function is not callable")
	}
	if src.IsNull() || src.IsUndefined() {
		return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
	}
	useMap := mapping
	if useMap && thisArg.IsObject() && vm.globalObj != NoHandle && thisArg.Handle() == vm.globalObj {
		useMap = false
	}
	apply := func(v Value, i int) (Value, error) {
		if !useMap {
			return v, nil
		}
		return vm.invoke(thisArg, mapfn, []Value{v, Int(int32(i))})
	}
	if s := vm.StringOf(src); s != nil {
		h := vm.heap.NewArray(s.Len())
		outV := ObjectValue(h)
		vm.heap.AddRoot(&outV)
		defer vm.heap.RemoveRoot(&outV)
		for i := 0; i < s.Len(); i++ {
			ch := vm.NewStringValue(s.Slice(i, i+1))
			mapped, err := apply(ch, i)
			if err != nil {
				return Undefined, err
			}
			vm.heap.SetElement(h, i, mapped)
		}
		return outV, nil
	}
	if src.IsObject() {
		meth, err := vm.getPropInvoke(src, SymbolIterator)
		if err != nil {
			return Undefined, err
		}
		_, hasOwnIter := vm.heap.GetOwnProperty(src.Handle(), SymbolIterator)
		if hasOwnIter && vm.isFunction(meth) {
			return vm.arrayFromIterator(src, meth, apply)
		}
		if hasOwnIter && !meth.IsUndefined() && !meth.IsNull() && !vm.isFunction(meth) {
			return Undefined, fmt.Errorf("TypeError: @@iterator is not a function")
		}
		n := vm.arrayLikeLength(src)
		h := vm.heap.NewArray(n)
		outV := ObjectValue(h)
		vm.heap.AddRoot(&outV)
		defer vm.heap.RemoveRoot(&outV)
		for i := 0; i < n; i++ {
			el, err := vm.arrayGetIndex(src, i)
			if err != nil {
				return Undefined, err
			}
			mapped, err := apply(el, i)
			if err != nil {
				return Undefined, err
			}
			vm.heap.SetElement(h, i, mapped)
		}
		return outV, nil
	}
	return ObjectValue(vm.heap.NewArray(0)), nil
}

func (vm *VM) arrayFromIterator(items, meth Value, apply func(Value, int) (Value, error)) (Value, error) {
	it, err := vm.invoke(items, meth, nil)
	if err != nil {
		return Undefined, err
	}
	if !it.IsObject() {
		return Undefined, fmt.Errorf("TypeError: iterator is not an object")
	}
	vm.heap.AddRoot(&it)
	defer vm.heap.RemoveRoot(&it)
	nextFn := vm.getProp(it, str.FromGo("next"))
	if !vm.isFunction(nextFn) {
		return Undefined, fmt.Errorf("TypeError: iterator.next is not a function")
	}
	h := vm.heap.NewArray(0)
	outV := ObjectValue(h)
	vm.heap.AddRoot(&outV)
	defer vm.heap.RemoveRoot(&outV)
	for k := 0; k < 10000; k++ {
		res, err := vm.invoke(it, nextFn, nil)
		if err != nil {
			_ = vm.closeIterator(it)
			return Undefined, err
		}
		if !vm.isJSObject(res) {
			_ = vm.closeIterator(it)
			return Undefined, fmt.Errorf("TypeError: iterator result is not an object")
		}
		if vm.truthy(vm.getProp(res, str.FromGo("done"))) {
			return outV, nil
		}
		val := vm.getProp(res, str.FromGo("value"))
		mapped, err := apply(val, k)
		if err != nil {
			_ = vm.closeIterator(it)
			return Undefined, err
		}
		vm.heap.SetElement(h, k, mapped)
	}
	return outV, nil
}

func (vm *VM) arrayConcat(arr Value, extra []Value) Value {
	out := vm.heap.NewArray(0)
	appendArr := func(v Value) {
		if v.IsObject() {
			if o := vm.heap.Get(v.Handle()); o != nil && o.kind == KindArray {
				for _, e := range o.elements {
					vm.heap.SetElement(out, len(vm.heap.MustGet(out).elements), e)
				}
				return
			}
		}
		o := vm.heap.MustGet(out)
		vm.heap.SetElement(out, len(o.elements), v)
	}
	appendArr(arr)
	for _, e := range extra {
		appendArr(e)
	}
	return ObjectValue(out)
}

func (vm *VM) arrayShift(arr Value) Value {
	if !arr.IsObject() {
		return Undefined
	}
	o := vm.heap.Get(arr.Handle())
	if o != nil && o.kind == KindArray {
		if len(o.elements) == 0 {
			return Undefined
		}
		v := o.elements[0]
		o.elements = o.elements[1:]
		return v
	}
	n := vm.arrayLikeLength(arr)
	if n == 0 {
		return Undefined
	}
	v := vm.getElem(arr, Int(0))
	for i := 1; i < n; i++ {
		el := vm.getElem(arr, Int(int32(i)))
		_ = vm.setElem(&frame{}, arr, Int(int32(i-1)), el)
	}
	vm.heap.SetProperty(arr.Handle(), vm.heap.Intern().InternGo("length"), Int(int32(n-1)))
	return v
}

func (vm *VM) arrayReverse(arr Value) Value {
	if !arr.IsObject() {
		return arr
	}
	o := vm.heap.Get(arr.Handle())
	if o != nil && o.kind == KindArray {
		n := len(o.elements)
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			o.elements[i], o.elements[j] = o.elements[j], o.elements[i]
		}
		return arr
	}
	n := vm.arrayLikeLength(arr)
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		vi := vm.getElem(arr, Int(int32(i)))
		vj := vm.getElem(arr, Int(int32(j)))
		_ = vm.setElem(&frame{}, arr, Int(int32(i)), vj)
		_ = vm.setElem(&frame{}, arr, Int(int32(j)), vi)
	}
	return arr
}

func (vm *VM) arrayUnshift(arr Value, items []Value) int {
	if !arr.IsObject() {
		return 0
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return 0
	}
	o.elements = append(append([]Value{}, items...), o.elements...)
	return len(o.elements)
}

func (vm *VM) arrayIndexOf(arr, needle Value) int {
	if !arr.IsObject() {
		return -1
	}
	o := vm.heap.Get(arr.Handle())
	if o != nil && o.kind == KindArray {
		n := len(o.elements)
		if n > 10000 {
			n = 10000
		}
		for i := 0; i < n; i++ {
			vm.GasLeft--
			if vm.GasLeft <= 0 {
				return -1
			}
			if vm.strictEq(o.elements[i], needle) {
				return i
			}
		}
		return -1
	}
	n := vm.arrayLikeLength(arr)
	for i := 0; i < n; i++ {
		if vm.strictEq(vm.getElem(arr, Int(int32(i))), needle) {
			return i
		}
	}
	return -1
}

func (vm *VM) arrayEach(arr, cb Value, _ bool) error {
	if !arr.IsObject() {
		return nil
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return nil
	}
	n := len(o.elements)
	for i := 0; i < n; i++ {
		o = vm.heap.Get(arr.Handle())
		if o == nil || i >= len(o.elements) {
			break
		}
		el := o.elements[i]
		if _, err := vm.invoke(Undefined, cb, []Value{el, Int(int32(i)), arr}); err != nil {
			return err
		}
	}
	return nil
}

func (vm *VM) arrayRequireObject(v Value) (Value, error) {
	if v.IsNull() || v.IsUndefined() {
		return Undefined, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
	}
	return vm.toObject(v), nil
}

func (vm *VM) arrayIndexPresent(arr Value, i int) bool {
	if s := vm.StringOf(arr); s != nil {
		return i >= 0 && i < s.Len()
	}
	if !arr.IsObject() {
		return false
	}
	_, ok := vm.heap.GetProperty(arr.Handle(), vm.heap.Intern().InternGo(strconv.Itoa(i)))
	return ok
}

func (vm *VM) arrayGetIndex(arr Value, i int) (Value, error) {
	if s := vm.StringOf(arr); s != nil {
		if i >= 0 && i < s.Len() {
			return vm.NewStringValue(s.Slice(i, i+1)), nil
		}
		return Undefined, nil
	}
	return vm.getPropInvoke(arr, vm.heap.Intern().InternGo(strconv.Itoa(i)))
}

func (vm *VM) arrayMap(arr, cb, thisArg Value) (Value, error) {
	o, err := vm.arrayRequireObject(arr)
	if err != nil {
		return Undefined, err
	}
	if !vm.isFunction(cb) {
		return Undefined, fmt.Errorf("TypeError: Array.prototype.map callback is not a function")
	}
	n := vm.arrayLikeLength(o)
	out := vm.heap.NewArray(n)
	outV := ObjectValue(out)
	vm.heap.AddRoot(&outV)
	defer vm.heap.RemoveRoot(&outV)
	for i := 0; i < n; i++ {
		if !vm.arrayIndexPresent(o, i) {
			continue
		}
		el, err := vm.arrayGetIndex(o, i)
		if err != nil {
			return Undefined, err
		}
		v, err := vm.invoke(thisArg, cb, []Value{el, Int(int32(i)), o})
		if err != nil {
			return Undefined, err
		}
		vm.heap.SetElement(out, i, v)
	}
	return outV, nil
}

func (vm *VM) arrayFilter(arr, cb, thisArg Value) (Value, error) {
	o, err := vm.arrayRequireObject(arr)
	if err != nil {
		return Undefined, err
	}
	if !vm.isFunction(cb) {
		return Undefined, fmt.Errorf("TypeError: Array.prototype.filter callback is not a function")
	}
	n := vm.arrayLikeLength(o)
	out := vm.heap.NewArray(0)
	outV := ObjectValue(out)
	vm.heap.AddRoot(&outV)
	defer vm.heap.RemoveRoot(&outV)
	for i := 0; i < n; i++ {
		if !vm.arrayIndexPresent(o, i) {
			continue
		}
		el, err := vm.arrayGetIndex(o, i)
		if err != nil {
			return Undefined, err
		}
		v, err := vm.invoke(thisArg, cb, []Value{el, Int(int32(i)), o})
		if err != nil {
			return Undefined, err
		}
		if vm.truthy(v) {
			oo := vm.heap.MustGet(out)
			vm.heap.SetElement(out, len(oo.elements), el)
		}
	}
	return outV, nil
}

func (vm *VM) arrayReduce(arr, cb Value, hasInit bool, init Value) (Value, error) {
	o, err := vm.arrayRequireObject(arr)
	if err != nil {
		return Undefined, err
	}
	if !vm.isFunction(cb) {
		return Undefined, fmt.Errorf("TypeError: Array.prototype.reduce callback is not a function")
	}
	n := vm.arrayLikeLength(o)
	acc := init
	i := 0
	if !hasInit {
		found := false
		for ; i < n; i++ {
			if !vm.arrayIndexPresent(o, i) {
				continue
			}
			acc, err = vm.arrayGetIndex(o, i)
			if err != nil {
				return Undefined, err
			}
			found = true
			i++
			break
		}
		if !found {
			return Undefined, fmt.Errorf("TypeError: Reduce of empty array with no initial value")
		}
	}
	for ; i < n; i++ {
		if !vm.arrayIndexPresent(o, i) {
			continue
		}
		el, err := vm.arrayGetIndex(o, i)
		if err != nil {
			return Undefined, err
		}
		v, err := vm.invoke(Undefined, cb, []Value{acc, el, Int(int32(i)), o})
		if err != nil {
			return Undefined, err
		}
		acc = v
	}
	return acc, nil
}

func (vm *VM) arrayFind(arr, cb Value) (Value, error) {
	if !arr.IsObject() {
		return Undefined, nil
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return Undefined, nil
	}
	n := len(o.elements)
	for i := 0; i < n; i++ {
		o = vm.heap.Get(arr.Handle())
		if o == nil || i >= len(o.elements) {
			break
		}
		el := o.elements[i]
		v, err := vm.invoke(Undefined, cb, []Value{el, Int(int32(i)), arr})
		if err != nil {
			return Undefined, err
		}
		if vm.truthy(v) {
			return el, nil
		}
	}
	return Undefined, nil
}

func (vm *VM) arrayAllocLength(v Value) (int, error) {
	n, err := vm.toNumberErr(v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n != math.Trunc(n) || n > 4294967295 || n >= float64(int(^uint(0)>>1)) {
		return 0, vm.throwText(nil, "RangeError: Invalid array length")
	}
	return int(n), nil
}

func (vm *VM) BuiltinArrayPush(arr Value, items ...Value) int {
	if !arr.IsObject() {
		return 0
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return 0
	}
	o.elements = append(o.elements, items...)
	return len(o.elements)
}

func (vm *VM) arrayLikeLength(arr Value) int {
	if !arr.IsObject() {
		return 0
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil {
		return 0
	}
	if o.kind == KindArray {
		if o.env != NoHandle && o.typedName != "" {
			buf, _, _, length, err := vm.typedArrayBounds(o)
			if err != nil || buf == nil {
				return 0
			}
			return length
		}
		return len(o.elements)
	}
	n := int(vm.toNumber(vm.getProp(arr, str.FromGo("length"))))
	if n < 0 {
		return 0
	}
	return n
}

func (vm *VM) BuiltinArrayPop(arr Value) Value {
	if !arr.IsObject() {
		return Undefined
	}
	o := vm.heap.Get(arr.Handle())
	if o != nil && o.kind == KindArray {
		if len(o.elements) == 0 {
			return Undefined
		}
		lastIdx := len(o.elements) - 1
		val := o.elements[lastIdx]
		o.elements = o.elements[:lastIdx]
		return val
	}
	n := vm.arrayLikeLength(arr)
	lk := vm.heap.Intern().InternGo("length")
	if n == 0 {
		vm.heap.SetProperty(arr.Handle(), lk, Int(0))
		return Undefined
	}
	v := vm.getElem(arr, Int(int32(n-1)))
	vm.heap.SetProperty(arr.Handle(), lk, Int(int32(n-1)))
	return v
}

func (vm *VM) BuiltinArraySlice(arr Value, start, end int) Value {
	if !arr.IsObject() {
		return Undefined
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray {
		return Undefined
	}
	length := len(o.elements)
	if start < 0 {
		start = length + start
		if start < 0 {
			start = 0
		}
	}
	if end < 0 {
		end = length + end
	}
	if end > length {
		end = length
	}
	if start >= end || start >= length {
		return ObjectValue(vm.heap.NewArray(0))
	}
	sliced := vm.heap.NewArray(end - start)
	for i := start; i < end; i++ {
		vm.heap.SetElement(sliced, i-start, o.elements[i])
	}
	return ObjectValue(sliced)
}
