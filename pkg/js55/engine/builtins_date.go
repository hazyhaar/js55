// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"math"
	"time"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// InstallDateBuiltins registers the Date constructor and methods.
func (vm *VM) InstallDateBuiltins() {
	dateProto := vm.heap.NewObject()
	protoV := ObjectValue(dateProto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)
	vm.defineNative(dateProto, "valueOf", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && (o.prim.IsNumber() || o.prim.IsInt()) {
				return o.prim, nil
			}
		}
		return Number(mathNaN()), nil
	})
	vm.defineNative(dateProto, "getTime", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil && (o.prim.IsNumber() || o.prim.IsInt()) {
				return o.prim, nil
			}
		}
		return Number(mathNaN()), nil
	})
	dateMs := func() float64 {
		this := vm.CurrentThis()
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil {
				return vm.toNumber(o.prim)
			}
		}
		return mathNaN()
	}
	installDateGetter := func(name string, extract func(time.Time) int32) {
		vm.defineNative(dateProto, name, 0, func(vm *VM, args []Value) (Value, error) {
			ms := dateMs()
			if math.IsNaN(ms) {
				return Number(mathNaN()), nil
			}
			return Int(extract(time.UnixMilli(int64(ms)).UTC())), nil
		})
	}
	installDateGetter("getFullYear", func(t time.Time) int32 { return int32(t.Year()) })
	installDateGetter("getUTCFullYear", func(t time.Time) int32 { return int32(t.Year()) })
	installDateGetter("getMonth", func(t time.Time) int32 { return int32(t.Month() - 1) })
	installDateGetter("getUTCMonth", func(t time.Time) int32 { return int32(t.Month() - 1) })
	installDateGetter("getDate", func(t time.Time) int32 { return int32(t.Day()) })
	installDateGetter("getUTCDate", func(t time.Time) int32 { return int32(t.Day()) })
	installDateGetter("getDay", func(t time.Time) int32 { return int32(t.Weekday()) })
	installDateGetter("getUTCDay", func(t time.Time) int32 { return int32(t.Weekday()) })
	installDateGetter("getHours", func(t time.Time) int32 { return int32(t.Hour()) })
	installDateGetter("getUTCHours", func(t time.Time) int32 { return int32(t.Hour()) })
	installDateGetter("getMinutes", func(t time.Time) int32 { return int32(t.Minute()) })
	installDateGetter("getUTCMinutes", func(t time.Time) int32 { return int32(t.Minute()) })
	installDateGetter("getSeconds", func(t time.Time) int32 { return int32(t.Second()) })
	installDateGetter("getUTCSeconds", func(t time.Time) int32 { return int32(t.Second()) })
	installDateGetter("getMilliseconds", func(t time.Time) int32 { return int32(t.Nanosecond() / 1e6) })
	installDateGetter("getUTCMilliseconds", func(t time.Time) int32 { return int32(t.Nanosecond() / 1e6) })
	vm.defineNative(dateProto, "toISOString", 0, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		ms := mathNaN()
		if this.IsObject() {
			if o := vm.heap.Get(this.Handle()); o != nil {
				ms = vm.toNumber(o.prim)
			}
		}
		return vm.NewStringValue(str.FromGo(BuiltinDateToISOString(ms))), nil
	})

	ctor := vm.heap.NewFunction(&Chunk{
		Name:      "Date",
		Params:    1,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			ms := BuiltinDateNow()
			if len(args) > 0 {
				ms = dateTimeClip(vm.toNumber(args[0]))
			}
			constructed := len(vm.newStack) > 0 && vm.newStack[len(vm.newStack)-1].IsObject()
			if !constructed {
				return vm.NewStringValue(str.FromGo(BuiltinDateToISOString(ms))), nil
			}
			this := vm.CurrentThis()
			if this.IsObject() {
				if o := vm.heap.Get(this.Handle()); o != nil {
					o.prim = Number(ms)
				}
			}
			return this, nil
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), protoV)
	vm.SetGlobal("Date", ctorV)
	vm.defineNative(ctor, "now", 0, func(vm *VM, args []Value) (Value, error) {
		return Number(BuiltinDateNow()), nil
	})
	vm.defineNative(ctor, "UTC", 7, func(vm *VM, args []Value) (Value, error) {
		year := 0
		if len(args) > 0 {
			year = int(vm.toNumber(args[0]))
		}
		month, day, h, m, s, ms := 0, 1, 0, 0, 0, 0
		if len(args) > 1 {
			month = int(vm.toNumber(args[1]))
		}
		if len(args) > 2 {
			day = int(vm.toNumber(args[2]))
		}
		if len(args) > 3 {
			h = int(vm.toNumber(args[3]))
		}
		if len(args) > 4 {
			m = int(vm.toNumber(args[4]))
		}
		if len(args) > 5 {
			s = int(vm.toNumber(args[5]))
		}
		if len(args) > 6 {
			ms = int(vm.toNumber(args[6]))
		}
		t := time.Date(year, time.Month(month+1), day, h, m, s, ms*1e6, time.UTC)
		return Number(dateTimeClip(float64(t.UnixMilli()))), nil
	})
	vm.defineNative(ctor, "parse", 1, func(vm *VM, args []Value) (Value, error) {
		if len(args) == 0 {
			return Number(mathNaN()), nil
		}
		s := vm.toDisplayString(args[0])
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t, err = time.Parse("2006-01-02", s)
		}
		if err != nil {
			return Number(mathNaN()), nil
		}
		return Number(dateTimeClip(float64(t.UnixMilli()))), nil
	})
}

func mathNaN() float64 {
	return math.NaN()
}

func dateTimeClip(ms float64) float64 {
	if math.IsNaN(ms) || math.IsInf(ms, 0) {
		return math.NaN()
	}
	if ms > 8.64e15 || ms < -8.64e15 {
		return math.NaN()
	}
	return math.Trunc(ms)
}

// BuiltinDateNow returns the current Unix epoch in milliseconds.
func BuiltinDateNow() float64 {
	return float64(time.Now().UnixNano() / int64(time.Millisecond))
}

// BuiltinDateToISOString formats a Unix timestamp to RFC3339/ISO string.
func BuiltinDateToISOString(msec float64) string {
	sec := int64(msec / 1000)
	nsec := int64((msec - float64(sec*1000)) * 1e6)
	return time.Unix(sec, nsec).UTC().Format(time.RFC3339Nano)
}
