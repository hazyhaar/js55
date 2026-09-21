// SPDX-License-Identifier: BUSL-1.1

package conformance

// Verdict représente le statut normatif d'un cas de test dans le manifeste.
type Verdict string

const (
	VerdictPass  Verdict = "pass"
	VerdictFail  Verdict = "fail"
	VerdictSkip  Verdict = "skip"
	VerdictError Verdict = "error"
)

// EvaluateVerdict calcule le verdict d'exécution d'un test.
// Règle 4 :
// - Pour un test négatif, le succès (pass) exige que l'erreur survienne à la phase attendue avec le type attendu.
// - Pour un test positif, le succès (pass) est l'absence d'erreur (evalErr == nil).
// - Un skip correspond à une fonctionnalité non supportée déclarée dans le profil.
// - Un error correspond à une défaillance interne du pilote (ex. I/O, parsing YAML corrompu).
func EvaluateVerdict(meta *Metadata, evalErr error) Verdict {
	if meta != nil && meta.Negative != nil {
		if evalErr == nil {
			// Le test attendait une exception mais s'est exécuté sans erreur -> échec.
			return VerdictFail
		}
		if jsErr, ok := evalErr.(JSError); ok {
			if jsErr.Phase() == meta.Negative.Phase && jsErr.Type() == meta.Negative.Type {
				return VerdictPass
			}
			return VerdictFail
		}
		// Une erreur non qualifiée (ex: NullEngine) ne prouve pas la parité de phase/type attendue -> échec.
		return VerdictFail
	}

	if evalErr == nil {
		return VerdictPass
	}
	return VerdictFail
}
