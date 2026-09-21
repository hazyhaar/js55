//go:build linux

package engine

import (
	"os"
	"runtime"
	"syscall"
	"testing"
)

func TestFrameActualAllocationDelta(t *testing.T) {
	// A campaign can opt into its shared lock without making the library's
	// unit tests depend on a particular application's audit directory.
	if lockPath := os.Getenv("JS55_BENCH_LOCK"); lockPath != "" {
		lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
			t.Fatal(err)
		}
		defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	}
	type sample struct {
		allocs         float64
		mallocs, bytes uint64
		charged        int64
		collections    int
	}
	measure := func(generic bool) sample {
		vm, drive := frameSetup(t, `function numeric(a,b,c){return a+b*c;}function drive(){var sum=0;for(var i=0;i<128;i++)sum+=numeric(i,2,3);return sum;}drive;`)
		f, _ := vm.GetGlobal("numeric")
		c := vm.heap.MustGet(f.Handle()).fn
		if !c.SafeEnv {
			t.Fatal("numeric body did not qualify")
		}
		if generic {
			c.SafeEnv = false
		}
		call := func() {
			v, e := vm.CallFunction(drive, Undefined, nil)
			if e != nil || v.ToInt() != 8896 {
				t.Fatalf("real JS result: %v %v", v, e)
			}
		}
		for i := 0; i < 5; i++ {
			call()
		}
		allocs := testing.AllocsPerRun(10, call)
		var charged int64
		vm.heap.QuotaTracker = func(n int64) error {
			if n > 0 {
				charged += n
			}
			return nil
		}
		collections := vm.heap.Collections
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for i := 0; i < 20; i++ {
			call()
		}
		runtime.ReadMemStats(&after)
		return sample{allocs, after.Mallocs - before.Mallocs, after.TotalAlloc - before.TotalAlloc, charged, vm.heap.Collections - collections}
	}
	generic, pooled := measure(true), measure(false)
	t.Logf("20 calls of precompiled drive, 128 real JS calls each; generic=%+v pooled=%+v; allocs is per drive after warmup", generic, pooled)
	if pooled.allocs >= generic.allocs {
		t.Fatalf("no allocation reduction: generic=%v pooled=%v", generic.allocs, pooled.allocs)
	}
	if generic.charged < 20*128*24 || pooled.charged != 0 {
		t.Fatalf("environment allocation accounting: generic=%d pooled=%d", generic.charged, pooled.charged)
	}
}
