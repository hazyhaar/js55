// SPDX-License-Identifier: BUSL-1.1
package engine_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hazyhaar/js55/pkg/js55"
	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

const (
	terrainPosPath = "/devhoros/GAFP/audits/comparatif-chromium-js55-20260909/mutations/input-terrain-positions"
	terrainIdxPath = "/devhoros/GAFP/audits/comparatif-chromium-js55-20260909/mutations/input-terrain-indices"
	terrainOracle  = "/devhoros/GAFP/audits/archtime-normals-20260910/oracle/node_normals.bin"
	terrainNverts  = 334400
	terrainNidx    = 1999098
	terrainNtris   = 666366
)

func loadThree(t *testing.T, iso *js55.Isolate) {
	t.Helper()
	b, err := os.ReadFile("/devhoros/GAFP/audits/comparatif-chromium-js55-20260909/mutations/input-three-source")
	if err != nil {
		t.Fatalf("read three: %v", err)
	}
	_, err = iso.EvalContext(context.Background(), string(b))
	if err != nil {
		t.Fatalf("eval three: %v", err)
	}
}

func smallGeomSetup() string {
	return `
var geom = new THREE.BufferGeometry();
var pos = new Float32Array(9);
pos[0]=0; pos[1]=0; pos[2]=0;
pos[3]=1; pos[4]=0; pos[5]=0;
pos[6]=0; pos[7]=1; pos[8]=0;
geom.setAttribute("position", new THREE.BufferAttribute(pos, 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array([0,1,2]), 1));
`
}

func smallNormalOracle() []float32 {
	return []float32{0, 0, 1, 0, 0, 1, 0, 0, 1}
}

func finiteNormals(t *testing.T, iso *js55.Isolate) {
	t.Helper()
	res, err := iso.EvalContext(context.Background(), `
(function(){
  var n = geom.getAttribute("normal");
  if (!n || !n.array || n.array.length < 3) return 0;
  for (var i=0;i<n.array.length;i++) if (!isFinite(n.array[i])) return 0;
  return n.array.length;
})()
`)
	if err != nil {
		t.Fatalf("normal inspect: %v", err)
	}
	if !res.IsNumber() || res.ToFloat() < 3 {
		t.Fatalf("normal attribute missing or non-finite: %v", res)
	}
}

func readNormalFloats(t *testing.T, iso *js55.Isolate) []float32 {
	t.Helper()
	res, err := iso.EvalContext(context.Background(), `geom.getAttribute("normal").array`)
	if err != nil {
		t.Fatalf("normal array: %v", err)
	}
	win, ok := iso.VM().TypedArrayByteWindow(res)
	if !ok || len(win) < 12 || len(win)%4 != 0 {
		t.Fatalf("normal window len=%d ok=%v", len(win), ok)
	}
	out := make([]float32, len(win)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(win[i*4:]))
	}
	return out
}

func requireNormalOracle(t *testing.T, iso *js55.Isolate, want []float32) {
	t.Helper()
	got := readNormalFloats(t, iso)
	if len(got) != len(want) {
		t.Fatalf("normal length %d want %d", len(got), len(want))
	}
	allZero := true
	for i := range got {
		if math.Float32bits(got[i]) != 0 {
			allZero = false
		}
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("normal[%d] bits %08x want %08x val=%g want=%g", i, math.Float32bits(got[i]), math.Float32bits(want[i]), got[i], want[i])
		}
	}
	if allZero {
		t.Fatal("normal bits are all zeros")
	}
}

func triangleNormalOracle(nverts int) []float32 {
	out := make([]float32, nverts*3)
	for i := 0; i < nverts; i++ {
		out[i*3+2] = 1
	}
	return out
}

func fillRepeatedTriangle(t *testing.T, iso *js55.Isolate) {
	t.Helper()
	posVal, err := iso.EvalContext(context.Background(), `geom.getAttribute("position").array`)
	if err != nil {
		t.Fatalf("position array: %v", err)
	}
	idxVal, err := iso.EvalContext(context.Background(), `geom.getIndex().array`)
	if err != nil {
		t.Fatalf("index array: %v", err)
	}
	posB, ok := iso.VM().TypedArrayByteWindow(posVal)
	if !ok {
		t.Fatal("position window")
	}
	idxB, ok := iso.VM().TypedArrayByteWindow(idxVal)
	if !ok {
		t.Fatal("index window")
	}
	tri := []float32{0, 0, 0, 1, 0, 0, 0, 1, 0}
	for i := 0; i+4 <= len(posB); i += 4 {
		f := tri[(i/4)%9]
		binary.LittleEndian.PutUint32(posB[i:i+4], math.Float32bits(f))
	}
	for i := 0; i+4 <= len(idxB); i += 4 {
		binary.LittleEndian.PutUint32(idxB[i:i+4], uint32(i/4))
	}
}

func kernelLinked(iso *js55.Isolate) bool {
	return !iso.VM().DisableArchtimeGeometry
}

func normalBytes(t *testing.T, iso *js55.Isolate) []byte {
	t.Helper()
	res, err := iso.EvalContext(context.Background(), `geom.getAttribute("normal").array`)
	if err != nil {
		t.Fatal(err)
	}
	win, ok := iso.VM().TypedArrayByteWindow(res)
	if !ok {
		t.Fatal("normal window")
	}
	out := make([]byte, len(win))
	copy(out, win)
	return out
}

func TestArchtimeNormalsON(t *testing.T) {
	iso, _ := js55.NewIsolate(js55.Config{GasLimit: 1e9})
	iso.VM().DisableArchtimeNormals = false
	loadThree(t, iso)
	if _, err := iso.EvalContext(context.Background(), smallGeomSetup()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	res, err := iso.EvalContext(context.Background(), "geom.computeVertexNormals();")
	if err != nil {
		t.Fatalf("fresh error: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	t.Logf("ON fresh Accepted=%d Rejected=%d DisableGeometry=%v", iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected, iso.VM().DisableArchtimeGeometry)
	if kernelLinked(iso) && iso.VM().KernelStats.Accepted != 1 {
		t.Fatalf("fresh case should be accepted")
	}
	finiteNormals(t, iso)
	requireNormalOracle(t, iso, smallNormalOracle())
	if kernelLinked(iso) {
		assertNeedsUpdate(t, iso, 1)
	}

	res, err = iso.EvalContext(context.Background(), "geom.computeVertexNormals();")
	if err != nil {
		t.Fatalf("oldnormal error: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	t.Logf("ON oldnormal Accepted=%d Rejected=%d", iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected)
	if kernelLinked(iso) && iso.VM().KernelStats.Accepted != 2 {
		t.Fatalf("oldnormal case should be accepted")
	}
	finiteNormals(t, iso)
	requireNormalOracle(t, iso, smallNormalOracle())
	if kernelLinked(iso) {
		assertNeedsUpdate(t, iso, 2)
	}
}

func TestArchtimeNormalsOFF(t *testing.T) {
	iso, _ := js55.NewIsolate(js55.Config{GasLimit: 1e9})
	iso.VM().DisableArchtimeNormals = true
	loadThree(t, iso)
	if _, err := iso.EvalContext(context.Background(), smallGeomSetup()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	res, err := iso.EvalContext(context.Background(), "geom.computeVertexNormals();")
	if err != nil {
		t.Fatalf("fresh generic: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	t.Logf("OFF fresh Accepted=%d Rejected=%d", iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected)
	if iso.VM().KernelStats.Accepted != 0 {
		t.Fatalf("kernel accepted while OFF")
	}
	finiteNormals(t, iso)
	requireNormalOracle(t, iso, smallNormalOracle())
}

func TestArchtimeNormalsInterruption(t *testing.T) {
	if !kernelLinked(mustIso(t)) {
		testArchtimeNormalsInterruptionOFF(t)
		return
	}
	testArchtimeNormalsInterruptionON(t)
}

func mustIso(t *testing.T) *js55.Isolate {
	t.Helper()
	iso, err := js55.NewIsolate(js55.Config{GasLimit: 1e9})
	if err != nil {
		t.Fatalf("isolate: %v", err)
	}
	return iso
}

func testArchtimeNormalsInterruptionON(t *testing.T) {
	pos, err := os.ReadFile(terrainPosPath)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := os.ReadFile(terrainIdxPath)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := os.ReadFile(terrainOracle)
	if err != nil {
		t.Fatal(err)
	}
	var armed bool
	var checkCount int
	iso, _ := js55.NewIsolate(js55.Config{
		GasLimit:        10e9,
		MaxMemoryBytes:  1536 * 1024 * 1024,
		CheckpointEvery: 2_000_000,
		HostNonMutating: true,
		OnCheckpoint: func() error {
			if !armed {
				return nil
			}
			checkCount++
			if checkCount == 1 {
				return fmt.Errorf("simulate interrupt")
			}
			return nil
		},
	})
	iso.VM().DisableArchtimeNormals = false
	loadThree(t, iso)
	setup := fmt.Sprintf(`
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array(%d), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array(%d), 1));
`, terrainNverts*3, terrainNidx)
	if _, err := iso.EvalContext(context.Background(), setup); err != nil {
		t.Fatalf("setup: %v", err)
	}
	posVal, err := iso.EvalContext(context.Background(), `geom.getAttribute("position").array`)
	if err != nil {
		t.Fatal(err)
	}
	idxVal, err := iso.EvalContext(context.Background(), `geom.getIndex().array`)
	if err != nil {
		t.Fatal(err)
	}
	posWin, ok := iso.VM().TypedArrayByteWindow(posVal)
	if !ok || len(posWin) != len(pos) {
		t.Fatalf("pos window %d", len(posWin))
	}
	idxWin, ok := iso.VM().TypedArrayByteWindow(idxVal)
	if !ok || len(idxWin) != len(idx) {
		t.Fatalf("idx window %d", len(idxWin))
	}
	copy(posWin, pos)
	copy(idxWin, idx)
	armed = true
	gas0 := iso.VM().GasLeft
	t0 := time.Now()
	_, err = iso.EvalContext(context.Background(), "geom.computeVertexNormals();")
	if err == nil || !strings.Contains(err.Error(), "simulate interrupt") {
		t.Fatalf("expected simulate interrupt error, got %v", err)
	}
	t.Logf("ON interrupt checkpoints=%d yieldcalls=%d Accepted=%d Rejected=%d gasCharged=%d elapsed=%s vertices=%d triangles=%d", checkCount, checkCount, iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected, gas0-iso.VM().GasLeft, time.Since(t0), terrainNverts, terrainNtris)
	if checkCount < 1 {
		t.Fatal("cadence 2M not exercised before interrupt")
	}

	t1 := time.Now()
	gas1 := iso.VM().GasLeft
	res, err := iso.EvalContext(context.Background(), "geom.computeVertexNormals();")
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	got := normalBytes(t, iso)
	if bytes.Equal(got, make([]byte, len(got))) {
		t.Fatal("resume normals all zeros")
	}
	if !bytes.Equal(got, oracle) {
		t.Fatalf("resume bits != Node oracle (len=%d)", len(got))
	}
	t.Logf("ON resume Accepted=%d Rejected=%d yieldcalls=%d gasCharged=%d elapsed=%s vertices=%d", iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected, checkCount, gas1-iso.VM().GasLeft, time.Since(t1), terrainNverts)
	if iso.VM().KernelStats.Accepted != 1 {
		t.Fatalf("should have accepted on resume, got %d. Rejected=%d", iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected)
	}
}

func testArchtimeNormalsInterruptionOFF(t *testing.T) {
	var checkCount int
	iso, _ := js55.NewIsolate(js55.Config{
		GasLimit:        10e9,
		MaxMemoryBytes:  1536 * 1024 * 1024,
		CheckpointEvery: 2_000_000,
		OnCheckpoint: func() error {
			checkCount++
			return nil
		},
	})
	loadThree(t, iso)
	if _, err := iso.EvalContext(context.Background(), smallGeomSetup()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if iso.VM().Interrupted == nil {
		t.Fatal("Interrupted is nil")
	}
	iso.VM().Interrupted.Store(true)
	_, err := iso.EvalContext(context.Background(), "geom.computeVertexNormals();")
	iso.VM().Interrupted.Store(false)
	if err == nil || !strings.Contains(err.Error(), "interrompue") {
		t.Fatalf("expected interrupt error, got %v", err)
	}
	t.Logf("OFF interrupt yieldcalls=%d Accepted=%d Rejected=%d err=%v", checkCount, iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected, err)

	t0 := time.Now()
	res, err := iso.EvalContext(context.Background(), "geom.computeVertexNormals();")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	t.Logf("OFF interruption Accepted=%d Rejected=%d yieldcalls=%d elapsed=%s", iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected, checkCount, time.Since(t0))
	if iso.VM().KernelStats.Accepted != 0 {
		t.Fatalf("OFF resume must not take the ON fast path")
	}
	finiteNormals(t, iso)
	requireNormalOracle(t, iso, smallNormalOracle())
}

func assertNeedsUpdate(t *testing.T, iso *js55.Isolate, wantVersion float64) {
	t.Helper()
	res, err := iso.EvalContext(context.Background(), `
(function(){
  var n = geom.getAttribute("normal");
  if (Object.prototype.hasOwnProperty.call(n, "needsUpdate")) return -1;
  return n.version;
})()
`)
	if err != nil {
		t.Fatalf("needsUpdate inspect: %v", err)
	}
	if !res.IsNumber() && !res.IsInt() {
		t.Fatalf("needsUpdate version not number: %v", res)
	}
	got := res.ToFloat()
	t.Logf("needsUpdate version=%g want=%g", got, wantVersion)
	if got == -1 {
		t.Fatal("needsUpdate became an own data slot")
	}
	if got != wantVersion {
		t.Fatalf("version %g want %g", got, wantVersion)
	}
}

func TestArchtimeNormalsON_Guards(t *testing.T) {
	if !kernelLinked(mustIso(t)) {
		t.Skip("kernel not linked")
	}
	iso, _ := js55.NewIsolate(js55.Config{GasLimit: 1e9})
	iso.VM().DisableArchtimeNormals = false
	loadThree(t, iso)

	cases := []struct {
		name, mutate, restore string
	}{
		{"ctor custom", `var saveF=Float32Array; Object.defineProperty(globalThis,"Float32Array",{configurable:true,writable:true,value:new Proxy(saveF,{})});`, `Object.defineProperty(globalThis,"Float32Array",{configurable:true,writable:true,value:saveF});`},
		{"proxy attributes", `var saveA=geom.attributes; geom.attributes=new Proxy(saveA,{});`, `geom.attributes=saveA;`},
		{"readonly version", `var n0=geom.getAttribute("position"); var saveDesc=Object.getOwnPropertyDescriptor(n0,"itemSize"); Object.defineProperty(n0,"itemSize",{writable:false,value:n0.itemSize});`, `Object.defineProperty(geom.getAttribute("position"),"itemSize",saveDesc);`},
		{"own setAttribute", `var saveS=geom.setAttribute; Object.defineProperty(geom,"setAttribute",{configurable:true,writable:true,value:function(k,v){return saveS.call(this,k,v);}});`, `delete geom.setAttribute;`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := iso.EvalContext(context.Background(), smallGeomSetup()); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if _, err := iso.EvalContext(context.Background(), tc.mutate); err != nil {
				t.Fatalf("mutate: %v", err)
			}
			beforeA := iso.VM().KernelStats.Accepted
			beforeR := iso.VM().KernelStats.Rejected
			if _, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`); err != nil {
				t.Logf("mutate compute err (generic may throw): %v", err)
			}
			t.Logf("%s after mutate Accepted=%d Rejected=%d (before A=%d R=%d)", tc.name, iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected, beforeA, beforeR)
			if iso.VM().KernelStats.Rejected <= beforeR {
				t.Fatalf("guard rejection unobserved")
			}
			afterA := iso.VM().KernelStats.Accepted
			if _, err := iso.EvalContext(context.Background(), tc.restore); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if _, err := iso.EvalContext(context.Background(), smallGeomSetup()); err != nil {
				t.Fatalf("setup2: %v", err)
			}
			res, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
			if err != nil {
				t.Fatalf("restored compute: %v", err)
			}
			if !res.IsUndefined() {
				t.Fatalf("expected undefined, got %v", res)
			}
			if iso.VM().KernelStats.Accepted != afterA+1 {
				t.Fatalf("restoration not reenabled Accepted=%d want %d", iso.VM().KernelStats.Accepted, afterA+1)
			}
			requireNormalOracle(t, iso, smallNormalOracle())
		})
	}
}

func TestArchtimeNormalsON_HostNonMutating(t *testing.T) {
	if !kernelLinked(mustIso(t)) {
		t.Skip("kernel not linked")
	}
	iso, _ := js55.NewIsolate(js55.Config{
		GasLimit:        1e9,
		CheckpointEvery: 2_000_000,
		OnCheckpoint:    func() error { return nil },
	})
	iso.VM().DisableArchtimeNormals = false
	loadThree(t, iso)
	if _, err := iso.EvalContext(context.Background(), smallGeomSetup()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	beforeR := iso.VM().KernelStats.Rejected
	beforeA := iso.VM().KernelStats.Accepted
	if _, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`); err != nil {
		t.Logf("generic after host-mutating reject: %v", err)
	}
	t.Logf("mutant host Accepted=%d Rejected=%d", iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected)
	if iso.VM().KernelStats.Rejected <= beforeR {
		t.Fatal("OnCheckpoint without HostNonMutating must reject before kernel write")
	}
	if iso.VM().KernelStats.Accepted != beforeA {
		t.Fatal("mutant host must not accept")
	}
	iso.VM().HostNonMutating = true
	if _, err := iso.EvalContext(context.Background(), smallGeomSetup()); err != nil {
		t.Fatalf("setup2: %v", err)
	}
	afterA := iso.VM().KernelStats.Accepted
	res, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	if err != nil {
		t.Fatalf("restored compute: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	if iso.VM().KernelStats.Accepted != afterA+1 {
		t.Fatalf("restoration not reenabled Accepted=%d want %d", iso.VM().KernelStats.Accepted, afterA+1)
	}
	requireNormalOracle(t, iso, smallNormalOracle())
}

func normalPresent(t *testing.T, iso *js55.Isolate) bool {
	t.Helper()
	res, err := iso.EvalContext(context.Background(), `(function(){ var n = geom.getAttribute("normal"); return n ? 1 : 0; })()`)
	if err != nil {
		t.Fatalf("normal present: %v", err)
	}
	return res.ToFloat() != 0
}

func mediumIndexedSetup(nverts, nidx int) string {
	return fmt.Sprintf(`
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array(%d), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array(%d), 1));
`, nverts*3, nidx)
}

func TestArchtimeNormalsON_B2RefuseBeforePublish(t *testing.T) {
	if !kernelLinked(mustIso(t)) {
		t.Skip("kernel not linked")
	}
	const nverts, nidx = 510, 510
	iso, err := js55.NewIsolate(js55.Config{GasLimit: 1e9})
	if err != nil {
		t.Fatal(err)
	}
	iso.VM().DisableArchtimeNormals = false
	loadThree(t, iso)
	if _, err := iso.EvalContext(context.Background(), mediumIndexedSetup(nverts, nidx)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	fillRepeatedTriangle(t, iso)
	if normalPresent(t, iso) {
		t.Fatal("setup must not precreate normal")
	}
	stack0 := iso.VM().Heap().StackLen()
	beforeA := iso.VM().KernelStats.Accepted
	beforeR := iso.VM().KernelStats.Rejected
	iso.VM().GasLeft = 128
	_, err = iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	iso.VM().GasLeft = 1e9
	if err == nil {
		t.Fatal("expected refusal with tiny gas budget")
	}
	t.Logf("B2 refuse err=%v Accepted=%d Rejected=%d present=%v stack=%d yielding=%v", err, iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected, normalPresent(t, iso), iso.VM().Heap().StackLen(), iso.VM().Yielding())
	if normalPresent(t, iso) {
		t.Fatal("attributes.normal must be absent after admission/ctor refusal")
	}
	if iso.VM().KernelStats.Accepted != beforeA {
		t.Fatalf("Accepted changed on refusal: %d", iso.VM().KernelStats.Accepted)
	}
	if iso.VM().Heap().StackLen() != stack0 {
		t.Fatalf("stack after refusal %d want %d", iso.VM().Heap().StackLen(), stack0)
	}
	if iso.VM().Yielding() {
		t.Fatal("yielding after refusal")
	}
	iso.VM().GasLeft = 1e9
	if normalPresent(t, iso) {
		t.Fatal("retry must not precreate normal")
	}
	res, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	if iso.VM().KernelStats.Accepted != beforeA+1 {
		t.Fatalf("retry Accepted=%d want %d Rejected=%d (was R=%d)", iso.VM().KernelStats.Accepted, beforeA+1, iso.VM().KernelStats.Rejected, beforeR)
	}
	requireNormalOracle(t, iso, triangleNormalOracle(nverts))
}

func TestArchtimeNormalsON_B2CtorThenNoPublish(t *testing.T) {
	if !kernelLinked(mustIso(t)) {
		t.Skip("kernel not linked")
	}
	const nverts, nidx = 1023, 1023
	iso, err := js55.NewIsolate(js55.Config{GasLimit: 1e9, MaxMemoryBytes: 64 * 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	iso.VM().DisableArchtimeNormals = false
	loadThree(t, iso)
	if _, err := iso.EvalContext(context.Background(), mediumIndexedSetup(nverts, nidx)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	fillRepeatedTriangle(t, iso)
	if normalPresent(t, iso) {
		t.Fatal("setup must not precreate normal")
	}
	h := iso.VM().Heap()
	orig := h.QuotaTracker
	var armed, sawLarge bool
	h.QuotaTracker = func(delta int64) error {
		if orig != nil {
			if err := orig(delta); err != nil {
				return err
			}
		}
		if armed && sawLarge && delta > 0 {
			return isolate.ErrMemoryLimitExceeded
		}
		if armed && delta >= 4096 {
			sawLarge = true
		}
		return nil
	}
	stack0 := iso.VM().Heap().StackLen()
	beforeA := iso.VM().KernelStats.Accepted
	armed = true
	_, err = iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	armed = false
	h.QuotaTracker = orig
	t.Logf("B2 ctor-quota err=%v Accepted=%d present=%v sawLarge=%v stack=%d yielding=%v", err, iso.VM().KernelStats.Accepted, normalPresent(t, iso), sawLarge, iso.VM().Heap().StackLen(), iso.VM().Yielding())
	if err == nil {
		t.Fatal("expected error after ctor allocation before publication")
	}
	if !sawLarge {
		t.Fatal("Float32Array backing was not allocated")
	}
	if normalPresent(t, iso) {
		t.Fatal("ctor may allocate but attributes.normal must stay unpublished")
	}
	if iso.VM().KernelStats.Accepted != beforeA {
		t.Fatalf("Accepted=%d want %d", iso.VM().KernelStats.Accepted, beforeA)
	}
	if iso.VM().Heap().StackLen() != stack0 {
		t.Fatalf("stack after ctor error %d want %d", iso.VM().Heap().StackLen(), stack0)
	}
	if iso.VM().Yielding() {
		t.Fatal("yielding after ctor error")
	}
	res, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	if iso.VM().KernelStats.Accepted != beforeA+1 {
		t.Fatalf("retry Accepted=%d want %d", iso.VM().KernelStats.Accepted, beforeA+1)
	}
	requireNormalOracle(t, iso, triangleNormalOracle(nverts))
}

func b3MutatingCompute(t *testing.T, disableNormals bool, nverts, nidx int, mutateBits uint32) (iso *js55.Isolate, posWin []byte, orig0 uint32, normals []byte, callbackCalls int, accepted, rejected uint64) {
	t.Helper()
	iso, err := js55.NewIsolate(js55.Config{
		GasLimit:        10e9,
		MaxMemoryBytes:  1536 * 1024 * 1024,
		CheckpointEvery: 2_000_000,
		OnCheckpoint: func() error {
			callbackCalls++
			if len(posWin) >= 4 {
				binary.LittleEndian.PutUint32(posWin[0:4], mutateBits)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	iso.VM().DisableArchtimeNormals = disableNormals
	loadThree(t, iso)
	if _, err := iso.EvalContext(context.Background(), mediumIndexedSetup(nverts, nidx)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	fillRepeatedTriangle(t, iso)
	if normalPresent(t, iso) {
		t.Fatal("setup must not precreate normal")
	}
	posVal, err := iso.EvalContext(context.Background(), `geom.getAttribute("position").array`)
	if err != nil {
		t.Fatal(err)
	}
	var ok bool
	posWin, ok = iso.VM().TypedArrayByteWindow(posVal)
	if !ok || len(posWin) < 4 {
		t.Fatal("position window")
	}
	orig0 = binary.LittleEndian.Uint32(posWin[0:4])
	if orig0 == mutateBits {
		t.Fatal("fixture already carries mutation sentinel")
	}
	iso.VM().GasLeft = 10e9
	if _, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`); err != nil {
		t.Logf("compute err: %v", err)
	}
	if callbackCalls < 1 {
		t.Fatalf("callbackCalls=%d want >= 1", callbackCalls)
	}
	got0 := binary.LittleEndian.Uint32(posWin[0:4])
	if got0 != mutateBits {
		t.Fatalf("position bytes not mutated: %08x want %08x calls=%d", got0, mutateBits, callbackCalls)
	}
	return iso, posWin, orig0, normalBytes(t, iso), callbackCalls, iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected
}

func TestArchtimeNormalsON_B3ViewMutation(t *testing.T) {
	if !kernelLinked(mustIso(t)) {
		t.Skip("kernel not linked")
	}
	const nverts, nidx = 60000, 60000
	mutateBits := math.Float32bits(99)
	iso, posWin, orig0, onNorm, onCalls, onA, onR := b3MutatingCompute(t, false, nverts, nidx, mutateBits)
	t.Logf("B3 mutator callbackCalls=%d Accepted=%d Rejected=%d nverts=%d", onCalls, onA, onR, nverts)
	if onR < 1 {
		t.Fatal("mutating callback without HostNonMutating must reject before native writes")
	}
	if onA != 0 {
		t.Fatal("mutating host must not accept")
	}
	_, _, _, offNorm, offCalls, offA, offR := b3MutatingCompute(t, true, nverts, nidx, mutateBits)
	t.Logf("B3 OFF oracle callbackCalls=%d Accepted=%d Rejected=%d", offCalls, offA, offR)
	if offA != 0 {
		t.Fatal("OFF oracle must not accept native")
	}
	if !bytes.Equal(onNorm, offNorm) {
		t.Fatalf("ON generic fallback != independent OFF oracle (onCalls=%d offCalls=%d onLen=%d offLen=%d)", onCalls, offCalls, len(onNorm), len(offNorm))
	}

	binary.LittleEndian.PutUint32(posWin[0:4], orig0)
	if _, err := iso.EvalContext(context.Background(), `delete geom.attributes.normal;`); err != nil {
		t.Fatalf("drop generic normal: %v", err)
	}
	if normalPresent(t, iso) {
		t.Fatal("restore must not precreate normal")
	}
	var callbackCallsPur int
	iso.VM().HostNonMutating = true
	iso.VM().OnCheckpoint = func() error {
		callbackCallsPur++
		return nil
	}
	iso.VM().DisableArchtimeNormals = false
	iso.VM().GasLeft = 10e9
	afterA := iso.VM().KernelStats.Accepted
	res, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	if err != nil {
		t.Fatalf("restored compute: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	if iso.VM().KernelStats.Accepted != afterA+1 {
		t.Fatalf("restoration not reenabled Accepted=%d want %d Rejected=%d", iso.VM().KernelStats.Accepted, afterA+1, iso.VM().KernelStats.Rejected)
	}
	if callbackCallsPur < 1 {
		t.Fatalf("callbackCallsPur=%d want >= 1", callbackCallsPur)
	}
	requireNormalOracle(t, iso, triangleNormalOracle(nverts))
	t.Logf("B3 same-iso restore callbackCallsPur=%d Accepted=%d", callbackCallsPur, iso.VM().KernelStats.Accepted)

	if _, err := iso.EvalContext(context.Background(), mediumIndexedSetup(nverts, nidx)); err != nil {
		t.Fatalf("fresh setup: %v", err)
	}
	fillRepeatedTriangle(t, iso)
	if normalPresent(t, iso) {
		t.Fatal("fresh 60k must not precreate normal")
	}
	iso.VM().GasLeft = 10e9
	freshA := iso.VM().KernelStats.Accepted
	purBefore := callbackCallsPur
	res, err = iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	if err != nil {
		t.Fatalf("fresh compute: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	if iso.VM().KernelStats.Accepted != freshA+1 {
		t.Fatalf("fresh Accepted=%d want %d", iso.VM().KernelStats.Accepted, freshA+1)
	}
	if callbackCallsPur <= purBefore {
		t.Fatalf("fresh callbackCallsPur=%d want > %d", callbackCallsPur, purBefore)
	}
	requireNormalOracle(t, iso, triangleNormalOracle(nverts))
	t.Logf("B3 same-iso fresh 60k callbackCallsPur=%d Accepted=%d", callbackCallsPur, iso.VM().KernelStats.Accepted)
}

func testArchtimeNormalsON_B3LieNonFinite(t *testing.T, pos, idx, oracle []byte, bits uint32) {
	t.Helper()
	var posWin []byte
	var armed bool
	var injected, checkCount int
	iso, err := js55.NewIsolate(js55.Config{
		GasLimit:        10e9,
		MaxMemoryBytes:  1536 * 1024 * 1024,
		CheckpointEvery: 2_000_000,
		HostNonMutating: true,
		OnCheckpoint: func() error {
			if !armed {
				return nil
			}
			checkCount++
			if injected == 0 && len(posWin) >= 4 {
				binary.LittleEndian.PutUint32(posWin[0:4], bits)
				injected++
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	iso.VM().DisableArchtimeNormals = false
	loadThree(t, iso)
	setup := fmt.Sprintf(`
var geom = new THREE.BufferGeometry();
geom.setAttribute("position", new THREE.BufferAttribute(new Float32Array(%d), 3));
geom.setIndex(new THREE.BufferAttribute(new Uint32Array(%d), 1));
`, terrainNverts*3, terrainNidx)
	if _, err := iso.EvalContext(context.Background(), setup); err != nil {
		t.Fatalf("setup: %v", err)
	}
	posVal, err := iso.EvalContext(context.Background(), `geom.getAttribute("position").array`)
	if err != nil {
		t.Fatal(err)
	}
	idxVal, err := iso.EvalContext(context.Background(), `geom.getIndex().array`)
	if err != nil {
		t.Fatal(err)
	}
	var ok bool
	posWin, ok = iso.VM().TypedArrayByteWindow(posVal)
	if !ok || len(posWin) != len(pos) {
		t.Fatalf("pos window %d", len(posWin))
	}
	idxWin, ok := iso.VM().TypedArrayByteWindow(idxVal)
	if !ok || len(idxWin) != len(idx) {
		t.Fatalf("idx window %d", len(idxWin))
	}
	copy(posWin, pos)
	copy(idxWin, idx)
	armed = true
	beforeA := iso.VM().KernelStats.Accepted
	_, err = iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	t.Logf("B3 lie checkpoints=%d injected=%d Accepted=%d Rejected=%d err=%v bits=%08x", checkCount, injected, iso.VM().KernelStats.Accepted, iso.VM().KernelStats.Rejected, err, bits)
	if checkCount < 1 {
		t.Fatal("cadence 2M not exercised before lie")
	}
	if injected != 1 {
		t.Fatal("non-finite was not injected into the view")
	}
	if err == nil {
		t.Fatal("lying callback must abort via Step guards")
	}
	if iso.VM().KernelStats.Accepted != beforeA {
		t.Fatal("lie must not accept, and must not fall back to generic")
	}
	iso.VM().GasLeft = 10e9
	posVal, err = iso.EvalContext(context.Background(), `geom.getAttribute("position").array`)
	if err != nil {
		t.Fatal(err)
	}
	posWin, ok = iso.VM().TypedArrayByteWindow(posVal)
	if !ok || len(posWin) != len(pos) {
		t.Fatalf("pos window after lie %d", len(posWin))
	}
	copy(posWin, pos)
	iso.VM().OnCheckpoint = func() error { return nil }
	afterA := iso.VM().KernelStats.Accepted
	t.Logf("B3 lie resume begin Accepted=%d", afterA)
	res, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
	if err != nil {
		t.Fatalf("resume after lie: %v", err)
	}
	if !res.IsUndefined() {
		t.Fatalf("expected undefined, got %v", res)
	}
	if iso.VM().KernelStats.Accepted != afterA+1 {
		t.Fatalf("resume Accepted=%d want %d Rejected=%d", iso.VM().KernelStats.Accepted, afterA+1, iso.VM().KernelStats.Rejected)
	}
	got := normalBytes(t, iso)
	if !bytes.Equal(got, oracle) {
		t.Fatalf("resume bits != Node oracle (len=%d)", len(got))
	}
}

func TestArchtimeNormalsON_B3LieNaN(t *testing.T) {
	// Finite non-NaN/Inf stores under a lying HostNonMutating contract can remain
	// undetected: Step guards only reject non-finite views. No general detection promise.
	if !kernelLinked(mustIso(t)) {
		t.Skip("kernel not linked")
	}
	pos, err := os.ReadFile(terrainPosPath)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := os.ReadFile(terrainIdxPath)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := os.ReadFile(terrainOracle)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		bits uint32
	}{
		{"NaN", math.Float32bits(float32(math.NaN()))},
		{"Inf", math.Float32bits(float32(math.Inf(1)))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testArchtimeNormalsON_B3LieNonFinite(t, pos, idx, oracle, tc.bits)
		})
	}
}
