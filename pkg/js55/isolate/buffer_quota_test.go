package isolate

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestBufferQuotaRejectThenRecover(t *testing.T) {
	iso, err := New(Config{MaxMemoryBytes: 8 << 20, GasLimit: 10000000})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, src := range []string{`new ArrayBuffer(1e12)`, `new Float32Array(1e12)`, `new Array(1000000000)`} {
		before := iso.AllocatedMemory()
		_, err = iso.Eval(ctx, src)
		if !errors.Is(err, ErrMemoryLimitExceeded) {
			t.Fatalf("%s: %v", src, err)
		}
		if delta := iso.AllocatedMemory() - before; delta > 65536 {
			t.Fatalf("rejected allocation charged %d", delta)
		}
		v, e := iso.Eval(ctx, `var small=new Float32Array([1,2]);var bytes=new Uint8Array(small.buffer);bytes[2]=0;bytes[3]=64;small[0]===2&&small[1]===2`)
		if e != nil || iso.VM().ToStringValue(v).GoString() != "true" {
			t.Fatalf("recovery: %v %v", v, e)
		}
	}
	v, err := iso.Eval(ctx, `var large=new Float32Array(1003200);large[1003199]=1.337;large.length===1003200&&large.byteLength===4012800&&large[1003199]===1.3370000123977661`)
	if err != nil || iso.VM().ToStringValue(v).GoString() != "true" {
		t.Fatalf("large allocation under quota: %v %v", v, err)
	}
	t.Logf("tracked=%d quota=%d", iso.AllocatedMemory(), 8<<20)
}

func TestBufferResizeQuotaRejectThenRecover(t *testing.T) {
	iso, err := New(Config{MaxMemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = iso.Eval(ctx, `var b=new ArrayBuffer(16,{maxByteLength:2000000});var a=new Float32Array(b);a[0]=1.337;b.resize(2000000)`)
	if !errors.Is(err, ErrMemoryLimitExceeded) {
		t.Fatalf("resize: %v", err)
	}
	v, err := iso.Eval(ctx, `var ok=b.byteLength===16&&a[0]===1.3370000123977661;b.resize(32);a[7]=9;ok&&a.length===8&&a[7]===9&&a[0]===1.3370000123977661`)
	if err != nil || iso.VM().ToStringValue(v).GoString() != "true" {
		t.Fatalf("resize recovery: %v %v", v, err)
	}
}

func TestArrayHostileLengthRejectThenRecover(t *testing.T) {
	iso, err := New(Config{MaxMemoryBytes: 8 << 20, GasLimit: 10000000})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, src := range []string{`new Array(2e18)`, `new Array(Infinity)`, `new Array(-1)`, `new Array(2.5)`, `new Array(NaN)`} {
		before := iso.AllocatedMemory()
		_, err = iso.Eval(ctx, src)
		if err == nil {
			t.Fatalf("%s: accepted", src)
		}
		if iso.AllocatedMemory() < 0 {
			t.Fatalf("%s: quota counter negative", src)
		}
		if delta := iso.AllocatedMemory() - before; delta > 65536 {
			t.Fatalf("%s: rejected allocation charged %d", src, delta)
		}
		v, e := iso.Eval(ctx, `var a=new Array(8);a[7]=1;a.length===8&&a[7]===1`)
		if e != nil || iso.VM().ToStringValue(v).GoString() != "true" {
			t.Fatalf("%s recovery: %v %v", src, v, e)
		}
	}
}

func TestBufferResizePeakQuotaRejectThenRecover(t *testing.T) {
	const quota int64 = 1 << 20
	iso, err := New(Config{MaxMemoryBytes: quota, GasLimit: 10000000})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = iso.Eval(ctx, `1`); err != nil {
		t.Fatal(err)
	}
	base := iso.AllocatedMemory()
	remain := quota - base
	old := int(remain * 2 / 3)
	if old < 4096 {
		t.Fatalf("remaining quota too small for peak fixture: base=%d", base)
	}
	maxLen := old * 2
	if int64(maxLen) > quota {
		maxLen = int(quota)
	}
	_, err = iso.Eval(ctx, fmt.Sprintf(`var b=new ArrayBuffer(%d,{maxByteLength:%d});var a=new Uint8Array(b);a[0]=7;a[%d]=9`, old, maxLen, old-1))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	used := iso.AllocatedMemory()
	newSize := old + 1
	if newSize > maxLen {
		t.Fatalf("newSize %d > maxByteLength %d", newSize, maxLen)
	}
	peak := used + int64(newSize)
	final := used + int64(newSize) - int64(old)
	if peak <= quota {
		t.Fatalf("fixture peak fits: used=%d old=%d new=%d peak=%d quota=%d", used, old, newSize, peak, quota)
	}
	if final > quota {
		t.Fatalf("fixture final exceeds: final=%d quota=%d", final, quota)
	}
	_, err = iso.Eval(ctx, fmt.Sprintf(`b.resize(%d)`, newSize))
	if !errors.Is(err, ErrMemoryLimitExceeded) {
		t.Fatalf("peak resize(%d): %v used=%d peak=%d final=%d", newSize, err, used, peak, final)
	}
	if iso.AllocatedMemory() < 0 {
		t.Fatal("quota counter negative")
	}
	if delta := iso.AllocatedMemory() - used; delta > 65536 {
		t.Fatalf("rejected peak charged %d", delta)
	}
	v, err := iso.Eval(ctx, fmt.Sprintf(`var ok=b.byteLength===%d&&a[0]===7&&a[%d]===9;b.resize(32);a[31]=3;ok&&b.byteLength===32&&a.length===32&&a[0]===7&&a[31]===3`, old, old-1))
	if err != nil || iso.VM().ToStringValue(v).GoString() != "true" {
		t.Fatalf("peak resize recovery: %v %v", v, err)
	}
}

func TestQuotaFreeNonNegativeThenRecover(t *testing.T) {
	iso, err := New(Config{MaxMemoryBytes: 8 << 20})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = iso.Eval(ctx, `var a=new Uint8Array(64); a[0]=7`); err != nil {
		t.Fatal(err)
	}
	used := iso.AllocatedMemory()
	if used <= 0 {
		t.Fatalf("expected allocation, got %d", used)
	}
	iso.Heap().TrackFree(used + 4096)
	if iso.AllocatedMemory() < 0 {
		t.Fatalf("quota counter negative after over-free: %d", iso.AllocatedMemory())
	}
	v, err := iso.Eval(ctx, `var b=new Uint8Array(8); b[7]=9; b[7]===9`)
	if err != nil || iso.VM().ToStringValue(v).GoString() != "true" {
		t.Fatalf("recovery: %v %v", v, err)
	}
	if iso.AllocatedMemory() < 0 {
		t.Fatal("quota counter negative after recovery")
	}
}

func TestQuotaMaxInt64Overflow(t *testing.T) {
	iso, err := New(Config{MaxMemoryBytes: 8 << 20})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = iso.Eval(ctx, `new ArrayBuffer(9223372036854775807)`)
	if err == nil {
		t.Fatal("accepted extreme size")
	}
	if iso.AllocatedMemory() < 0 {
		t.Fatal("quota counter negative due to overflow")
	}
	v, err := iso.Eval(ctx, `var a=new Uint8Array(2);a[1]=42;a[1]===42`)
	if err != nil || iso.VM().ToStringValue(v).GoString() != "true" {
		t.Fatalf("recovery: %v %v", v, err)
	}
}
