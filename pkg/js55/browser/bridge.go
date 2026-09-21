// SPDX-License-Identifier: BUSL-1.1
//go:build js && wasm

// Package browser connects JS55 values to real browser host APIs. It does not
// evaluate source with the browser's JavaScript engine.
package browser

import (
	"fmt"
	"syscall/js"

	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/str"
)

type Bridge struct {
	VM                  *engine.VM
	ids                 js.Value
	host                map[engine.Value]js.Value
	roots               []*engine.Value
	callbacks           map[engine.Value]js.Func
	closed              bool
	OnError             func(error)
	LastError           error
	MaxObjects          int
	MaxCallbacks        int
	CopyBackElements    int
	callbackErrors      uint64
	DrawElementsCalls   int
	DrawArraysCalls     int
	UniformMatrix4Calls int
	BufferDataCalls     int
	LastBufferSize      int
}

func recordBridgeStats(b *Bridge) {
	st := js.Global().Get("Object").New()
	st.Set("drawElementsCalls", b.DrawElementsCalls)
	st.Set("drawArraysCalls", b.DrawArraysCalls)
	st.Set("uniformMatrix4Calls", b.UniformMatrix4Calls)
	st.Set("bufferDataCalls", b.BufferDataCalls)
	st.Set("lastBufferSize", b.LastBufferSize)
	js.Global().Set("__webglBridgeStats", st)
}

func New(vm *engine.VM) *Bridge {
	return &Bridge{VM: vm, ids: js.Global().Get("Map").New(), host: make(map[engine.Value]js.Value), callbacks: make(map[engine.Value]js.Func), MaxObjects: 4096, MaxCallbacks: 128}
}

func (b *Bridge) root(v engine.Value) engine.Value {
	p := new(engine.Value)
	*p = v
	b.roots = append(b.roots, p)
	b.VM.Heap().AddRoot(p)
	return v
}

// Close must follow removal of host listeners by the application. Callback
// wrappers and object roots have bridge lifetime, not invocation lifetime.
func (b *Bridge) Close() {
	if b.closed {
		return
	}
	b.closed = true
	for _, f := range b.callbacks {
		f.Release()
	}
	for _, p := range b.roots {
		b.VM.Heap().RemoveRoot(p)
	}
	b.callbacks = nil
	b.roots = nil
	b.host = nil
	b.ids.Call("clear")
}

func recoverHost(err *error) {
	if p := recover(); p != nil {
		*err = fmt.Errorf("browser host: %v", p)
	}
}

func (b *Bridge) native(name string, fn func([]engine.Value) (engine.Value, error)) engine.Value {
	return b.root(engine.ObjectValue(b.VM.Heap().NewFunction(&engine.Chunk{Name: name, Native: func(_ *engine.VM, a []engine.Value) (v engine.Value, err error) {
		defer recoverHost(&err)
		if b.closed {
			return engine.Undefined, fmt.Errorf("browser bridge closed")
		}
		if b.VM.Yielding() {
			return engine.Undefined, nil
		}
		return fn(a)
	}}, engine.NoHandle)))
}

func (b *Bridge) Import(x js.Value) (value engine.Value, err error) {
	defer recoverHost(&err)
	if b.closed {
		return engine.Undefined, fmt.Errorf("browser bridge closed")
	}
	if x.Equal(js.Global().Get("eval")) || x.Equal(js.Global().Get("Function")) {
		return engine.Undefined, fmt.Errorf("browser source evaluator forbidden")
	}
	switch x.Type() {
	case js.TypeUndefined:
		return engine.Undefined, nil
	case js.TypeNull:
		return engine.Null, nil
	case js.TypeBoolean:
		return engine.Bool(x.Bool()), nil
	case js.TypeNumber:
		return engine.Number(x.Float()), nil
	case js.TypeString:
		return b.VM.NewStringValue(str.FromGo(x.String())), nil
	case js.TypeObject, js.TypeFunction:
	default:
		return engine.Undefined, fmt.Errorf("unsupported browser value %s", x.Type())
	}
	if id := b.ids.Call("get", x); !id.IsUndefined() {
		return *b.roots[id.Int()], nil
	}
	if len(b.host) >= b.MaxObjects {
		return engine.Undefined, fmt.Errorf("browser object quota exceeded")
	}
	var v engine.Value
	if x.Type() == js.TypeFunction {
		v = b.native("browser method", func(a []engine.Value) (engine.Value, error) {
			this, err := b.Export(b.VM.CurrentThis())
			if err != nil {
				return engine.Undefined, err
			}
			args := js.Global().Get("Array").New(len(a))
			for i, item := range a {
				y, e := b.Export(item)
				if e != nil {
					return engine.Undefined, e
				}
				args.SetIndex(i, y)
			}
			fnName := x.Get("name").String()
			switch fnName {
			case "drawElements":
				b.DrawElementsCalls++
				recordBridgeStats(b)
			case "drawArrays":
				b.DrawArraysCalls++
				recordBridgeStats(b)
			case "uniformMatrix4fv":
				b.UniformMatrix4Calls++
				if len(a) > 2 {
					if mat := args.Index(2); !mat.IsUndefined() && !mat.IsNull() {
						l := mat.Get("length").Int()
						if l > 16 {
							l = 16
						}
						sample := make([]any, l)
						for k := 0; k < l; k++ {
							sample[k] = mat.Index(k).Float()
						}
						js.Global().Set("__lastUniformMatrix4fv", js.ValueOf(sample))
					}
				}
				recordBridgeStats(b)
			case "bufferData":
				b.BufferDataCalls++
				if len(a) > 1 {
					if buf := args.Index(1); !buf.IsUndefined() && !buf.IsNull() {
						b.LastBufferSize = buf.Get("byteLength").Int()
					}
				}
				recordBridgeStats(b)
			}
			before := b.callbackErrors
			result := js.Global().Get("Reflect").Call("apply", x, this, args)
			if b.callbackErrors != before {
				return engine.Undefined, b.LastError
			}
			// Only declared output arguments are copied back. Input-only WebGL
			// uploads and uniforms must never trigger a second full snapshot.
			for i, item := range a {
				if !writesBuffer(x, i) {
					continue
				}
				if name, _, _, isTA := b.VM.TypedArrayInfo(item); isTA {
					data, ok := b.VM.TypedArrayByteWindow(item)
					if ok {
						y := args.Index(i)
						uint8View := js.Global().Get("Uint8Array").New(y.Get("buffer"), y.Get("byteOffset"), y.Get("byteLength"))
						js.CopyBytesToGo(data, uint8View)
						bpe := 1
						switch name {
						case "Float64Array", "BigInt64Array", "BigUint64Array":
							bpe = 8
						case "Float32Array", "Int32Array", "Uint32Array":
							bpe = 4
						case "Int16Array", "Uint16Array":
							bpe = 2
						}
						b.CopyBackElements += len(data) / bpe
					}
					continue
				}

				name, _, ok, e := b.VM.Sequence(item)
				if e != nil {
					return engine.Undefined, e
				}
				if ok && name != "" {
					return engine.Undefined, fmt.Errorf("unsupported copy-back sequence %s", name)
				}
			}
			return b.Import(result)
		})
	} else {
		h := b.VM.Heap()
		target := b.root(engine.ObjectValue(h.NewObject()))
		handler := b.root(engine.ObjectValue(h.NewObject()))
		get := b.native("browser get", func(a []engine.Value) (engine.Value, error) {
			key := b.VM.ToStringValue(a[1]).GoString()
			if key == "getExtension" {
				return b.native("browser getExtension", func(a []engine.Value) (engine.Value, error) {
					if len(a) < 1 {
						return engine.Null, nil
					}
					extName := b.VM.ToStringValue(a[0]).GoString()
					ext := x.Call("getExtension", extName)
					if ext.IsNull() || ext.IsUndefined() {
						return engine.Null, nil
					}
					return b.Import(ext)
				}), nil
			}
			return b.Import(js.Global().Get("Reflect").Call("get", x, hostKey(key)))
		})
		set := b.native("browser set", func(a []engine.Value) (engine.Value, error) {
			key := b.VM.ToStringValue(a[1]).GoString()
			if key == "innerHTML" || key == "outerHTML" || key == "srcdoc" {
				return engine.Undefined, fmt.Errorf("browser bridge: forbidden markup sink %q", key)
			}
			y, e := b.Export(a[2])
			if e != nil {
				return engine.Undefined, e
			}
			if !js.Global().Get("Reflect").Call("set", x, hostKey(key), y).Bool() {
				return engine.Undefined, fmt.Errorf("browser set rejected: %s", key)
			}
			return engine.True, nil
		})
		h.SetProperty(handler.Handle(), h.Intern().InternGo("get"), get)
		h.SetProperty(handler.Handle(), h.Intern().InternGo("set"), set)
		v = b.root(b.VM.NewProxy(target, handler))
	}
	b.host[v] = x
	b.ids.Call("set", x, len(b.roots)-1)
	return v, nil
}

func (b *Bridge) Export(v engine.Value) (value js.Value, err error) {
	defer recoverHost(&err)
	return b.export(v, make(map[engine.Value]bool), 0)
}

func (b *Bridge) export(v engine.Value, active map[engine.Value]bool, depth int) (js.Value, error) {
	if b.closed {
		return js.Undefined(), fmt.Errorf("browser bridge closed")
	}
	if x, ok := b.host[v]; ok {
		return x, nil
	}
	if s := b.VM.StringOf(v); s != nil {
		return js.ValueOf(s.GoString()), nil
	}
	if v.IsUndefined() {
		return js.Undefined(), nil
	}
	if v.IsNull() {
		return js.Null(), nil
	}
	if v.IsBool() {
		return js.ValueOf(v.ToBool()), nil
	}
	if v.IsNumber() || v.IsInt() {
		return js.ValueOf(v.ToFloat()), nil
	}
	if f, ok := b.callbacks[v]; ok {
		return f.Value, nil
	}
	if b.VM.IsCallable(v) {
		if len(b.callbacks) >= b.MaxCallbacks {
			return js.Undefined(), fmt.Errorf("browser callback quota exceeded")
		}
		if b.OnError == nil {
			return js.Undefined(), fmt.Errorf("browser callback requires error sink")
		}
		b.root(v)
		f := js.FuncOf(func(this js.Value, args []js.Value) any {
			if b.closed {
				return nil
			}
			t, e := b.Import(this)
			b.VM.Heap().AddRoot(&t)
			defer b.VM.Heap().RemoveRoot(&t)
			var a []engine.Value
			for _, x := range args {
				y, err := b.Import(x)
				if err != nil {
					e = err
					break
				}
				a = append(a, y)
				p := new(engine.Value)
				*p = y
				b.VM.Heap().AddRoot(p)
				defer b.VM.Heap().RemoveRoot(p)
			}
			var result engine.Value
			var output js.Value
			if e == nil {
				result, e = b.VM.CallFunction(v, t, a)
			}
			if e == nil {
				output, e = b.Export(result)
			}
			if e != nil {
				b.LastError = e
				b.callbackErrors++
				b.OnError(e)
				return nil
			}
			return output
		})
		b.callbacks[v] = f
		return f.Value, nil
	}
	if depth >= 64 || active[v] {
		return js.Undefined(), fmt.Errorf("browser export: cyclic or excessively deep object")
	}
	active[v] = true
	defer delete(active, v)
	if name, _, _, isTA := b.VM.TypedArrayInfo(v); isTA {
		data, ok := b.VM.TypedArrayByteWindow(v)
		if !ok {
			return js.Undefined(), fmt.Errorf("browser export: failed to read TypedArray")
		}
		uint8Array := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(uint8Array, data)
		buffer := uint8Array.Get("buffer")

		bpe := 1
		switch name {
		case "Float64Array", "BigInt64Array", "BigUint64Array":
			bpe = 8
		case "Float32Array", "Int32Array", "Uint32Array":
			bpe = 4
		case "Int16Array", "Uint16Array":
			bpe = 2
		}
		length := len(data) / bpe
		x := js.Global().Get(name).New(buffer, 0, length)
		return x, nil
	}

	name, items, ok, err := b.VM.Sequence(v)
	if err != nil {
		return js.Undefined(), err
	}
	if ok {
		ctor := "Array"
		if name != "" {
			return js.Undefined(), fmt.Errorf("unsupported host sequence %s", name)
		}
		x := js.Global().Get(ctor).New(len(items))
		for i, item := range items {
			y, e := b.export(item, active, depth+1)
			if e != nil {
				return js.Undefined(), e
			}
			x.SetIndex(i, y)
		}
		return x, nil
	}
	if !v.IsObject() {
		return js.Undefined(), fmt.Errorf("unsupported JS55 value")
	}
	x := js.Global().Get("Object").New()
	for _, key := range b.VM.BuiltinObjectKeys(v) {
		item, e := b.VM.GetProperty(v, key.GoString())
		if e != nil {
			return js.Undefined(), e
		}
		y, e := b.export(item, active, depth+1)
		if e != nil {
			return js.Undefined(), e
		}
		x.Set(key.GoString(), y)
	}
	return x, nil
}

// JS55 currently represents well-known symbol property keys by these names.
func hostKey(key string) any {
	switch key {
	case "Symbol.iterator", "Symbol.toPrimitive", "Symbol.hasInstance", "Symbol.toStringTag":
		return js.Global().Get("Symbol").Get(key[7:])
	default:
		return key
	}
}

// Identity checks, not the function's mutable name, select host output APIs.
// Other mutating APIs are outside this proof's transfer contract.
func writesBuffer(fn js.Value, index int) bool {
	for _, entry := range []struct {
		ctor, method string
		index        int
	}{
		{"WebGLRenderingContext", "readPixels", 6},
		{"WebGL2RenderingContext", "readPixels", 6},
		{"WebGL2RenderingContext", "getBufferSubData", 2},
		{"Crypto", "getRandomValues", 0},
	} {
		ctor := js.Global().Get(entry.ctor)
		if index == entry.index && !ctor.IsUndefined() && fn.Equal(ctor.Get("prototype").Get(entry.method)) {
			return true
		}
	}
	return false
}

func (b *Bridge) Install() error {
	for _, name := range []string{"document", "performance", "console"} {
		v, e := b.Import(js.Global().Get(name))
		if e != nil {
			return e
		}
		b.VM.SetGlobal(name, v)
	}
	// The application's global remains JS55's, never the browser global object.
	b.VM.SetGlobal("window", b.VM.GlobalObject())
	b.VM.SetGlobal("self", b.VM.GlobalObject())
	return nil
}

// YieldToHost cède la file d'événements du navigateur. Un rappel hôte synchrone
// qui réentre pendant cette attente voit VM.Yielding et ne touche pas l'interpréteur.
func YieldToHost() {
	done := make(chan struct{}, 1)
	var f js.Func
	f = js.FuncOf(func(this js.Value, args []js.Value) any {
		f.Release()
		done <- struct{}{}
		return nil
	})
	js.Global().Call("setTimeout", f, 0)
	<-done
}
