// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"encoding/json"
	"fmt"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

func (vm *VM) InstallJSONBuiltins() {
	obj := vm.heap.NewObject()
	vm.SetGlobal("JSON", ObjectValue(obj))
	set := func(name string, v Value) {
		vm.heap.SetProperty(obj, vm.heap.Intern().InternGo(name), v)
	}
	st := &Chunk{
		Name:   "stringify",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			if len(args) == 0 {
				return Undefined, nil
			}
			s, err := vm.BuiltinJSONStringify(args[0])
			if err != nil {
				return Undefined, err
			}
			if s == "undefined" {
				return Undefined, nil
			}
			return vm.NewStringValue(str.FromGo(s)), nil
		},
	}
	set("stringify", ObjectValue(vm.heap.NewFunction(st, NoHandle)))
	pr := &Chunk{
		Name:   "parse",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			if len(args) == 0 {
				return Null, nil
			}
			s := vm.StringOf(args[0])
			if s == nil {
				return Undefined, fmt.Errorf("SyntaxError: JSON.parse")
			}
			var raw any
			if err := json.Unmarshal([]byte(s.GoString()), &raw); err != nil {
				return Undefined, fmt.Errorf("SyntaxError: JSON.parse")
			}
			return vm.valueFromJSON(raw), nil
		},
	}
	set("parse", ObjectValue(vm.heap.NewFunction(pr, NoHandle)))
}

func (vm *VM) valueFromJSON(raw any) Value {
	switch t := raw.(type) {
	case nil:
		return Null
	case bool:
		return Bool(t)
	case float64:
		return Number(t)
	case string:
		return vm.NewStringValue(str.FromGo(t))
	case []any:
		h := vm.heap.NewArray(len(t))
		v := ObjectValue(h)
		vm.heap.AddRoot(&v)
		for i, el := range t {
			vm.heap.SetElement(h, i, vm.valueFromJSON(el))
		}
		vm.heap.RemoveRoot(&v)
		return v
	case map[string]any:
		h := vm.heap.NewObject()
		v := ObjectValue(h)
		vm.heap.AddRoot(&v)
		for k, el := range t {
			vm.heap.SetProperty(h, vm.heap.Intern().InternGo(k), vm.valueFromJSON(el))
		}
		vm.heap.RemoveRoot(&v)
		return v
	default:
		return Null
	}
}

// BuiltinJSONStringify converts a Value into its JSON string representation.
func (vm *VM) BuiltinJSONStringify(v Value) (string, error) {
	if v.IsUndefined() {
		return "undefined", nil
	}
	if v.IsNull() {
		return "null", nil
	}
	if v.IsBool() {
		if v.ToBool() {
			return "true", nil
		}
		return "false", nil
	}
	if v.IsInt() {
		return fmt.Sprintf("%d", v.ToInt()), nil
	}
	if v.IsNumber() {
		return fmt.Sprintf("%g", v.ToFloat()), nil
	}
	if s := vm.StringOf(v); s != nil {
		b, err := json.Marshal(s.GoString())
		return string(b), err
	}
	return vm.jsonStringifyValue(v, map[Handle]bool{})
}

func (vm *VM) jsonStringifyValue(v Value, seen map[Handle]bool) (string, error) {
	if v.IsUndefined() {
		return "undefined", nil
	}
	if v.IsNull() {
		return "null", nil
	}
	if v.IsBool() {
		if v.ToBool() {
			return "true", nil
		}
		return "false", nil
	}
	if v.IsInt() {
		return fmt.Sprintf("%d", v.ToInt()), nil
	}
	if v.IsNumber() {
		return fmt.Sprintf("%g", v.ToFloat()), nil
	}
	if s := vm.StringOf(v); s != nil {
		b, err := json.Marshal(s.GoString())
		return string(b), err
	}
	if v.IsObject() {
		return vm.jsonStringifyObject(v, seen)
	}
	return "null", nil
}

func (vm *VM) jsonStringifyObject(v Value, seen map[Handle]bool) (string, error) {
	h := v.Handle()
	if seen[h] {
		return "", fmt.Errorf("TypeError: cyclic JSON")
	}
	seen[h] = true
	defer delete(seen, h)
	o := vm.heap.Get(h)
	if o == nil {
		return "null", nil
	}
	if o.kind == KindArray {
		parts := make([]string, 0, len(o.elements))
		for _, el := range o.elements {
			s, err := vm.jsonStringifyValue(el, seen)
			if err != nil {
				return "", err
			}
			if s == "undefined" {
				s = "null"
			}
			parts = append(parts, s)
		}
		out := "["
		for i, p := range parts {
			if i > 0 {
				out += ","
			}
			out += p
		}
		return out + "]", nil
	}
	keys := vm.BuiltinObjectKeys(v)
	out := "{"
	first := true
	for _, k := range keys {
		val, ok := vm.heap.GetProperty(h, k)
		if !ok || val.IsUndefined() {
			continue
		}
		ks, err := json.Marshal(k.GoString())
		if err != nil {
			return "", err
		}
		vs, err := vm.jsonStringifyValue(val, seen)
		if err != nil {
			return "", err
		}
		if vs == "undefined" {
			continue
		}
		if !first {
			out += ","
		}
		first = false
		out += string(ks) + ":" + vs
	}
	return out + "}", nil
}
