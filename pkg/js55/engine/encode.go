// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

type encodeThrow struct{ err error }

func (vm *VM) encodeValue(v Value) string {
	s, _ := vm.encodeValueErr(v)
	return s
}

func (vm *VM) encodeValueErr(v Value) (s string, err error) {
	defer func() {
		if r := recover(); r != nil {
			if et, ok := r.(encodeThrow); ok {
				s, err = "", et.err
				return
			}
			panic(r)
		}
	}()
	var b bytes.Buffer
	vm.encodeWalk(&b, v, map[Handle]bool{})
	return b.String(), nil
}

func (vm *VM) encodeWalk(b *bytes.Buffer, v Value, seen map[Handle]bool) {
	if v.IsUndefined() {
		b.WriteString(`{"t":"u"}`)
		return
	}
	if v.IsNull() {
		b.WriteString(`{"t":"n"}`)
		return
	}
	if v.IsBool() {
		if v.ToBool() {
			b.WriteString(`{"t":"b","v":true}`)
		} else {
			b.WriteString(`{"t":"b","v":false}`)
		}
		return
	}
	if s := vm.StringOf(v); s != nil {
		b.WriteString(`{"t":"s","v":`)
		enc, _ := json.Marshal(s.GoString())
		b.Write(enc)
		b.WriteByte('}')
		return
	}
	if v.IsInt() || v.IsNumber() {
		f := v.ToFloat()
		sv := ""
		if f == 0 && !math.IsNaN(f) && math.Signbit(f) {
			sv = "-0"
		} else if math.IsNaN(f) {
			sv = "NaN"
		} else if math.IsInf(f, 1) {
			sv = "Infinity"
		} else if math.IsInf(f, -1) {
			sv = "-Infinity"
		} else if v.IsInt() {
			sv = strconv.FormatInt(int64(v.ToInt()), 10)
		} else {
			sv = numberToString(f)
		}
		b.WriteString(`{"t":"num","v":`)
		enc, _ := json.Marshal(sv)
		b.Write(enc)
		b.WriteByte('}')
		return
	}
	if !v.IsObject() {
		b.WriteString(`{"t":"other","v":`)
		enc, _ := json.Marshal(vm.toDisplayString(v))
		b.Write(enc)
		b.WriteByte('}')
		return
	}
	h := v.Handle()
	if seen[h] {
		b.WriteString(`{"t":"cycle"}`)
		return
	}
	o := vm.heap.Get(h)
	if o == nil {
		b.WriteString(`{"t":"n"}`)
		return
	}
	if o.kind == KindSymbol {
		desc := "Symbol()"
		if o.text != nil {
			s := o.text.GoString()
			if s == "Symbol.iterator" || s == "Symbol.toPrimitive" || s == "Symbol.hasInstance" || s == "Symbol.toStringTag" {
				desc = "Symbol(" + s + ")"
			} else if s != "" {
				desc = "Symbol(" + s + ")"
			}
		}
		b.WriteString(`{"t":"sym","v":`)
		enc, _ := json.Marshal(desc)
		b.Write(enc)
		b.WriteByte('}')
		return
	}
	if o.kind == KindBigInt {
		b.WriteString(`{"t":"bi","v":`)
		enc, _ := json.Marshal(strconv.FormatInt(o.bigInt, 10))
		b.Write(enc)
		b.WriteByte('}')
		return
	}
	if o.kind == KindFunction {
		name := ""
		if nv, ok := vm.heap.GetOwnProperty(h, vm.heap.Intern().InternGo("name")); ok {
			if s := vm.StringOf(nv); s != nil {
				name = s.GoString()
			}
		} else if o.fn != nil {
			name = o.fn.Name
		}
		length := 0
		if lv, ok := vm.heap.GetOwnProperty(h, vm.heap.Intern().InternGo("length")); ok {
			length = int(lv.ToInt())
		} else if o.fn != nil {
			length = o.fn.Params
		}
		b.WriteString(`{"t":"fn","name":`)
		enc, _ := json.Marshal(name)
		b.Write(enc)
		b.WriteString(`,"length":`)
		b.WriteString(strconv.Itoa(length))
		b.WriteByte('}')
		return
	}
	if o.kind == KindPromise && o.promise != nil {
		switch o.promise.state {
		case promiseFulfilled:
			vm.encodeWalk(b, o.promise.result, seen)
			return
		case promiseRejected:
			name, msg, _ := vm.rejectedPromise(v)
			b.WriteString(`{"t":"err","name":`)
			enc, _ := json.Marshal(name)
			b.Write(enc)
			b.WriteString(`,"message":`)
			enc, _ = json.Marshal(msg)
			b.Write(enc)
			b.WriteByte('}')
			return
		default:
			b.WriteString(`{"t":"promise","s":"pending"}`)
			return
		}
	}
	seen[h] = true
	if o.kind == KindArray {
		b.WriteString(`{"t":"arr","els":[`)
		for i, e := range o.elements {
			if i > 0 {
				b.WriteByte(',')
			}
			vm.encodeWalk(b, e, seen)
		}
		b.WriteString(`],"extraKeys":[`)
		var extraKeys []*str.String
		for _, k := range o.shape.Keys() {
			if k.Equal(str.FromGo("length")) {
				continue
			}
			if _, isIdx := arrayIndexKey(k); isIdx {
				continue
			}
			extraKeys = append(extraKeys, k)
		}
		for i, k := range extraKeys {
			if i > 0 {
				b.WriteByte(',')
			}
			enc, _ := json.Marshal(k.GoString())
			b.Write(enc)
		}
		b.WriteString(`],"extra":[`)
		for i, k := range extraKeys {
			if i > 0 {
				b.WriteByte(',')
			}
			ev, _ := vm.heap.GetOwnProperty(h, k)
			vm.encodeWalk(b, ev, seen)
		}
		b.WriteString(`]}`)
		return
	}
	keys := vm.ownPropertyNames(v, true)
	b.WriteString(`{"t":"obj","keys":[`)
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		enc, _ := json.Marshal(k.GoString())
		b.Write(enc)
	}
	b.WriteString(`],"vals":[`)
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		ev, err := vm.getPropInvoke(v, k)
		if err != nil {
			panic(encodeThrow{err: err})
		}
		vm.encodeWalk(b, ev, seen)
	}
	b.WriteString(`]}`)
}

func (vm *VM) rejectedPromise(v Value) (name, msg string, ok bool) {
	if !v.IsObject() {
		return "", "", false
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind != KindPromise || o.promise == nil || o.promise.state != promiseRejected {
		return "", "", false
	}
	res := o.promise.result
	name = "Error"
	msg = vm.toDisplayString(res)
	if res.IsObject() {
		if nv, found := vm.heap.GetProperty(res.Handle(), vm.heap.Intern().InternGo("name")); found {
			if s := vm.StringOf(nv); s != nil && s.GoString() != "" {
				name = s.GoString()
			}
		}
		if mv, found := vm.heap.GetProperty(res.Handle(), vm.heap.Intern().InternGo("message")); found {
			if s := vm.StringOf(mv); s != nil {
				msg = s.GoString()
			}
		}
	}
	return name, msg, true
}
