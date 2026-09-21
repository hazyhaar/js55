// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"fmt"
	"math"
	"strconv"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// maxDenseArrayGrowth borne la matérialisation dense d'un tableau lors d'une
// redéfinition de "length" ou d'un index. Le stockage des tableaux étant dense
// (length == len(elements)), une longueur proche de 2^32 matérialiserait des
// milliards de Value et épuiserait la mémoire du processus. Au-delà de cette
// borne la croissance est refusée plutôt que tentée.
const maxDenseArrayGrowth = 1 << 20

// PropertyDescriptor holds property attributes.
type PropertyDescriptor struct {
	Value        Value
	Writable     bool
	Enumerable   bool
	Configurable bool
	Getter       Value
	Setter       Value
}

func (vm *VM) ownPropertyNames(v Value, enumerableOnly bool) []*str.String {
	if s := vm.StringOf(v); s != nil {
		keys := make([]*str.String, 0, s.Len()+1)
		for i := 0; i < s.Len(); i++ {
			keys = append(keys, vm.heap.Intern().InternGo(strconv.Itoa(i)))
		}
		if !enumerableOnly {
			keys = append(keys, vm.heap.Intern().InternGo("length"))
		}
		return keys
	}
	if !v.IsObject() {
		return nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.shape == nil {
		return nil
	}
	seen := map[*str.String]bool{}
	var ints []int
	var rest []*str.String
	if o.kind == KindArray {
		for i := 0; i < len(o.elements); i++ {
			ints = append(ints, i)
		}
	}
	if o.kind == KindStringObject && o.text != nil {
		for i := 0; i < o.text.Len(); i++ {
			ints = append(ints, i)
		}
	}
	for _, k := range o.shape.Keys() {
		if i, ok := arrayIndexKey(k); ok {
			dup := false
			for _, x := range ints {
				if x == i {
					dup = true
					break
				}
			}
			if !dup {
				ints = append(ints, i)
			}
			continue
		}
		rest = append(rest, k)
	}
	for i := 1; i < len(ints); i++ {
		for j := i; j > 0 && ints[j] < ints[j-1]; j-- {
			ints[j], ints[j-1] = ints[j-1], ints[j]
		}
	}
	keys := make([]*str.String, 0, len(ints)+len(rest))
	for _, i := range ints {
		k := vm.heap.Intern().InternGo(strconv.Itoa(i))
		seen[k] = true
		keys = append(keys, k)
	}
	for _, k := range rest {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if (o.kind == KindArray || o.kind == KindStringObject) && !enumerableOnly {
		lenKey := vm.heap.Intern().InternGo("length")
		if !seen[lenKey] {
			seen[lenKey] = true
			keys = append(keys, lenKey)
		}
	}
	if o.kind == KindFunction && !enumerableOnly {
		lenKey := vm.heap.Intern().InternGo("length")
		if !seen[lenKey] {
			seen[lenKey] = true
			keys = append(keys, lenKey)
		}
		nameKey := vm.heap.Intern().InternGo("name")
		if !seen[nameKey] {
			seen[nameKey] = true
			keys = append(keys, nameKey)
		}
	}
	if v.Handle() == vm.globalObj && vm.globalObj != NoHandle && !enumerableOnly {
		for gk := range vm.globals {
			if !seen[gk] {
				seen[gk] = true
				keys = append(keys, gk)
			}
		}
	}
	if !enumerableOnly && len(o.attrs) == 0 {
		return keys
	}
	out := make([]*str.String, 0, len(keys))
	for _, k := range keys {
		if enumerableOnly {
			if o.kind == KindArray && k.Equal(str.FromGo("length")) {
				continue
			}
			if o.kind == KindArguments && (k.Equal(str.FromGo("length")) || k.Equal(str.FromGo("callee"))) {
				continue
			}
			// lastIndex d'une expression régulière est posé sans attribut
			// explicite, donc énumérable par défaut, alors que la spécification
			// le veut non énumérable. Il apparaîtrait sinon dans Object.keys et
			// ferait échouer Object.defineProperties(o, new RegExp()) sur une
			// entrée parasite.
			if o.kind == KindRegExp && k.Equal(str.FromGo("lastIndex")) {
				continue
			}
			if o.kind == KindFunction && (k.Equal(str.FromGo("length")) || k.Equal(str.FromGo("name"))) {
				slot := o.shape.Lookup(k)
				if slot < 0 || o.slotAttr(slot)&attrEnumerable == 0 {
					continue
				}
			}
			slot := o.shape.Lookup(k)
			if slot >= 0 && o.slotAttr(slot)&attrEnumerable == 0 {
				continue
			}
		}
		out = append(out, k)
	}
	return out
}

// descEntry associe une clé de Properties au descripteur lu pour elle.
type descEntry struct {
	key  *str.String
	desc Value
}

// readPropertiesDescriptors applique la première moitié d'ObjectDefineProperties :
// pour chaque clé propre énumérable de Properties, la valeur est LUE par [[Get]],
// donc un accesseur est invoqué avec Properties pour receveur.
//
// La lecture est complète avant la moindre définition : la spécification exige
// que Properties soit entièrement dépouillé avant que l'objet cible ne soit
// touché, sinon un getter qui lance laisse la cible à demi construite.
func (vm *VM) readPropertiesDescriptors(props Value) ([]descEntry, error) {
	if props.IsNull() || props.IsUndefined() {
		return nil, fmt.Errorf("TypeError: Cannot convert undefined or null to object")
	}
	propsObj := props
	if s := vm.StringOf(props); s != nil {
		propsObj = vm.toObject(props)
	} else if !props.IsObject() {
		propsObj = vm.toObject(props)
	} else if o := vm.heap.Get(props.Handle()); o != nil && (o.kind == KindSymbol || o.kind == KindBigInt) {
		propsObj = vm.toObject(props)
	}
	names := vm.ownPropertyNames(propsObj, true)
	out := make([]descEntry, 0, len(names))
	for _, k := range names {
		desc, err := vm.getPropInvoke(propsObj, k)
		if err != nil {
			return nil, err
		}
		if !vm.isJSObject(desc) {
			return nil, fmt.Errorf("TypeError: Property description must be an object")
		}
		out = append(out, descEntry{key: k, desc: desc})
	}
	return out, nil
}

func (vm *VM) descField(desc Value, field string) (Value, bool) {
	if !desc.IsObject() {
		return Undefined, false
	}
	k := vm.heap.Intern().InternGo(field)
	exists := false
	h := desc.Handle()
	depth := 0
	for h != NoHandle && depth < 128 {
		if _, ok := vm.heap.GetProperty(h, k); ok {
			exists = true
			break
		}
		o := vm.heap.Get(h)
		if o == nil {
			break
		}
		h = o.proto
		depth++
	}
	if !exists {
		return Undefined, false
	}
	v, err := vm.getPropInvoke(desc, k)
	if err != nil {
		return Undefined, true
	}
	return v, true
}

func (vm *VM) defineDataFromDesc(target Handle, name *str.String, desc Value) error {
	o := vm.heap.Get(target)
	if o == nil {
		return fmt.Errorf("TypeError: cannot define property on null or undefined")
	}

	getV, hasGet := vm.descField(desc, "get")
	setV, hasSet := vm.descField(desc, "set")
	valV, hasVal := vm.descField(desc, "value")
	vm.heap.AddRoot(&desc)
	vm.heap.AddRoot(&getV)
	vm.heap.AddRoot(&setV)
	vm.heap.AddRoot(&valV)
	defer vm.heap.RemoveRoot(&desc)
	defer vm.heap.RemoveRoot(&getV)
	defer vm.heap.RemoveRoot(&setV)
	defer vm.heap.RemoveRoot(&valV)
	_, hasWritable := vm.descField(desc, "writable")
	_, hasEnum := vm.descField(desc, "enumerable")
	_, hasConf := vm.descField(desc, "configurable")

	// 1. Validation de compatibilité de descripteur : rejet si accesseurs ET valeur/inscriptibilité
	if (hasGet || hasSet) && (hasVal || hasWritable) {
		return fmt.Errorf("TypeError: Invalid property descriptor. Cannot both specify accessors and a value or writable attribute")
	}

	// 2. Validation des accesseurs get/set (doivent être fonction ou undefined)
	if hasGet && !vm.isFunction(getV) && !getV.IsUndefined() {
		return fmt.Errorf("TypeError: Getter must be a function: %v", vm.toDisplayString(getV))
	}
	if hasSet && !vm.isFunction(setV) && !setV.IsUndefined() {
		return fmt.Errorf("TypeError: Setter must be a function: %v", vm.toDisplayString(setV))
	}

	// Cas particulier : propriété "length" sur un tableau
	if o.kind == KindArray && name.Equal(str.FromGo("length")) {
		if hasGet || hasSet {
			return fmt.Errorf("TypeError: Invalid property descriptor. Cannot specify accessors on array length")
		}
		if hasEnum {
			enumV, _ := vm.descField(desc, "enumerable")
			if vm.truthy(enumV) {
				return fmt.Errorf("TypeError: Cannot make array length enumerable")
			}
		}
		if hasConf {
			confV, _ := vm.descField(desc, "configurable")
			if vm.truthy(confV) {
				return fmt.Errorf("TypeError: Cannot make array length configurable")
			}
		}
		lenSlot := o.shape.Lookup(name)
		lenAttrs := attrWritable
		if lenSlot >= 0 {
			lenAttrs = o.slotAttr(lenSlot)
		}
		// L'inscriptibilité qui gouverne le refus de changer la longueur est
		// celle qui précède la redéfinition : la spécification retire
		// l'attribut d'écriture APRÈS avoir appliqué la nouvelle longueur.
		wasWritable := lenAttrs&attrWritable != 0
		if hasWritable {
			writV, _ := vm.descField(desc, "writable")
			if !vm.truthy(writV) {
				lenAttrs &^= attrWritable
			} else if !wasWritable {
				return fmt.Errorf("TypeError: Cannot make non-writable array length writable")
			}
		}
		if hasVal {
			f := vm.toNumber(valV)
			u := uint32(f)
			if float64(u) != f || f < 0 {
				return fmt.Errorf("RangeError: Invalid array length")
			}
			targetLen := int(u)
			if !wasWritable && targetLen != len(o.elements) {
				return fmt.Errorf("TypeError: Cannot change length of non-writable array")
			}
			if targetLen < len(o.elements) {
				// Troncature descendante : elle s'arrête à la première
				// propriété indexée non configurable rencontrée, la longueur
				// est figée juste au-dessus d'elle, et l'opération échoue.
				stop := targetLen
				for i := len(o.elements) - 1; i >= targetLen; i-- {
					k := vm.heap.Intern().InternGo(strconv.Itoa(i))
					if slot := o.shape.Lookup(k); slot >= 0 && o.slotAttr(slot)&attrConfigurable == 0 {
						stop = i + 1
						break
					}
					vm.heap.DeleteProperty(target, k)
				}
				o.elements = o.elements[:stop]
				if stop != targetLen {
					vm.heap.DefineDataProperty(target, name, Int(int32(stop)), lenAttrs)
					return fmt.Errorf("TypeError: Cannot delete non-configurable array element at index %d", stop-1)
				}
			} else if targetLen > len(o.elements) {
				if targetLen > maxDenseArrayGrowth {
					return fmt.Errorf("RangeError: array length %d exceeds dense element budget", targetLen)
				}
				for len(o.elements) < targetLen {
					o.elements = append(o.elements, Undefined)
				}
			}
		}
		vm.heap.DefineDataProperty(target, name, Int(int32(len(o.elements))), lenAttrs)
		return nil
	}

	// Vérification de la propriété existante et de sa configurabilité
	existingVal, exists := vm.heap.GetOwnProperty(target, name)
	existingAttrs := uint8(0)
	if exists && o.shape != nil {
		if slot := o.shape.Lookup(name); slot >= 0 {
			existingAttrs = o.slotAttr(slot)
		} else {
			existingAttrs = attrDefault
		}
		if o.kind == KindFunction && (name.Equal(str.FromGo("length")) || name.Equal(str.FromGo("name"))) {
			if slot := o.shape.Lookup(name); slot >= 0 && o.slotAttr(slot) == attrDefault {
				existingAttrs = attrConfigurable
				vm.heap.SetPropertyAttrs(target, name, attrConfigurable)
			}
		}
	} else if !exists && o.kind == KindFunction && o.fn != nil {
		if name.Equal(str.FromGo("length")) {
			exists = true
			existingVal = Int(int32(o.fn.Params))
			existingAttrs = attrConfigurable
		} else if name.Equal(str.FromGo("name")) {
			exists = true
			existingVal = vm.NewStringValue(str.FromGo(o.fn.Name))
			existingAttrs = attrConfigurable
		}
	}

	if !exists && o.frozen {
		return fmt.Errorf("TypeError: Cannot define property on non-extensible object")
	}

	if exists && (existingAttrs&attrConfigurable == 0) {
		if hasConf {
			confV, _ := vm.descField(desc, "configurable")
			if vm.truthy(confV) {
				return fmt.Errorf("TypeError: Cannot redefine non-configurable property")
			}
		}
		if hasEnum {
			enumV, _ := vm.descField(desc, "enumerable")
			if vm.truthy(enumV) != (existingAttrs&attrEnumerable != 0) {
				return fmt.Errorf("TypeError: Cannot redefine non-configurable property enumerable attribute")
			}
		}
		isExistingAccessor := existingVal.IsObject() && vm.heap.Get(existingVal.Handle()) != nil && vm.heap.Get(existingVal.Handle()).kind == KindAccessor
		if isExistingAccessor {
			if hasVal || hasWritable {
				return fmt.Errorf("TypeError: Cannot change accessor property to data property on non-configurable property")
			}
			acc := vm.heap.Get(existingVal.Handle())
			curGet := Undefined
			curSet := Undefined
			if len(acc.elements) > 0 {
				curGet = acc.elements[0]
			}
			if len(acc.elements) > 1 {
				curSet = acc.elements[1]
			}
			if hasGet && !vm.sameValue(getV, curGet) {
				return fmt.Errorf("TypeError: Cannot change getter on non-configurable property")
			}
			if hasSet && !vm.sameValue(setV, curSet) {
				return fmt.Errorf("TypeError: Cannot change setter on non-configurable property")
			}
		} else {
			if hasGet || hasSet {
				return fmt.Errorf("TypeError: Cannot change data property to accessor property on non-configurable property")
			}
			if existingAttrs&attrWritable == 0 {
				if hasWritable {
					writV, _ := vm.descField(desc, "writable")
					if vm.truthy(writV) {
						return fmt.Errorf("TypeError: Cannot change non-writable to writable on non-configurable property")
					}
				}
				if hasVal && !vm.sameValue(valV, existingVal) {
					return fmt.Errorf("TypeError: Cannot change value of non-writable non-configurable property")
				}
			}
		}
	}

	attrs := uint8(0)
	if exists {
		attrs = existingAttrs
	}
	attrs = vm.applyDescBit(desc, "writable", attrWritable, attrs)
	attrs = vm.applyDescBit(desc, "enumerable", attrEnumerable, attrs)
	attrs = vm.applyDescBit(desc, "configurable", attrConfigurable, attrs)

	if hasGet || hasSet {
		if cur, ok := vm.heap.GetOwnProperty(target, name); ok && cur.IsObject() && vm.heap.Get(cur.Handle()) != nil && vm.heap.Get(cur.Handle()).kind == KindAccessor {
			accH := vm.heap.prepareWrite(cur.Handle())
			acc := vm.heap.getLocal(accH)
			if acc != nil {
				if hasGet {
					acc.elements[0] = getV
				}
				if hasSet {
					acc.elements[1] = setV
				}
			}
			vm.heap.SetProperty(target, name, ObjectValue(accH))
		} else {
			accH := vm.heap.NewObject()
			if acc := vm.heap.Get(accH); acc != nil {
				acc.kind = KindAccessor
				acc.elements = []Value{getV, setV}
			}
			av := ObjectValue(accH)
			vm.heap.AddRoot(&av)
			vm.heap.SetProperty(target, name, av)
			vm.heap.RemoveRoot(&av)
		}
		vm.heap.SetPropertyAttrs(target, name, attrs&^attrWritable)
		return nil
	}

	val := Undefined
	if exists {
		val = existingVal
	}
	if hasVal {
		val = valV
	}
	vm.heap.DefineDataProperty(target, name, val, attrs)
	if o.kind == KindArray {
		if idx, ok := arrayIndexKey(name); ok && idx < maxDenseArrayGrowth {
			for len(o.elements) <= idx {
				o.elements = append(o.elements, Undefined)
			}
			o.elements[idx] = val
		}
	}
	return nil
}

func (vm *VM) applyDescBit(desc Value, field string, bit uint8, attrs uint8) uint8 {
	v, ok := vm.descField(desc, field)
	if !ok {
		return attrs
	}
	if vm.truthy(v) {
		return attrs | bit
	}
	return attrs &^ bit
}

// stringSlotDescriptor rend le descripteur d'une position ou de la longueur
// d'une chaîne : ni inscriptible ni configurable, énumérable pour les seules
// positions.
func (vm *VM) stringSlotDescriptor(val Value, enumerable bool) Value {
	desc := vm.heap.NewObject()
	d := ObjectValue(desc)
	vm.heap.AddRoot(&d)
	defer vm.heap.RemoveRoot(&d)
	vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("value"), val)
	vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("writable"), False)
	vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), Bool(enumerable))
	vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), False)
	return d
}

func (vm *VM) dataDescriptorObject(v Value, key *str.String) Value {
	if s := vm.StringOf(v); s != nil {
		if key.Equal(str.FromGo("length")) {
			desc := vm.heap.NewObject()
			d := ObjectValue(desc)
			vm.heap.AddRoot(&d)
			defer vm.heap.RemoveRoot(&d)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("value"), Int(int32(s.Len())))
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("writable"), False)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), False)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), False)
			return d
		}
		if i, ok := arrayIndexKey(key); ok && i >= 0 && i < s.Len() {
			desc := vm.heap.NewObject()
			d := ObjectValue(desc)
			vm.heap.AddRoot(&d)
			defer vm.heap.RemoveRoot(&d)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("value"), vm.NewStringValue(s.Slice(i, i+1)))
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("writable"), False)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), True)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), False)
			return d
		}
		return Undefined
	}
	if !v.IsObject() {
		return Undefined
	}
	o := vm.heap.Get(v.Handle())
	if o == nil {
		return Undefined
	}

	// Un objet enveloppe String expose la longueur et les positions de la
	// chaîne enveloppée comme propriétés propres ; sans ce traitement le
	// descripteur d'un index rendrait undefined.
	if o.kind == KindStringObject && o.text != nil {
		s := o.text
		if key.Equal(str.FromGo("length")) {
			return vm.stringSlotDescriptor(Int(int32(s.Len())), false)
		}
		if i, ok := arrayIndexKey(key); ok && i >= 0 && i < s.Len() {
			return vm.stringSlotDescriptor(vm.NewStringValue(s.Slice(i, i+1)), true)
		}
	}

	// Traitement spécifique de "length" sur un tableau
	if o.kind == KindArray && key.Equal(str.FromGo("length")) {
		attrs := attrWritable
		if slot := o.shape.Lookup(key); slot >= 0 {
			attrs = o.slotAttr(slot)
		}
		desc := vm.heap.NewObject()
		d := ObjectValue(desc)
		vm.heap.AddRoot(&d)
		defer vm.heap.RemoveRoot(&d)
		vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("value"), Int(int32(len(o.elements))))
		vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("writable"), Bool(attrs&attrWritable != 0))
		vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), Bool(attrs&attrEnumerable != 0))
		vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), Bool(attrs&attrConfigurable != 0))
		return d
	}

	if o.kind == KindFunction && o.fn != nil {
		if key.Equal(str.FromGo("length")) {
			val := Int(int32(o.fn.Params))
			attrs := attrConfigurable
			if own, ok := vm.heap.GetOwnProperty(v.Handle(), key); ok {
				val = own
				if slot := o.shape.Lookup(key); slot >= 0 {
					if a := o.slotAttr(slot); a != attrDefault {
						attrs = a
					} else {
						vm.heap.SetPropertyAttrs(v.Handle(), key, attrConfigurable)
					}
				}
			} else {
				vm.heap.DefineDataProperty(v.Handle(), key, val, attrConfigurable)
			}
			desc := vm.heap.NewObject()
			d := ObjectValue(desc)
			vm.heap.AddRoot(&d)
			defer vm.heap.RemoveRoot(&d)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("value"), val)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("writable"), Bool(attrs&attrWritable != 0))
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), Bool(attrs&attrEnumerable != 0))
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), Bool(attrs&attrConfigurable != 0))
			return d
		}
		if key.Equal(str.FromGo("name")) {
			val := vm.NewStringValue(str.FromGo(o.fn.Name))
			attrs := attrConfigurable
			if own, ok := vm.heap.GetOwnProperty(v.Handle(), key); ok {
				val = own
				if slot := o.shape.Lookup(key); slot >= 0 {
					if a := o.slotAttr(slot); a != attrDefault {
						attrs = a
					} else {
						vm.heap.SetPropertyAttrs(v.Handle(), key, attrConfigurable)
					}
				}
			} else {
				vm.heap.DefineDataProperty(v.Handle(), key, val, attrConfigurable)
			}
			desc := vm.heap.NewObject()
			d := ObjectValue(desc)
			vm.heap.AddRoot(&d)
			defer vm.heap.RemoveRoot(&d)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("value"), val)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("writable"), Bool(attrs&attrWritable != 0))
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), Bool(attrs&attrEnumerable != 0))
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), Bool(attrs&attrConfigurable != 0))
			return d
		}
	}

	own, ok := vm.heap.GetOwnProperty(v.Handle(), key)
	if !ok {
		if v.Handle() == vm.globalObj && vm.globalObj != NoHandle {
			kIntern := vm.heap.Intern().Intern(key)
			if gv, exists := vm.globals[kIntern]; exists {
				desc := vm.heap.NewObject()
				d := ObjectValue(desc)
				vm.heap.AddRoot(&d)
				defer vm.heap.RemoveRoot(&d)
				vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("value"), gv)
				vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("writable"), True)
				vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), False)
				vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), True)
				return d
			}
		}
		return Undefined
	}
	attrs := attrDefault
	if o.shape != nil {
		if slot := o.shape.Lookup(key); slot >= 0 {
			attrs = o.slotAttr(slot)
		} else if o.kind == KindArray {
			if _, isIdx := arrayIndexKey(key); isIdx && o.frozen {
				attrs = attrEnumerable
			}
		}
	}
	if attrs == attrDefault && own.IsObject() {
		if fnObj := vm.heap.Get(own.Handle()); fnObj != nil && fnObj.kind == KindFunction && fnObj.fn != nil && fnObj.fn.Native != nil {
			attrs = attrWritable | attrConfigurable
			vm.heap.SetPropertyAttrs(v.Handle(), key, attrs)
		}
	}
	desc := vm.heap.NewObject()
	d := ObjectValue(desc)
	vm.heap.AddRoot(&d)
	defer vm.heap.RemoveRoot(&d)

	if own.IsObject() {
		if acc := vm.heap.Get(own.Handle()); acc != nil && acc.kind == KindAccessor {
			getFn := Undefined
			setFn := Undefined
			if len(acc.elements) > 0 {
				getFn = acc.elements[0]
			}
			if len(acc.elements) > 1 {
				setFn = acc.elements[1]
			}
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("get"), getFn)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("set"), setFn)
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), Bool(attrs&attrEnumerable != 0))
			vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), Bool(attrs&attrConfigurable != 0))
			return d
		}
	}

	vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("value"), own)
	vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("writable"), Bool(attrs&attrWritable != 0))
	vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("enumerable"), Bool(attrs&attrEnumerable != 0))
	vm.heap.SetProperty(desc, vm.heap.Intern().InternGo("configurable"), Bool(attrs&attrConfigurable != 0))
	return d
}

func (vm *VM) ownDescriptorsObject(v Value) (Value, error) {
	if !v.IsObject() {
		return Undefined, fmt.Errorf("TypeError: Object.getOwnPropertyDescriptors called on non-object")
	}
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	for _, k := range vm.ownPropertyNames(v, false) {
		d := vm.dataDescriptorObject(v, k)
		if d.IsUndefined() {
			continue
		}
		vm.heap.SetProperty(h, k, d)
	}
	return hv, nil
}

func (vm *VM) sameValue(a, b Value) bool {
	if a.IsNumber() && b.IsNumber() {
		fa, fb := a.ToFloat(), b.ToFloat()
		if math.IsNaN(fa) && math.IsNaN(fb) {
			return true
		}
		if fa == 0 && fb == 0 {
			return math.Signbit(fa) == math.Signbit(fb)
		}
		return fa == fb
	}
	return vm.strictEq(a, b)
}

// BuiltinGetOwnPropertyDescriptor returns the descriptor of an own property.
func (vm *VM) BuiltinGetOwnPropertyDescriptor(obj Value, key *str.String) (PropertyDescriptor, bool) {
	if !obj.IsObject() {
		return PropertyDescriptor{}, false
	}
	o := vm.heap.Get(obj.Handle())
	if o == nil {
		return PropertyDescriptor{}, false
	}
	if o.kind == KindArray && key.Equal(str.FromGo("length")) {
		attrs := attrWritable
		if slot := o.shape.Lookup(key); slot >= 0 {
			attrs = o.slotAttr(slot)
		}
		return PropertyDescriptor{
			Value:        Int(int32(len(o.elements))),
			Writable:     attrs&attrWritable != 0,
			Enumerable:   attrs&attrEnumerable != 0,
			Configurable: attrs&attrConfigurable != 0,
		}, true
	}
	if o.kind == KindFunction && o.fn != nil {
		if key.Equal(str.FromGo("length")) {
			val := Int(int32(o.fn.Params))
			attrs := attrConfigurable
			if own, ok := vm.heap.GetOwnProperty(obj.Handle(), key); ok {
				val = own
				if slot := o.shape.Lookup(key); slot >= 0 {
					if a := o.slotAttr(slot); a != attrDefault {
						attrs = a
					} else {
						vm.heap.SetPropertyAttrs(obj.Handle(), key, attrConfigurable)
					}
				}
			} else {
				vm.heap.DefineDataProperty(obj.Handle(), key, val, attrConfigurable)
			}
			return PropertyDescriptor{
				Value:        val,
				Writable:     attrs&attrWritable != 0,
				Enumerable:   attrs&attrEnumerable != 0,
				Configurable: attrs&attrConfigurable != 0,
			}, true
		}
		if key.Equal(str.FromGo("name")) {
			val := vm.NewStringValue(str.FromGo(o.fn.Name))
			attrs := attrConfigurable
			if own, ok := vm.heap.GetOwnProperty(obj.Handle(), key); ok {
				val = own
				if slot := o.shape.Lookup(key); slot >= 0 {
					if a := o.slotAttr(slot); a != attrDefault {
						attrs = a
					} else {
						vm.heap.SetPropertyAttrs(obj.Handle(), key, attrConfigurable)
					}
				}
			} else {
				vm.heap.DefineDataProperty(obj.Handle(), key, val, attrConfigurable)
			}
			return PropertyDescriptor{
				Value:        val,
				Writable:     attrs&attrWritable != 0,
				Enumerable:   attrs&attrEnumerable != 0,
				Configurable: attrs&attrConfigurable != 0,
			}, true
		}
	}
	own, ok := vm.heap.GetOwnProperty(obj.Handle(), key)
	if !ok {
		return PropertyDescriptor{}, false
	}
	attrs := attrDefault
	if o.shape != nil {
		if slot := o.shape.Lookup(key); slot >= 0 {
			attrs = o.slotAttr(slot)
		}
	}
	if attrs == attrDefault && own.IsObject() {
		if fnObj := vm.heap.Get(own.Handle()); fnObj != nil && fnObj.kind == KindFunction && fnObj.fn != nil && fnObj.fn.Native != nil {
			attrs = attrWritable | attrConfigurable
			vm.heap.SetPropertyAttrs(obj.Handle(), key, attrs)
		}
	}
	if own.IsObject() {
		if acc := vm.heap.Get(own.Handle()); acc != nil && acc.kind == KindAccessor {
			getFn := Undefined
			setFn := Undefined
			if len(acc.elements) > 0 {
				getFn = acc.elements[0]
			}
			if len(acc.elements) > 1 {
				setFn = acc.elements[1]
			}
			return PropertyDescriptor{
				Getter:       getFn,
				Setter:       setFn,
				Enumerable:   attrs&attrEnumerable != 0,
				Configurable: attrs&attrConfigurable != 0,
			}, true
		}
	}
	return PropertyDescriptor{
		Value:        own,
		Writable:     attrs&attrWritable != 0,
		Enumerable:   attrs&attrEnumerable != 0,
		Configurable: attrs&attrConfigurable != 0,
	}, true
}
