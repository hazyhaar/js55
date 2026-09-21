package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
	"github.com/hazyhaar/js55/pkg/js55/str"
)

// Language vectors, not training data: these exercise the specified conversions
// and compare both nominal and hostile operations in one lifecycle.
func TestTypedBackingSemantics(t *testing.T) {
	cases := map[string]string{
		"metadata":                 `var a=new Float32Array([1,2]); a.length===2 && a.byteLength===8 && a.byteOffset===0 && a.buffer instanceof ArrayBuffer && new Float32Array(a.buffer).length===2`,
		"IEEE_roundtrip":           `var f=new Float32Array([1]);var u=new Uint8Array(f.buffer);var ok=u[0]===0&&u[1]===0&&u[2]===128&&u[3]===63;u[2]=0;u[3]=64;ok&&f[0]===2`,
		"rounding":                 `var a=new Float32Array([1.337,16777217,-0,Infinity,NaN]);a[0]===1.3370000123977661&&a[1]===16777216&&1/a[2]===-Infinity&&a[3]===Infinity&&a[4]!==a[4]`,
		"uint8_conversion":         `var a=new Uint8Array([-1,257,3.9,NaN,Infinity]);a[0]===255&&a[1]===1&&a[2]===3&&a[3]===0&&a[4]===0`,
		"subarray_shared":          `var a=new Float32Array([1,2,3,4]);var b=a.subarray(-3,-1);b[0]=9;var c=b.subarray(1);c[0]=8;a[1]===9&&a[2]===8&&b.length===2&&b.buffer===a.buffer&&b.byteOffset===4&&c.byteOffset===8&&a.subarray(3,1).length===0`,
		"overlap_forward_backward": `var a=new Float32Array([1,2,3,4]);a.set(a.subarray(0,3),1);var ok=a[0]===1&&a[1]===1&&a[2]===2&&a[3]===3;a.set(a.subarray(1),0);ok&&a[0]===1&&a[1]===2&&a[2]===3&&a[3]===3`,
		"mixed_overlap":            `var b=new ArrayBuffer(8);var f=new Float32Array(b);f[0]=1;f[1]=2;var u=new Uint8Array(b);u.set(f,2);u[2]===1&&u[3]===2&&u[7]===64`,
		"bounds_recovery":          `var b=new ArrayBuffer(16);var a=new Float32Array(b,4,2);var n=0;try{new Float32Array(b,1)}catch(e){if(e instanceof RangeError)n++}try{new Float32Array(b,12,2)}catch(e){if(e instanceof RangeError)n++}try{a.set([1,2],1)}catch(e){if(e instanceof RangeError)n++}a[2]=99;a.set([7,8]);n===3&&a.length===2&&a[2]===undefined&&a[0]===7&&a[1]===8&&new Float32Array(b)[3]===0`,
		"invalid_lengths_recovery": `var n=0;try{new ArrayBuffer(-1)}catch(e){if(e instanceof RangeError)n++}try{new Float32Array(Infinity)}catch(e){if(e instanceof RangeError)n++}try{new ArrayBuffer(8,{maxByteLength:4})}catch(e){if(e instanceof RangeError)n++}var a=new Float32Array(2.9);a[1]=7;n===3&&a.length===2&&a[1]===7`,
		"array_length_recovery":    `var n=0;try{new Array(2e18)}catch(e){if(e instanceof RangeError)n++}try{new Array(Infinity)}catch(e){if(e instanceof RangeError)n++}try{new Array(-1)}catch(e){if(e instanceof RangeError)n++}try{new Array(2.5)}catch(e){if(e instanceof RangeError)n++}try{new Array(NaN)}catch(e){if(e instanceof RangeError)n++}var a=new Array(4);a[3]=7;n===5&&a.length===4&&a[3]===7`,
		"resize_recovery":          `var b=new ArrayBuffer(16,{maxByteLength:32});var a=new Float32Array(b);var fixed=new Float32Array(b,8,2);a[0]=1;a[3]=9;b.resize(4);var ok=a.length===1&&fixed.length===0&&fixed.byteLength===0;b.resize(16);fixed[0]=7;ok&&fixed.length===2&&a[0]===1&&a[2]===7&&a[3]===0`,
		"typed_copy":               `var a=new Float32Array([1.337,2]);var b=new Float32Array(a);b[1]=8;a[1]===2&&b[0]===1.3370000123977661&&b.buffer!==a.buffer`,
		"metadata_not_storage":     `var a=new Float32Array([1,2]);a.byteOffset=4;a.length=99;a.buffer=new ArrayBuffer(0);a.BYTES_PER_ELEMENT=1;a[0]=9;a.length===2&&a.byteOffset===0&&a.byteLength===8&&new Float32Array(a.buffer)[0]===9`,
		"other_numeric_types":      `var b=new ArrayBuffer(8);var i=new Int32Array(b);var u=new Uint32Array(b);i[0]=-1;var f=new Float64Array(b);var ok=u[0]===4294967295;f[0]=1.337;ok&&f[0]===1.337`,
	}
	for name, src := range cases {
		for _, stress := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stress=%v", name, stress), func(t *testing.T) {
				got, err := compileAndRun(t, src, stress)
				if err != nil || got != "true" {
					t.Fatalf("got %q err=%v", got, err)
				}
			})
		}
	}
}

func TestTypedBackingLargeLengths(t *testing.T) {
	for _, src := range []string{
		`var a=new Float32Array(1003200);a[1003199]=1.337;a.length===1003200&&a.byteLength===4012800&&a[1003199]===1.3370000123977661`,
		`var a=new Array(1003200);a[1003199]=7;a.length===1003200&&a[1003199]===7`,
		`var a=[];for(var i=0;i<10001;i++)a.push(i+0.5);var b=new Float32Array(a);b.length===10001&&b[10000]===10000.5`,
		`var a=new Float32Array({length:10001,10000:7});a.length===10001&&a[10000]===7`,
		`new ArrayBuffer(1000001).byteLength===1000001`,
	} {
		got, err := compileAndRun(t, src, false)
		if err != nil || got != "true" {
			t.Fatalf("%s: got %q err=%v", src, got, err)
		}
	}
}

func TestTypedBackingRealLidar(t *testing.T) {
	const path = "/devhoros/GAFP/data/lidar/meta/view3d/terrain.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var terrain struct {
		Positions []float64 `json:"positions"`
		Indices   []uint32  `json:"indices"`
	}
	if err = json.Unmarshal(data, &terrain); err != nil {
		t.Fatal(err)
	}
	if len(terrain.Positions) != 1003200 {
		t.Fatalf("unexpected terrain length %d", len(terrain.Positions))
	}
	t.Logf("source=%s sha256=%x positions=%d indices=%d", path, sha256.Sum256(data), len(terrain.Positions), len(terrain.Indices))
	h := NewHeap()
	vm := NewVM(h)
	vm.GasLeft = 200000000
	var used int64
	h.QuotaTracker = func(n int64) error {
		if n > 0 && used+n > 64<<20 {
			return fmt.Errorf("test quota exceeded")
		}
		used += n
		return nil
	}
	p := ObjectValue(h.NewArray(len(terrain.Positions)))
	vm.SetGlobal("lidarPositions", p)
	for i, x := range terrain.Positions {
		h.SetElement(p.Handle(), i, Number(x))
	}
	run := func(src string) Value {
		t.Helper()
		prog, e := parser.Parse(src, parser.Options{})
		if e != nil {
			t.Fatal(e)
		}
		ch, e := Compile(h, prog, "lidar")
		if e != nil {
			t.Fatal(e)
		}
		v, e := vm.Run(ch)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	v := run(`var lidarFloats=new Float32Array(lidarPositions);var lidarBytes=new Uint8Array(lidarFloats.buffer);lidarFloats`)
	want := make([]byte, len(terrain.Positions)*4)
	for i, x := range terrain.Positions {
		binary.NativeEndian.PutUint32(want[i*4:], math.Float32bits(float32(x)))
	}
	readBytes := func() []byte {
		u, _ := vm.GetGlobal("lidarBytes")
		if vm.arrayLikeLength(u) != len(want) {
			t.Fatalf("byte view length=%d want=%d", vm.arrayLikeLength(u), len(want))
		}
		out := make([]byte, len(want))
		for i := range out {
			x, e := vm.typedArrayIndex(u, i)
			if e != nil {
				t.Fatal(e)
			}
			out[i] = byte(vm.toNumber(x))
		}
		return out
	}
	if !bytes.Equal(readBytes(), want) {
		t.Fatal("full LiDAR IEEE byte differential failed")
	}
	for i, x := range terrain.Positions {
		got, e := vm.typedArrayIndex(v, i)
		if e != nil || math.Float32bits(float32(vm.toNumber(got))) != math.Float32bits(float32(x)) {
			t.Fatalf("position %d: %v %v", i, got, e)
		}
	}
	// Mutate the whole store through Uint8 using bytes from other real samples.
	run(`for(var i=0;i<lidarBytes.length;i+=4){var x=lidarBytes[i];lidarBytes[i]=lidarBytes[i+1];lidarBytes[i+1]=x;}lidarFloats`)
	for i := 0; i < len(want); i += 4 {
		want[i], want[i+1] = want[i+1], want[i]
	}
	if !bytes.Equal(readBytes(), want) {
		t.Fatal("Uint8 mutation did not reach backing store")
	}
	for i := range terrain.Positions {
		got, e := vm.typedArrayIndex(v, i)
		expected := math.Float32frombits(binary.NativeEndian.Uint32(want[i*4:]))
		if e != nil || (!math.IsNaN(float64(expected)) && math.Float32bits(float32(vm.toNumber(got))) != math.Float32bits(expected)) {
			t.Fatalf("mutated position %d", i)
		}
	}
	// Retain only a subarray across a real collection, then write and reread.
	run(`var retained=lidarFloats.subarray(10000);lidarFloats=null;lidarBytes=null;lidarPositions=null;retained`)
	h.Collect()
	got := run(`retained[0]=1.337;retained[0]`)
	if vm.toNumber(got) != float64(float32(1.337)) {
		t.Fatal("view did not retain buffer")
	}
	t.Logf("verified=%d components, bytes=%d, tracked_after_collection=%d", len(terrain.Positions), len(want), used)
}

func TestTypedBackingHostWindow(t *testing.T) {
	h := NewHeap()
	vm := NewVM(h)
	prog, err := parser.Parse(`var a=new Float32Array([1,2]);a`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := Compile(h, prog, "host")
	if err != nil {
		t.Fatal(err)
	}
	v, err := vm.Run(ch)
	if err != nil {
		t.Fatal(err)
	}
	win, ok := vm.TypedArrayByteWindow(v)
	if !ok || len(win) != 8 {
		t.Fatalf("window ok=%v len=%d", ok, len(win))
	}
	want := make([]byte, 8)
	binary.NativeEndian.PutUint32(want[0:], math.Float32bits(1))
	binary.NativeEndian.PutUint32(want[4:], math.Float32bits(2))
	if !bytes.Equal(win, want) {
		t.Fatalf("host window %v want %v", win, want)
	}
	buf := vm.heap.ArrayBufferBytes(v.Handle())
	if buf != nil {
		t.Fatal("typed view is not the ArrayBuffer")
	}
	ab := vm.getProp(v, str.FromGo("buffer"))
	buf = vm.heap.ArrayBufferBytes(ab.Handle())
	if len(buf) != 8 || &buf[0] != &win[0] {
		t.Fatal("ArrayBufferBytes did not alias the view window")
	}
	binary.NativeEndian.PutUint32(win[:4], math.Float32bits(4))
	got, err := vm.typedArrayIndex(v, 0)
	if err != nil || vm.toNumber(got) != 4 {
		t.Fatalf("host write not visible: %v %v", got, err)
	}
	repl := make([]byte, 8)
	binary.NativeEndian.PutUint32(repl[0:], math.Float32bits(8))
	binary.NativeEndian.PutUint32(repl[4:], math.Float32bits(9))
	if !vm.heap.ReplaceArrayBufferBytes(ab.Handle(), repl) {
		t.Fatal("replace rejected exact-length payload")
	}
	if vm.heap.ReplaceArrayBufferBytes(ab.Handle(), repl[:4]) {
		t.Fatal("replace silently accepted a short payload")
	}
	if vm.heap.ReplaceArrayBufferBytes(ab.Handle(), append(repl, 0)) {
		t.Fatal("replace silently accepted a long payload")
	}
	got, err = vm.typedArrayIndex(v, 0)
	if err != nil || vm.toNumber(got) != 8 {
		t.Fatalf("short reject mutated view: %v %v", got, err)
	}
	got, err = vm.typedArrayIndex(v, 1)
	if err != nil || vm.toNumber(got) != 9 {
		t.Fatalf("replace not visible: %v %v", got, err)
	}
	repl2 := make([]byte, 8)
	binary.NativeEndian.PutUint32(repl2[0:], math.Float32bits(16))
	binary.NativeEndian.PutUint32(repl2[4:], math.Float32bits(17))
	if !vm.heap.ReplaceArrayBufferBytes(ab.Handle(), repl2) {
		t.Fatal("replace rejected exact-length payload after bad-length rejection")
	}
	got, err = vm.typedArrayIndex(v, 0)
	if err != nil || vm.toNumber(got) != 16 {
		t.Fatalf("post-reject replace [0]: %v %v", got, err)
	}
	got, err = vm.typedArrayIndex(v, 1)
	if err != nil || vm.toNumber(got) != 17 {
		t.Fatalf("post-reject replace [1]: %v %v", got, err)
	}
	if !bytes.Equal(win, repl2) {
		t.Fatalf("post-reject window %v want %v", win, repl2)
	}
}

func TestHeapArraySizeRejectThenRecover(t *testing.T) {
	h := NewHeap()
	var used int64
	const quota int64 = 1 << 20
	h.QuotaTracker = func(delta int64) error {
		next := used + delta
		if next < 0 {
			return fmt.Errorf("quota counter negative: used=%d delta=%d", used, delta)
		}
		if delta > 0 && next > quota {
			return fmt.Errorf("quota")
		}
		used = next
		return nil
	}
	tryNew := func(n int) (panicked bool) {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		h.NewArray(n)
		return false
	}
	trySet := func(obj Handle, i int) (panicked bool) {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		h.SetElement(obj, i, Undefined)
		return false
	}
	before := used
	if !tryNew(int(2e18)) {
		t.Fatal("NewArray(2e18) accepted")
	}
	if used < 0 || used < before {
		t.Fatalf("quota after overflow NewArray: %d (before %d)", used, before)
	}
	a := h.NewArray(4)
	h.SetElement(a, 3, Int(7))
	if h.GetElement(a, 3).ToInt() != 7 {
		t.Fatal("nominal NewArray/SetElement")
	}
	before = used
	if !trySet(a, math.MaxInt) {
		t.Fatal("SetElement(MaxInt) accepted")
	}
	if used < 0 || used < before {
		t.Fatalf("quota after SetElement MaxInt: %d (before %d)", used, before)
	}
	h.SetElement(a, 1, Int(9))
	if h.GetElement(a, 1).ToInt() != 9 || h.GetElement(a, 3).ToInt() != 7 {
		t.Fatal("SetElement recovery")
	}
	if int64(math.MaxInt) > math.MaxInt64/8 {
		before = used
		if !trySet(a, math.MaxInt64/8+2) {
			t.Fatal("SetElement growth overflow accepted")
		}
		if used < 0 || used < before {
			t.Fatalf("quota after growth overflow: %d", used)
		}
		h.SetElement(a, 2, Int(3))
		if h.GetElement(a, 2).ToInt() != 3 || h.GetElement(a, 3).ToInt() != 7 {
			t.Fatal("growth recovery")
		}
	}
}
