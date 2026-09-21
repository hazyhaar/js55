// SPDX-License-Identifier: BUSL-1.1

package isolate

import (
	"sync"

	"github.com/hazyhaar/js55/pkg/js55/engine"
)

// Microtask represents a queued callback job (Promises, queueMicrotask).
type Microtask func() error

// MicrotaskQueue manages execution of asynchronous jobs in FIFO order.
type MicrotaskQueue struct {
	mu    sync.Mutex
	queue []Microtask
}

// NewMicrotaskQueue creates a new isolated microtask queue.
func NewMicrotaskQueue() *MicrotaskQueue {
	return &MicrotaskQueue{
		queue: make([]Microtask, 0, 32),
	}
}

// Enqueue adds a microtask to the back of the queue.
func (mq *MicrotaskQueue) Enqueue(task Microtask) {
	if task == nil {
		return
	}
	mq.mu.Lock()
	defer mq.mu.Unlock()
	mq.queue = append(mq.queue, task)
}

// RunMicrotasks executes all queued microtasks until the queue is drained.
func (mq *MicrotaskQueue) RunMicrotasks() error {
	mq.mu.Lock()
	if len(mq.queue) == 0 {
		mq.mu.Unlock()
		return nil
	}
	current := mq.queue
	mq.queue = make([]Microtask, 0, cap(current))
	mq.mu.Unlock()

	for _, task := range current {
		if err := task(); err != nil {
			return err
		}
	}
	return nil
}

// Len returns the number of pending microtasks.
func (mq *MicrotaskQueue) Len() int {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return len(mq.queue)
}

// PromiseState tracks Promise resolution state according to ECMAScript standard.
type PromiseState uint8

const (
	PromisePending PromiseState = iota
	PromiseFulfilled
	PromiseRejected
)

// Promise represents a native Promise object.
type Promise struct {
	State  PromiseState
	Result engine.Value
}
