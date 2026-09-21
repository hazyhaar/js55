// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

// AsyncContext manages the resolution state of an async function.
type AsyncContext struct {
	VM        *VM
	QueueTask func(func() error)
	Resolved  bool
	Result    Value
	Err       error
}

// NewAsyncContext creates an async execution context.
func NewAsyncContext(vm *VM, queueTask func(func() error)) *AsyncContext {
	return &AsyncContext{
		VM:        vm,
		QueueTask: queueTask,
	}
}

// Resolve fulfills the async context with a value.
func (ac *AsyncContext) Resolve(v Value) {
	ac.Resolved = true
	ac.Result = v
	if ac.QueueTask != nil {
		ac.QueueTask(func() error {
			return nil
		})
	}
}
