package isolate

import (
	"context"
	"testing"
	"time"
)

func TestEvalTimeoutSuccessfulCallsDoNotInterruptNextCall(t *testing.T) {
	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if _, err = iso.EvalTimeout(context.Background(), "1+1", time.Second); err != nil {
			t.Fatalf("evaluation %d: %v", i, err)
		}
	}
	if _, err = iso.Eval(context.Background(), "2+2"); err != nil {
		t.Fatal(err)
	}
}
