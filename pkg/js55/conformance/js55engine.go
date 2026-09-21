// SPDX-License-Identifier: BUSL-1.1
package conformance

import (
	"strings"

	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/parser"
)

// JS55Engine branche l'interpréteur du moteur sur le pilote de conformité.
//
// Chaque cas s'exécute dans un tas NEUF : deux cas ne partagent ni objets, ni
// globales, ni chaînes internées. C'est plus lent qu'un tas réutilisé, mais la
// suite de conformité ne mesure alors que le programme, jamais la trace laissée
// par le précédent.
type JS55Engine struct {
	// Gas borne le nombre d'instructions par cas. Sans borne, une boucle sans
	// fin dans un test bloquerait la suite entière.
	Gas int64
	// MaxDepth borne la profondeur d'appel.
	MaxDepth int
	// Stress active le mode stress du ramasse-miettes. La suite doit rendre le
	// même verdict dans les deux modes (plan T3.1).
	Stress bool
}

// NewJS55Engine crée l'adaptateur avec des bornes raisonnables.
func NewJS55Engine() *JS55Engine {
	return &JS55Engine{Gas: 5_000_000, MaxDepth: 256}
}

// Eval analyse, compile et exécute la source.
//
// La phase de l'erreur est renseignée : le pilote en a besoin pour départager
// un test négatif de phase « parse » d'un test négatif d'exécution. Un refus du
// COMPILATEUR est rapporté en phase runtime et non parse : une construction que
// le moteur ne compile pas encore n'est pas une erreur de syntaxe, et la
// confondre avec une SyntaxError ferait passer des tests négatifs pour de
// mauvaises raisons.
func (e *JS55Engine) Eval(source string, strict bool) error {
	prog, err := parser.Parse(source, parser.Options{Strict: strict})
	if err != nil {
		return NewEvalError("parse", "SyntaxError", err.Error())
	}

	h := engine.NewHeap()
	chunk, err := engine.CompileMode(h, prog, "test262", strict)
	if err != nil {
		return NewEvalError("runtime", "Unsupported", err.Error())
	}

	h.SetStress(e.Stress)
	vm := engine.NewVM(h)
	if e.Gas > 0 {
		vm.GasLeft = e.Gas
	}
	if e.MaxDepth > 0 {
		vm.MaxDepth = e.MaxDepth
	}

	if _, err := vm.Run(chunk); err != nil {
		return NewEvalError("runtime", errorTypeOf(err), err.Error())
	}
	return nil
}

// errorTypeOf déduit le type d'erreur ECMAScript d'une erreur du moteur. Le
// texte lancé porte le nom du constructeur lorsque le moteur l'a produit
// lui-même ; sinon le type reste générique plutôt que d'être deviné.
func errorTypeOf(err error) string {
	th, ok := err.(*engine.Throw)
	if !ok {
		return "Error"
	}
	for _, name := range []string{
		"RangeError", "TypeError", "ReferenceError", "SyntaxError",
		"EvalError", "URIError", "Test262Error",
	} {
		if strings.HasPrefix(th.Text, name) {
			return name
		}
	}
	return "Error"
}
