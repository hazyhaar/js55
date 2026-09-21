// SPDX-License-Identifier: BUSL-1.1

// Package js55 provides a sovereign pure-Go 1.27 (0-CGO) JavaScript runtime.
package js55

import (
	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

type Config = isolate.Config
type Isolate = isolate.Isolate
type Value = engine.Value

func NewIsolate(cfg Config) (*Isolate, error) {
	return isolate.New(cfg)
}
