// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"encoding/binary"
	"runtime"
	"unsafe"
)

// Admission never calls JavaScript or a host callback. No engine allocation/GC
// occurs between obtaining the byte window and publishing all six slots.
func (vm *VM) tryArchtimeGeometry(fr *frame, c *Chunk, argc int, thisVal Value) (bool, Value, error) {
	if c.archtimeTag != archtimeTagBox3SetFromBufferAttribute {
		return false, Undefined, nil
	}
	reject := func() (bool, Value, error) { vm.KernelStats.Rejected++; return false, Undefined, nil }
	if argc < 1 || c.Native != nil || c.archtimeHash == "" || c.archtimeHash != hashChunk(c) {
		return reject()
	}
	if !thisVal.IsObject() {
		return reject()
	}
	attr := vm.peek(argc - 1)
	if !attr.IsObject() {
		return reject()
	}
	a, b := vm.heap.Get(attr.Handle()), vm.heap.Get(thisVal.Handle())
	if !archtimeOrdinary(a) || !archtimeOrdinary(b) {
		return reject()
	}
	// The only global read in the closed compiled body is the compiler TDZ token.
	if key := vm.heap.intern.InternGo("\x00tdz"); vm.globals != nil {
		if cur, ok := vm.globals[key]; ok && cur != vm.tdzTok {
			return reject()
		}
	}
	av, ok := vm.getOrdinaryDataSlot(a, "array")
	if !ok || !av.IsObject() {
		return reject()
	}
	size, ok := vm.getOrdinaryDataSlot(a, "itemSize")
	if !ok || !size.IsNumber() || size.ToFloat() != 3 {
		return reject()
	}
	norm, ok := vm.getOrdinaryDataSlot(a, "normalized")
	if !ok || norm != False {
		return reject()
	}
	arr := vm.heap.Get(av.Handle())
	if arr == nil || arr.kind != KindArray || arr.typedName != "Float32Array" || arr.lengthTracking || len(arr.deleted) != 0 || arr.frozen || arr.byteOffset < 0 || arr.byteOffset%4 != 0 || arr.typedLength <= 0 || arr.typedLength%3 != 0 {
		return reject()
	}
	backing := vm.heap.Get(arr.env)
	if !archtimeOrdinary(backing) || !backing.arrayBuffer || backing.resizable {
		return reject()
	}
	// Subtraction-based bounds; no float-to-int conversion or overflowing addition.
	off := arr.byteOffset
	if off > len(backing.bytes) || arr.typedLength > (len(backing.bytes)-off)/4 {
		return reject()
	}
	n := arr.typedLength / 3
	// Large atomic calls are native-only while a cooperative callback is
	// configured. The existing callback granularity is never silently relaxed.
	if n > archtimeMaxCallbackPoints && vm.OnCheckpoint != nil {
		return reject()
	}
	count, ok := vm.getOrdinaryDataSlot(a, "count")
	if !ok || !count.IsNumber() || count.ToFloat() != float64(n) {
		return reject()
	}
	// Indexed typed-array reads precede ordinary properties in JS55. Nonetheless
	// custom descriptors on the view are excluded from this first closed contract.
	for i := range arr.slots {
		if arr.slotAttr(i) != attrDefault {
			return reject()
		}
		v := arr.slots[i]
		if v.IsObject() {
			o := vm.heap.Get(v.Handle())
			if o != nil && o.kind == KindAccessor {
				return reject()
			}
		}
	}
	minv, ok := vm.getOrdinaryDataSlot(b, "min")
	if !ok || !minv.IsObject() {
		return reject()
	}
	maxv, ok := vm.getOrdinaryDataSlot(b, "max")
	if !ok || !maxv.IsObject() {
		return reject()
	}
	mino, maxo := vm.heap.Get(minv.Handle()), vm.heap.Get(maxv.Handle())
	if !archtimeOrdinary(mino) || !archtimeOrdinary(maxo) || !vm.checkVector3Slots(mino) || !vm.checkVector3Slots(maxo) {
		return reject()
	}
	if !vm.pureResolveMethod(a, "getX", archtimeTagBufferAttributeGetX) || !vm.pureResolveMethod(a, "getY", archtimeTagBufferAttributeGetY) || !vm.pureResolveMethod(a, "getZ", archtimeTagBufferAttributeGetZ) || !vm.pureResolveMethod(mino, "set", archtimeTagVector3Set) || !vm.pureResolveMethod(maxo, "set", archtimeTagVector3Set) {
		return reject()
	}
	// 1024 opcodes/point exceeds the sum of bytes of the closed loop body and
	// its three getters; 2048 covers entry, final setters and fixed guard work.
	// Checkpoints cannot be crossed silently. A rejected finite scan is billed too.
	// Derive the admissible length from remaining gas, not a terrain-excluding
	// fixed cap. Division precedes multiplication on both 32- and 64-bit hosts.
	if vm.GasLeft <= archtimeGasBase || int64(n) > (vm.GasLeft-archtimeGasBase-1)/archtimeGasPerPoint {
		return reject()
	}
	cost := int64(archtimeGasBase) + int64(archtimeGasPerPoint)*int64(n)
	if vm.GasLeft <= cost || vm.interrupted() || len(vm.frames)+3 >= vm.MaxDepth {
		return reject()
	}
	if vm.CheckpointEvery > 0 {
		rem := vm.GasLeft % vm.CheckpointEvery
		if rem == 0 || cost >= rem {
			return reject()
		}
	}
	data := backing.bytes[off : off+arr.typedLength*4]
	if binary.NativeEndian.Uint32([]byte{1, 0, 0, 0}) != 1 || uintptr(unsafe.Pointer(&data[0]))%unsafe.Alignof(float32(0)) != 0 {
		return reject()
	}
	points := unsafe.Slice((*float32)(unsafe.Pointer(&data[0])), arr.typedLength)
	vm.GasLeft -= cost
	var out [6]float64
	accepted := archtimeBoundsXYZ(points, &out)
	runtime.KeepAlive(data)
	runtime.KeepAlive(backing)
	if !accepted {
		return reject()
	}
	if err := vm.commitArchtimeBounds(mino, maxo, &out); err != nil {
		return true, Undefined, err
	}
	vm.KernelStats.Accepted++
	return true, thisVal, nil
}

// No user code executes between admission and this publication boundary. The
// synchronous kernel only writes its private stack result. An asynchronous
// interrupt arriving during the calculation therefore aborts without publishing
// a partial result or replaying any JavaScript effects.
func (vm *VM) commitArchtimeBounds(mino, maxo *Object, out *[6]float64) error {
	if vm.interrupted() {
		return ErrInterrupted
	}
	vm.setVector3SlotsUnsafe(mino, out[0], out[1], out[2])
	vm.setVector3SlotsUnsafe(maxo, out[3], out[4], out[5])
	return nil
}

func archtimeOrdinary(o *Object) bool {
	return o != nil && o.kind == KindOrdinary && !o.frozen && len(o.deleted) == 0
}

func (vm *VM) getOrdinaryDataSlot(o *Object, name string) (Value, bool) {
	if !archtimeOrdinary(o) {
		return Undefined, false
	}
	slot := o.shape.Lookup(vm.heap.intern.InternGo(name))
	if slot < 0 || slot >= len(o.slots) || o.slotAttr(slot) != attrDefault {
		return Undefined, false
	}
	v := o.slots[slot]
	if v.IsObject() {
		x := vm.heap.Get(v.Handle())
		if x != nil && x.kind == KindAccessor {
			return Undefined, false
		}
	}
	return v, true
}

func (vm *VM) checkVector3Slots(o *Object) bool {
	for _, name := range [...]string{"x", "y", "z"} {
		if _, ok := vm.getOrdinaryDataSlot(o, name); !ok {
			return false
		}
	}
	return true
}

func (vm *VM) pureResolveMethod(o *Object, name string, tag archtimeGeometryTag) bool {
	key := vm.heap.intern.InternGo(name)
	for depth := 0; o != nil && depth < 10; depth++ {
		if !archtimeOrdinary(o) {
			return false
		}
		slot := o.shape.Lookup(key)
		if slot >= 0 {
			if slot >= len(o.slots) {
				return false
			}
			attrs := o.slotAttr(slot)
			// Object-literal and class method descriptors, respectively.
			if attrs != attrDefault && attrs != attrWritable|attrConfigurable {
				return false
			}
			v := o.slots[slot]
			if !v.IsObject() {
				return false
			}
			f := vm.heap.Get(v.Handle())
			if f == nil || f.kind != KindFunction || f.fn == nil || f.fn.Native != nil {
				return false
			}
			return f.fn.archtimeTag == tag && f.fn.archtimeHash != "" && f.fn.archtimeHash == hashChunk(f.fn)
		}
		o = vm.heap.Get(o.proto)
	}
	return false
}

func (vm *VM) setVector3SlotsUnsafe(o *Object, x, y, z float64) {
	o.slots[o.shape.Lookup(vm.heap.intern.InternGo("x"))] = Number(x)
	o.slots[o.shape.Lookup(vm.heap.intern.InternGo("y"))] = Number(y)
	o.slots[o.shape.Lookup(vm.heap.intern.InternGo("z"))] = Number(z)
}
