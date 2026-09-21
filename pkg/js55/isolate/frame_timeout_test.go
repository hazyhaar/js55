// SPDX-License-Identifier: BUSL-1.1
package isolate_test

import (
	"context"
	"errors"
	"github.com/hazyhaar/js55/pkg/js55/isolate"
	"testing"
	"time"
)

func TestFrameActualEvalTimeoutThenSameFunction(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = iso.EvalContext(ctx, `function work(stop){if(stop){while(true){}} return 42;}`); err != nil {
		t.Fatal(err)
	}
	_, err = iso.EvalTimeout(ctx, `work(true);`, 20*time.Millisecond)
	if !errors.Is(err, isolate.ErrInterrupted) {
		t.Fatalf("expected wall-clock interruption: %v", err)
	}
	v, err := iso.EvalContext(ctx, `work(false);`)
	if err != nil || v.ToInt() != 42 {
		t.Fatalf("same function after timeout: %v %v", v, err)
	}
}
