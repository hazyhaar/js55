// SPDX-License-Identifier: BUSL-1.1
//go:build archtime_geometry

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

const threePath = "/devhoros/GAFP/internal/webui/static/three/three.min.js"

func archtimeRun(vm *VM, src string) (Value, error) {
	p, err := parser.Parse(src, parser.Options{})
	if err != nil {
		return Undefined, err
	}
	c, err := Compile(vm.heap, p, "archtime-proof")
	if err != nil {
		return Undefined, err
	}
	return vm.Run(c)
}

func archtimeThree(t *testing.T) (*VM, string) {
	t.Helper()
	b, err := os.ReadFile(threePath)
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(NewHeap())
	if _, err := archtimeRun(vm, string(b)); err != nil {
		t.Fatal(err)
	}
	return vm, string(b)
}

func archtimeJSON(t *testing.T, vm *VM, src string) string {
	t.Helper()
	v, err := archtimeRun(vm, src)
	if err != nil {
		t.Fatal(err)
	}
	s := vm.StringOf(v)
	if s == nil {
		t.Fatalf("expected JSON string, got %v", v)
	}
	return s.GoString()
}

// Both VMs execute the identical original bundle and program. Decimal round-trip
// encoding preserves finite doubles exactly; signed zero has an explicit marker.
const archtimeEncode = `
function packed(b){return [b.min.x,b.min.y,b.min.z,b.max.x,b.max.y,b.max.z].map(function(x){return Object.is(x,-0)?"-0":String(x)})}
`

func archtimeNode(t *testing.T, src string) string {
	t.Helper()
	input, err := json.Marshal(map[string]any{"programs": []string{src}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "testdata/eval_oracle.js")
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Node: %v: %s", err, out)
	}
	var r struct {
		Results []struct{ Display, Error, Message string }
	}
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Results) != 1 || r.Results[0].Error != "" {
		t.Fatalf("Node: %s", out)
	}
	return r.Results[0].Display
}

func TestArchtimeOriginalThree(t *testing.T) {
	vm, three := archtimeThree(t)
	// Four actual Three-generated geometry buffers, not arbitrary random tuples.
	src := archtimeEncode + `var gs=[new THREE.BoxGeometry(2,4,6),new THREE.SphereGeometry(3,8,6),new THREE.PlaneGeometry(4,8,2,3),new THREE.CylinderGeometry(1,2,3,8)];
var result=gs.map(function(g){var b=new THREE.Box3().setFromBufferAttribute(g.attributes.position);g.computeBoundingSphere();return [packed(b),[g.boundingSphere.center.x,g.boundingSphere.center.y,g.boundingSphere.center.z,g.boundingSphere.radius].map(String)]});JSON.stringify(result);`
	got := archtimeJSON(t, vm, src)
	want := archtimeNode(t, three+"\n"+src)
	if got != want {
		t.Fatalf("Three Node mismatch\ngot %s\nwant %s", got, want)
	}
	if vm.KernelStats.Accepted < 8 {
		t.Fatalf("original Box3 not executed by kernel: %+v", vm.KernelStats)
	}
	t.Logf("original Box3 calls: %+v", vm.KernelStats)
}

func TestArchtimeSceneThreeNode(t *testing.T) {
	vm, three := archtimeThree(t)
	b, err := os.ReadFile("/devhoros/pkg/treego/probes/scene.js")
	if err != nil {
		t.Fatal(err)
	}
	got := archtimeJSON(t, vm, string(b))
	want := archtimeNode(t, three+"\n"+string(b))
	if got != want {
		t.Fatalf("unchanged scene mismatch: %s / %s", got, want)
	}
}

const archtimeSetup = archtimeEncode + `
var geometryProof = new THREE.BoxGeometry(2,4,6);
var attrProof = geometryProof.attributes.position;
var boxProof = new THREE.Box3();
var traceProof=[];
boxProof.setFromBufferAttribute(attrProof);
`
const archtimeCall = `boxProof.setFromBufferAttribute(attrProof);JSON.stringify(packed(boxProof));`

func TestArchtimeMutationAndRestoration(t *testing.T) {
	cases := []struct {
		name, mutate, restore string
		reject                bool
	}{
		{"getX body", `var save=THREE.BufferAttribute.prototype.getX;THREE.BufferAttribute.prototype.getX=function getX(i){return 999};`, `THREE.BufferAttribute.prototype.getX=save;`, true},
		{"getY constant", `var save=THREE.BufferAttribute.prototype.getY;THREE.BufferAttribute.prototype.getY=function getY(i){return this.array[i*this.itemSize+1.00000001]};`, `THREE.BufferAttribute.prototype.getY=save;`, true},
		{"getZ body", `var save=THREE.BufferAttribute.prototype.getZ;THREE.BufferAttribute.prototype.getZ=function getZ(i){return -17};`, `THREE.BufferAttribute.prototype.getZ=save;`, true},
		{"set body", `var save=THREE.Vector3.prototype.set;THREE.Vector3.prototype.set=function set(x,y,z){this.x=x+1;this.y=y;this.z=z;return this};`, `THREE.Vector3.prototype.set=save;`, true},
		{"count getter", `var save=attrProof.count;Object.defineProperty(attrProof,'count',{configurable:true,get:function(){traceProof.push('count');return save}});`, `Object.defineProperty(attrProof,'count',{value:save,writable:true,configurable:true,enumerable:true});`, true},
		{"array getter", `var save=attrProof.array;Object.defineProperty(attrProof,'array',{configurable:true,get:function(){traceProof.push('array');return save}});`, `Object.defineProperty(attrProof,'array',{value:save,writable:true,configurable:true,enumerable:true});`, true},
		{"proxy attribute", `var save=attrProof;attrProof=new Proxy(attrProof,{get:function(t,k){traceProof.push(String(k));return t[k]}});`, `attrProof=save;`, true},
		{"readonly input", `Object.defineProperty(attrProof,'count',{writable:false});`, `Object.defineProperty(attrProof,'count',{writable:true});`, true},
		{"normalized", `attrProof.normalized=true;`, `attrProof.normalized=false;`, true},
		{"normalized accessor", `Object.defineProperty(attrProof,'normalized',{configurable:true,get:function(){throw Error('must not call normalized')}});`, `Object.defineProperty(attrProof,'normalized',{value:false,writable:true,configurable:true,enumerable:true});`, true},
		{"count mismatch", `var save=attrProof.count;attrProof.count=1;`, `attrProof.count=save;`, true},
		{"same name other body", `var save=boxProof.setFromBufferAttribute;boxProof.setFromBufferAttribute=function setFromBufferAttribute(a){this.min.x=888;return this};`, `boxProof.setFromBufferAttribute=save;`, false},
		{"proxied main method", `var save=boxProof.setFromBufferAttribute;boxProof.setFromBufferAttribute=new Proxy(save,{});`, `boxProof.setFromBufferAttribute=save;`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vm, three := archtimeThree(t)
			if _, e := archtimeRun(vm, archtimeSetup); e != nil {
				t.Fatal(e)
			}
			if vm.KernelStats.Accepted != 1 {
				t.Fatalf("warmup %+v", vm.KernelStats)
			}
			before := vm.KernelStats
			middle := tc.mutate + `boxProof.setFromBufferAttribute(attrProof);JSON.stringify([packed(boxProof),traceProof]);`
			got := archtimeJSON(t, vm, middle)
			want := archtimeNode(t, three+"\n"+archtimeSetup+middle)
			if got != want {
				t.Fatalf("fallback mismatch: %s / %s", got, want)
			}
			if vm.KernelStats.Accepted != before.Accepted {
				t.Fatal("mutated method accepted")
			}
			if tc.reject && vm.KernelStats.Rejected <= before.Rejected {
				t.Fatal("guard rejection unobserved")
			}
			// Deletion leaves an engine tombstone; reinstating an own ordinary method
			// restores a clean traversed shape without changing the function semantics.
			restore := tc.restore
			if tc.name == "method getter" {
				restore += `attrProof.getX=THREE.BufferAttribute.prototype.getX;`
			}
			got = archtimeJSON(t, vm, restore+archtimeCall)
			want = archtimeNode(t, three+"\n"+archtimeSetup+middle+";"+restore+archtimeCall)
			if got != want {
				t.Fatalf("recovery mismatch: %s / %s", got, want)
			}
			if vm.KernelStats.Accepted != before.Accepted+1 {
				t.Fatalf("restoration not reenabled %+v", vm.KernelStats)
			}
		})
	}
}

func TestArchtimeReadonlyExceptionResume(t *testing.T) {
	vm, three := archtimeThree(t)
	if _, err := archtimeRun(vm, archtimeSetup); err != nil {
		t.Fatal(err)
	}
	script := `Object.defineProperty(boxProof.max,'x',{writable:false});var thrown='';try{boxProof.setFromBufferAttribute(attrProof)}catch(e){thrown=e.name}JSON.stringify([thrown,packed(boxProof)]);`
	got := archtimeJSON(t, vm, script)
	want := archtimeNode(t, three+"\n"+archtimeSetup+script)
	generic, _ := archtimeThree(t)
	generic.DisableArchtimeGeometry = true
	baseline := archtimeJSON(t, generic, archtimeSetup+script)
	if got != baseline {
		t.Fatalf("bridge changed baseline exception: %s / %s", got, baseline)
	}
	if got != want {
		t.Errorf("BASELINE BLOCKER readonly exception parity: %s / Node %s", got, want)
	}
	if vm.KernelStats.Accepted != 1 || vm.KernelStats.Rejected != 1 {
		t.Fatalf("unsafe acceptance %+v", vm.KernelStats)
	}
	archtimeJSON(t, vm, `Object.defineProperty(boxProof.max,'x',{writable:true});`+archtimeCall)
	if vm.KernelStats.Accepted != 2 {
		t.Fatal("no recovery")
	}
}

func TestArchtimeMethodAccessorBaselineAndResume(t *testing.T) {
	vm, three := archtimeThree(t)
	if _, err := archtimeRun(vm, archtimeSetup); err != nil {
		t.Fatal(err)
	}
	script := `var save=THREE.BufferAttribute.prototype.getX;Object.defineProperty(attrProof,'getX',{configurable:true,get:function(){traceProof.push('getX');return save}});var thrown='';try{boxProof.setFromBufferAttribute(attrProof)}catch(e){thrown=e.name}JSON.stringify([thrown,packed(boxProof),traceProof]);`
	got := archtimeJSON(t, vm, script)
	generic, _ := archtimeThree(t)
	generic.DisableArchtimeGeometry = true
	baseline := archtimeJSON(t, generic, archtimeSetup+script)
	if got != baseline {
		t.Fatalf("bridge changed accessor fallback: %s / %s", got, baseline)
	}
	want := archtimeNode(t, three+"\n"+archtimeSetup+script)
	if got != want {
		t.Errorf("BASELINE BLOCKER method accessor parity: %s / Node %s", got, want)
	}
	if vm.KernelStats.Accepted != 1 || vm.KernelStats.Rejected != 1 {
		t.Fatal("accessor improperly accepted")
	}
	archtimeJSON(t, vm, `Object.defineProperty(attrProof,'getX',{value:save,writable:true,configurable:true,enumerable:true});`+archtimeCall)
	if vm.KernelStats.Accepted != 2 {
		t.Fatal("accessor restoration failed")
	}
}

func TestArchtimeBackendMutationResume(t *testing.T) {
	vm, _ := archtimeThree(t)
	if _, err := archtimeRun(vm, archtimeSetup); err != nil {
		t.Fatal(err)
	}
	attr, _ := vm.GetGlobal("attrProof")
	av, ok := vm.getOrdinaryDataSlot(vm.heap.Get(attr.Handle()), "array")
	if !ok {
		t.Fatal("array missing")
	}
	arr := vm.heap.Get(av.Handle())
	old := arr.byteOffset
	// Misalignment is host-only (JS Float32Array constructor rejects it).
	// Exercise the actual generic little-endian loads with a shortened count.
	arr.byteOffset = 1
	got := archtimeJSON(t, vm, archtimeCall)
	vm.DisableArchtimeGeometry = true
	want := archtimeJSON(t, vm, archtimeCall)
	if got != want {
		t.Errorf("invalid backing differs from generic: %s / %s", got, want)
	}
	vm.DisableArchtimeGeometry = false
	if vm.KernelStats.Accepted != 1 || vm.KernelStats.Rejected != 1 {
		t.Fatal("invalid backing admitted")
	}
	arr.byteOffset = old
	archtimeJSON(t, vm, archtimeCall)
	if vm.KernelStats.Accepted != 2 {
		t.Fatal("backend restoration failed")
	}
}

func TestArchtimeAliasAndDisable(t *testing.T) {
	vm, _ := archtimeThree(t)
	if _, err := archtimeRun(vm, archtimeSetup); err != nil {
		t.Fatal(err)
	}
	archtimeJSON(t, vm, `boxProof.anotherName=boxProof.setFromBufferAttribute;boxProof.anotherName(attrProof);JSON.stringify(packed(boxProof));`)
	if vm.KernelStats.Accepted != 2 {
		t.Fatal("same body alias rejected")
	}
	vm.DisableArchtimeGeometry = true
	archtimeJSON(t, vm, archtimeCall)
	if vm.KernelStats.Accepted != 2 {
		t.Fatal("disable ignored")
	}
	vm.DisableArchtimeGeometry = false
	archtimeJSON(t, vm, archtimeCall)
	if vm.KernelStats.Accepted != 3 {
		t.Fatal("enable not restored")
	}
}

func TestArchtimeNumericRejectionResume(t *testing.T) {
	for _, bad := range []string{"NaN", "Infinity", "-Infinity"} {
		t.Run(bad, func(t *testing.T) {
			vm, three := archtimeThree(t)
			if _, err := archtimeRun(vm, archtimeSetup); err != nil {
				t.Fatal(err)
			}
			src := `var savedCoordinate=attrProof.array[0];attrProof.array[0]=` + bad + `;` + archtimeCall
			got := archtimeJSON(t, vm, src)
			want := archtimeNode(t, three+"\n"+archtimeSetup+src)
			if got != want {
				t.Errorf("nonfinite fallback %s / %s", got, want)
			}
			if vm.KernelStats.Accepted != 1 || vm.KernelStats.Rejected != 1 {
				t.Fatal("nonfinite not rejected")
			}
			archtimeJSON(t, vm, `attrProof.array[0]=savedCoordinate;`+archtimeCall)
			if vm.KernelStats.Accepted != 2 {
				t.Fatal("finite restoration failed")
			}
		})
	}
}

func TestArchtimeAlignedOffsetAndGC(t *testing.T) {
	vm, three := archtimeThree(t)
	setup := archtimeSetup + `var source=attrProof.array;var raw=new ArrayBuffer(source.length*4+16);var windowProof=new Float32Array(raw,8,source.length);windowProof.set(source);attrProof.array=windowProof;`
	if _, err := archtimeRun(vm, setup); err != nil {
		t.Fatal(err)
	}
	// Both collectors run before each actual call; roots remain in VM globals.
	for i := 0; i < 8; i++ {
		vm.heap.SetStress(true)
		runtime.GC()
		vm.heap.Collect()
		got := archtimeJSON(t, vm, archtimeCall)
		if i == 0 {
			want := archtimeNode(t, three+"\n"+setup+archtimeCall)
			if got != want {
				t.Fatal("offset mismatch")
			}
		}
	}
	if vm.KernelStats.Accepted != 9 {
		t.Fatalf("offset fast path lost %+v", vm.KernelStats)
	}
}

func TestArchtimeGasCheckpointAndResume(t *testing.T) {
	vm, _ := archtimeThree(t)
	if _, err := archtimeRun(vm, archtimeSetup); err != nil {
		t.Fatal(err)
	}
	vm.GasLeft = 100
	_, err := archtimeRun(vm, archtimeCall)
	if !errors.Is(err, ErrGasExhausted) {
		t.Fatalf("expected gas exhaustion: %v", err)
	}
	if vm.KernelStats.Accepted != 1 || vm.KernelStats.Rejected == 0 {
		t.Fatalf("quota bypass %+v", vm.KernelStats)
	}
	vm.GasLeft = 1 << 30
	archtimeJSON(t, vm, archtimeCall)
	if vm.KernelStats.Accepted != 2 {
		t.Fatal("gas restoration did not complete")
	}
	stop := errors.New("checkpoint interruption")
	vm.CheckpointEvery = 20
	vm.OnCheckpoint = func() error { return stop }
	_, err = archtimeRun(vm, archtimeCall)
	if !errors.Is(err, stop) {
		t.Fatalf("checkpoint not delivered: %v", err)
	}
	vm.OnCheckpoint = nil
	vm.CheckpointEvery = 0
	vm.GasLeft = 1 << 30
	archtimeJSON(t, vm, archtimeCall)
	if vm.KernelStats.Accepted != 3 {
		t.Fatal("checkpoint restoration did not complete")
	}
	var interrupted atomic.Bool
	interrupted.Store(true)
	vm.Interrupted = &interrupted
	_, err = archtimeRun(vm, archtimeCall)
	if err == nil {
		t.Fatal("pending interruption ignored")
	}
	if vm.KernelStats.Accepted != 3 {
		t.Fatal("kernel ran while interrupted")
	}
	interrupted.Store(false)
	archtimeJSON(t, vm, archtimeCall)
	if vm.KernelStats.Accepted != 4 {
		t.Fatal("interrupt restoration did not complete")
	}
}
