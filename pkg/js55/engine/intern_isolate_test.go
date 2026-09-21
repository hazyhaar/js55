package engine_test

import (
	"context"
	"errors"
	"github.com/hazyhaar/js55/pkg/js55/isolate"
	"strings"
	"testing"
)

func TestInternTablesArePerIsolate(t *testing.T) {
	a, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sa := a.Heap().Intern().InternGo("size")
	sb := b.Heap().Intern().InternGo("size")
	if sa == sb {
		t.Fatal("deux isolats partagent le même pointeur interné")
	}
	if a.Heap().Intern().Intern(sb) == sb {
		t.Fatal("l'isolat A a renvoyé le pointeur interné de B")
	}
}

func TestInternRealIsolateQuotaRejectThenEvalRecovery(t *testing.T) {
	iso, err := isolate.New(isolate.Config{MaxMemoryBytes: 512 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = iso.Eval(ctx, `var preserved = {answer: 7}; preserved.answer`); err != nil {
		t.Fatal(err)
	}
	before, count := iso.AllocatedMemory(), iso.Heap().Intern().Len()
	var rejected error
	func() {
		defer func() {
			if r := recover(); r != nil {
				var ok bool
				rejected, ok = r.(error)
				if !ok {
					panic(r)
				}
			}
		}()
		iso.Heap().Intern().InternGo(strings.Repeat("界", 512*1024))
	}()
	if !errors.Is(rejected, isolate.ErrMemoryLimitExceeded) {
		t.Fatalf("expected memory rejection, got %v", rejected)
	}
	if iso.AllocatedMemory() != before || iso.Heap().Intern().Len() != count {
		t.Fatal("rejection altered quota or table")
	}
	value, err := iso.Eval(ctx, `preserved.answer = preserved.answer + 2; preserved.answer`)
	if err != nil || value.ToInt() != 9 {
		t.Fatalf("real property mutation/eval did not resume: %v %v", value, err)
	}
}
