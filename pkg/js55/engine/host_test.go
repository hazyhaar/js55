package engine

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func TestHostSequenceRejectAndRecover(t *testing.T) {
	h := NewHeap()
	vm := NewVM(h)
	p, err := parser.Parse(`var a=new Uint8Array([3,7,11]); a`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Compile(h, p, "test.js")
	if err != nil {
		t.Fatal(err)
	}
	v, err := vm.Run(c)
	if err != nil {
		t.Fatal(err)
	}
	h.AddRoot(&v)
	defer h.RemoveRoot(&v)

	name, byteLen, offset, ok := vm.TypedArrayInfo(v)
	if !ok || name != "Uint8Array" || byteLen != 3 || offset != 0 {
		t.Fatalf("TypedArrayInfo: name=%s byteLen=%d offset=%d ok=%v", name, byteLen, offset, ok)
	}
	win, ok := vm.TypedArrayByteWindow(v)
	if !ok || len(win) != 3 {
		t.Fatalf("window ok=%v len=%d", ok, len(win))
	}
	if win[0] != 3 || win[1] != 7 || win[2] != 11 {
		t.Fatalf("initial window %v", win)
	}

	ab, err := vm.GetProperty(v, "buffer")
	if err != nil || !ab.IsObject() {
		t.Fatalf("buffer: %v %v", ab, err)
	}
	buf := h.ArrayBufferBytes(ab.Handle())
	if len(buf) != 3 || &buf[0] != &win[0] {
		t.Fatal("TypedArrayByteWindow does not alias ArrayBufferBytes")
	}

	if h.ReplaceArrayBufferBytes(ab.Handle(), []byte{1}) {
		t.Fatal("short payload accepted")
	}
	if h.ReplaceArrayBufferBytes(ab.Handle(), []byte{1, 2, 3, 4}) {
		t.Fatal("long payload accepted")
	}
	if win[0] != 3 || win[1] != 7 || win[2] != 11 {
		t.Fatalf("rejection mutated storage %v", win)
	}

	if !h.ReplaceArrayBufferBytes(ab.Handle(), []byte{1, 8, 12}) {
		t.Fatal("exact-length payload rejected")
	}
	win2, ok := vm.TypedArrayByteWindow(v)
	if !ok || win2[0] != 1 || win2[1] != 8 || win2[2] != 12 {
		t.Fatalf("nominal replace not visible on window %v ok=%v", win2, ok)
	}

	vm.SetGlobal("a", v)
	prog, err := parser.Parse(`a[0]===1 && a[1]===8 && a[2]===12 && a.byteLength===3 && a.byteOffset===0`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := Compile(h, prog, "read.js")
	if err != nil {
		t.Fatal(err)
	}
	got, err := vm.Run(ch)
	if err != nil || vm.ToStringValue(got).GoString() != "true" {
		t.Fatalf("JS view read: %v %v", got, err)
	}

	prog, err = parser.Parse(`a.subarray(1)`, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ch, err = Compile(h, prog, "sub.js")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := vm.Run(ch)
	if err != nil {
		t.Fatal(err)
	}
	h.AddRoot(&sub)
	defer h.RemoveRoot(&sub)
	sName, sLen, sOff, sOK := vm.TypedArrayInfo(sub)
	if !sOK || sName != "Uint8Array" || sLen != 2 || sOff != 1 {
		t.Fatalf("subarray info name=%s byteLen=%d offset=%d ok=%v", sName, sLen, sOff, sOK)
	}
	subWin, ok := vm.TypedArrayByteWindow(sub)
	if !ok || len(subWin) != 2 {
		t.Fatalf("subarray window ok=%v len=%d", ok, len(subWin))
	}
	if &subWin[0] != &buf[1] {
		t.Fatal("subarray window does not alias buffer at byteOffset")
	}
	if subWin[0] != 8 || subWin[1] != 12 {
		t.Fatalf("subarray bytes %v", subWin)
	}
}
