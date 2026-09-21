// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"errors"
	"testing"
)

func TestEvaluateVerdict_Positive(t *testing.T) {
	meta := &Metadata{
		Description: "Positive test",
	}

	// Succès quand aucune erreur n'est levée
	if v := EvaluateVerdict(meta, nil); v != VerdictPass {
		t.Errorf("Attendu VerdictPass pour un test positif sans erreur, obtenu %v", v)
	}

	// Échec quand une erreur est levée
	if v := EvaluateVerdict(meta, errors.New("runtime exception")); v != VerdictFail {
		t.Errorf("Attendu VerdictFail pour un test positif avec erreur, obtenu %v", v)
	}

	// Échec avec NullEngine
	nullEng := &NullEngine{}
	if v := EvaluateVerdict(meta, nullEng.Eval("", false)); v != VerdictFail {
		t.Errorf("Attendu VerdictFail avec NullEngine sur test positif, obtenu %v", v)
	}
}

func TestEvaluateVerdict_Negative(t *testing.T) {
	meta := &Metadata{
		Description: "Negative test",
		Negative: &Negative{
			Phase: "parse",
			Type:  "SyntaxError",
		},
	}

	// Échec si aucune erreur n'est levée alors qu'une erreur est attendue
	if v := EvaluateVerdict(meta, nil); v != VerdictFail {
		t.Errorf("Attendu VerdictFail si aucune erreur levée sur test négatif, obtenu %v", v)
	}

	// Échec avec NullEngine (erreur générique non qualifiée)
	nullEng := &NullEngine{}
	if v := EvaluateVerdict(meta, nullEng.Eval("", false)); v != VerdictFail {
		t.Errorf("Attendu VerdictFail avec NullEngine sur test négatif, obtenu %v", v)
	}

	// Échec si la phase ne correspond pas
	mismatchPhase := NewEvalError("runtime", "SyntaxError", "erreur de syntaxe différée")
	if v := EvaluateVerdict(meta, mismatchPhase); v != VerdictFail {
		t.Errorf("Attendu VerdictFail si la phase ne correspond pas, obtenu %v", v)
	}

	// Échec si le type d'erreur ne correspond pas
	mismatchType := NewEvalError("parse", "TypeError", "mauvais type")
	if v := EvaluateVerdict(meta, mismatchType); v != VerdictFail {
		t.Errorf("Attendu VerdictFail si le type ne correspond pas, obtenu %v", v)
	}

	// Succès si la phase et le type correspondent exactement
	exactMatch := NewEvalError("parse", "SyntaxError", "caractère inattendu")
	if v := EvaluateVerdict(meta, exactMatch); v != VerdictPass {
		t.Errorf("Attendu VerdictPass quand phase et type concordent, obtenu %v", v)
	}
}
