// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"fmt"
	"strings"
)

// Regression décrit un cas qui était passant et ne l'est plus.
type Regression struct {
	RelPath        string
	Mode           Mode
	GoldenVerdict  Verdict
	CurrentVerdict Verdict
}

// Progression décrit un cas qui n'était pas passant et qui est devenu passant.
type Progression struct {
	RelPath        string
	Mode           Mode
	GoldenVerdict  Verdict
	CurrentVerdict Verdict
}

// RatchetReport contient les résultats de la comparaison entre manifeste doré et manifeste courant.
type RatchetReport struct {
	TotalGoldenCases  int
	TotalCurrentCases int
	Regressions       []Regression
	Progressions      []Progression
}

// HasRegressions indique si au moins un cas passant a régressé.
func (r *RatchetReport) HasRegressions() bool {
	return len(r.Regressions) > 0
}

// Error produit une description détaillée de la régression en listant nommément les cas perdus.
func (r *RatchetReport) Error() string {
	if !r.HasRegressions() {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("RÉGRESSION RATCHET DÉTECTÉE (%d cas perdus) :\n", len(r.Regressions)))
	for _, reg := range r.Regressions {
		sb.WriteString(fmt.Sprintf("  - %s [%s] était %s, devenu %s\n",
			reg.RelPath, reg.Mode, reg.GoldenVerdict, reg.CurrentVerdict))
	}
	return sb.String()
}

// VerifyRatchet compare le manifeste doré de référence au manifeste de l'exécution courante.
// Règle 7 : Échoue si et seulement si un cas passant est redevenu non-passant (régression),
// en nommant explicitement les cas perdus. Les progressions (nouveaux cas passants) ne provoquent
// aucun échec.
func VerifyRatchet(goldenCases, currentCases []TestCase) (*RatchetReport, error) {
	goldenMap := make(map[string]TestCase, len(goldenCases))
	for _, c := range goldenCases {
		goldenMap[c.Key()] = c
	}

	currentMap := make(map[string]TestCase, len(currentCases))
	for _, c := range currentCases {
		currentMap[c.Key()] = c
	}

	report := &RatchetReport{
		TotalGoldenCases:  len(goldenCases),
		TotalCurrentCases: len(currentCases),
	}

	// 1. Contrôle des régressions : tout cas passant dans le doré doit rester passant
	for key, gold := range goldenMap {
		if gold.Verdict == VerdictPass {
			curr, exists := currentMap[key]
			if !exists {
				report.Regressions = append(report.Regressions, Regression{
					RelPath:        gold.RelPath,
					Mode:           gold.Mode,
					GoldenVerdict:  gold.Verdict,
					CurrentVerdict: Verdict("missing"),
				})
			} else if curr.Verdict != VerdictPass {
				report.Regressions = append(report.Regressions, Regression{
					RelPath:        gold.RelPath,
					Mode:           gold.Mode,
					GoldenVerdict:  gold.Verdict,
					CurrentVerdict: curr.Verdict,
				})
			}
		}
	}

	// 2. Détection des progressions (nouveaux cas passants)
	for key, curr := range currentMap {
		if curr.Verdict == VerdictPass {
			gold, exists := goldenMap[key]
			if !exists || gold.Verdict != VerdictPass {
				goldenVerdict := Verdict("missing")
				if exists {
					goldenVerdict = gold.Verdict
				}
				report.Progressions = append(report.Progressions, Progression{
					RelPath:        curr.RelPath,
					Mode:           curr.Mode,
					GoldenVerdict:  goldenVerdict,
					CurrentVerdict: curr.Verdict,
				})
			}
		}
	}

	if report.HasRegressions() {
		return report, fmt.Errorf("%s", report.Error())
	}

	return report, nil
}
