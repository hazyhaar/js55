// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import "errors"

// Engine définit l'interface minimale requise pour brancher un moteur d'exécution ECMAScript sur le pilote.
type Engine interface {
	Eval(source string, strict bool) error
}

// JSError permet à une implémentation de moteur de renvoyer une erreur qualifiée
// pour la validation bit-exacte des tests négatifs test262.
type JSError interface {
	error
	Phase() string // "parse", "resolution", "runtime"
	Type() string  // "SyntaxError", "TypeError", "ReferenceError", etc.
}

// EvalError est une structure d'erreur qualifiée conforme à JSError.
type EvalError struct {
	phase string
	typ   string
	msg   string
}

// NewEvalError instancie une erreur qualifiée.
func NewEvalError(phase, typ, msg string) *EvalError {
	return &EvalError{phase: phase, typ: typ, msg: msg}
}

func (e *EvalError) Error() string {
	return e.phase + " " + e.typ + ": " + e.msg
}

func (e *EvalError) Phase() string {
	return e.phase
}

func (e *EvalError) Type() string {
	return e.typ
}

// NullEngine implémente l'interface Engine en renvoyant systématiquement une erreur.
// Utilisé pour établir la ligne de base du projet avec un moteur vide qui échoue partout.
type NullEngine struct{}

// ErrNullEngine est l'erreur renvoyée par le NullEngine.
var ErrNullEngine = errors.New("null engine: evaluation not supported")

// Eval retourne systématiquement une erreur pour simuler un moteur non implémenté.
func (n *NullEngine) Eval(source string, strict bool) error {
	return ErrNullEngine
}
