// SPDX-License-Identifier: BUSL-1.1
package engine

import "testing"

func TestGetPropResumeAfterDescriptorProtoAccessorProxy(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "descriptor",
			src: `var o = {};
				Object.defineProperty(o, "a", {value: 1, writable: true, configurable: true});
				var x = o.a;
				Object.defineProperty(o, "a", {value: 9, writable: true, configurable: true});
				"" + x + "," + o.a`,
			want: "1,9",
		},
		{
			name: "prototype",
			src: `var p = {a: 1};
				var o = Object.create(p);
				var x = o.a;
				p.a = 7;
				"" + x + "," + o.a`,
			want: "1,7",
		},
		{
			name: "accessor",
			src: `var n = 1;
				var o = {};
				Object.defineProperty(o, "a", {get: function() { return n; }, configurable: true});
				var x = o.a;
				n = 8;
				"" + x + "," + o.a`,
			want: "1,8",
		},
		{
			name: "proxy",
			src: `var t = {a: 1};
				var p = new Proxy(t, {get: function(o, k) { return o[k]; }});
				var x = p.a;
				t.a = 4;
				"" + x + "," + p.a`,
			want: "1,4",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compileAndRun(t, tc.src, false)
			if err != nil {
				t.Fatalf("%s : %v", tc.name, err)
			}
			if got != tc.want {
				t.Fatalf("%s : obtenu %q, attendu %q", tc.name, got, tc.want)
			}
			gotStress, err := compileAndRun(t, tc.src, true)
			if err != nil {
				t.Fatalf("%s stress : %v", tc.name, err)
			}
			if gotStress != tc.want {
				t.Fatalf("%s stress : obtenu %q, attendu %q", tc.name, gotStress, tc.want)
			}
		})
	}
}

func TestGetPropMapSizeResumeAfterMutation(t *testing.T) {
	src := `var m = new Map();
		m.set("a", 1);
		var x = m.size;
		m.set("b", 2);
		"" + x + "," + m.size`
	got, err := compileAndRun(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1,2" {
		t.Fatalf("map size : obtenu %q, attendu 1,2", got)
	}
}
