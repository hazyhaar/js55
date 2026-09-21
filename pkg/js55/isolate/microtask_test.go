// SPDX-License-Identifier: BUSL-1.1

package isolate

import (
	"sync/atomic"
	"testing"
)

func TestMicrotaskQueue_FIFOExecution(t *testing.T) {
	mq := NewMicrotaskQueue()

	var counter int64
	var order []int

	mq.Enqueue(func() error {
		order = append(order, 1)
		atomic.AddInt64(&counter, 1)
		return nil
	})

	mq.Enqueue(func() error {
		order = append(order, 2)
		atomic.AddInt64(&counter, 1)
		return nil
	})

	if mq.Len() != 2 {
		t.Fatalf("Expected queue length 2, got %d", mq.Len())
	}

	err := mq.RunMicrotasks()
	if err != nil {
		t.Fatalf("Unexpected microtask execution error: %v", err)
	}

	if mq.Len() != 0 {
		t.Fatalf("Expected queue to be empty after run, got %d", mq.Len())
	}

	if counter != 2 {
		t.Fatalf("Expected counter 2, got %d", counter)
	}

	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("Expected FIFO order [1, 2], got %v", order)
	}
}
