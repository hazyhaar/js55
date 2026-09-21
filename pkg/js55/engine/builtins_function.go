// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"fmt"
	"strings"

	"github.com/hazyhaar/js55/pkg/js55/ast"
	"github.com/hazyhaar/js55/pkg/js55/parser"
	"github.com/hazyhaar/js55/pkg/js55/str"
)

func (vm *VM) InstallFunctionBuiltins() {
	proto := vm.heap.NewObject()
	vm.heap.functionProto = proto
	protoV := ObjectValue(proto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)

	ctor := vm.heap.NewFunction(&Chunk{
		Name:      "Function",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			body := ""
			var params []string
			if len(args) > 0 {
				body = vm.toDisplayString(args[len(args)-1])
				for i := 0; i < len(args)-1; i++ {
					params = append(params, vm.toDisplayString(args[i]))
				}
			}
			src := "function anonymous(" + strings.Join(params, ",") + ") {\n" + body + "\n}"
			prog, err := parser.Parse(src, parser.Options{})
			if err != nil {
				return Undefined, fmt.Errorf("SyntaxError: %v", err)
			}
			if len(prog.Body) != 1 {
				return Undefined, fmt.Errorf("SyntaxError: Function body")
			}
			fd, ok := prog.Body[0].(*ast.FunctionDecl)
			if !ok || fd.Fn == nil {
				return Undefined, fmt.Errorf("SyntaxError: Function body")
			}
			var fnChunk *Chunk
			var cerr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						if ce, ok := r.(*CompileError); ok {
							cerr = ce
							return
						}
						panic(r)
					}
				}()
				c := &Compiler{heap: vm.heap, chunk: NewChunk("Function")}
				fnChunk = c.compileFunction(fd.Fn, "anonymous", false)
			}()
			if cerr != nil {
				return Undefined, fmt.Errorf("SyntaxError: %v", cerr)
			}
			h := vm.heap.NewFunction(fnChunk, NoHandle)
			return ObjectValue(h), nil
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	if o := vm.heap.Get(ctor); o != nil {
		o.proto = proto
	}
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), protoV)
	vm.heap.SetProperty(proto, vm.heap.Intern().InternGo("constructor"), ctorV)

	vm.defineNative(proto, "call", 1, func(vm *VM, args []Value) (Value, error) {
		fn := vm.CurrentThis()
		thisArg := Undefined
		if len(args) > 0 {
			thisArg = args[0]
		}
		rest := []Value{}
		if len(args) > 1 {
			rest = args[1:]
		}
		return vm.CallFunction(fn, thisArg, rest)
	})
	vm.defineNative(proto, "apply", 2, func(vm *VM, args []Value) (Value, error) {
		fn := vm.CurrentThis()
		thisArg := Undefined
		if len(args) > 0 {
			thisArg = args[0]
		}
		var list []Value
		if len(args) > 1 {
			list = vm.arrayValues(args[1])
		}
		return vm.CallFunction(fn, thisArg, list)
	})
	vm.defineNative(proto, "bind", 1, func(vm *VM, args []Value) (Value, error) {
		target := vm.CurrentThis()
		boundThis := Undefined
		if len(args) > 0 {
			boundThis = args[0]
		}
		var boundArgs []Value
		if len(args) > 1 {
			boundArgs = append([]Value{}, args[1:]...)
		}
		ch := &Chunk{
			Name:   "bound",
			Params: 0,
			Native: func(vm *VM, extra []Value) (Value, error) {
				call := make([]Value, 0, len(boundArgs)+len(extra))
				call = append(call, boundArgs...)
				call = append(call, extra...)
				return vm.CallFunction(target, boundThis, call)
			},
		}
		h := vm.heap.NewFunction(ch, NoHandle)
		return ObjectValue(h), nil
	})
	vm.defineNative(proto, "toString", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if !vm.isFunction(this) {
			return Undefined, fmt.Errorf("TypeError: Function.prototype.toString requires that 'this' be a Function")
		}
		name := "anonymous"
		if o := vm.heap.Get(this.Handle()); o != nil && o.fn != nil && o.fn.Name != "" {
			name = o.fn.Name
		}
		return vm.NewStringValue(str.FromGo("function " + name + "() { [native code] }")), nil
	})
	vm.SetGlobal("Function", ctorV)
	evalCh := &Chunk{
		Name:   "eval",
		Params: 1,
		Native: func(vm *VM, args []Value) (Value, error) {
			src := ""
			if len(args) > 0 {
				src = vm.toDisplayString(args[0])
			}
			return vm.evalSource(src)
		},
	}
	ev := ObjectValue(vm.heap.NewFunction(evalCh, NoHandle))
	vm.heap.AddRoot(&ev)
	vm.SetGlobal("eval", ev)
	vm.heap.RemoveRoot(&ev)
}

func (vm *VM) evalSource(src string) (Value, error) {
	prog, err := parser.Parse(src, parser.Options{})
	if err != nil {
		return Undefined, vm.throwNamedError(&frame{}, "SyntaxError", "SyntaxError: "+err.Error())
	}
	chunk, err := Compile(vm.heap, prog, "eval")
	if err != nil {
		return Undefined, err
	}
	env := vm.heap.NewEnv(chunk.Locals, NoHandle)
	envV := ObjectValue(env)
	vm.heap.AddRoot(&envV)
	defer vm.heap.RemoveRoot(&envV)
	saved := vm.retMin
	savedFrames := len(vm.frames)
	vm.retMin = savedFrames
	defer func() {
		vm.retMin = saved
		if len(vm.frames) > savedFrames {
			vm.frames = vm.frames[:savedFrames]
		}
	}()
	vm.frames = append(vm.frames, frame{chunk: chunk, env: env, base: vm.heap.StackLen()})
	v, err := vm.interpret(vm.retMin)
	if fin, ok := err.(*finished); ok {
		return fin.value, nil
	}
	return v, err
}
