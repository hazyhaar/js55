// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

type RegExpData struct {
	find      regexpFinder
	source    string
	flags     string
	global    bool
	sticky    bool
	lastIndex int
}

func (vm *VM) InstallRegExpBuiltins() {
	proto := vm.heap.NewObject()
	protoV := ObjectValue(proto)
	vm.heap.AddRoot(&protoV)
	defer vm.heap.RemoveRoot(&protoV)
	vm.regexpProto = proto

	ctor := vm.heap.NewFunction(&Chunk{
		Name:      "RegExp",
		Params:    2,
		Construct: true,
		Native: func(vm *VM, args []Value) (Value, error) {
			this := vm.ctorThis()
			pattern := ""
			flags := ""
			if len(args) > 0 {
				if rd0 := vm.regexpOf(args[0]); rd0 != nil {
					pattern = rd0.source
					flags = rd0.flags
				} else if !args[0].IsUndefined() {
					pattern = vm.toDisplayString(args[0])
				}
			}
			if len(args) > 1 && !args[1].IsUndefined() {
				flags = vm.toDisplayString(args[1])
			}
			re, err := compileJSRegexp(pattern, flags)
			if err != nil {
				return Undefined, fmt.Errorf("SyntaxError: Invalid regular expression: %v", err)
			}
			if o := vm.heap.Get(this.Handle()); o != nil {
				o.kind = KindRegExp
				o.regexp = &RegExpData{
					find:      re,
					source:    pattern,
					flags:     flags,
					global:    strings.Contains(flags, "g"),
					sticky:    strings.Contains(flags, "y"),
					lastIndex: 0,
				}
				if len(vm.newStack) == 0 || !vm.newStack[len(vm.newStack)-1].IsObject() {
					o.proto = vm.regexpProto
				}
			}
			vm.heap.SetProperty(this.Handle(), vm.heap.Intern().InternGo("lastIndex"), Int(0))
			return this, nil
		},
	}, NoHandle)
	ctorV := ObjectValue(ctor)
	vm.heap.AddRoot(&ctorV)
	defer vm.heap.RemoveRoot(&ctorV)
	vm.heap.SetProperty(ctor, vm.heap.Intern().InternGo("prototype"), protoV)

	vm.defineNative(proto, "exec", 1, func(vm *VM, args []Value) (Value, error) {
		thisVal := vm.CurrentThis()
		rd := vm.regexpOf(thisVal)
		if rd == nil {
			return Null, fmt.Errorf("TypeError: RegExp.prototype.exec called on incompatible receiver")
		}
		if rd.find == nil {
			return Null, nil
		}
		s := ""
		if len(args) > 0 {
			s = vm.toRegExpString(args[0])
		}
		return vm.regexpBuiltinExec(thisVal, rd, s)
	})

	vm.defineNative(proto, "test", 1, func(vm *VM, args []Value) (Value, error) {
		thisVal := vm.CurrentThis()
		execProp := vm.getProp(thisVal, vm.heap.Intern().InternGo("exec"))
		if vm.isFunction(execProp) {
			res, err := vm.invoke(thisVal, execProp, args)
			if err != nil {
				return False, err
			}
			return Bool(!res.IsNull() && !res.IsUndefined()), nil
		}
		res, err := vm.callMethodOrExec(thisVal, args)
		if err != nil {
			return False, err
		}
		return res, nil
	})

	vm.defineNative(proto, "toString", 0, func(vm *VM, args []Value) (Value, error) {
		thisVal := vm.CurrentThis()
		src := "(?:)"
		flags := ""
		if thisVal.IsObject() {
			sVal := vm.getProp(thisVal, vm.heap.Intern().InternGo("source"))
			if !sVal.IsUndefined() {
				src = vm.toDisplayString(sVal)
			}
			fVal := vm.getProp(thisVal, vm.heap.Intern().InternGo("flags"))
			if !fVal.IsUndefined() {
				flags = vm.toDisplayString(fVal)
			}
		}
		return vm.NewStringValue(str.FromGo("/" + src + "/" + flags)), nil
	})

	vm.defineRegExpGetters(protoV)

	vm.SetGlobal("RegExp", ctorV)
}

func (vm *VM) callMethodOrExec(thisVal Value, args []Value) (Value, error) {
	rd := vm.regexpOf(thisVal)
	if rd == nil {
		return False, fmt.Errorf("TypeError: RegExp.prototype.test called on incompatible receiver")
	}
	if rd.find == nil {
		return False, nil
	}
	s := ""
	if len(args) > 0 {
		s = vm.toRegExpString(args[0])
	}
	res, err := vm.regexpBuiltinExec(thisVal, rd, s)
	if err != nil {
		return False, err
	}
	return Bool(!res.IsNull() && !res.IsUndefined()), nil
}

func (vm *VM) defineRegExpGetters(protoV Value) {
	defineFlagGetter := func(name string, flagByte byte) {
		fn := vm.heap.NewFunction(&Chunk{
			Name: "get " + name,
			Native: func(vm *VM, args []Value) (Value, error) {
				thisVal := vm.CurrentThis()
				rd := vm.regexpOf(thisVal)
				if rd == nil {
					return False, nil
				}
				return Bool(strings.ContainsRune(rd.flags, rune(flagByte))), nil
			},
		}, NoHandle)
		_ = vm.defineAccessor(protoV, vm.heap.Intern().InternGo(name), ObjectValue(fn), false)
	}

	defineFlagGetter("global", 'g')
	defineFlagGetter("ignoreCase", 'i')
	defineFlagGetter("multiline", 'm')
	defineFlagGetter("dotAll", 's')
	defineFlagGetter("unicode", 'u')
	defineFlagGetter("sticky", 'y')
	defineFlagGetter("hasIndices", 'd')

	sourceGetter := vm.heap.NewFunction(&Chunk{
		Name: "get source",
		Native: func(vm *VM, args []Value) (Value, error) {
			thisVal := vm.CurrentThis()
			rd := vm.regexpOf(thisVal)
			if rd == nil {
				return vm.NewStringValue(str.FromGo("(?:)")), nil
			}
			if rd.source == "" {
				return vm.NewStringValue(str.FromGo("(?:)")), nil
			}
			return vm.NewStringValue(str.FromGo(rd.source)), nil
		},
	}, NoHandle)
	_ = vm.defineAccessor(protoV, vm.heap.Intern().InternGo("source"), ObjectValue(sourceGetter), false)

	flagsGetter := vm.heap.NewFunction(&Chunk{
		Name: "get flags",
		Native: func(vm *VM, args []Value) (Value, error) {
			thisVal := vm.CurrentThis()
			rd := vm.regexpOf(thisVal)
			if rd == nil {
				return vm.NewStringValue(str.FromGo("")), nil
			}
			return vm.NewStringValue(str.FromGo(rd.flags)), nil
		},
	}, NoHandle)
	_ = vm.defineAccessor(protoV, vm.heap.Intern().InternGo("flags"), ObjectValue(flagsGetter), false)
}

func (vm *VM) regexpOf(v Value) *RegExpData {
	if !v.IsObject() {
		return nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind != KindRegExp {
		return nil
	}
	return o.regexp
}

func splitRegexpRaw(raw string) (pattern, flags string, ok bool) {
	if len(raw) < 2 || raw[0] != '/' {
		return "", "", false
	}
	end := strings.LastIndex(raw, "/")
	if end <= 0 {
		return "", "", false
	}
	return raw[1:end], raw[end+1:], true
}

func compileJSRegexp(pattern, flags string) (regexpFinder, error) {
	seen := make(map[rune]bool, len(flags))
	for _, f := range flags {
		if seen[f] {
			return nil, fmt.Errorf("duplicate flag %c", f)
		}
		seen[f] = true
		switch f {
		case 'g', 'i', 'm', 's', 'u', 'y', 'd', 'v':
		default:
			return nil, fmt.Errorf("invalid flags %q", flags)
		}
	}
	prefix := ""
	if strings.ContainsRune(flags, 'i') {
		prefix += "i"
	}
	if strings.ContainsRune(flags, 'm') {
		prefix += "m"
	}
	if strings.ContainsRune(flags, 's') {
		prefix += "s"
	}

	if jsRegexpNeedsBacktrack(pattern) {
		return compileJSRE(pattern, flags)
	}
	pat := pattern
	pat = strings.ReplaceAll(pat, `\v`, `\x0b`)
	pat = translateJSRegexpUnicodeEscapes(pat)
	if prefix != "" {
		pat = "(?" + prefix + ")" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, err
	}
	return re2Finder{re: re}, nil
}

func jsRegexpNeedsBacktrack(pat string) bool {
	return jsRegexpUnsupported(pat) != nil
}

func jsRegexpUnsupported(pat string) error {
	inClass := false
	esc := false
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		if esc {
			if !inClass && c == 'k' && i+1 < len(pat) && pat[i+1] == '<' {
				return fmt.Errorf("named backreference")
			}
			if !inClass && c >= '1' && c <= '9' {
				return fmt.Errorf("backreference")
			}
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
			continue
		}
		if inClass {
			if c == ']' {
				inClass = false
			}
			continue
		}
		if c == '[' {
			inClass = true
			continue
		}
		if c != '(' || i+2 >= len(pat) || pat[i+1] != '?' {
			continue
		}
		rest := pat[i+2:]
		switch {
		case strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, "!"):
			return fmt.Errorf("lookahead")
		case strings.HasPrefix(rest, "<=") || strings.HasPrefix(rest, "<!"):
			return fmt.Errorf("lookbehind")
		case strings.HasPrefix(rest, "<"):
			return fmt.Errorf("named group")
		}
	}
	return nil
}

func (vm *VM) toRegExpString(v Value) string {
	if s := vm.StringOf(v); s != nil {
		return s.GoString()
	}
	if v.IsObject() {
		if o := vm.heap.Get(v.Handle()); o != nil {
			if o.kind == KindStringObject && o.text != nil {
				return o.text.GoString()
			}
		}
		toStr := vm.getProp(v, vm.heap.Intern().InternGo("toString"))
		if vm.isFunction(toStr) {
			res, err := vm.invoke(v, toStr, nil)
			if err == nil {
				if s := vm.StringOf(res); s != nil {
					return s.GoString()
				}
			}
		}
		valOf := vm.getProp(v, vm.heap.Intern().InternGo("valueOf"))
		if vm.isFunction(valOf) {
			res, err := vm.invoke(v, valOf, nil)
			if err == nil {
				if s := vm.StringOf(res); s != nil {
					return s.GoString()
				}
			}
		}
	}
	return vm.toDisplayString(v)
}

func (vm *VM) regexpBuiltinExec(thisVal Value, rd *RegExpData, s string) (Value, error) {
	isGlobal := rd.global || strings.Contains(rd.flags, "g")
	isSticky := rd.sticky || strings.Contains(rd.flags, "y")
	fullUnicode := strings.ContainsRune(rd.flags, 'u') || strings.ContainsRune(rd.flags, 'v')
	lastIdx := vm.regexpLastIndex(thisVal, isGlobal || isSticky)
	utf16Len := str.UTF16LenOfUTF8Prefix(s, len(s))
	if lastIdx > utf16Len {
		if isGlobal || isSticky {
			vm.setRegexpLastIndex(thisVal, rd, 0)
		}
		return Null, nil
	}
	byteStart := utf8IndexForUTF16(s, lastIdx)
	subStr := s[byteStart:]
	loc := rd.find.FindStringSubmatchIndex(subStr)
	if loc == nil {
		if isGlobal || isSticky {
			vm.setRegexpLastIndex(thisVal, rd, 0)
		}
		return Null, nil
	}
	if isSticky && loc[0] != 0 {
		vm.setRegexpLastIndex(thisVal, rd, 0)
		return Null, nil
	}
	matchStart := lastIdx + str.UTF16LenOfUTF8Prefix(subStr, loc[0])
	matchEnd := lastIdx + str.UTF16LenOfUTF8Prefix(subStr, loc[1])
	endForLast := matchEnd
	if matchStart == matchEnd {
		endForLast = advanceStringIndex(s, matchEnd, fullUnicode)
	}
	if isGlobal || isSticky {
		vm.setRegexpLastIndex(thisVal, rd, endForLast)
	}
	numCaptures := len(loc) / 2
	arr := vm.heap.NewArray(numCaptures)
	arrV := ObjectValue(arr)
	vm.heap.AddRoot(&arrV)
	defer vm.heap.RemoveRoot(&arrV)
	for i := 0; i < numCaptures; i++ {
		cStart := loc[2*i]
		cEnd := loc[2*i+1]
		if cStart == -1 || cEnd == -1 {
			vm.heap.SetElement(arr, i, Undefined)
		} else {
			sub := subStr[cStart:cEnd]
			vm.heap.SetElement(arr, i, vm.NewStringValue(str.FromGo(sub)))
		}
	}
	vm.heap.SetProperty(arr, vm.heap.Intern().InternGo("index"), Int(int32(matchStart)))
	vm.heap.SetProperty(arr, vm.heap.Intern().InternGo("input"), vm.NewStringValue(str.FromGo(s)))
	if names := rd.find.SubexpNames(); namedGroupCount(names) > 0 {
		gh := vm.heap.NewObject()
		gv := ObjectValue(gh)
		vm.heap.AddRoot(&gv)
		for i, n := range names {
			if i == 0 || n == "" {
				continue
			}
			cStart, cEnd := -1, -1
			if 2*i+1 < len(loc) {
				cStart, cEnd = loc[2*i], loc[2*i+1]
			}
			var cv Value = Undefined
			if cStart >= 0 && cEnd >= 0 {
				cv = vm.NewStringValue(str.FromGo(subStr[cStart:cEnd]))
			}
			vm.heap.SetProperty(gh, vm.heap.Intern().InternGo(n), cv)
		}
		vm.heap.SetProperty(arr, vm.heap.Intern().InternGo("groups"), gv)
		vm.heap.RemoveRoot(&gv)
	}
	return arrV, nil
}

func (vm *VM) regexpLastIndex(thisVal Value, globalOrSticky bool) int {
	if !globalOrSticky || !thisVal.IsObject() {
		return 0
	}
	lProp := vm.getProp(thisVal, vm.heap.Intern().InternGo("lastIndex"))
	if !lProp.IsInt() && !lProp.IsNumber() {
		return 0
	}
	f := lProp.ToFloat()
	if math.IsNaN(f) || f <= 0 {
		return 0
	}
	if math.IsInf(f, 1) || f > float64(math.MaxInt32) {
		return math.MaxInt32
	}
	return int(math.Trunc(f))
}

func (vm *VM) setRegexpLastIndex(thisVal Value, rd *RegExpData, idx int) {
	if idx < 0 {
		idx = 0
	}
	if idx > math.MaxInt32 {
		idx = math.MaxInt32
	}
	if thisVal.IsObject() {
		vm.heap.SetProperty(thisVal.Handle(), vm.heap.Intern().InternGo("lastIndex"), Int(int32(idx)))
	}
	rd.lastIndex = idx
}

func utf8IndexForUTF16(s string, utf16Idx int) int {
	if utf16Idx <= 0 {
		return 0
	}
	n := 0
	for i, r := range s {
		if n >= utf16Idx {
			return i
		}
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return len(s)
}

func advanceStringIndex(s string, idx int, fullUnicode bool) int {
	if !fullUnicode {
		return idx + 1
	}
	n := 0
	for _, r := range s {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if n == idx {
			return idx + w
		}
		n += w
		if n > idx {
			return idx + 1
		}
	}
	return idx + 1
}

func translateJSRegexpUnicodeEscapes(pat string) string {
	var b strings.Builder
	b.Grow(len(pat) + 8)
	i := 0
	for i < len(pat) {
		if pat[i] != '\\' || i+1 >= len(pat) {
			b.WriteByte(pat[i])
			i++
			continue
		}
		next := pat[i+1]
		if next == 'u' {
			rest := pat[i+2:]
			if len(rest) >= 1 && rest[0] == '{' {
				j := 1
				for j < len(rest) && rest[j] != '}' {
					j++
				}
				if j < len(rest) && j > 1 && regexpHex(rest[1:j]) {
					b.WriteString(`\x{`)
					b.WriteString(rest[1:j])
					b.WriteByte('}')
					i += 2 + j + 1
					continue
				}
			}
			if len(rest) >= 4 && regexpHex(rest[:4]) {
				b.WriteString(`\x{`)
				b.WriteString(rest[:4])
				b.WriteByte('}')
				i += 6
				continue
			}
		}
		b.WriteByte('\\')
		b.WriteByte(next)
		i += 2
	}
	return b.String()
}

func regexpHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func (vm *VM) coerceRegExp(v Value) (Value, *RegExpData, error) {
	if rd := vm.regexpOf(v); rd != nil {
		return v, rd, nil
	}
	pat := vm.toDisplayString(v)
	find, err := compileJSRegexp(pat, "")
	if err != nil {
		return Undefined, nil, fmt.Errorf("SyntaxError: Invalid regular expression: %v", err)
	}
	h := vm.heap.NewObject()
	o := vm.heap.Get(h)
	o.kind = KindRegExp
	o.regexp = &RegExpData{find: find, source: pat, flags: ""}
	o.proto = vm.regexpProto
	hv := ObjectValue(h)
	return hv, o.regexp, nil
}

func (vm *VM) stringMatch(s string, reVal Value) (Value, error) {
	thisRE, rd, err := vm.coerceRegExp(reVal)
	if err != nil {
		return Undefined, err
	}
	vm.heap.AddRoot(&thisRE)
	defer vm.heap.RemoveRoot(&thisRE)
	if !rd.global {
		return vm.regexpBuiltinExec(thisRE, rd, s)
	}
	vm.setRegexpLastIndex(thisRE, rd, 0)
	var parts []Value
	for i := 0; i < len(s)+1; i++ {
		res, err := vm.regexpBuiltinExec(thisRE, rd, s)
		if err != nil {
			return Undefined, err
		}
		if res.IsNull() {
			break
		}
		zero := vm.heap.GetElement(res.Handle(), 0)
		parts = append(parts, zero)
	}
	if len(parts) == 0 {
		return Null, nil
	}
	arr := vm.heap.NewArray(len(parts))
	arrV := ObjectValue(arr)
	vm.heap.AddRoot(&arrV)
	defer vm.heap.RemoveRoot(&arrV)
	for i, p := range parts {
		vm.heap.SetElement(arr, i, p)
	}
	return arrV, nil
}

func (vm *VM) stringSearch(s string, reVal Value) (Value, error) {
	thisRE, rd, err := vm.coerceRegExp(reVal)
	if err != nil {
		return Undefined, err
	}
	vm.heap.AddRoot(&thisRE)
	defer vm.heap.RemoveRoot(&thisRE)
	saved := rd.lastIndex
	vm.setRegexpLastIndex(thisRE, rd, 0)
	res, err := vm.regexpBuiltinExec(thisRE, rd, s)
	vm.setRegexpLastIndex(thisRE, rd, saved)
	if err != nil {
		return Undefined, err
	}
	if res.IsNull() {
		return Int(-1), nil
	}
	idx := vm.getProp(res, vm.heap.Intern().InternGo("index"))
	return idx, nil
}

func (vm *VM) stringSplitRegexp(s string, rd *RegExpData, lim int) (Value, error) {
	if lim <= 0 {
		return ObjectValue(vm.heap.NewArray(0)), nil
	}
	if rd == nil || rd.find == nil {
		arr := vm.heap.NewArray(1)
		vm.heap.SetElement(arr, 0, vm.NewStringValue(str.FromGo(s)))
		return ObjectValue(arr), nil
	}
	type part struct {
		s     string
		undef bool
	}
	var parts []part
	pos := 0
	for len(parts) < lim {
		if pos > len(s) {
			break
		}
		loc := rd.find.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			parts = append(parts, part{s: s[pos:]})
			break
		}
		if pos == len(s) && loc[0] == 0 && loc[1] == 0 {
			parts = append(parts, part{})
			break
		}
		if loc[0] == 0 && loc[1] == 0 {
			if pos == len(s) {
				break
			}
			_, sz := utf8.DecodeRuneInString(s[pos:])
			if sz < 1 {
				sz = 1
			}
			parts = append(parts, part{s: s[pos : pos+sz]})
			pos += sz
			continue
		}
		parts = append(parts, part{s: s[pos : pos+loc[0]]})
		for i := 1; i < len(loc)/2 && len(parts) < lim; i++ {
			a, e := loc[2*i], loc[2*i+1]
			if a < 0 || e < 0 {
				parts = append(parts, part{undef: true})
			} else {
				parts = append(parts, part{s: s[pos+a : pos+e]})
			}
		}
		if loc[1] <= 0 {
			pos++
		} else {
			pos += loc[1]
		}
	}
	if len(parts) > lim {
		parts = parts[:lim]
	}
	arr := vm.heap.NewArray(len(parts))
	arrV := ObjectValue(arr)
	vm.heap.AddRoot(&arrV)
	defer vm.heap.RemoveRoot(&arrV)
	for i, p := range parts {
		if p.undef {
			vm.heap.SetElement(arr, i, Undefined)
			continue
		}
		vm.heap.SetElement(arr, i, vm.NewStringValue(str.FromGo(p.s)))
	}
	return arrV, nil
}

func (vm *VM) stringReplaceRegexp(s string, reVal Value, rd *RegExpData, repl Value) (Value, error) {
	vm.heap.AddRoot(&reVal)
	defer vm.heap.RemoveRoot(&reVal)
	vm.heap.AddRoot(&repl)
	defer vm.heap.RemoveRoot(&repl)
	if rd.find == nil {
		return vm.NewStringValue(str.FromGo(s)), nil
	}
	global := rd.global
	pos := 0
	var b strings.Builder
	n := 0
	for pos <= len(s) && n < len(s)+2 {
		n++
		loc := rd.find.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			b.WriteString(s[pos:])
			break
		}
		b.WriteString(s[pos : pos+loc[0]])
		matched := s[pos+loc[0] : pos+loc[1]]
		var piece string
		if vm.isFunction(repl) {
			args := []Value{vm.NewStringValue(str.FromGo(matched))}
			for i := 1; i < len(loc)/2; i++ {
				a, e := loc[2*i], loc[2*i+1]
				if a < 0 || e < 0 {
					args = append(args, Undefined)
				} else {
					args = append(args, vm.NewStringValue(str.FromGo(s[pos+a:pos+e])))
				}
			}
			args = append(args, Int(int32(pos+loc[0])), vm.NewStringValue(str.FromGo(s)))
			callRes, err := vm.invoke(Undefined, repl, args)
			if err != nil {
				return Undefined, err
			}
			piece = vm.toDisplayString(callRes)
		} else {
			piece = expandReplace(vm.toDisplayString(repl), s, pos, loc, rd.find.SubexpNames())
		}
		b.WriteString(piece)
		adv := loc[1]
		if adv == loc[0] {
			if pos+adv >= len(s) {
				break
			}
			_, sz := utf8.DecodeRuneInString(s[pos+adv:])
			if sz < 1 {
				sz = 1
			}
			b.WriteString(s[pos+adv : pos+adv+sz])
			pos += adv + sz
		} else {
			pos += adv
		}
		if !global {
			b.WriteString(s[pos:])
			break
		}
	}
	return vm.NewStringValue(str.FromGo(b.String())), nil
}

func namedGroupCount(names []string) int {
	n := 0
	for i, s := range names {
		if i > 0 && s != "" {
			n++
		}
	}
	return n
}

func expandReplace(rep, s string, pos int, loc []int, names []string) string {
	var b strings.Builder
	for i := 0; i < len(rep); i++ {
		if rep[i] != '$' || i+1 >= len(rep) {
			b.WriteByte(rep[i])
			continue
		}
		switch rep[i+1] {
		case '$':
			b.WriteByte('$')
			i++
		case '&':
			b.WriteString(s[pos+loc[0] : pos+loc[1]])
			i++
		case '`':
			b.WriteString(s[:pos+loc[0]])
			i++
		case '\'':
			b.WriteString(s[pos+loc[1]:])
			i++
		default:
			if rep[i+1] == '<' {
				end := strings.IndexByte(rep[i+2:], '>')
				if end < 0 {
					b.WriteByte('$')
					continue
				}
				gname := rep[i+2 : i+2+end]
				idx := -1
				for j, n := range names {
					if j > 0 && n == gname {
						idx = j
						break
					}
				}
				if idx >= 0 && idx*2+1 < len(loc) && loc[2*idx] >= 0 {
					b.WriteString(s[pos+loc[2*idx] : pos+loc[2*idx+1]])
				}
				i += 2 + end
				continue
			}
			if rep[i+1] >= '1' && rep[i+1] <= '9' {
				idx := int(rep[i+1] - '0')
				i++
				if i+1 < len(rep) && rep[i+1] >= '0' && rep[i+1] <= '9' {
					n := idx*10 + int(rep[i+1]-'0')
					if n*2+1 < len(loc) {
						idx = n
						i++
					}
				}
				if idx*2+1 < len(loc) && loc[2*idx] >= 0 {
					b.WriteString(s[pos+loc[2*idx] : pos+loc[2*idx+1]])
				}
			} else {
				b.WriteByte('$')
			}
		}
	}
	return b.String()
}
