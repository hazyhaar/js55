// SPDX-License-Identifier: BUSL-1.1
//go:build archtime_geometry

package engine_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hazyhaar/js55/pkg/js55"
)

func TestArchtimeNormalsON_Terrain(t *testing.T) {
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
	if len(pos) != terrainNverts*12 || len(idx) != terrainNidx*4 || len(oracle) != terrainNverts*12 {
		t.Fatalf("sizes pos=%d idx=%d oracle=%d", len(pos), len(idx), len(oracle))
	}
	sum := sha256.Sum256(oracle)
	if fmt.Sprintf("%x", sum) != "8314e6ec78b36f2e24b10a6e6c044b05ccf962f9f8e5cf32e7c5f0e34826feec" {
		t.Fatalf("oracle sha %x", sum)
	}

	var yieldcalls int
	iso, err := js55.NewIsolate(js55.Config{
		GasLimit:        10e9,
		MaxMemoryBytes:  1536 * 1024 * 1024,
		CheckpointEvery: 2_000_000,
		HostNonMutating: true,
		OnCheckpoint: func() error {
			yieldcalls++
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

	t.Run("once", func(t *testing.T) {
		gas0 := iso.VM().GasLeft
		t0 := time.Now()
		res, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
		elapsed := time.Since(t0)
		if err != nil {
			t.Fatalf("fresh: %v", err)
		}
		if !res.IsUndefined() {
			t.Fatalf("expected undefined, got %v", res)
		}
		got := normalBytes(t, iso)
		if bytes.Equal(got, make([]byte, len(got))) {
			t.Fatal("fresh normals all zeros")
		}
		if !bytes.Equal(got, oracle) {
			t.Fatalf("fresh bits != Node oracle (len=%d)", len(got))
		}
		t.Logf("terrain once hits=%d triangles=%d vertices=%d gasCharged=%d yieldcalls=%d elapsed=%s Accepted=%d", iso.VM().KernelStats.Accepted, terrainNtris, terrainNverts, gas0-iso.VM().GasLeft, yieldcalls, elapsed, iso.VM().KernelStats.Accepted)
		if yieldcalls < 1 {
			t.Fatalf("cadence 2M not exercised: yieldcalls=%d", yieldcalls)
		}
		if iso.VM().KernelStats.Accepted != 1 {
			t.Fatalf("fresh Accepted=%d", iso.VM().KernelStats.Accepted)
		}
	})

	t.Run("oldnormal", func(t *testing.T) {
		gas0 := iso.VM().GasLeft
		t0 := time.Now()
		res, err := iso.EvalContext(context.Background(), `geom.computeVertexNormals();`)
		elapsed := time.Since(t0)
		if err != nil {
			t.Fatalf("oldnormal: %v", err)
		}
		if !res.IsUndefined() {
			t.Fatalf("expected undefined, got %v", res)
		}
		got := normalBytes(t, iso)
		if !bytes.Equal(got, oracle) {
			t.Fatalf("oldnormal bits != Node oracle")
		}
		t.Logf("terrain oldnormal hits=%d triangles=%d vertices=%d gasCharged=%d yieldcalls=%d elapsed=%s Accepted=%d", iso.VM().KernelStats.Accepted, terrainNtris, terrainNverts, gas0-iso.VM().GasLeft, yieldcalls, elapsed, iso.VM().KernelStats.Accepted)
		if iso.VM().KernelStats.Accepted != 2 {
			t.Fatalf("oldnormal Accepted=%d", iso.VM().KernelStats.Accepted)
		}
	})
}
