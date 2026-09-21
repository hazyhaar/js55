//go:build archtime_geometry

package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"unsafe"

	"github.com/hazyhaar/c2pkg/c2archtsim/geometry"
)

const archtimeNormalsWork = uint64(2_000_000)

func (vm *VM) syncConstruct(fr *frame, ctor Value, args []Value) (Value, error) {
	pre := vm.frameBoundary()
	vm.heap.AddRoot(&ctor)
	defer vm.heap.RemoveRoot(&ctor)
	for i := range args {
		vm.heap.AddRoot(&args[i])
		defer vm.heap.RemoveRoot(&args[i])
	}
	vm.push(ctor)
	for _, a := range args {
		vm.push(a)
	}
	use := fr
	dummy := frame{}
	if use == nil {
		use = &dummy
	}
	if err := vm.instantiateAs(use, len(args), ctor); err != nil {
		vm.restoreFrameBoundary(pre)
		return Undefined, err
	}
	if len(vm.frames) == pre.frames {
		if vm.heap.StackLen() > pre.stack {
			return vm.pop(), nil
		}
		return Undefined, nil
	}
	return vm.interpret(pre.frames)
}

func (vm *VM) syncCall(fr *frame, fn, thisVal Value, args []Value) (Value, error) {
	pre := vm.frameBoundary()
	vm.heap.AddRoot(&fn)
	defer vm.heap.RemoveRoot(&fn)
	vm.heap.AddRoot(&thisVal)
	defer vm.heap.RemoveRoot(&thisVal)
	for i := range args {
		vm.heap.AddRoot(&args[i])
		defer vm.heap.RemoveRoot(&args[i])
	}
	vm.push(fn)
	for _, a := range args {
		vm.push(a)
	}
	use := fr
	dummy := frame{}
	if use == nil {
		use = &dummy
	}
	if err := vm.callInternal(use, len(args), thisVal, Undefined, Undefined); err != nil {
		vm.restoreFrameBoundary(pre)
		return Undefined, err
	}
	if len(vm.frames) == pre.frames {
		if vm.heap.StackLen() > pre.stack {
			return vm.pop(), nil
		}
		return Undefined, nil
	}
	return vm.interpret(pre.frames)
}

func (vm *VM) tryArchtimeNormals(fr *frame, fn *Object, argc int, thisVal Value) (bool, Value, error) {
	reject := func(reason string) (bool, Value, error) {
		vm.KernelStats.Rejected++
		return false, Undefined, nil
	}
	fail := func(err error) (bool, Value, error) {
		return true, Undefined, err
	}

	if fn == nil || fn.fn == nil || fn.fn.Native != nil || argc != 0 {
		return reject("callee")
	}
	if fn.fn.archtimeHash == "" || fn.fn.archtimeHash != hashChunk(fn.fn) {
		return reject("hash")
	}
	isCompute := fn.fn.archtimeTag == archtimeTagComputeVertexNormals
	if !isCompute && fn.fn.archtimeTag != archtimeTagNormalizeNormals {
		return reject("tag")
	}
	if vm.OnCheckpoint != nil && !vm.HostNonMutating {
		return reject("hostMutating")
	}
	if !thisVal.IsObject() {
		return reject("this")
	}

	a := vm.heap.Get(thisVal.Handle())
	if !archtimeOrdinary(a) {
		return reject("geom")
	}
	attrsVal, ok := vm.getOrdinaryDataSlot(a, "attributes")
	if !ok || !attrsVal.IsObject() {
		return reject("attributes")
	}
	attrs := vm.heap.Get(attrsVal.Handle())
	posVal, ok := vm.getOrdinaryDataSlot(attrs, "position")
	if !ok || !posVal.IsObject() {
		return reject("position")
	}
	posAttr := vm.heap.Get(posVal.Handle())
	if !vm.pureResolveMethod(posAttr, "getX", archtimeTagBufferAttributeGetX) ||
		!vm.pureResolveMethod(posAttr, "getY", archtimeTagBufferAttributeGetY) ||
		!vm.pureResolveMethod(posAttr, "getZ", archtimeTagBufferAttributeGetZ) {
		return reject("getXYZ")
	}
	itemSize, ok := vm.getOrdinaryDataSlot(posAttr, "itemSize")
	if !ok || !itemSize.IsNumber() || itemSize.ToFloat() != 3 {
		return reject("itemSize")
	}
	normFlag, ok := vm.getOrdinaryDataSlot(posAttr, "normalized")
	if !ok || normFlag != False {
		return reject("normalized")
	}
	posArr, posLen, ok := vm.extractFloat32Array(posAttr)
	if !ok || posLen == 0 || posLen%3 != 0 {
		return reject("posArray")
	}
	count, ok := vm.getOrdinaryDataSlot(posAttr, "count")
	if !ok || !count.IsNumber() || count.ToFloat() != float64(posLen/3) {
		return reject("count")
	}
	if !checkFinite32All(posArr) {
		return reject("posFinite")
	}

	var idxArr []uint32
	if isCompute {
		idxVal, okIdx := vm.getOrdinaryDataSlot(a, "index")
		if !okIdx || !idxVal.IsObject() {
			return reject("index")
		}
		idxAttr := vm.heap.Get(idxVal.Handle())
		if !vm.pureResolveMethod(idxAttr, "getX", archtimeTagBufferAttributeGetX) {
			return reject("indexGetX")
		}
		var okIdxArr bool
		idxArr, _, okIdxArr = vm.extractUint32Array(idxAttr)
		if !okIdxArr || len(idxArr) == 0 || len(idxArr)%3 != 0 {
			return reject("idxArray")
		}
		if !checkIndicesOK(idxArr, uint64(posLen/3)) {
			return reject("idxBounds")
		}
	}

	envObj := vm.heap.Get(fn.env)
	if envObj == nil || envObj.kind != KindEnv {
		return reject("env")
	}

	norVal, hasNormal := vm.getOrdinaryDataSlot(attrs, "normal")
	fresh := !hasNormal || norVal.IsUndefined()
	if fresh && !isCompute {
		return reject("normalizeMissing")
	}

	var snVal, f32Ctor, setAttr, tnVal Value
	if fresh {
		d, slot, okCap := scanNewCapture(fn.fn, 2)
		if !okCap {
			return reject("snBytecode")
		}
		snVal, okCap = vm.capturedLocal(envObj, d, slot)
		if !okCap || !snVal.IsObject() {
			return reject("snCapture")
		}
		snObj := vm.heap.Get(snVal.Handle())
		if snObj == nil || snObj.kind != KindFunction || snObj.fn == nil || snObj.fn.Native != nil {
			return reject("snFn")
		}
		snHash := hashFnBody(snObj.fn)
		if snHash == "" {
			return reject("snHash")
		}
		posCtor, okCtor := vm.protoConstructor(posAttr)
		if !okCtor || !posCtor.IsObject() {
			return reject("snIdentity")
		}
		ctorObj := vm.heap.Get(posCtor.Handle())
		if ctorObj == nil || ctorObj.kind != KindFunction || ctorObj.fn == nil || ctorObj.fn.Native != nil {
			return reject("snCtor")
		}
		if hashFnBody(ctorObj.fn) != snHash {
			return reject("snIdentity")
		}
		f32Ctor, ok = vm.nativeTypedCtor("Float32Array")
		if !ok {
			return reject("f32")
		}
		if vm.hasOwnSlot(a, "setAttribute") {
			return reject("setAttributeOwn")
		}
		setAttr, ok = vm.protoOwnFn(a, "setAttribute")
		if !ok || hashFnBody(vm.heap.Get(setAttr.Handle()).fn) == "" {
			return reject("setAttribute")
		}
		if _, ok = vm.inheritedSetter(posAttr, "needsUpdate"); !ok {
			return reject("needsUpdate")
		}
	} else {
		if !norVal.IsObject() {
			return reject("normalType")
		}
		norAttr := vm.heap.Get(norVal.Handle())
		norArr, norLen, okExt := vm.extractFloat32Array(norAttr)
		if !okExt || norLen != posLen {
			return reject("norArray")
		}
		if !vm.pureResolveMethod(norAttr, "getX", archtimeTagBufferAttributeGetX) ||
			!vm.pureResolveMethod(norAttr, "getY", archtimeTagBufferAttributeGetY) ||
			!vm.pureResolveMethod(norAttr, "getZ", archtimeTagBufferAttributeGetZ) {
			return reject("norGetXYZ")
		}
		if isCompute {
			if _, okNU := vm.inheritedSetter(norAttr, "needsUpdate"); !okNU {
				return reject("needsUpdate")
			}
		}
		if !isCompute {
			if !checkFinite32All(norArr) {
				return reject("norFinite")
			}
			d, slot, okCap := scanCallMethodCapture(fn.fn, "fromBufferAttribute")
			if !okCap {
				return reject("tnBytecode")
			}
			tnVal, okCap = vm.capturedLocal(envObj, d, slot)
			if !okCap || !tnVal.IsObject() {
				return reject("tnCapture")
			}
			tnObj := vm.heap.Get(tnVal.Handle())
			if !archtimeOrdinary(tnObj) || !vm.checkVector3Slots(tnObj) {
				return reject("tnVector")
			}
		}
	}
	if isCompute {
		if !vm.pureResolveMethod(a, "normalizeNormals", archtimeTagNormalizeNormals) {
			return reject("normalizeNormals")
		}
		nnVal, okNN := vm.resolveOrdinaryFn(a, "normalizeNormals")
		if !okNN || !nnVal.IsObject() {
			return reject("normalizeNormalsFn")
		}
		nnObj := vm.heap.Get(nnVal.Handle())
		if nnObj == nil || nnObj.fn == nil || nnObj.env == NoHandle {
			return reject("normalizeNormalsChunk")
		}
		nnEnv := vm.heap.Get(nnObj.env)
		dNN, slotNN, okCap := scanCallMethodCapture(nnObj.fn, "fromBufferAttribute")
		if !okCap {
			return reject("tnBytecodeCompute")
		}
		tnVal, okCap = vm.capturedLocal(nnEnv, dNN, slotNN)
		if !okCap || !tnVal.IsObject() {
			return reject("tnCaptureCompute")
		}
		tnObj := vm.heap.Get(tnVal.Handle())
		if !archtimeOrdinary(tnObj) || !vm.checkVector3Slots(tnObj) {
			return reject("tnVectorCompute")
		}
	}

	vm.heap.AddRoot(&thisVal)
	defer vm.heap.RemoveRoot(&thisVal)
	vm.heap.AddRoot(&attrsVal)
	defer vm.heap.RemoveRoot(&attrsVal)
	vm.heap.AddRoot(&posVal)
	defer vm.heap.RemoveRoot(&posVal)
	if norVal.IsObject() {
		vm.heap.AddRoot(&norVal)
		defer vm.heap.RemoveRoot(&norVal)
	}
	if snVal.IsObject() {
		vm.heap.AddRoot(&snVal)
		defer vm.heap.RemoveRoot(&snVal)
	}
	if f32Ctor.IsObject() {
		vm.heap.AddRoot(&f32Ctor)
		defer vm.heap.RemoveRoot(&f32Ctor)
	}
	if setAttr.IsObject() {
		vm.heap.AddRoot(&setAttr)
		defer vm.heap.RemoveRoot(&setAttr)
	}
	if tnVal.IsObject() {
		vm.heap.AddRoot(&tnVal)
		defer vm.heap.RemoveRoot(&tnVal)
	}

	minEnter := geometry.QuotaAdmission + geometry.QuotaBindingGuard
	if vm.GasLeft < 0 || uint64(vm.GasLeft) < minEnter {
		return fail(ErrInterrupted)
	}
	var beginCost uint64
	if isCompute {
		beginCost = geometry.NormalsBeginCost(len(posArr), len(idxArr))
	} else {
		nNor := posLen
		if !fresh {
			if norAttr := vm.heap.Get(norVal.Handle()); norAttr != nil {
				if _, n, okN := vm.extractFloat32Array(norAttr); okN {
					nNor = n
				}
			}
		}
		beginCost = geometry.NormalizeBeginCost(nNor)
	}
	if uint64(vm.GasLeft) < beginCost {
		return fail(ErrInterrupted)
	}
	vm.GasLeft -= int64(beginCost)

	var norArr []float32
	var created Value
	if fresh {
		var err error
		created, err = vm.constructNormalAttr(fr, snVal, f32Ctor, posLen)
		if err != nil {
			return fail(err)
		}
		vm.heap.AddRoot(&created)
		defer vm.heap.RemoveRoot(&created)
		norObj := vm.heap.Get(created.Handle())
		var okExt bool
		norArr, _, okExt = vm.extractFloat32Array(norObj)
		if !okExt || len(norArr) != posLen {
			return fail(fmt.Errorf("fresh normal not Float32Array"))
		}
	} else {
		var okExt bool
		posArr, norArr, idxArr, okExt = vm.normalsViews(thisVal, posVal, norVal, isCompute)
		if !okExt {
			return fail(fmt.Errorf("normal views lost"))
		}
	}

	var state geometry.NormState

	if isCompute {
		if !geometry.NormalsBegin(posArr, idxArr, norArr, &state) {
			return fail(fmt.Errorf("NormalsBegin refused"))
		}
	} else {
		if !geometry.NormalizeBegin(norArr, &state) {
			return fail(fmt.Errorf("NormalizeBegin refused"))
		}
	}

	if fresh {
		if err := vm.publishNormalAttr(fr, thisVal, created, setAttr); err != nil {
			return fail(err)
		}
		a = vm.heap.Get(thisVal.Handle())
		attrsVal, ok = vm.getOrdinaryDataSlot(a, "attributes")
		if !ok || !attrsVal.IsObject() {
			return fail(fmt.Errorf("attributes lost after setAttribute"))
		}
		attrs = vm.heap.Get(attrsVal.Handle())
		norVal, ok = vm.getOrdinaryDataSlot(attrs, "normal")
		if !ok || !norVal.IsObject() {
			return fail(fmt.Errorf("normal missing after setAttribute"))
		}
		if norVal.Handle() != created.Handle() {
			return fail(fmt.Errorf("setAttribute did not store constructor result"))
		}
		vm.heap.AddRoot(&norVal)
		defer vm.heap.RemoveRoot(&norVal)
	}

	fixed := geometry.QuotaAdmission + geometry.QuotaBindingGuard
	for state.Phase() != geometry.PhaseDone {
		if vm.GasLeft <= 0 || uint64(vm.GasLeft) < fixed {
			return fail(ErrInterrupted)
		}
		work := vm.normalsStepWork()
		maxByGas := (uint64(vm.GasLeft) - fixed) / geometry.QuotaAccumOne
		if work > maxByGas {
			work = maxByGas
		}
		if work == 0 {
			return fail(ErrInterrupted)
		}
		need, overflow := mulSat(work, geometry.QuotaAccumOne)
		if overflow {
			return fail(ErrInterrupted)
		}
		need += fixed
		if uint64(vm.GasLeft) < need {
			return fail(ErrInterrupted)
		}
		vm.GasLeft -= int64(need)
		var done, okStep bool
		if isCompute {
			done, okStep = geometry.NormalsStep(posArr, idxArr, norArr, &state, work)
		} else {
			done, okStep = geometry.NormalizeStep(norArr, &state, work)
		}
		if !okStep {
			return fail(fmt.Errorf("archtimeNormalsStep failed"))
		}
		vm.writeLexicalLastNormalized(tnVal, &state)
		used := state.Quota()
		if used > need {
			extra := used - need
			if uint64(vm.GasLeft) < extra {
				vm.GasLeft = 0
				return fail(ErrInterrupted)
			}
			vm.GasLeft -= int64(extra)
		} else if used < need {
			vm.GasLeft += int64(need - used)
		}
		if !done {
			if err := vm.checkpoint(); err != nil {
				return fail(err)
			}
			var okV bool
			posArr, norArr, idxArr, okV = vm.normalsViews(thisVal, posVal, norVal, isCompute)
			if !okV {
				return fail(fmt.Errorf("views invalidated at checkpoint"))
			}
		}
	}

	vm.KernelStats.Accepted++
	if len(vm.frames) > 0 {
		fr = &vm.frames[len(vm.frames)-1]
	}
	if isCompute {
		norAttr := vm.heap.Get(norVal.Handle())
		setter, okSet := vm.inheritedSetter(norAttr, "needsUpdate")
		if !okSet {
			return fail(fmt.Errorf("needsUpdate setter lost"))
		}
		if _, err := vm.syncCall(fr, setter, norVal, []Value{True}); err != nil {
			return fail(err)
		}
	}
	return true, Undefined, nil
}

func (vm *VM) writeLexicalLastNormalized(tnVal Value, state *geometry.NormState) {
	if state == nil || !tnVal.IsObject() {
		return
	}
	v, ok := state.LastNormalized()
	if !ok {
		return
	}
	tnObj := vm.heap.Get(tnVal.Handle())
	if !archtimeOrdinary(tnObj) || !vm.checkVector3Slots(tnObj) {
		return
	}
	vm.setVector3SlotsUnsafe(tnObj, v[0], v[1], v[2])
}

func (vm *VM) constructNormalAttr(fr *frame, snVal, f32Ctor Value, posLen int) (Value, error) {
	f32Obj, err := vm.syncConstruct(fr, f32Ctor, []Value{Number(float64(posLen))})
	if err != nil {
		return Undefined, err
	}
	vm.heap.AddRoot(&f32Obj)
	defer vm.heap.RemoveRoot(&f32Obj)
	return vm.syncConstruct(fr, snVal, []Value{f32Obj, Number(3)})
}

func (vm *VM) publishNormalAttr(fr *frame, thisVal, nAttr, setAttr Value) error {
	name := vm.NewStringValue(vm.heap.Intern().InternGo("normal"))
	vm.heap.AddRoot(&name)
	defer vm.heap.RemoveRoot(&name)
	_, err := vm.syncCall(fr, setAttr, thisVal, []Value{name, nAttr})
	return err
}

func (vm *VM) extractFloat32Array(attr *Object) ([]float32, int, bool) {
	if !archtimeOrdinary(attr) {
		return nil, 0, false
	}
	av, ok := vm.getOrdinaryDataSlot(attr, "array")
	if !ok || !av.IsObject() {
		return nil, 0, false
	}
	arr := vm.heap.Get(av.Handle())
	if arr == nil || arr.kind != KindArray || arr.typedName != "Float32Array" || arr.lengthTracking || len(arr.deleted) != 0 || arr.frozen || arr.byteOffset < 0 || arr.byteOffset%4 != 0 || arr.typedLength <= 0 {
		return nil, 0, false
	}
	backing := vm.heap.Get(arr.env)
	if !archtimeOrdinary(backing) || !backing.arrayBuffer || backing.resizable {
		return nil, 0, false
	}
	off := arr.byteOffset
	if off > len(backing.bytes) || arr.typedLength > (len(backing.bytes)-off)/4 {
		return nil, 0, false
	}
	data := backing.bytes[off : off+arr.typedLength*4]
	return unsafe.Slice((*float32)(unsafe.Pointer(&data[0])), arr.typedLength), arr.typedLength, true
}

func (vm *VM) extractUint32Array(attr *Object) ([]uint32, int, bool) {
	if !archtimeOrdinary(attr) {
		return nil, 0, false
	}
	av, ok := vm.getOrdinaryDataSlot(attr, "array")
	if !ok || !av.IsObject() {
		return nil, 0, false
	}
	arr := vm.heap.Get(av.Handle())
	if arr == nil || arr.kind != KindArray || arr.typedName != "Uint32Array" || arr.lengthTracking || len(arr.deleted) != 0 || arr.frozen || arr.byteOffset < 0 || arr.byteOffset%4 != 0 || arr.typedLength <= 0 {
		return nil, 0, false
	}
	backing := vm.heap.Get(arr.env)
	if !archtimeOrdinary(backing) || !backing.arrayBuffer || backing.resizable {
		return nil, 0, false
	}
	off := arr.byteOffset
	if off > len(backing.bytes) || arr.typedLength > (len(backing.bytes)-off)/4 {
		return nil, 0, false
	}
	data := backing.bytes[off : off+arr.typedLength*4]
	return unsafe.Slice((*uint32)(unsafe.Pointer(&data[0])), arr.typedLength), arr.typedLength, true
}

func (vm *VM) capturedLocal(defEnv *Object, depth, slot int) (Value, bool) {
	if depth < 1 {
		return Undefined, false
	}
	env := defEnv
	for d := 0; d < depth-1; d++ {
		if env == nil || env.kind != KindEnv {
			return Undefined, false
		}
		env = vm.heap.Get(env.proto)
	}
	if env == nil || env.kind != KindEnv || slot < 0 || slot >= len(env.elements) {
		return Undefined, false
	}
	return env.elements[slot], true
}

func (vm *VM) protoConstructor(o *Object) (Value, bool) {
	if o == nil || o.proto == NoHandle {
		return Undefined, false
	}
	p := vm.heap.Get(o.proto)
	return vm.getOrdinaryDataSlot(p, "constructor")
}

func (vm *VM) nativeTypedCtor(name string) (Value, bool) {
	key := vm.heap.Intern().InternGo(name)
	v := vm.getProp(ObjectValue(vm.globalObj), key)
	if !v.IsObject() {
		return Undefined, false
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind != KindFunction || o.fn == nil || o.fn.Native == nil || !o.fn.Construct || o.fn.Name != name {
		return Undefined, false
	}
	return v, true
}

func hashFnBody(c *Chunk) string {
	if c == nil || c.Native != nil {
		return ""
	}
	h := sha256.New()
	binary.Write(h, binary.LittleEndian, int64(len(c.Code)))
	h.Write(c.Code)
	binary.Write(h, binary.LittleEndian, int64(len(c.Consts)))
	for _, k := range c.Consts {
		binary.Write(h, binary.LittleEndian, uint8(k.Kind))
		switch k.Kind {
		case ConstNumber:
			binary.Write(h, binary.LittleEndian, math.Float64bits(k.Num))
		case ConstString:
			if k.Text == nil {
				return ""
			}
			s := k.Text.GoString()
			binary.Write(h, binary.LittleEndian, int64(len(s)))
			h.Write([]byte(s))
		case ConstFunction:
			nested := hashFnBody(k.Fn)
			if nested == "" {
				return ""
			}
			h.Write([]byte(nested))
		case ConstBigInt:
			binary.Write(h, binary.LittleEndian, k.Big)
		default:
			return ""
		}
	}
	binary.Write(h, binary.LittleEndian, int64(c.Params))
	binary.Write(h, binary.LittleEndian, int64(c.Locals))
	return hex.EncodeToString(h.Sum(nil))
}

func (vm *VM) protoOwnFn(o *Object, name string) (Value, bool) {
	if o == nil || o.proto == NoHandle {
		return Undefined, false
	}
	p := vm.heap.Get(o.proto)
	if !archtimeOrdinary(p) {
		return Undefined, false
	}
	key := vm.heap.Intern().InternGo(name)
	slot := p.shape.Lookup(key)
	if slot < 0 || slot >= len(p.slots) {
		return Undefined, false
	}
	v := p.slots[slot]
	if !v.IsObject() {
		return Undefined, false
	}
	f := vm.heap.Get(v.Handle())
	if f == nil || f.kind != KindFunction || f.fn == nil || f.fn.Native != nil {
		return Undefined, false
	}
	return v, true
}

func (vm *VM) normalsViews(thisVal, posVal, norVal Value, withIdx bool) (posArr, norArr []float32, idxArr []uint32, ok bool) {
	if !posVal.IsObject() || !norVal.IsObject() {
		return nil, nil, nil, false
	}
	posAttr := vm.heap.Get(posVal.Handle())
	var posLen int
	posArr, posLen, ok = vm.extractFloat32Array(posAttr)
	if !ok {
		return nil, nil, nil, false
	}
	norAttr := vm.heap.Get(norVal.Handle())
	var norLen int
	norArr, norLen, ok = vm.extractFloat32Array(norAttr)
	if !ok || norLen != posLen {
		return nil, nil, nil, false
	}
	if withIdx {
		a := vm.heap.Get(thisVal.Handle())
		idxVal, okIdx := vm.getOrdinaryDataSlot(a, "index")
		if !okIdx || !idxVal.IsObject() {
			return nil, nil, nil, false
		}
		idxAttr := vm.heap.Get(idxVal.Handle())
		idxArr, _, ok = vm.extractUint32Array(idxAttr)
		if !ok {
			return nil, nil, nil, false
		}
	}
	return posArr, norArr, idxArr, true
}

func (vm *VM) resolveOrdinaryFn(o *Object, name string) (Value, bool) {
	key := vm.heap.Intern().InternGo(name)
	for depth := 0; o != nil && depth < 10; depth++ {
		if !archtimeOrdinary(o) {
			return Undefined, false
		}
		slot := o.shape.Lookup(key)
		if slot >= 0 {
			if slot >= len(o.slots) {
				return Undefined, false
			}
			attrs := o.slotAttr(slot)
			if attrs != attrDefault && attrs != attrWritable|attrConfigurable {
				return Undefined, false
			}
			v := o.slots[slot]
			if !v.IsObject() {
				return Undefined, false
			}
			f := vm.heap.Get(v.Handle())
			if f == nil || f.kind != KindFunction || f.fn == nil || f.fn.Native != nil {
				return Undefined, false
			}
			return v, true
		}
		o = vm.heap.Get(o.proto)
	}
	return Undefined, false
}

func scanNewCapture(c *Chunk, argcWant uint32) (depth, slot int, ok bool) {
	if c == nil {
		return 0, 0, false
	}
	var lastDepth, lastSlot int
	var have bool
	for ip := 0; ip < len(c.Code); {
		op := Op(c.Code[ip])
		w := op.Width()
		if w < 0 || ip+1+w > len(c.Code) {
			return 0, 0, false
		}
		switch op {
		case OpGetLocal:
			arg := binary.LittleEndian.Uint32(c.Code[ip+1 : ip+5])
			d, s := int(arg>>16), int(arg&0xffff)
			if d > 0 {
				lastDepth, lastSlot, have = d, s, true
			}
		case OpNew:
			arg := uint32(binary.LittleEndian.Uint16(c.Code[ip+1 : ip+3]))
			if arg == argcWant && have {
				return lastDepth, lastSlot, true
			}
		}
		ip += 1 + w
	}
	return 0, 0, false
}

func scanCallMethodCapture(c *Chunk, method string) (depth, slot int, ok bool) {
	if c == nil {
		return 0, 0, false
	}
	var lastDepth, lastSlot int
	var have bool
	for ip := 0; ip < len(c.Code); {
		op := Op(c.Code[ip])
		w := op.Width()
		if w < 0 || ip+1+w > len(c.Code) {
			return 0, 0, false
		}
		switch op {
		case OpGetLocal:
			arg := binary.LittleEndian.Uint32(c.Code[ip+1 : ip+5])
			d, s := int(arg>>16), int(arg&0xffff)
			if d > 0 {
				lastDepth, lastSlot, have = d, s, true
			}
		case OpCallMethod:
			arg := binary.LittleEndian.Uint32(c.Code[ip+1 : ip+5])
			idx := int(arg & 0xffff)
			if have && idx < len(c.Consts) && c.Consts[idx].Kind == ConstString && c.Consts[idx].Text != nil && c.Consts[idx].Text.GoString() == method {
				return lastDepth, lastSlot, true
			}
		}
		ip += 1 + w
	}
	return 0, 0, false
}

func (vm *VM) normalsStepWork() uint64 {
	work := archtimeNormalsWork
	if vm.CheckpointEvery > 0 {
		q := uint64(vm.CheckpointEvery)
		maxW := q / geometry.QuotaAccumOne
		if maxW == 0 {
			maxW = 1
		}
		if work > maxW {
			work = maxW
		}
	}
	if work == 0 {
		return 0
	}
	return work
}

func mulSat(a, b uint64) (uint64, bool) {
	if a != 0 && b > ^uint64(0)/a {
		return 0, true
	}
	return a * b, false
}

func (vm *VM) hasOwnSlot(o *Object, name string) bool {
	if o == nil || o.shape == nil {
		return false
	}
	slot := o.shape.Lookup(vm.heap.Intern().InternGo(name))
	return slot >= 0 && slot < len(o.slots)
}

func (vm *VM) inheritedSetter(start *Object, name string) (Value, bool) {
	key := vm.heap.Intern().InternGo(name)
	o := start
	own := true
	for depth := 0; o != nil && depth < 10; depth++ {
		if !archtimeOrdinary(o) {
			return Undefined, false
		}
		slot := o.shape.Lookup(key)
		if slot >= 0 {
			if own || slot >= len(o.slots) {
				return Undefined, false
			}
			v := o.slots[slot]
			if !v.IsObject() {
				return Undefined, false
			}
			acc := vm.heap.Get(v.Handle())
			if acc == nil || acc.kind != KindAccessor || len(acc.elements) < 2 {
				return Undefined, false
			}
			s := acc.elements[1]
			if !s.IsObject() {
				return Undefined, false
			}
			f := vm.heap.Get(s.Handle())
			if f == nil || f.kind != KindFunction || f.fn == nil {
				return Undefined, false
			}
			return s, true
		}
		o = vm.heap.Get(o.proto)
		own = false
	}
	return Undefined, false
}
