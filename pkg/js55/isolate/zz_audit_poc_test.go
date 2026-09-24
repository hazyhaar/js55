// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 HazyHaar. See LICENSE and NOTICE.

package isolate

import (
	"context"
	"sync"
	"testing"
)

func evalStr(t *testing.T, iso *Isolate, src string) string {
	t.Helper()
	v, err := iso.EvalContext(context.Background(), src)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return iso.VM().ToStringValue(v).GoString()
}

func TestAudit_CrossIsolate_NonCowPaths(t *testing.T) {
	warm, _ := New(Config{})
	_ = warm.Close()
	cases := []struct{ name, attack, probe string }{
		{"setPrototypeOf(Object.prototype, nullproto obj)", `var p = Object.create(null); p.evil = "POLLUTED"; Object.setPrototypeOf(Object.prototype, p); "ok"`, `String(({}).evil) + "|" + String(Object.getPrototypeOf(Object.prototype))`},
		{"setPrototypeOf(Object.prototype, null) puis restore", `Object.setPrototypeOf(Object.prototype, null); "ok"`, `typeof ({}).toString`},
		{"Array.prototype.push", `Array.prototype.push("POLLUTED"); "ok"`, `String(Array.prototype.length) + ":" + String(Array.prototype[0]) + ":" + String([].concat === undefined)`},
		{"Array.prototype.reverse après push", `Array.prototype.push("A","B"); Array.prototype.reverse(); "ok"`, `String(Array.prototype[0])`},
		{"Reflect.setPrototypeOf(Function.prototype, nullproto)", `var q = Object.create(null); q.evilFn = 7; Reflect.setPrototypeOf(Function.prototype, q); "ok"`, `String((function(){}).evilFn)`},
		{"Map.prototype.set", `try { Map.prototype.set("k","v"); "ok" } catch(e) { "throw:"+e }`, `(function(){ try { return String(Map.prototype.get("k")) } catch(e) { return "throw" } })()`},
		{"Object.freeze(Object.prototype)", `Object.freeze(Object.prototype); "ok"`, `Object.isFrozen(Object.prototype) + "|" + (function(){ Object.prototype.z = 1; return String(({}).z) })()`},
		{"Object.defineProperty(Array.prototype, '0')", `Object.defineProperty(Array.prototype, "0", {value:"DP", configurable:true}); "ok"`, `String(Array.prototype[0])`},
	}
	for _, c := range cases {
		base, _ := New(Config{})
		before := evalStr(t, base, c.probe)
		_ = base.Close()
		att, _ := New(Config{})
		r := evalStr(t, att, c.attack)
		vic, _ := New(Config{})
		after := evalStr(t, vic, c.probe)
		_ = vic.Close()
		_ = att.Close()
		status := "ETANCHE"
		if after != before {
			status = "FUITE"
		}
		t.Logf("%-52s attaque=%q avant=%q après=%q => %s", c.name, r, before, after, status)
	}
}

func TestAudit_Race_ReadOnlyIntrinsics(t *testing.T) {
	warm, _ := New(Config{})
	_ = warm.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			iso, _ := New(Config{})
			defer iso.Close()
			for j := 0; j < 30; j++ {
				_, _ = iso.EvalContext(context.Background(), `var a=[1,2,3].map(function(x){return x*2}); var o={a:1,b:2}; Object.keys(o).join(","); JSON.stringify({x:[1,2]}); "abc".toUpperCase(); Math.max(1,2); new Map().set(1,2).get(1); /a+/.test("aaa"); Promise.resolve(1); class A { m(){} }; new A().m(); [].push(1);`)
				_ = iso.Reset()
			}
		}()
	}
	wg.Wait()
}
