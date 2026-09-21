// SPDX-License-Identifier: BUSL-1.1

package ast

// ScopeBinding classifies identifier resolution status.
type ScopeBinding uint8

const (
	BindingVar ScopeBinding = iota
	BindingLet
	BindingConst
	BindingFunction
)

// ScopeResolver binds variable declarations and manages hoisting.
type ScopeResolver struct {
	Bindings map[string]ScopeBinding
}

// NewScopeResolver creates a static scope analysis pass.
func NewScopeResolver() *ScopeResolver {
	return &ScopeResolver{
		Bindings: make(map[string]ScopeBinding),
	}
}

// Declare records a variable declaration within the scope.
func (sr *ScopeResolver) Declare(name string, kind ScopeBinding) {
	sr.Bindings[name] = kind
}

// IsHoisted returns true if declaration is elevated to scope top.
func (sr *ScopeResolver) IsHoisted(name string) bool {
	kind, ok := sr.Bindings[name]
	return ok && (kind == BindingVar || kind == BindingFunction)
}
