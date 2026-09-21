// SPDX-License-Identifier: BUSL-1.1
package isolate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

func TestClosureObjectStringQuotaAndRecovery(t *testing.T) {
	iso, err := isolate.New(isolate.Config{MaxMemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := iso.EvalContext(ctx, `var closureText = "abcd";`); err != nil {
		t.Fatal(err)
	}
	chunk, err := iso.Compile(`for (var closureIndex=0; closureIndex<64; closureIndex++) { Object(closureText); } 7;`, "wrapper-quota", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iso.Execute(ctx, chunk); err != nil {
		t.Fatal(err)
	}
	iso.Heap().Collect()
	baseline := iso.AllocatedMemory()
	for i := 0; i < 4; i++ {
		v, err := iso.Execute(ctx, chunk)
		if err != nil || v.ToInt() != 7 {
			t.Fatalf("Object(text) execution: %v %v", v, err)
		}
		iso.Heap().Collect()
		if balance := iso.AllocatedMemory(); balance != baseline {
			t.Errorf("Object(text) cycle %d: balance=%d, warmed baseline=%d", i, balance, baseline)
		}
	}
	if _, err := iso.EvalContext(ctx, `new ArrayBuffer(1e12)`); !errors.Is(err, isolate.ErrMemoryLimitExceeded) {
		t.Fatalf("quota should still reject: %v", err)
	}
	v, err := iso.EvalContext(ctx, `var closureBox=Object(closureText); closureBox.answer=9; closureBox.answer;`)
	if err != nil || v.ToInt() != 9 {
		t.Fatalf("valid evaluation after quota rejection: %v %v", v, err)
	}
}
