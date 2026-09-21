// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package js55 provides a pure Go 1.27 sovereign JavaScript/TypeScript runtime
// designed as a 0-CGO replacement for Node.js.
package js55

import (
	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/isolate"
	"github.com/hazyhaar/js55/pkg/js55/runtime"
)

type Config = isolate.Config
type Isolate = isolate.Isolate
type Value = engine.Value
type Buffer = runtime.Buffer

func NewIsolate(cfg Config) (*Isolate, error) {
	return isolate.New(cfg)
}
