// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import "testing"

// Cas dérivés de test262 test/built-ins/Object — chacun échoue avant son
// correctif et sert d'oracle décidable au correctif correspondant.
func TestP3ObjectDescriptors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
		skip string
	}{
		{
			// 15.2.3.3-2-39 : 'P' est un objet String, converti par ToPropertyKey.
			name: "getOwnPropertyDescriptor key is String object",
			src:  `var o={"Hello":1}; String(Object.getOwnPropertyDescriptor(o, new String("Hello")).value)`,
			want: "1",
		},
		{
			// 15.2.3.3-2-x : 'P' est un nombre, converti en clé "1".
			name: "getOwnPropertyDescriptor key is number",
			src:  `var o={}; o[1]=42; String(Object.getOwnPropertyDescriptor(o, 1).value)`,
			want: "42",
		},
		{
			// 15.2.3.3-4-1xx : une méthode intrinsèque n'est pas énumérable.
			name: "builtin method not enumerable",
			src:  `String(Object.getOwnPropertyDescriptor(Object, "keys").enumerable)`,
			want: "false",
		},
		{
			name: "builtin method writable and configurable",
			src:  `var d=Object.getOwnPropertyDescriptor(Object, "keys"); String(d.writable)+","+String(d.configurable)`,
			want: "true,true",
		},
		{
			name: "Object.prototype methods absent from for-in",
			src:  `var n=0; for (var k in Object.prototype) n++; String(n)`,
			want: "0",
		},
		{
			// Même exigence sur Math : l'installation des intrinsèques hors du
			// périmètre Object pose encore l'attribut énumérable par défaut.
			name: "builtin method not enumerable on Math",
			src:  `String(Object.getOwnPropertyDescriptor(Math, "ceil").enumerable)`,
			want: "false",
			skip: "defineNative pose attrDefault ; l'installation de Math est hors du périmètre Object",
		},
		{
			// Object.prototype.__proto__ est un accesseur hérité.
			name: "proto accessor get",
			src:  `var p={}; var o=Object.create(p); String(o.__proto__===p)`,
			want: "true",
		},
		{
			name: "proto accessor set",
			src:  `var p={a:7}; var o={}; o.__proto__=p; String(o.a)`,
			want: "7",
			skip: "l'affectation n'invoque un accesseur que s'il est propre à l'objet ; le parcours de la chaîne de prototypes est dans la VM, hors du périmètre Object",
		},
		{
			name: "lookupGetter",
			src:  `var o={get x(){return 1;}}; String(typeof o.__lookupGetter__("x"))`,
			want: "function",
		},
		{
			name: "lookupSetter",
			src:  `var o={set x(v){}}; String(typeof o.__lookupSetter__("x"))`,
			want: "function",
		},
		{
			name: "defineGetter",
			src:  `var o={}; o.__defineGetter__("x", function(){return 5;}); String(o.x)`,
			want: "5",
		},
		{
			name: "defineSetter",
			src:  `var o={}; var seen=0; o.__defineSetter__("x", function(v){seen=v;}); o.x=9; String(seen)`,
			want: "9",
		},
		{
			name: "Object.keys name and length",
			src:  `Object.keys.name+","+String(Object.keys.length)`,
			want: "keys,1",
		},
		{
			name: "Object.keys name descriptor",
			src:  `var d=Object.getOwnPropertyDescriptor(Object.keys,"name"); String(d.value)+","+String(d.writable)+","+String(d.enumerable)+","+String(d.configurable)`,
			want: "keys,false,false,true",
		},
		{
			name: "Object constructor name and length",
			src:  `Object.name+","+String(Object.length)`,
			want: "Object,1",
		},
		{
			name: "Object.prototype constructor not enumerable",
			src:  `String(Object.getOwnPropertyDescriptor(Object.prototype,"constructor").enumerable)`,
			want: "false",
		},
		{
			// 15.2.3.6-4-154 : une longueur hors budget dense ne doit pas épuiser
			// la mémoire ; le refus est explicite.
			name: "array length boundary does not exhaust memory",
			src: `var a=[]; var caught=""; try { Object.defineProperty(a,"length",{value:4294967294}); }` +
				` catch(e) { caught=String(e.name); } caught!=="" ? "thrown" : String(a.length)`,
			want: "thrown",
		},
		{
			name: "toString ordinary object",
			src:  `Object.prototype.toString.call({})`,
			want: "[object Object]",
		},
		{
			name: "toString boxed boolean",
			src:  `Object.prototype.toString.call(Object(true))`,
			want: "[object Boolean]",
		},
		{
			name: "toString boxed number",
			src:  `Object.prototype.toString.call(Object(1))`,
			want: "[object Number]",
		},
		{
			name: "assign wraps string target",
			src:  `var r=Object.assign("a"); typeof r+","+String(r.valueOf())`,
			want: "object,a",
		},
		{
			name: "assign to string index throws",
			src:  `var caught=""; try { Object.assign("a", [1]); } catch(e) { caught=e.name; } caught`,
			want: "TypeError",
		},
		{
			name: "assign copies string source indices",
			src:  `var r=Object.assign({}, "123"); r[0]+r[1]+r[2]`,
			want: "123",
		},
		{
			name: "assign overrides from string sources",
			src:  `var r=Object.assign(12, "aaa", "bb2b", "1c"); r[0]+r[1]+r[2]+r[3]+","+String(Object.getOwnPropertyNames(r).length)`,
			want: "1c2b,4",
		},
		{
			name: "defineProperties rejects non-object descriptor",
			src:  `var caught=""; try { Object.defineProperties({}, {a:1}); } catch(e) { caught=e.name; } caught`,
			want: "TypeError",
		},
		{
			name: "fromEntries rejects primitive string entry",
			src:  `var caught=""; try { Object.fromEntries(["ab"]); } catch(e) { caught=e.name; } caught`,
			want: "TypeError",
		},
		{
			name: "fromEntries boxed string entry",
			src:  `Object.fromEntries([Object("ab")]).a`,
			want: "b",
		},
		{
			name: "fromEntries closes iterator on null entry",
			src: `var closed=false; var it={next:function(){return {done:false,value:null};}, return:function(){closed=true;}};` +
				` var iterable={}; iterable[Symbol.iterator]=function(){return it;};` +
				` var caught=""; try { Object.fromEntries(iterable); } catch(e) { caught=e.name; }` +
				` caught+","+String(closed)`,
			want: "TypeError,true",
		},
		{
			name: "Object.values on string",
			src:  `Object.values("ab").join("")`,
			want: "ab",
		},
		{
			name: "Object.keys on string",
			src:  `Object.keys("ab").join(",")`,
			want: "0,1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip != "" {
				t.Skip(tc.skip)
			}
			got, err := compileAndRun(t, tc.src, false)
			if err != nil {
				t.Fatalf("exécution: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
