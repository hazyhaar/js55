// SPDX-License-Identifier: Apache-2.0 OR MIT

package runtime

import (
	"testing"
)

func TestInstanceType_HierarchyAndRangeChecks(t *testing.T) {
	// 1. Strings Range Verification
	stringTypes := []InstanceType{
		SeqOneByteString, SeqTwoByteString,
		ConsOneByteString, ConsTwoByteString,
		SlicedOneByteString, SlicedTwoByteString,
		ThinOneByteString, ThinTwoByteString,
		InternalizedOneByteString, InternalizedTwoByteString,
	}

	for _, st := range stringTypes {
		if !st.IsString() {
			t.Fatalf("Expected %v to be recognized as String", st)
		}
		if st >= FirstNonStringType {
			t.Fatalf("String type %v violated FirstNonStringType boundary", st)
		}
		if st.IsJSReceiver() || st.IsJSObject() {
			t.Fatalf("String type %v must not be JSReceiver or JSObject", st)
		}
	}

	// 2. Non-String Primitives
	nonStrings := []InstanceType{SymbolType, OddballType, BigIntType}
	for _, nst := range nonStrings {
		if nst.IsString() {
			t.Fatalf("Primitive %v must not be String", nst)
		}
		if nst < FirstNonStringType {
			t.Fatalf("Primitive %v is below FirstNonStringType boundary", nst)
		}
	}

	// 3. JSReceivers & JSObjects Range Checks
	jsObjects := []InstanceType{
		JSObjectType, JSArrayType, JSFunctionType, JSAsyncFunctionType,
		JSGeneratorType, JSPromiseType, JSMapType, JSSetType,
		JSDateType, JSRegExpType, JSErrorType,
	}

	for _, obj := range jsObjects {
		if !obj.IsJSReceiver() {
			t.Fatalf("Expected %v to be recognized as JSReceiver", obj)
		}
		if !obj.IsJSObject() {
			t.Fatalf("Expected %v to be recognized as JSObject", obj)
		}
	}

	// 4. JSProxy Special Case (Receiver but not JSObject)
	if !JSProxyType.IsJSReceiver() {
		t.Fatalf("JSProxyType must be JSReceiver")
	}

	// 5. Zero-Alloc Call Verification
	allocs := testing.AllocsPerRun(1000, func() {
		_ = JSArrayType.IsJSReceiver()
		_ = SeqOneByteString.IsString()
		_ = JSFunctionType.IsJSFunction()
	})
	if allocs != 0 {
		t.Fatalf("InstanceType methods must be zero-alloc, got %f", allocs)
	}
}
