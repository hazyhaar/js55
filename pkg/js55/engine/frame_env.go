// SPDX-License-Identifier: BUSL-1.1
package engine

import "encoding/binary"

// regularFrameBody is called only after compiling an ordinary function. It is
// a closed proof over the emitted unit, never a runtime heuristic. New opcodes
// take the generic path until their environment effects are reviewed here.
func regularFrameBody(c *Chunk) bool {
	if c.Native != nil || c.Generator || c.Async || c.Arrow || c.ArgumentsSlot >= 0 || c.RestSlot >= 0 || c.Locals < 0 {
		return false
	}
	for _, k := range c.Consts {
		if k.Kind == ConstFunction || (k.Kind == ConstString && k.Text != nil && k.Text.GoString() == "eval") {
			return false
		}
	}
	for _, h := range c.Handlers {
		if h.Depth != 0 {
			return false
		}
	}
	for ip := 0; ip < len(c.Code); {
		op := Op(c.Code[ip])
		width := op.Width()
		if width > len(c.Code)-ip-1 {
			return false
		}
		switch op {
		case OpGetLocal, OpSetLocal:
			arg := binary.LittleEndian.Uint32(c.Code[ip+1 : ip+5])
			if arg>>16 != 0 || int(arg&0xffff) >= c.Locals {
				return false
			}
		case OpNop, OpUndefined, OpNull, OpTrue, OpFalse, OpInt32, OpConst,
			OpPop, OpDup, OpAdd, OpSub, OpMul, OpDiv, OpMod, OpPow, OpNeg, OpPos,
			OpBitAnd, OpBitOr, OpBitXor, OpBitNot, OpShl, OpShr, OpUShr,
			OpEq, OpNe, OpStrictEq, OpStrictNe, OpLt, OpGt, OpLe, OpGe,
			OpInstanceof, OpIn, OpNot, OpTypeof, OpIsNullish,
			OpGetGlobal, OpGetGlobalSoft,
			OpGetProp, OpSetProp, OpGetElem, OpSetElem, OpNewObject, OpNewArray,
			OpJump, OpJumpIfFalse, OpJumpIfTrue, OpCall, OpCallMethod, OpNew,
			OpThis, OpNewTarget, OpReturn, OpThrow:
		default:
			return false
		}
		ip += 1 + width
	}
	return len(c.Code) > 0
}

// The cache retains at most 64 idle environments per VM, additionally capped
// by MaxDepth. Deep recursion may allocate beyond this retention budget; it
// does not turn an unbounded call-depth setting into an unbounded idle cache.
const maxIdleFrameEnvs = 64

func (vm *VM) acquireFrameEnv(c *Chunk, parent Handle) Handle {
	if !c.SafeEnv || !c.safeEnvVerified || len(vm.safeEnvs) == 0 {
		return vm.heap.NewEnv(c.Locals, parent)
	}
	i := len(vm.safeEnvs) - 1
	e := vm.safeEnvs[i]
	o := vm.heap.MustGet(e)
	if cap(o.elements) < c.Locals {
		// Keep the reserve rooted and unmodified until the quota accepts the
		// FULL new backing store. Charge the transient coexistence of both
		// stores, then credit the discarded (already cleared) old store.
		vm.heap.TrackAlloc(int64(c.Locals) * 8)
		oldBytes := int64(cap(o.elements)) * 8
		o.elements = make([]Value, c.Locals)
		for j := range o.elements {
			o.elements[j] = Undefined
		}
		vm.heap.TrackFree(oldBytes)
	}
	o.proto = parent
	// No VM allocation occurs between this removal and the caller's AddRoot.
	vm.safeEnvs[i] = NoHandle
	vm.safeEnvs = vm.safeEnvs[:i]
	return e
}

// releaseFrameEnv consumes ownership once. Generic/suspended environments are
// never cleared. The caller removes the now-empty frame from the active stack.
func (vm *VM) releaseFrameEnv(fr *frame) {
	if fr.env == NoHandle || fr.chunk == nil || !fr.chunk.SafeEnv || !fr.chunk.safeEnvVerified {
		return
	}
	e := fr.env
	fr.env = NoHandle
	o := vm.heap.MustGet(e)
	els := o.elements[:cap(o.elements)]
	for i := range els {
		els[i] = Undefined
	}
	// Env cells carry no JS properties. Reset the complete Object so no
	// accidental reference survives in an inactive cell, including its proto.
	*o = Object{kind: KindEnv, shape: vm.heap.rootShape, elements: els, proto: NoHandle}
	limit := min(maxIdleFrameEnvs, vm.MaxDepth)
	if len(vm.safeEnvs) < limit {
		vm.safeEnvs = append(vm.safeEnvs, e)
	}
}

type frameBoundary struct {
	frames, stack, this, news int
	keep                      Value
}

// Only callInternal's ordinary frames own these stacks. Generator execution
// and eval frames have separate owners (generatorUnwindFrame / evalSource).
func (vm *VM) popFrameCallStacks(fr *frame) Value {
	if !fr.callee.IsObject() {
		return Undefined
	}
	vm.thisStack[len(vm.thisStack)-1] = Undefined
	vm.thisStack = vm.thisStack[:len(vm.thisStack)-1]
	v := vm.newStack[len(vm.newStack)-1]
	vm.newStack[len(vm.newStack)-1] = Undefined
	vm.newStack = vm.newStack[:len(vm.newStack)-1]
	return v
}

func (vm *VM) frameBoundary() frameBoundary {
	return frameBoundary{len(vm.frames), vm.heap.StackLen(), len(vm.thisStack), len(vm.newStack), vm.keep}
}

func (vm *VM) restoreFrameBoundary(b frameBoundary) {
	for len(vm.frames) > b.frames {
		i := len(vm.frames) - 1
		vm.releaseFrameEnv(&vm.frames[i])
		vm.frames[i] = frame{}
		vm.frames = vm.frames[:i]
	}
	vm.heap.TruncateStack(b.stack)
	clear(vm.thisStack[b.this:])
	vm.thisStack = vm.thisStack[:b.this]
	clear(vm.newStack[b.news:])
	vm.newStack = vm.newStack[:b.news]
	vm.keep = b.keep
}
