// SPDX-License-Identifier: BUSL-1.1

package isolate

import (
	"context"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/engine"
)

func TestRunInContextSandboxPersists(t *testing.T) {
	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	h := iso.Heap()
	sb := engine.ObjectValue(h.NewObject())
	h.SetProperty(sb.Handle(), h.Intern().InternGo("a"), engine.Int(1))
	ctx := context.Background()
	v, err := iso.RunInContext(ctx, `a + 1`, sb)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToInt() != 2 {
		t.Fatalf("a+1 = %v", v)
	}
	_, err = iso.RunInContext(ctx, `a = 5; b = 6; this.c = 7`)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := h.GetOwnProperty(sb.Handle(), h.Intern().InternGo("a")); !ok || got.ToInt() != 5 {
		t.Fatalf("sandbox.a = %v ok=%v", got, ok)
	}
	if got, ok := h.GetOwnProperty(sb.Handle(), h.Intern().InternGo("b")); !ok || got.ToInt() != 6 {
		t.Fatalf("sandbox.b = %v ok=%v", got, ok)
	}
	if got, ok := h.GetOwnProperty(sb.Handle(), h.Intern().InternGo("c")); !ok || got.ToInt() != 7 {
		t.Fatalf("sandbox.c = %v ok=%v", got, ok)
	}
}

func TestRunInContextThisIsSandbox(t *testing.T) {
	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	h := iso.Heap()
	sb := engine.ObjectValue(h.NewObject())
	h.SetProperty(sb.Handle(), h.Intern().InternGo("x"), engine.Int(9))
	v, err := iso.RunInContext(context.Background(), `this.x`, sb)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToInt() != 9 {
		t.Fatalf("this.x = %v", v)
	}
}
