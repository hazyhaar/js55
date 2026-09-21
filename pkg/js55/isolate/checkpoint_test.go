// SPDX-License-Identifier: BUSL-1.1
package isolate

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckpointSeesInterrupt(t *testing.T) {
	var n atomic.Int64
	iso, err := New(Config{
		GasLimit:        1 << 40,
		CheckpointEvery: 4096,
		OnCheckpoint: func() error {
			n.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		_, e := iso.EvalContext(context.Background(), `while (true) {}`)
		errCh <- e
	}()
	deadline := time.Now().Add(2 * time.Second)
	for n.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n.Load() == 0 {
		t.Fatal("checkpoint never ran")
	}
	iso.Interrupt()
	select {
	case e := <-errCh:
		if !errors.Is(e, ErrInterrupted) {
			t.Fatalf("got %v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("checkpoint did not observe interrupt")
	}
}

func TestEvalTimeoutInterruptsDuringCheckpoint(t *testing.T) {
	iso, err := New(Config{
		GasLimit:        1 << 40,
		CheckpointEvery: 1024,
		OnCheckpoint: func() error {
			time.Sleep(2 * time.Millisecond)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = iso.EvalTimeout(context.Background(), `while (true) {}`, 50*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("timeout: %v after %s", err, elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("timeout took %s, cooperation absent", elapsed)
	}
}

func TestCheckpointDoesNotBreakOrdinaryEval(t *testing.T) {
	iso, err := New(Config{
		CheckpointEvery: 8,
		OnCheckpoint:    func() error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := iso.EvalContext(context.Background(), `var s=0; for (var i=0;i<1000;i++) s+=i; s`)
	if err != nil {
		t.Fatal(err)
	}
	if iso.VM().ToStringValue(v).GoString() != "499500" {
		t.Fatalf("got %s", iso.VM().ToStringValue(v).GoString())
	}
}
