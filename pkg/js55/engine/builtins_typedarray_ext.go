// SPDX-License-Identifier: BUSL-1.1

package engine

func (vm *VM) InstallTypedArrayExtendedBuiltins() {
	types := []string{
		"Int8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array",
		"Int32Array", "Uint32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array",
	}
	for _, name := range types {
		vm.installTypedArrayCtor(name)
	}
}
