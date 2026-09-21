// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"strings"
	"testing"
)

// TestDescriptorAccessors validates accessor descriptors (get, set, enumerable, configurable).
func TestDescriptorAccessors(t *testing.T) {
	// 1. Definition and invocation of getter and setter
	got, err := compileAndRun(t, `
		var obj = { _x: 10 };
		Object.defineProperty(obj, "x", {
			get: function() { return this._x * 2; },
			set: function(v) { this._x = v + 1; },
			enumerable: true,
			configurable: true
		});
		var initial = obj.x;
		obj.x = 20;
		var updated = obj.x;
		"" + initial + "," + updated + "," + obj._x;
	`, false)
	if err != nil {
		t.Fatalf("échec accessor get/set : %v", err)
	}
	if got != "20,42,21" {
		t.Fatalf("obtenu %q, attendu 20,42,21", got)
	}

	// 2. getOwnPropertyDescriptor on accessor must return get and set, no value/writable
	got, err = compileAndRun(t, `
		var obj = {};
		var getter = function() { return 42; };
		Object.defineProperty(obj, "prop", {
			get: getter,
			enumerable: true,
			configurable: false
		});
		var desc = Object.getOwnPropertyDescriptor(obj, "prop");
		var hasVal = "value" in desc;
		var hasWrit = "writable" in desc;
		var isGetFn = typeof desc.get === "function";
		var isSetUndef = desc.set === undefined;
		"" + hasVal + "," + hasWrit + "," + isGetFn + "," + isSetUndef + "," + desc.enumerable + "," + desc.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec getOwnPropertyDescriptor accessor : %v", err)
	}
	if got != "false,false,true,true,true,false" {
		t.Fatalf("obtenu %q, attendu false,false,true,true,true,false", got)
	}
}

// TestDescriptorIncompatibleRejection validates that incompatible descriptors are rejected with TypeError.
func TestDescriptorIncompatibleRejection(t *testing.T) {
	// Rejection when combining get/set with value
	_, err := compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "a", {
			get: function() { return 1; },
			value: 42
		});
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour combinaison get + value, obtenu : %v", err)
	}

	// Rejection when combining set with writable
	_, err = compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "b", {
			set: function(v) {},
			writable: true
		});
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour combinaison set + writable, obtenu : %v", err)
	}

	// Rejection when getter is not a function or undefined
	_, err = compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "c", {
			get: 123
		});
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour getter non-fonction, obtenu : %v", err)
	}

	// Rejection when setter is not a function or undefined
	_, err = compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "d", {
			set: "invalid"
		});
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour setter non-fonction, obtenu : %v", err)
	}
}

// TestDescriptorNonConfigurableProtection validates invariants on non-configurable properties.
func TestDescriptorNonConfigurableProtection(t *testing.T) {
	// Cannot change configurable from false to true
	_, err := compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "x", { value: 1, configurable: false });
		Object.defineProperty(obj, "x", { configurable: true });
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour re-configuration configurable:true, obtenu : %v", err)
	}

	// Cannot change enumerable on non-configurable property
	_, err = compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "x", { value: 1, enumerable: false, configurable: false });
		Object.defineProperty(obj, "x", { enumerable: true });
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour changement enumerable sur non-configurable, obtenu : %v", err)
	}

	// Cannot change non-writable non-configurable value
	_, err = compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "x", { value: 1, writable: false, configurable: false });
		Object.defineProperty(obj, "x", { value: 2 });
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour changement valeur sur non-writable non-configurable, obtenu : %v", err)
	}

	// Can update with same value on non-writable non-configurable property
	got, err := compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "x", { value: 1, writable: false, configurable: false });
		Object.defineProperty(obj, "x", { value: 1 });
		obj.x;
	`, false)
	if err != nil || got != "1" {
		t.Fatalf("redéfinition même valeur autorisée : got %q, err %v", got, err)
	}

	// Cannot convert data property to accessor property on non-configurable property
	_, err = compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "x", { value: 1, configurable: false });
		Object.defineProperty(obj, "x", { get: function() { return 2; } });
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour conversion data vers accessor non-configurable, obtenu : %v", err)
	}
}

// TestDescriptorAttributeInheritance validates that updating existing properties retains unspecified attributes.
func TestDescriptorAttributeInheritance(t *testing.T) {
	got, err := compileAndRun(t, `
		var obj = { a: 1 };
		// obj.a is writable: true, enumerable: true, configurable: true
		Object.defineProperty(obj, "a", { value: 100 });
		var desc = Object.getOwnPropertyDescriptor(obj, "a");
		"" + desc.value + "," + desc.writable + "," + desc.enumerable + "," + desc.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec rétention attributs : %v", err)
	}
	if got != "100,true,true,true" {
		t.Fatalf("obtenu %q, attendu 100,true,true,true", got)
	}
}

// TestArrayDescriptorLength validates property descriptor operations on Array length.
func TestArrayDescriptorLength(t *testing.T) {
	// 1. getOwnPropertyDescriptor on Array length
	got, err := compileAndRun(t, `
		var arr = [10, 20, 30];
		var d = Object.getOwnPropertyDescriptor(arr, "length");
		"" + d.value + "," + d.writable + "," + d.enumerable + "," + d.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec descriptor array length : %v", err)
	}
	if got != "3,true,false,false" {
		t.Fatalf("obtenu %q, attendu 3,true,false,false", got)
	}

	// 2. defineProperty length truncation
	got, err = compileAndRun(t, `
		var arr = [1, 2, 3, 4, 5];
		Object.defineProperty(arr, "length", { value: 2 });
		"" + arr.length + "," + arr.join();
	`, false)
	if err != nil {
		t.Fatalf("échec troncature length : %v", err)
	}
	if got != "2,1,2" {
		t.Fatalf("obtenu %q, attendu 2,1,2", got)
	}

	// 3. defineProperty length expansion
	got, err = compileAndRun(t, `
		var arr = [1, 2];
		Object.defineProperty(arr, "length", { value: 4 });
		"" + arr.length + "," + (arr[2] === undefined) + "," + (arr[3] === undefined);
	`, false)
	if err != nil {
		t.Fatalf("échec expansion length : %v", err)
	}
	if got != "4,true,true" {
		t.Fatalf("obtenu %q, attendu 4,true,true", got)
	}

	// 4. defineProperty length RangeError on invalid length
	_, err = compileAndRun(t, `
		var arr = [1, 2];
		Object.defineProperty(arr, "length", { value: -1 });
	`, false)
	if err == nil || !strings.Contains(err.Error(), "RangeError") {
		t.Fatalf("rejet RangeError attendu pour length négative, obtenu : %v", err)
	}

	// 5. defineProperty length non-writable
	got, err = compileAndRun(t, `
		var arr = [1, 2, 3];
		Object.defineProperty(arr, "length", { writable: false });
		var caught = false;
		try {
			Object.defineProperty(arr, "length", { value: 1 });
		} catch(e) {
			caught = (e instanceof TypeError) || ("" + e).indexOf("TypeError") !== -1;
		}
		"" + arr.length + "," + caught;
	`, false)
	if err != nil {
		t.Fatalf("échec length non-writable : %v", err)
	}
	if got != "3,true" {
		t.Fatalf("obtenu %q, attendu 3,true", got)
	}

	// 6. hasOwnProperty and Object.hasOwn on Array length
	got, err = compileAndRun(t, `
		var arr = [1, 2];
		"" + arr.hasOwnProperty("length") + "," + Object.hasOwn(arr, "length") + "," + ("length" in arr);
	`, false)
	if err != nil {
		t.Fatalf("échec hasOwnProperty/hasOwn array length : %v", err)
	}
	if got != "true,true,true" {
		t.Fatalf("obtenu %q, attendu true,true,true", got)
	}

	// 7. propertyIsEnumerable on Array length must be false
	got, err = compileAndRun(t, `
		var arr = [1, 2];
		arr.propertyIsEnumerable("length");
	`, false)
	if err != nil || got != "false" {
		t.Fatalf("propertyIsEnumerable(length) attendu false, obtenu %q, err %v", got, err)
	}

	// 8. getOwnPropertyNames includes length, keys excludes length
	got, err = compileAndRun(t, `
		var arr = ["a", "b"];
		var names = Object.getOwnPropertyNames(arr).join();
		var keys = Object.keys(arr).join();
		names + "|" + keys;
	`, false)
	if err != nil {
		t.Fatalf("échec getOwnPropertyNames vs keys sur array : %v", err)
	}
	if got != "0,1,length|0,1" {
		t.Fatalf("obtenu %q, attendu 0,1,length|0,1", got)
	}
}

// TestArrayDescriptorElements validates property descriptors on array indexed elements.
func TestArrayDescriptorElements(t *testing.T) {
	// Read-after-write with defineProperty on array element
	got, err := compileAndRun(t, `
		var arr = [10, 20];
		Object.defineProperty(arr, "0", { value: 99, writable: false });
		var desc = Object.getOwnPropertyDescriptor(arr, "0");
		var val = arr[0];
		"" + val + "," + desc.value + "," + desc.writable + "," + desc.enumerable + "," + desc.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec defineProperty array element : %v", err)
	}
	if got != "99,99,false,true,true" {
		t.Fatalf("obtenu %q, attendu 99,99,false,true,true", got)
	}
}

// TestObjectGetOwnPropertyDescriptorsComplete validates Object.getOwnPropertyDescriptors across diverse types.
func TestObjectGetOwnPropertyDescriptorsComplete(t *testing.T) {
	got, err := compileAndRun(t, `
		var target = {};
		Object.defineProperty(target, "readOnly", { value: 42, writable: false, enumerable: true, configurable: false });
		Object.defineProperty(target, "acc", { get: function() { return 100; }, enumerable: false, configurable: true });
		var descs = Object.getOwnPropertyDescriptors(target);
		var r = descs.readOnly;
		var a = descs.acc;
		"" + r.value + "," + r.writable + "," + r.enumerable + "," + r.configurable + "|" +
		typeof a.get + "," + a.enumerable + "," + a.configurable + "," + ("value" in a);
	`, false)
	if err != nil {
		t.Fatalf("échec getOwnPropertyDescriptors : %v", err)
	}
	if got != "42,false,true,false|function,false,true,false" {
		t.Fatalf("obtenu %q, attendu 42,false,true,false|function,false,true,false", got)
	}

	// TypeError on null/undefined
	_, err = compileAndRun(t, `Object.getOwnPropertyDescriptors(undefined)`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu sur getOwnPropertyDescriptors(undefined), obtenu : %v", err)
	}
}

// TestObjectCreateWithDescriptors validates Object.create(proto, descriptors).
func TestObjectCreateWithDescriptors(t *testing.T) {
	got, err := compileAndRun(t, `
		var proto = { protoVal: 10 };
		var obj = Object.create(proto, {
			a: { value: 1, writable: true, enumerable: true, configurable: true },
			b: { get: function() { return 2; }, enumerable: false, configurable: true }
		});
		"" + obj.protoVal + "," + obj.a + "," + obj.b + "," + Object.keys(obj).join();
	`, false)
	if err != nil {
		t.Fatalf("échec Object.create avec descripteurs : %v", err)
	}
	if got != "10,1,2,a" {
		t.Fatalf("obtenu %q, attendu 10,1,2,a", got)
	}
}

// TestObjectSealFreezeExtensible validates seal, isSealed, freeze, isFrozen, preventExtensions, isExtensible.
func TestObjectSealFreezeExtensible(t *testing.T) {
	// 1. Extensibility
	got, err := compileAndRun(t, `
		var obj = { a: 1 };
		var ext1 = Object.isExtensible(obj);
		Object.preventExtensions(obj);
		var ext2 = Object.isExtensible(obj);
		"" + ext1 + "," + ext2;
	`, false)
	if err != nil {
		t.Fatalf("échec extensibility : %v", err)
	}
	if got != "true,false" {
		t.Fatalf("obtenu %q, attendu true,false", got)
	}

	// 2. Seal
	got, err = compileAndRun(t, `
		var obj = { a: 1, b: 2 };
		var sealed1 = Object.isSealed(obj);
		Object.seal(obj);
		var sealed2 = Object.isSealed(obj);
		var descA = Object.getOwnPropertyDescriptor(obj, "a");
		"" + sealed1 + "," + sealed2 + "," + descA.configurable + "," + descA.writable;
	`, false)
	if err != nil {
		t.Fatalf("échec seal : %v", err)
	}
	if got != "false,true,false,true" {
		t.Fatalf("obtenu %q, attendu false,true,false,true", got)
	}

	// 3. Freeze
	got, err = compileAndRun(t, `
		var obj = { a: 1 };
		var frozen1 = Object.isFrozen(obj);
		Object.freeze(obj);
		var frozen2 = Object.isFrozen(obj);
		var desc = Object.getOwnPropertyDescriptor(obj, "a");
		"" + frozen1 + "," + frozen2 + "," + desc.writable + "," + desc.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec freeze : %v", err)
	}
	if got != "false,true,false,false" {
		t.Fatalf("obtenu %q, attendu false,true,false,false", got)
	}
}

// TestStringPrimitiveDescriptors validates getOwnPropertyDescriptor on string primitives.
func TestStringPrimitiveDescriptors(t *testing.T) {
	got, err := compileAndRun(t, `
		var d0 = Object.getOwnPropertyDescriptor("hello", "0");
		var dLen = Object.getOwnPropertyDescriptor("hello", "length");
		"" + d0.value + "," + d0.writable + "," + d0.enumerable + "," + d0.configurable + "|" +
		dLen.value + "," + dLen.writable + "," + dLen.enumerable + "," + dLen.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec string descriptor : %v", err)
	}
	if got != "h,false,true,false|5,false,false,false" {
		t.Fatalf("obtenu %q, attendu h,false,true,false|5,false,false,false", got)
	}
}

// TestArrayFromWithArrayLike validates Array.from on array-like objects with length.
func TestArrayFromWithArrayLike(t *testing.T) {
	got, err := compileAndRun(t, `
		var arrayLike = { length: 3, 0: "x", 1: "y", 2: "z" };
		var arr = Array.from(arrayLike);
		Array.isArray(arr) + "," + arr.length + "," + arr.join("-");
	`, false)
	if err != nil {
		t.Fatalf("échec Array.from array-like : %v", err)
	}
	if got != "true,3,x-y-z" {
		t.Fatalf("obtenu %q, attendu true,3,x-y-z", got)
	}
}

// TestEmptyDescriptorDefaults validates that defining a new property with {} sets false/undefined defaults.
func TestEmptyDescriptorDefaults(t *testing.T) {
	got, err := compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "empty", {});
		var d = Object.getOwnPropertyDescriptor(obj, "empty");
		"" + d.value + "," + d.writable + "," + d.enumerable + "," + d.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec empty descriptor : %v", err)
	}
	if got != "undefined,false,false,false" {
		t.Fatalf("obtenu %q, attendu undefined,false,false,false", got)
	}
}

// TestAccessorWithoutSetterOrGetter validates asymmetric accessor descriptors.
func TestAccessorWithoutSetterOrGetter(t *testing.T) {
	// Setter only: read returns undefined
	got, err := compileAndRun(t, `
		var obj = { value: 0 };
		Object.defineProperty(obj, "setOnly", {
			set: function(v) { this.value = v * 3; },
			enumerable: true,
			configurable: true
		});
		var readVal = obj.setOnly;
		obj.setOnly = 7;
		"" + readVal + "," + obj.value;
	`, false)
	if err != nil {
		t.Fatalf("échec setter-only accessor : %v", err)
	}
	if got != "undefined,21" {
		t.Fatalf("obtenu %q, attendu undefined,21", got)
	}

	// Getter only: assign throws error
	_, err = compileAndRun(t, `
		var obj = {};
		Object.defineProperty(obj, "getOnly", {
			get: function() { return 99; },
			configurable: true
		});
		obj.getOnly = 100;
	`, false)
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("rejet TypeError attendu pour affectation sur accesseur sans setter, obtenu : %v", err)
	}
}

// TestSequentialPropertyRedefinition validates chained refinement of property descriptors.
func TestSequentialPropertyRedefinition(t *testing.T) {
	got, err := compileAndRun(t, `
		var obj = {};
		// 1. Initial creation
		Object.defineProperty(obj, "p", { value: 1, writable: true, configurable: true, enumerable: true });
		// 2. Modify value only (retains flags)
		Object.defineProperty(obj, "p", { value: 2 });
		var d2 = Object.getOwnPropertyDescriptor(obj, "p");
		// 3. Make non-writable (retains configurable: true)
		Object.defineProperty(obj, "p", { writable: false });
		var d3 = Object.getOwnPropertyDescriptor(obj, "p");
		// 4. Make non-configurable
		Object.defineProperty(obj, "p", { configurable: false });
		var d4 = Object.getOwnPropertyDescriptor(obj, "p");

		"" + d2.value + "," + d2.writable + "," + d2.configurable + "|" +
		d3.value + "," + d3.writable + "," + d3.configurable + "|" +
		d4.value + "," + d4.writable + "," + d4.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec redéfinition séquentielle : %v", err)
	}
	if got != "2,true,true|2,false,true|2,false,false" {
		t.Fatalf("obtenu %q, attendu 2,true,true|2,false,true|2,false,false", got)
	}
}

// TestObjectAssignGetterInvocation validates that Object.assign invokes getters on source objects.
func TestObjectAssignGetterInvocation(t *testing.T) {
	got, err := compileAndRun(t, `
		var src = {
			_val: 5
		};
		Object.defineProperty(src, "computed", {
			get: function() { return this._val * 10; },
			enumerable: true
		});
		var target = {};
		Object.assign(target, src);
		var desc = Object.getOwnPropertyDescriptor(target, "computed");
		"" + target.computed + "," + desc.value + "," + desc.writable + "," + desc.enumerable + "," + desc.configurable;
	`, false)
	if err != nil {
		t.Fatalf("échec Object.assign avec getter : %v", err)
	}
	if got != "50,50,true,true,true" {
		t.Fatalf("obtenu %q, attendu 50,50,true,true,true", got)
	}
}
