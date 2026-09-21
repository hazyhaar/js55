//go:build archtime_geometry

package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hazyhaar/c2pkg/c2archtsim/geometry"
)

const terrainPositionsPath = "/devhoros/GAFP/audits/js55-perf-20260909/native/pos.bin"
const terrainProgram = `
var boxProof=new THREE.Box3().setFromBufferAttribute(attrProof);
geometryProof.computeBoundingSphere();
JSON.stringify([packed(boxProof),[geometryProof.boundingSphere.center.x,geometryProof.boundingSphere.center.y,geometryProof.boundingSphere.center.z,geometryProof.boundingSphere.radius].map(String)]);`

func loadArchtimeTerrain(t *testing.T) (*VM, []byte) {
	t.Helper()
	vm, _ := archtimeThree(t)
	data, err := os.ReadFile(terrainPositionsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 334400*12 {
		t.Fatalf("actual full terrain length %d", len(data))
	}
	src := archtimeEncode + `var attrProof=new THREE.BufferAttribute(new Float32Array(1003200),3);var geometryProof=new THREE.BufferGeometry();geometryProof.setAttribute('position',attrProof);`
	if _, err = archtimeRun(vm, src); err != nil {
		t.Fatal(err)
	}
	attr, _ := vm.GetGlobal("attrProof")
	arr, ok := vm.getOrdinaryDataSlot(vm.heap.Get(attr.Handle()), "array")
	if !ok {
		t.Fatal("missing positions")
	}
	window, ok := vm.TypedArrayByteWindow(arr)
	if !ok || len(window) != len(data) {
		t.Fatal("wrong input storage")
	}
	// Input transfer only. No bounds or sphere result is injected or cached.
	copy(window, data)
	return vm, data
}

func TestArchtimeFullTerrain334400(t *testing.T) {
	lock, err := os.OpenFile("/devhoros/GAFP/audits/js55-perf-20260909/campaign.lock", os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	// Node consumes exactly the same raw little-endian Float32 positions.
	// Use vm.runInNewContext so the original expression's completion value is
	// obtained without rewriting Box3 or the sphere method.
	src := `const fs=require('fs');const THREE=require(` + fmt.Sprintf("%q", threePath) + `);const b=fs.readFileSync(` + fmt.Sprintf("%q", terrainPositionsPath) + `);const a=new Float32Array(b.buffer,b.byteOffset,b.length/4);const c={THREE,attrProof:new THREE.BufferAttribute(a,3)};c.geometryProof=new THREE.BufferGeometry();c.geometryProof.setAttribute('position',c.attrProof);process.stdout.write(require('vm').runInNewContext(` + fmt.Sprintf("%q", archtimeEncode+terrainProgram) + `,c));`
	cmd := exec.Command("node", "-e", src)
	oracle, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Node: %v %s", err, oracle)
	}
	var measurements []map[string]any
	for _, checkpoint := range []int64{0, 2_000_000} {
		for _, disable := range []bool{true, false} {
			vm, data := loadArchtimeTerrain(t)
			vm.DisableArchtimeGeometry = disable
			vm.GasLeft = 1 << 34
			vm.CheckpointEvery = checkpoint
			checkpoints := 0
			if checkpoint > 0 {
				vm.OnCheckpoint = func() error { checkpoints++; return nil }
			}
			start := time.Now()
			got := archtimeJSON(t, vm, terrainProgram)
			elapsed := time.Since(start)
			if !bytes.Equal([]byte(got), oracle) {
				t.Fatalf("334400 Node mismatch checkpoint=%d disabled=%t: %s / %s", checkpoint, disable, got, oracle)
			}
			want := uint64(0)
			if !disable && checkpoint == 0 {
				want = 2
			}
			if vm.KernelStats.Accepted != want {
				t.Fatalf("full-terrain calls: %+v want %d", vm.KernelStats, want)
			}
			if checkpoint > 0 && checkpoints == 0 {
				t.Fatal("checkpoint cadence not exercised")
			}
			if !disable && checkpoint > 0 && vm.KernelStats.Rejected != 2 {
				t.Fatal("full-terrain fallback not observed")
			}
			m := map[string]any{"vertices": 334400, "position_sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "disabled": disable, "checkpoint_every": checkpoint, "callbacks": checkpoints, "accepted": vm.KernelStats.Accepted, "rejected": vm.KernelStats.Rejected, "elapsed_ns": elapsed.Nanoseconds(), "remaining_gas": vm.GasLeft, "node_parity": true}
			measurements = append(measurements, m)
			t.Logf("terrain: %v", m)
		}
	}
	if dest := os.Getenv("ARCHTIME_TERRAIN_REPORT"); dest != "" {
		b, _ := json.MarshalIndent(measurements, "", "  ")
		if err = os.WriteFile(dest, append(b, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestArchtimeAtomicPublicationInterruptResume(t *testing.T) {
	vm, _ := archtimeThree(t)
	if _, err := archtimeRun(vm, archtimeSetup); err != nil {
		t.Fatal(err)
	}
	bv, _ := vm.GetGlobal("boxProof")
	b := vm.heap.Get(bv.Handle())
	minv, _ := vm.getOrdinaryDataSlot(b, "min")
	maxv, _ := vm.getOrdinaryDataSlot(b, "max")
	mino, maxo := vm.heap.Get(minv.Handle()), vm.heap.Get(maxv.Handle())
	// Execute the real kernel on a real Three-generated geometry, then interrupt
	// exactly at the production publication boundary (no timing race or fake kernel).
	av, _ := vm.GetGlobal("attrProof")
	arrv, _ := vm.getOrdinaryDataSlot(vm.heap.Get(av.Handle()), "array")
	raw, ok := vm.TypedArrayByteWindow(arrv)
	if !ok {
		t.Fatal("no window")
	}
	points := make([]float32, len(raw)/4)
	for i := range points {
		v, err := vm.typedArrayIndex(arrv, i)
		if err != nil {
			t.Fatal(err)
		}
		points[i] = float32(v.ToFloat())
	}
	var out [6]float64
	if !geometry.BoundsXYZ(points, &out) {
		t.Fatal("actual kernel rejected")
	}
	vm.setVector3SlotsUnsafe(mino, 77, 88, 99)
	vm.setVector3SlotsUnsafe(maxo, 111, 222, 333)
	beforeMin := append([]Value(nil), mino.slots...)
	beforeMax := append([]Value(nil), maxo.slots...)
	var stop atomic.Bool
	stop.Store(true)
	vm.Interrupted = &stop
	if err := vm.commitArchtimeBounds(mino, maxo, &out); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected atomic abort: %v", err)
	}
	for i, v := range beforeMin {
		if mino.slots[i] != v {
			t.Fatal("partial min write")
		}
	}
	for i, v := range beforeMax {
		if maxo.slots[i] != v {
			t.Fatal("partial max write")
		}
	}
	stop.Store(false)
	if err := vm.commitArchtimeBounds(mino, maxo, &out); err != nil {
		t.Fatal(err)
	}
	archtimeJSON(t, vm, archtimeCall)
	if vm.KernelStats.Accepted != 2 {
		t.Fatal("nominal restart after abort failed")
	}
}

func TestArchtimeCheckpointMutationAndReadmission(t *testing.T) {
	for _, mutateCode := range []bool{false, true} {
		t.Run(fmt.Sprintf("bytecode=%t", mutateCode), func(t *testing.T) {
			var results []string
			for _, disable := range []bool{true, false} {
				vm, _ := archtimeThree(t)
				if _, err := archtimeRun(vm, archtimeSetup+`function overrideProof(i){return 999}`); err != nil {
					t.Fatal(err)
				}
				attr, _ := vm.GetGlobal("attrProof")
				override, _ := vm.GetGlobal("overrideProof")
				getter, err := vm.GetProperty(attr, "getX")
				if err != nil {
					t.Fatal(err)
				}
				original := vm.heap.Get(getter.Handle()).fn
				replacement := vm.heap.Get(override.Handle()).fn
				code, constants := original.Code, original.Consts
				vm.DisableArchtimeGeometry = disable
				vm.GasLeft = 1 << 30
				vm.CheckpointEvery = 1
				mutated := false
				vm.OnCheckpoint = func() error {
					if !mutated {
						if mutateCode {
							original.Code = replacement.Code
							original.Consts = replacement.Consts
						} else {
							vm.heap.SetProperty(attr.Handle(), vm.heap.intern.InternGo("getX"), override)
						}
						mutated = true
					}
					return nil
				}
				got := archtimeJSON(t, vm, archtimeCall)
				results = append(results, got)
				if !mutated || vm.KernelStats.Accepted != 1 {
					t.Fatal("callback mutation was bypassed")
				}
				vm.OnCheckpoint = nil
				vm.CheckpointEvery = 0
				original.Code = code
				original.Consts = constants
				if !mutateCode {
					vm.heap.SetProperty(attr.Handle(), vm.heap.intern.InternGo("getX"), getter)
				}
				vm.DisableArchtimeGeometry = false
				archtimeJSON(t, vm, archtimeCall)
				if vm.KernelStats.Accepted != 2 {
					t.Fatal("callback restoration not admitted")
				}
			}
			if results[0] != results[1] {
				t.Fatalf("callback generic differential: %v", results)
			}
		})
	}
}

func TestArchtimeTerrainGasAbortAndResume(t *testing.T) {
	vm, _ := loadArchtimeTerrain(t)
	if _, err := archtimeRun(vm, `var boxProof=new THREE.Box3();boxProof.min.set(77,88,99);boxProof.max.set(111,222,333);`); err != nil {
		t.Fatal(err)
	}
	vm.GasLeft = 100
	if _, err := archtimeRun(vm, archtimeCall); !errors.Is(err, ErrGasExhausted) {
		t.Fatalf("expected terrain gas exhaustion: %v", err)
	}
	if vm.KernelStats.Accepted != 0 || vm.KernelStats.Rejected != 1 {
		t.Fatalf("terrain budget bypass: %+v", vm.KernelStats)
	}
	vm.GasLeft = 1 << 34
	preserved := archtimeJSON(t, vm, `JSON.stringify(packed(boxProof));`)
	if preserved != `["77","88","99","111","222","333"]` {
		t.Fatalf("prefix published: %s", preserved)
	}
	archtimeJSON(t, vm, archtimeCall)
	if vm.KernelStats.Accepted != 1 {
		t.Fatal("full terrain gas restoration failed")
	}
}
