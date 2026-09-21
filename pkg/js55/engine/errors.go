package engine

import "errors"

var (
	ErrGasExhausted = errors.New("quota d'instructions épuisé")
	ErrInterrupted  = errors.New("exécution interrompue")
)
