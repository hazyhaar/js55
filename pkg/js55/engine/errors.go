// SPDX-License-Identifier: BUSL-1.1
package engine

import "errors"

var (
	ErrGasExhausted        = errors.New("quota d'instructions épuisé")
	ErrInterrupted         = errors.New("exécution interrompue")
	ErrRegExpInputTooLarge = errors.New("expression régulière : entrée au-delà du plafond admis")
)
