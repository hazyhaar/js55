// SPDX-License-Identifier: BUSL-1.1

package conformance

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const (
	sigMsgMax      = 96
	sigExamplesMax = 3
)

var ligneRe = regexp.MustCompile(`\s*\(ligne \d+\)`)

// Aggregate groupe des échecs partageant une signature d'erreur normalisée.
type Aggregate struct {
	Signature string
	Size      int
	Examples  []string
}

// NormalizeSignature réduit un message d'échec à type + texte tronqué sans numéro de ligne.
func NormalizeSignature(errMsg string) string {
	msg := strings.TrimSpace(errMsg)
	if msg == "" {
		return "fail-silent"
	}
	typ := "Error"
	rest := msg
	if i := strings.IndexByte(rest, ' '); i > 0 {
		head := rest[:i]
		tail := rest[i+1:]
		if j := strings.IndexByte(tail, ':'); j > 0 {
			cand := strings.TrimSpace(tail[:j])
			if cand != "" && !strings.ContainsAny(cand, " ") {
				typ = cand
				rest = strings.TrimSpace(tail[j+1:])
			}
			_ = head
		}
	}
	rest = ligneRe.ReplaceAllString(rest, "")
	rest = strings.TrimSpace(rest)
	if len(rest) > sigMsgMax {
		rest = rest[:sigMsgMax]
	}
	return typ + " | " + rest
}

// ClusterBySignature groupe les cas par signature, triés par taille décroissante.
func ClusterBySignature(cases []TestCase) []Aggregate {
	idx := map[string]*Aggregate{}
	order := []string{}
	for _, c := range cases {
		if c.Verdict != VerdictFail && c.Verdict != VerdictError {
			continue
		}
		sig := NormalizeSignature(c.Error)
		ag, ok := idx[sig]
		if !ok {
			ag = &Aggregate{Signature: sig}
			idx[sig] = ag
			order = append(order, sig)
		}
		ag.Size++
		if len(ag.Examples) < sigExamplesMax {
			ag.Examples = append(ag.Examples, c.RelPath+" ["+string(c.Mode)+"]")
		}
	}
	out := make([]Aggregate, 0, len(order))
	for _, sig := range order {
		out = append(out, *idx[sig])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Size != out[j].Size {
			return out[i].Size > out[j].Size
		}
		return out[i].Signature < out[j].Signature
	})
	return out
}

// ClusterPrefix ré-exécute un sous-arbre Test262 et groupe les échecs.
func ClusterPrefix(root, prefix string) ([]Aggregate, *Report, error) {
	rep, err := RunSuite(RunnerConfig{
		Test262Root: root,
		RelPrefix:   prefix,
		Engine:      NewJS55Engine(),
		Workers:     1,
	})
	if err != nil {
		return nil, nil, err
	}
	return ClusterBySignature(rep.Cases), rep, nil
}

// WriteAggregates écrit le tableau agrégat → taille → exemples.
func WriteAggregates(w io.Writer, title string, aggs []Aggregate) error {
	if _, err := fmt.Fprintf(w, "AGRÉGATS %s (%d signatures)\n", title, len(aggs)); err != nil {
		return err
	}
	for i, a := range aggs {
		ex := strings.Join(a.Examples, " ; ")
		if _, err := fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", i+1, a.Size, a.Signature, ex); err != nil {
			return err
		}
	}
	return nil
}
