// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"encoding/binary"
	"github.com/hazyhaar/js55/pkg/js55/str"
	"math"
)

func taByteSize(name string) int {
	switch name {
	case "Int8Array", "Uint8Array", "Uint8ClampedArray":
		return 1
	case "Int16Array", "Uint16Array", "Float16Array":
		return 2
	case "Int32Array", "Uint32Array", "Float32Array":
		return 4
	case "Float64Array", "BigInt64Array", "BigUint64Array":
		return 8
	default:
		return 1
	}
}

func (vm *VM) coerceTypedElement(v Value, name string) (Value, error) {
	if name == "BigInt64Array" || name == "BigUint64Array" {
		if _, ok := vm.bigIntOf(v); ok {
			return v, nil
		}
		return Undefined, vm.throwText(nil, "TypeError: BigInt typed array requires BigInt elements")
	}
	n, err := vm.toNumberErr(v)
	if err != nil {
		return Undefined, err
	}
	switch name {
	case "Float32Array":
		return Number(float64(float32(n))), nil
	case "Float64Array":
		return Number(n), nil
	case "Uint8ClampedArray":
		if math.IsNaN(n) || n < 0 {
			n = 0
		}
		if n > 255 {
			n = 255
		}
		return Number(math.RoundToEven(n)), nil
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		n = 0
	}
	n = math.Mod(math.Trunc(n), 4294967296)
	if n < 0 {
		n += 4294967296
	}
	u := uint32(n)
	switch name {
	case "Int8Array":
		return Int(int32(int8(u))), nil
	case "Uint8Array":
		return Int(int32(uint8(u))), nil
	case "Int16Array":
		return Int(int32(int16(u))), nil
	case "Uint16Array":
		return Int(int32(uint16(u))), nil
	case "Int32Array":
		return Int(int32(u)), nil
	case "Uint32Array":
		return Number(float64(u)), nil
	}
	return v, nil
}

func (vm *VM) InstallTypedArrayBuiltins() {
	vm.installBigInt()
	vm.installArrayBuffer()
	vm.installTypedArrayCtor("Uint8Array")
	vm.installTypedArrayCtor("DataView")
}

func (vm *VM) installBigInt() {
	proto := vm.heap.NewObject()
	protoV := ObjectValue(proto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)
	ctor := vm.heap.NewFunction(&Chunk{
		Name:   "BigInt",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			n := int64(0)
			if len(args) > 0 {
				if bi, ok := vm.bigIntOf(args[0]); ok {
					n = bi
				} else {
					n = int64(vm.toNumber(args[0]))
				}
			}
			return vm.newBigInt(n), nil
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), protoV)
	vm.heap.SetProperty(proto, vm.heap.Intern().InternGo("constructor"), ctorV)
	vm.SetGlobal("BigInt", ctorV)
}

func (vm *VM) installArrayBuffer() {
	proto := vm.heap.NewObject()
	protoV := ObjectValue(proto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)

	ctor := vm.heap.NewFunction(&Chunk{
		Name:      "ArrayBuffer",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			return vm.arrayBufferFromArgs(args, proto)
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), protoV)
	vm.heap.SetProperty(proto, vm.heap.Intern().InternGo("constructor"), ctorV)
	vm.defineNative(proto, "resize", 1, func(vm *VM, args []Value) (Value, error) {
		return Undefined, vm.arrayBufferResize(vm.CurrentThis(), args)
	})
	vm.SetGlobal("ArrayBuffer", ctorV)
}

func (vm *VM) arrayBufferFromArgs(args []Value, proto Handle) (Value, error) {
	n := 0
	var err error
	if len(args) > 0 {
		n, err = vm.bufferIndex(args[0])
		if err != nil {
			return Undefined, err
		}
	}
	max := n
	resizable := false
	if len(args) > 1 && args[1].IsObject() {
		if v := vm.getProp(args[1], str.FromGo("maxByteLength")); !v.IsUndefined() {
			max, err = vm.bufferIndex(v)
			if err != nil {
				return Undefined, err
			}
			if max < n {
				return Undefined, vm.throwText(nil, "RangeError: maxByteLength is smaller than byteLength")
			}
			resizable = true
		}
	}
	h := vm.heap.NewObject()
	o := vm.heap.Get(h)
	if o != nil {
		o.proto = proto
		vm.heap.TrackAlloc(int64(n))
		o.bytes = make([]byte, n)
		o.arrayBuffer = true
		o.resizable = resizable
		o.maxByteLength = max
	}
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	return hv, nil
}

// ToIndex, with an explicit host-size rejection instead of integer overflow.
func (vm *VM) bufferIndex(v Value) (int, error) {
	n, err := vm.toNumberErr(v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) {
		return 0, nil
	}
	n = math.Trunc(n)
	if n < 0 || n > 9007199254740991 || n >= float64(int(^uint(0)>>1)) {
		return 0, vm.throwText(nil, "RangeError: invalid buffer index")
	}
	return int(n), nil
}

func (vm *VM) arrayBufferResize(this Value, args []Value) error {
	if !this.IsObject() || !vm.isArrayBuffer(this) {
		return vm.throwText(&frame{}, "TypeError: ArrayBuffer.prototype.resize called on incompatible receiver")
	}
	n := 0
	var err error
	if len(args) > 0 {
		n, err = vm.bufferIndex(args[0])
		if err != nil {
			return err
		}
	}
	max := int(vm.toNumber(vm.getProp(this, str.FromGo("maxByteLength"))))
	if n < 0 || n > max {
		return vm.throwText(&frame{}, "RangeError: Invalid array buffer length")
	}
	o := vm.heap.Get(this.Handle())
	if o == nil {
		return vm.throwText(&frame{}, "TypeError: ArrayBuffer.prototype.resize called on incompatible receiver")
	}
	if !o.resizable {
		return vm.throwText(nil, "TypeError: ArrayBuffer is not resizable")
	}
	old := len(o.bytes)
	if n == old {
		return nil
	}
	if n > 0 {
		vm.heap.TrackAlloc(int64(n))
	}
	data := make([]byte, n)
	copy(data, o.bytes)
	o.bytes = data
	if old > 0 {
		vm.heap.TrackFree(int64(old))
	}
	return nil
}

func (vm *VM) isArrayBuffer(v Value) bool {
	if !v.IsObject() {
		return false
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind == KindArray || o.kind == KindFunction {
		return false
	}
	return o.arrayBuffer
}

func (vm *VM) installTypedArrayCtor(name string) {
	bpe := taByteSize(name)
	proto := vm.heap.NewObject()
	protoV := ObjectValue(proto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)

	ch := &Chunk{
		Name:      name,
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			return vm.typedArrayFromArgs(args, proto, bpe, name)
		},
	}
	h := vm.heap.NewFunction(ch, NoHandle)
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("prototype"), protoV)
	vm.heap.SetProperty(proto, vm.heap.Intern().InternGo("constructor"), hv)
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("BYTES_PER_ELEMENT"), Int(int32(bpe)))
	vm.heap.SetProperty(proto, vm.heap.Intern().InternGo("BYTES_PER_ELEMENT"), Int(int32(bpe)))
	if vm.heap.arrayProto != NoHandle {
		if valFn, ok := vm.heap.GetProperty(vm.heap.arrayProto, vm.heap.Intern().InternGo("values")); ok {
			vm.heap.SetProperty(proto, str.FromGo("values"), valFn)
			vm.heap.SetProperty(proto, SymbolIterator, valFn)
		}
	}
	if name != "DataView" {
		vm.defineNative(proto, "subarray", 2, func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			o := vm.heap.Get(this.Handle())
			if !this.IsObject() || o == nil || o.typedName == "" || vm.taOutOfBounds(this) {
				return Undefined, vm.throwText(nil, "TypeError: incompatible TypedArray receiver")
			}
			n := vm.arrayLikeLength(this)
			bounds := [2]int{0, n}
			for i := 0; i < len(args) && i < 2; i++ {
				if args[i].IsUndefined() {
					continue
				}
				x, err := vm.toNumberErr(args[i])
				if err != nil {
					return Undefined, err
				}
				if math.IsNaN(x) {
					x = 0
				}
				x = math.Trunc(x)
				if x < 0 {
					x = math.Max(float64(n)+x, 0)
				} else {
					x = math.Min(x, float64(n))
				}
				bounds[i] = int(x)
			}
			length := bounds[1] - bounds[0]
			if length < 0 {
				length = 0
			}
			viewArgs := []Value{ObjectValue(o.env), Number(float64(o.byteOffset + bounds[0]*taByteSize(o.typedName))), Number(float64(length))}
			if o.lengthTracking && (len(args) < 2 || args[1].IsUndefined()) {
				viewArgs = viewArgs[:2]
			}
			return vm.typedArrayFromBuffer(viewArgs, o.proto, taByteSize(o.typedName), o.typedName)
		})
		vm.defineNative(proto, "set", 1, func(vm *VM, args []Value) (Value, error) {
			this := vm.CurrentThis()
			o := vm.heap.Get(this.Handle())
			if !this.IsObject() || o == nil || o.typedName == "" {
				return Undefined, vm.throwText(&frame{}, "TypeError: incompatible TypedArray receiver")
			}
			if len(args) == 0 || args[0].IsNull() || args[0].IsUndefined() {
				return Undefined, vm.throwText(&frame{}, "TypeError: invalid TypedArray source")
			}
			offset := 0.0
			if len(args) > 1 {
				var err error
				offset, err = vm.toNumberErr(args[1])
				if err != nil {
					return Undefined, err
				}
			}
			if math.IsNaN(offset) {
				offset = 0
			}
			offset = math.Trunc(offset)
			n, capacity := vm.arrayLikeLength(args[0]), vm.arrayLikeLength(this)
			if offset < 0 || offset > float64(capacity) || float64(n) > float64(capacity)-offset {
				return Undefined, vm.throwText(&frame{}, "RangeError: TypedArray source exceeds destination")
			}
			if vm.taOutOfBounds(this) {
				return Undefined, vm.throwText(nil, "TypeError: out of bounds TypedArray")
			}
			if source := vm.heap.Get(args[0].Handle()); args[0].IsObject() && source != nil && source.typedName != "" {
				if vm.taOutOfBounds(args[0]) {
					return Undefined, vm.throwText(nil, "TypeError: out of bounds TypedArray source")
				}
				if source.typedName == o.typedName {
					size := taByteSize(o.typedName)
					dstObj := vm.heap.Get(o.env)
					srcObj := vm.heap.Get(source.env)
					if dstObj == nil || srcObj == nil {
						return Undefined, vm.throwText(nil, "TypeError: detached TypedArray")
					}
					start := o.byteOffset + int(offset)*size
					copy(dstObj.bytes[start:start+n*size], srcObj.bytes[source.byteOffset:source.byteOffset+n*size])
					return Undefined, nil
				}
			}
			// Snapshot before writes: source and target can share a buffer.
			vm.heap.TrackAlloc(int64(n) * 8)
			defer vm.heap.TrackFree(int64(n) * 8)
			values := make([]Value, n)
			defer func() {
				for i := range values {
					if values[i].IsObject() {
						vm.heap.RemoveRoot(&values[i])
					}
				}
			}()
			for i := range values {
				v, err := vm.arrayIterIndex(args[0], i)
				if err != nil {
					return Undefined, err
				}
				values[i] = v
				if v.IsObject() {
					vm.heap.AddRoot(&values[i])
				}
			}
			for i, v := range values {
				if err := vm.setElem(&frame{}, this, Int(int32(int(offset)+i)), v); err != nil {
					return Undefined, err
				}
			}
			return Undefined, nil
		})
	}
	vm.SetGlobal(name, hv)
}

func (vm *VM) typedArrayFromArgs(args []Value, proto Handle, bpe int, name string) (Value, error) {
	if len(args) > 0 && vm.isArrayBuffer(args[0]) {
		return vm.typedArrayFromBuffer(args, proto, bpe, name)
	}
	n := 0
	var err error
	if len(args) > 0 {
		if args[0].IsObject() {
			n = vm.arrayLikeLength(args[0])
		} else {
			n, err = vm.bufferIndex(args[0])
			if err != nil {
				return Undefined, err
			}
		}
	}
	if n > int(^uint(0)>>1)/bpe {
		return Undefined, vm.throwText(nil, "RangeError: TypedArray is too large")
	}
	ctor, _ := vm.GetGlobal("ArrayBuffer")
	abProto := vm.getProp(ctor, str.FromGo("prototype"))
	buf, err := vm.arrayBufferFromArgs([]Value{Number(float64(n) * float64(bpe))}, abProto.Handle())
	if err != nil {
		return Undefined, err
	}
	vm.heap.AddRoot(&buf)
	defer vm.heap.RemoveRoot(&buf)
	hv, err := vm.typedArrayFromBuffer([]Value{buf, Int(0), Number(float64(n))}, proto, bpe, name)
	if err != nil {
		return Undefined, err
	}
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	if len(args) > 0 && args[0].IsObject() {
		for i := 0; i < n; i++ {
			v, err := vm.arrayIterIndex(args[0], i)
			if err != nil {
				return Undefined, err
			}
			v, err = vm.coerceTypedElement(v, name)
			if err != nil {
				return Undefined, err
			}
			if err = vm.typedArraySetIndex(nil, hv, i, v); err != nil {
				return Undefined, err
			}
		}
	}
	return hv, nil
}

func (vm *VM) typedArrayFromBuffer(args []Value, proto Handle, bpe int, name string) (Value, error) {
	if bpe < 1 {
		bpe = 1
	}
	buf := args[0]
	offset := 0
	var err error
	if len(args) > 1 && !args[1].IsUndefined() {
		offset, err = vm.bufferIndex(args[1])
		if err != nil {
			return Undefined, err
		}
	}
	bo := vm.heap.Get(buf.Handle())
	if bo == nil || !bo.arrayBuffer {
		return Undefined, vm.throwText(nil, "TypeError: invalid ArrayBuffer")
	}
	bufLen := len(bo.bytes)
	if offset > bufLen || offset%bpe != 0 {
		return Undefined, vm.throwText(nil, "RangeError: invalid TypedArray offset")
	}
	omitted := len(args) < 3 || args[2].IsUndefined()
	tracking := omitted && bo.resizable
	length := 0
	if omitted {
		if !tracking && (bufLen-offset)%bpe != 0 {
			return Undefined, vm.throwText(nil, "RangeError: unaligned TypedArray length")
		}
		length = (bufLen - offset) / bpe
	} else {
		length, err = vm.bufferIndex(args[2])
		if err != nil {
			return Undefined, err
		}
		if length > (bufLen-offset)/bpe {
			return Undefined, vm.throwText(nil, "RangeError: TypedArray exceeds buffer")
		}
	}
	h := vm.heap.NewArray(0)
	o := vm.heap.Get(h)
	if o != nil {
		o.typedName = name
		o.env = buf.Handle()
		o.byteOffset = offset
		o.typedLength = length
		o.lengthTracking = tracking
		o.proto = proto
	}
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	return hv, nil
}

func (vm *VM) typedArrayView(arr Value) (buf *Object, offset, bpe, fixedLen int, tracking, ok bool) {
	if !arr.IsObject() {
		return
	}
	o := vm.heap.Get(arr.Handle())
	if o == nil || o.kind != KindArray || o.env == NoHandle {
		return
	}
	buf = vm.heap.Get(o.env)
	if buf == nil {
		return
	}
	offset = o.byteOffset
	bpe = taByteSize(o.typedName)
	tracking = o.lengthTracking
	fixedLen = o.typedLength
	ok = true
	return
}

func (vm *VM) taOutOfBounds(arr Value) bool {
	buf, offset, bpe, fixedLen, tracking, ok := vm.typedArrayView(arr)
	if !ok {
		return false
	}
	n := len(buf.bytes)
	if n < offset {
		return true
	}
	if tracking {
		return false
	}
	need := offset + fixedLen*bpe
	return n < need
}

func (vm *VM) typedArrayBounds(o *Object) (buf *Object, offset, bpe, length int, err error) {
	if o == nil || o.kind != KindArray || o.env == NoHandle {
		return nil, 0, 0, 0, nil
	}
	buf = vm.heap.Get(o.env)
	if buf == nil {
		return nil, 0, 0, 0, nil
	}
	offset = o.byteOffset
	bpe = taByteSize(o.typedName)
	n := len(buf.bytes)
	if n < offset {
		return nil, 0, 0, 0, vm.throwText(&frame{}, "TypeError: Cannot perform TypedArray index get on a typed array that is out of bounds")
	}
	if o.lengthTracking {
		length = (n - offset) / bpe
		if length < 0 {
			length = 0
		}
		return buf, offset, bpe, length, nil
	}
	if n < offset+o.typedLength*bpe {
		return nil, 0, 0, 0, vm.throwText(&frame{}, "TypeError: Cannot perform TypedArray index get on a typed array that is out of bounds")
	}
	return buf, offset, bpe, o.typedLength, nil
}

func (vm *VM) typedArrayIndex(arr Value, i int) (Value, error) {
	if !arr.IsObject() {
		return Undefined, nil
	}
	return vm.typedArrayIndexObj(vm.heap.Get(arr.Handle()), i)
}

func (vm *VM) typedArrayIndexObj(o *Object, i int) (Value, error) {
	buf, offset, bpe, length, err := vm.typedArrayBounds(o)
	if err != nil {
		return Undefined, err
	}
	if buf == nil || i < 0 || i >= length {
		return Undefined, nil
	}
	idx := offset + i*bpe
	data := buf.bytes[idx : idx+bpe]
	switch o.typedName {
	case "Float32Array":
		return Number(float64(math.Float32frombits(binary.NativeEndian.Uint32(data)))), nil
	case "Float64Array":
		return Number(math.Float64frombits(binary.NativeEndian.Uint64(data))), nil
	case "Int8Array":
		return Int(int32(int8(data[0]))), nil
	case "Int16Array":
		return Int(int32(int16(binary.NativeEndian.Uint16(data)))), nil
	case "Uint16Array":
		return Int(int32(binary.NativeEndian.Uint16(data))), nil
	case "Int32Array":
		return Int(int32(binary.NativeEndian.Uint32(data))), nil
	case "Uint32Array":
		return Number(float64(binary.NativeEndian.Uint32(data))), nil
	case "BigInt64Array", "BigUint64Array":
		return vm.newBigInt(int64(binary.NativeEndian.Uint64(data))), nil
	default:
		return Int(int32(data[0])), nil
	}
}

func (vm *VM) typedArraySetIndex(fr *frame, arr Value, i int, v Value) error {
	if !arr.IsObject() {
		return nil
	}
	return vm.typedArraySetIndexObj(fr, vm.heap.Get(arr.Handle()), i, v)
}

func (vm *VM) typedArraySetIndexObj(fr *frame, o *Object, i int, v Value) error {
	buf, offset, bpe, length, err := vm.typedArrayBounds(o)
	if err != nil {
		if fr != nil {
			return vm.throwText(fr, "TypeError: Cannot perform TypedArray index set on a typed array that is out of bounds")
		}
		return err
	}
	if buf == nil || i < 0 || i >= length {
		return nil
	}
	idx := offset + i*bpe
	data := buf.bytes[idx : idx+bpe]
	n := vm.toNumber(v)
	switch o.typedName {
	case "Float32Array":
		binary.NativeEndian.PutUint32(data, math.Float32bits(float32(n)))
	case "Float64Array":
		binary.NativeEndian.PutUint64(data, math.Float64bits(n))
	case "Int16Array", "Uint16Array":
		binary.NativeEndian.PutUint16(data, uint16(int64(n)))
	case "Int32Array", "Uint32Array":
		binary.NativeEndian.PutUint32(data, uint32(int64(n)))
	case "BigInt64Array", "BigUint64Array":
		bi, _ := vm.bigIntOf(v)
		binary.NativeEndian.PutUint64(data, uint64(bi))
	default:
		data[0] = byte(int64(n))
	}
	return nil
}

func (vm *VM) TypedArrayByteWindow(v Value) (data []byte, ok bool) {
	if vm.taOutOfBounds(v) {
		return nil, false
	}
	buf, offset, bpe, _, _, ok := vm.typedArrayView(v)
	if !ok || buf == nil {
		return nil, false
	}
	n := vm.arrayLikeLength(v)
	end := offset + n*bpe
	if offset < 0 || end < offset || end > len(buf.bytes) {
		return nil, false
	}
	return buf.bytes[offset:end], true
}
