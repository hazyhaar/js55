// SPDX-License-Identifier: BUSL-1.1

package runtime

// InstanceType encodes the concrete heap object type of an ECMAScript value.
// It directly mirrors the V8 engine instance-type hierarchy (src/objects/instance-type.h)
// to enable branchless type range checks and zero-allocation dynamic dispatch.
type InstanceType uint16

const (
	// String Types Range (Bits 7-15 cleared, so string < 0x80)
	kIsNotStringMask uint32 = ^uint32((1 << 7) - 1)
	kStringTag       uint32 = 0x0

	// 1. Sequential Strings
	SeqTwoByteString InstanceType = 0x00
	SeqOneByteString InstanceType = 0x08

	// 2. Cons Strings
	ConsTwoByteString InstanceType = 0x01
	ConsOneByteString InstanceType = 0x09

	// 3. Sliced Strings
	SlicedTwoByteString InstanceType = 0x03
	SlicedOneByteString InstanceType = 0x0B

	// 4. Thin Strings
	ThinTwoByteString InstanceType = 0x05
	ThinOneByteString InstanceType = 0x0D

	// 5. Internalized Strings
	InternalizedTwoByteString InstanceType = 0x20
	InternalizedOneByteString InstanceType = 0x28

	// First Non-String Type Boundary (Bit 7 set = 0x80)
	FirstNonStringType InstanceType = 0x80

	// Symbols & Primitives
	SymbolType  InstanceType = 0x80
	OddballType InstanceType = 0x81
	BigIntType  InstanceType = 0x82

	// Internal VM Structures
	MapType             InstanceType = 0x50
	FixedArrayType      InstanceType = 0x51
	WeakFixedArrayType  InstanceType = 0x52
	PropertyArrayType   InstanceType = 0x53
	DescriptorArrayType InstanceType = 0x54

	// Receivers & Objects Range (FIRST_JS_RECEIVER_TYPE .. LAST_JS_RECEIVER_TYPE)
	FirstJSReceiverType InstanceType = 0x100
	JSProxyType         InstanceType = 0x100

	// JSObject Range (FIRST_JS_OBJECT_TYPE .. LAST_JS_OBJECT_TYPE)
	FirstJSObjectType   InstanceType = 0x101
	JSObjectType        InstanceType = 0x101
	JSArrayType         InstanceType = 0x102
	JSFunctionType      InstanceType = 0x103
	JSAsyncFunctionType InstanceType = 0x104
	JSGeneratorType     InstanceType = 0x105
	JSPromiseType       InstanceType = 0x106
	JSMapType           InstanceType = 0x107
	JSSetType           InstanceType = 0x108
	JSWeakMapType       InstanceType = 0x109
	JSWeakSetType       InstanceType = 0x10A
	JSDateType          InstanceType = 0x10B
	JSRegExpType        InstanceType = 0x10C
	JSArrayBufferType   InstanceType = 0x10D
	JSTypedArrayType    InstanceType = 0x10E
	JSDataViewType      InstanceType = 0x10F
	JSArgumentsType     InstanceType = 0x110
	JSErrorType         InstanceType = 0x111
	LastJSObjectType    InstanceType = 0x1FF
	LastJSReceiverType  InstanceType = 0x1FF
)

// IsString tests if the instance type represents an ECMAScript string (branchless mask).
func (t InstanceType) IsString() bool {
	return (uint32(t) & kIsNotStringMask) == kStringTag
}

// IsJSReceiver tests if the type is a JSReceiver (JSObject or JSProxy).
func (t InstanceType) IsJSReceiver() bool {
	return t >= FirstJSReceiverType && t <= LastJSReceiverType
}

// IsJSObject tests if the type is a JSObject.
func (t InstanceType) IsJSObject() bool {
	return t >= FirstJSObjectType && t <= LastJSObjectType
}

// IsJSFunction tests if the type is a callable JSFunction.
func (t InstanceType) IsJSFunction() bool {
	return t >= JSFunctionType && t <= JSAsyncFunctionType
}

// IsJSArray tests if the type is a JSArray.
func (t InstanceType) IsJSArray() bool {
	return t == JSArrayType
}

// IsSymbol tests if the type is a Symbol.
func (t InstanceType) IsSymbol() bool {
	return t == SymbolType
}

// IsBigInt tests if the type is a BigInt.
func (t InstanceType) IsBigInt() bool {
	return t == BigIntType
}
