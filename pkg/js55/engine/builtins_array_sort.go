package engine

import (
	"sort"
	"strconv"
)

func (vm *VM) installArraySort(proto Handle) {
	vm.defineNative(proto, "sort", 1, func(vm *VM, args []Value) (Value, error) {
		this := vm.CurrentThis()
		compare := Undefined
		if len(args) > 0 {
			compare = args[0]
		}
		if !compare.IsUndefined() && !vm.isFunction(compare) {
			return Undefined, vm.throwText(nil, "TypeError: invalid sort comparator")
		}
		if this.IsNull() || this.IsUndefined() {
			return Undefined, vm.throwText(nil, "TypeError: invalid sort receiver")
		}
		n := vm.arrayLikeLength(this)
		vm.heap.TrackAlloc(int64(n) * 8)
		defer vm.heap.TrackFree(int64(n) * 8)
		values := make([]Value, 0, n)
		for i := 0; i < n; i++ {
			if !vm.arrayIndexPresent(this, i) {
				continue
			}
			v, err := vm.arrayGetIndex(this, i)
			if err != nil {
				return Undefined, err
			}
			values = append(values, v)
			vm.heap.AddRoot(&values[len(values)-1])
			defer vm.heap.RemoveRoot(&values[len(values)-1])
		}
		var failure error
		sort.SliceStable(values, func(i, j int) bool {
			if failure != nil {
				return false
			}
			a, b := values[i], values[j]
			if a.IsUndefined() {
				return false
			}
			if b.IsUndefined() {
				return true
			}
			if !compare.IsUndefined() {
				v, err := vm.invoke(Undefined, compare, []Value{a, b})
				if err != nil {
					failure = err
					return false
				}
				n, err := vm.toNumberErr(v)
				failure = err
				return err == nil && n < 0
			}
			x, err := vm.toPrimitive(a, "string")
			if err != nil {
				failure = err
				return false
			}
			vm.heap.AddRoot(&x)
			defer vm.heap.RemoveRoot(&x)
			y, err := vm.toPrimitive(b, "string")
			if err != nil {
				failure = err
				return false
			}
			return vm.ToStringValue(x).Compare(vm.ToStringValue(y)) < 0
		})
		if failure != nil {
			return Undefined, failure
		}
		for i, v := range values {
			if err := vm.setElem(nil, this, Int(int32(i)), v); err != nil {
				return Undefined, err
			}
		}
		for i := len(values); i < n; i++ {
			if !vm.deleteKey(this, vm.heap.Intern().InternGo(strconv.Itoa(i))) {
				return Undefined, vm.throwText(nil, "TypeError: cannot delete sorted property")
			}
		}
		return this, nil
	})
}
