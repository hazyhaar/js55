//go:build archtime_geometry

package engine

import (
	"math"
	"os"
	"testing"

	"github.com/hazyhaar/c2pkg/c2archtsim/geometry"
)

func loadThreeVM(t *testing.T) *VM {
	t.Helper()
	b, err := os.ReadFile("/devhoros/GAFP/audits/comparatif-chromium-js55-20260909/mutations/input-three-source")
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(NewHeap())
	vm.DisableArchtimeNormals = false
	if _, err := archtimeRun(vm, string(b)); err != nil {
		t.Fatal(err)
	}
	return vm
}

func lexicalTn(t *testing.T, vm *VM) [3]float64 {
	t.Helper()
	three, ok := vm.GetGlobal("THREE")
	if !ok {
		t.Fatal("THREE")
	}
	bg, err := vm.GetProperty(three, "BufferGeometry")
	if err != nil {
		t.Fatal(err)
	}
	proto, err := vm.GetProperty(bg, "prototype")
	if err != nil {
		t.Fatal(err)
	}
	nn, err := vm.GetProperty(proto, "normalizeNormals")
	if err != nil || !nn.IsObject() {
		t.Fatalf("normalizeNormals fn: %v %v", nn, err)
	}
	fnObj := vm.heap.Get(nn.Handle())
	if fnObj == nil || fnObj.fn == nil {
		t.Fatal("normalizeNormals chunk")
	}
	envObj := vm.heap.Get(fnObj.env)
	d, slot, okCap := scanCallMethodCapture(fnObj.fn, "fromBufferAttribute")
	if !okCap {
		t.Fatal("Tn bytecode")
	}
	tnVal, okCap := vm.capturedLocal(envObj, d, slot)
	if !okCap || !tnVal.IsObject() {
		t.Fatalf("Tn capture d=%d slot=%d", d, slot)
	}
	tnObj := vm.heap.Get(tnVal.Handle())
	xv, okX := vm.getOrdinaryDataSlot(tnObj, "x")
	yv, okY := vm.getOrdinaryDataSlot(tnObj, "y")
	zv, okZ := vm.getOrdinaryDataSlot(tnObj, "z")
	if !okX || !okY || !okZ {
		t.Fatal("Tn xyz slots")
	}
	return [3]float64{xv.ToFloat(), yv.ToFloat(), zv.ToFloat()}
}

func cLastNormalized(t *testing.T, nor []float32) [3]float64 {
	t.Helper()
	var st geometry.NormState
	if !geometry.NormalizeBegin(nor, &st) {
		t.Fatal("NormalizeBegin")
	}
	if _, ok := st.LastNormalized(); ok {
		t.Fatal("Begin must leave ok=false")
	}
	done, ok := geometry.NormalizeStep(nor, &st, uint64(len(nor)))
	if !ok || !done {
		t.Fatalf("NormalizeStep done=%v ok=%v", done, ok)
	}
	v, ok := st.LastNormalized()
	if !ok {
		t.Fatal("LastNormalized after normalize")
	}
	return v
}

func assertBits(t *testing.T, label string, got, want [3]float64) {
	t.Helper()
	for i := 0; i < 3; i++ {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s [%d] got=%016x want=%016x", label, i, math.Float64bits(got[i]), math.Float64bits(want[i]))
		}
	}
}

func f32Recast(v [3]float64) [3]float64 {
	return [3]float64{float64(float32(v[0])), float64(float32(v[1])), float64(float32(v[2]))}
}

func TestArchtimeNormalsON_TnDouble(t *testing.T) {
	b, err := os.ReadFile("/devhoros/GAFP/audits/comparatif-chromium-js55-20260909/mutations/input-three-source")
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(NewHeap())
	vm.DisableArchtimeNormals = false
	if _, err := archtimeRun(vm, string(b)); err != nil {
		t.Fatal(err)
	}
	src := `
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array([0,0,0,1,0,0,0,1,0]), 3));
geom.setAttribute("normal", new THREE.BufferAttribute(new Float32Array([1,1,1, 2,0,0, 1,1,1]), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array([0,1,2]), 1));
geom.normalizeNormals();
`
	if _, err := archtimeRun(vm, src); err != nil {
		t.Fatalf("normalizeNormals: %v", err)
	}
	t.Logf("normalizeNormals Accepted=%d Rejected=%d", vm.KernelStats.Accepted, vm.KernelStats.Rejected)
	if vm.KernelStats.Accepted < 1 {
		t.Fatalf("normalizeNormals not accepted")
	}
	three, ok := vm.GetGlobal("THREE")
	if !ok {
		t.Fatal("THREE")
	}
	bg, err := vm.GetProperty(three, "BufferGeometry")
	if err != nil {
		t.Fatal(err)
	}
	proto, err := vm.GetProperty(bg, "prototype")
	if err != nil {
		t.Fatal(err)
	}
	nn, err := vm.GetProperty(proto, "normalizeNormals")
	if err != nil || !nn.IsObject() {
		t.Fatalf("normalizeNormals fn: %v %v", nn, err)
	}
	fnObj := vm.heap.Get(nn.Handle())
	if fnObj == nil || fnObj.fn == nil {
		t.Fatal("normalizeNormals chunk")
	}
	envObj := vm.heap.Get(fnObj.env)
	d, slot, okCap := scanCallMethodCapture(fnObj.fn, "fromBufferAttribute")
	if !okCap {
		t.Fatal("Tn bytecode")
	}
	tnVal, okCap := vm.capturedLocal(envObj, d, slot)
	if !okCap || !tnVal.IsObject() {
		t.Fatalf("Tn capture d=%d slot=%d", d, slot)
	}
	tnObj := vm.heap.Get(tnVal.Handle())
	xv, okX := vm.getOrdinaryDataSlot(tnObj, "x")
	yv, okY := vm.getOrdinaryDataSlot(tnObj, "y")
	zv, okZ := vm.getOrdinaryDataSlot(tnObj, "z")
	if !okX || !okY || !okZ {
		t.Fatal("Tn xyz slots")
	}
	got := [3]float64{xv.ToFloat(), yv.ToFloat(), zv.ToFloat()}
	inv := 1.0 / math.Sqrt(3)
	want := [3]float64{inv, inv, inv}
	f32 := [3]float64{float64(float32(want[0])), float64(float32(want[1])), float64(float32(want[2]))}
	t.Logf("Tn=%v double=%v f32recast=%v", got, want, f32)
	if got != want {
		t.Fatalf("Tn lexical %v want double %v (not f32 recast %v)", got, want, f32)
	}
	cWant := cLastNormalized(t, []float32{1, 1, 1, 2, 0, 0, 1, 1, 1})
	assertBits(t, "Tn vs C LastNormalized", got, cWant)
	if math.Float64bits(got[0]) == math.Float64bits(f32[0]) {
		t.Fatal("Tn must diverge from float64(f32 recast)")
	}
}

func TestArchtimeNormalsON_B1ComputeTn(t *testing.T) {
	vm := loadThreeVM(t)
	src := `
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array([0,0,0, 1,1,0, 0,1,1]), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array([0,1,2]), 1));
geom.computeVertexNormals();
`
	if _, err := archtimeRun(vm, src); err != nil {
		t.Fatalf("computeVertexNormals: %v", err)
	}
	if vm.KernelStats.Accepted < 1 {
		t.Fatalf("compute not accepted Accepted=%d Rejected=%d", vm.KernelStats.Accepted, vm.KernelStats.Rejected)
	}
	got := lexicalTn(t, vm)
	inv := 1.0 / math.Sqrt(3)
	want := [3]float64{inv, -inv, inv}
	f32 := f32Recast(want)
	t.Logf("compute Tn=%v double=%v f32recast=%v", got, want, f32)
	assertBits(t, "compute Tn", got, want)
	if math.Float64bits(got[0]) == math.Float64bits(f32[0]) {
		t.Fatal("compute Tn must diverge from float64(f32 recast)")
	}
}

func TestArchtimeNormalsON_B1PartialFinal(t *testing.T) {
	vm := loadThreeVM(t)
	vm.HostNonMutating = true
	vm.CheckpointEvery = 1
	var partial [3]float64
	var sawPartial bool
	vm.OnCheckpoint = func() error {
		got := lexicalTn(t, vm)
		if !sawPartial && (got[0] != 0 || got[1] != 0 || got[2] != 0) {
			partial = got
			sawPartial = true
		}
		return nil
	}
	src := `
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array([0,0,0,1,0,0,0,1,0,1,1,0,0,1,1,1,0,1]), 3));
geom.setAttribute("normal", new THREE.BufferAttribute(new Float32Array([1,1,1, 2,0,0, 0,3,0, 4,4,4, 1,0,1, 0,0,5]), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array([0,1,2, 3,4,5]), 1));
geom.normalizeNormals();
`
	if _, err := archtimeRun(vm, src); err != nil {
		t.Fatalf("normalizeNormals: %v", err)
	}
	if vm.KernelStats.Accepted < 1 {
		t.Fatalf("not accepted Accepted=%d Rejected=%d", vm.KernelStats.Accepted, vm.KernelStats.Rejected)
	}
	if !sawPartial {
		t.Fatal("callback never observed a partial Tn before yield")
	}
	final := lexicalTn(t, vm)
	first := cLastNormalized(t, []float32{1, 1, 1})
	last := cLastNormalized(t, []float32{0, 0, 5})
	t.Logf("partial=%v final=%v firstC=%v lastC=%v", partial, final, first, last)
	assertBits(t, "partial Tn", partial, first)
	assertBits(t, "final Tn", final, last)
	if math.Float64bits(partial[0]) == math.Float64bits(final[0]) &&
		math.Float64bits(partial[1]) == math.Float64bits(final[1]) &&
		math.Float64bits(partial[2]) == math.Float64bits(final[2]) {
		t.Fatal("partial Tn must differ from final Tn")
	}
}

func TestArchtimeNormalsON_B1CallbackObservesTn(t *testing.T) {
	vm := loadThreeVM(t)
	vm.HostNonMutating = true
	vm.CheckpointEvery = 1
	var observed [3]float64
	var n int
	vm.OnCheckpoint = func() error {
		got := lexicalTn(t, vm)
		if n == 0 && (got[0] != 0 || got[1] != 0 || got[2] != 0) {
			observed = got
			n++
		}
		return nil
	}
	src := `
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array([0,0,0,1,0,0,0,1,0]), 3));
geom.setAttribute("normal", new THREE.BufferAttribute(new Float32Array([1,1,1, 2,0,0, 0,0,4]), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array([0,1,2]), 1));
geom.normalizeNormals();
`
	if _, err := archtimeRun(vm, src); err != nil {
		t.Fatalf("normalizeNormals: %v", err)
	}
	if vm.KernelStats.Accepted < 1 {
		t.Fatalf("not accepted Accepted=%d Rejected=%d", vm.KernelStats.Accepted, vm.KernelStats.Rejected)
	}
	if n < 1 {
		t.Fatal("readonly callback did not observe Tn before yield")
	}
	want := cLastNormalized(t, []float32{1, 1, 1})
	assertBits(t, "callback Tn before yield", observed, want)
	f32 := f32Recast(want)
	if math.Float64bits(observed[0]) == math.Float64bits(f32[0]) {
		t.Fatal("callback Tn must be C double, not f32 recast")
	}
}

func TestArchtimeNormalsON_B1NoInventBeforeNorm(t *testing.T) {
	vm := loadThreeVM(t)
	vm.HostNonMutating = true
	vm.CheckpointEvery = 1
	var sawNonZero bool
	var firstNonZeroPhase int
	var ck int
	vm.OnCheckpoint = func() error {
		ck++
		got := lexicalTn(t, vm)
		if !sawNonZero && (got[0] != 0 || got[1] != 0 || got[2] != 0) {
			sawNonZero = true
			firstNonZeroPhase = ck
		}
		return nil
	}
	src := `
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array([0,0,0, 1,1,0, 0,1,1]), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array([0,1,2]), 1));
geom.computeVertexNormals();
`
	if _, err := archtimeRun(vm, src); err != nil {
		t.Fatalf("computeVertexNormals: %v", err)
	}
	if vm.KernelStats.Accepted < 1 {
		t.Fatalf("not accepted Accepted=%d Rejected=%d", vm.KernelStats.Accepted, vm.KernelStats.Rejected)
	}
	if ck < 2 {
		t.Fatalf("expected Zero/Accum checkpoints before Norm, got %d", ck)
	}
	if !sawNonZero {
		t.Fatal("Tn never written after a normalizing Step")
	}
	if firstNonZeroPhase <= 1 {
		t.Fatalf("Tn invented before first normalize checkpoint=%d", firstNonZeroPhase)
	}
	got := lexicalTn(t, vm)
	inv := 1.0 / math.Sqrt(3)
	assertBits(t, "final compute Tn", got, [3]float64{inv, -inv, inv})
}

func TestArchtimeNormalsON_B1SignedZero(t *testing.T) {
	vm := loadThreeVM(t)
	src := `
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array([0,0,0,1,0,0,0,1,0]), 3));
geom.setAttribute("normal", new THREE.BufferAttribute(new Float32Array([1,0,0, 0,1,0, -0,0,0]), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array([0,1,2]), 1));
geom.normalizeNormals();
`
	if _, err := archtimeRun(vm, src); err != nil {
		t.Fatalf("normalizeNormals: %v", err)
	}
	if vm.KernelStats.Accepted < 1 {
		t.Fatalf("not accepted Accepted=%d Rejected=%d", vm.KernelStats.Accepted, vm.KernelStats.Rejected)
	}
	got := lexicalTn(t, vm)
	if math.Float64bits(got[0]) != 0x8000000000000000 {
		t.Fatalf("signed zero x bits=%016x want 8000000000000000 Tn=%v", math.Float64bits(got[0]), got)
	}
}
