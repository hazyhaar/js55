// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// Negative décrit les erreurs attendues d'un test négatif test262.
type Negative struct {
	Phase string `yaml:"phase"`
	Type  string `yaml:"type"`
}

// Metadata regroupe les métadonnées déclarées dans le bloc YAML du frontmatter.
type Metadata struct {
	Description string    `yaml:"description"`
	ESID        string    `yaml:"esid"`
	ES5ID       string    `yaml:"es5id"`
	ES6ID       string    `yaml:"es6id"`
	Flags       []string  `yaml:"flags"`
	Includes    []string  `yaml:"includes"`
	Features    []string  `yaml:"features"`
	Negative    *Negative `yaml:"negative"`
	Info        string    `yaml:"info"`
	Locale      []string  `yaml:"locale"`
}

// Mode qualifie le mode d'exécution ECMAScript (sloppy ou strict).
type Mode string

const (
	ModeSloppy Mode = "sloppy"
	ModeStrict Mode = "strict"
)

// HasFlag indique si un drapeau d'exécution est présent dans les métadonnées.
func (m *Metadata) HasFlag(flag string) bool {
	if m == nil {
		return false
	}
	for _, f := range m.Flags {
		if strings.EqualFold(strings.TrimSpace(f), flag) {
			return true
		}
	}
	return false
}

// HasFeature indique si une fonctionnalité est déclarée dans le test.
func (m *Metadata) HasFeature(feat string) bool {
	if m == nil {
		return false
	}
	for _, f := range m.Features {
		if strings.EqualFold(strings.TrimSpace(f), feat) {
			return true
		}
	}
	return false
}

// Modes retourne la liste ordonnée des modes d'exécution applicables au test.
// Conformément à INTERPRETING.md :
// - raw : exécuté 1 seule fois en mode non-strict (sloppy), sans harnais
// - module : exécuté 1 seule fois en mode strict
// - onlyStrict : exécuté 1 seule fois en mode strict
// - noStrict : exécuté 1 seule fois en mode non-strict (sloppy)
// - défaut : exécuté 2 fois (sloppy puis strict)
func (m *Metadata) Modes() []Mode {
	if m == nil {
		return []Mode{ModeSloppy, ModeStrict}
	}
	if m.HasFlag("raw") {
		return []Mode{ModeSloppy}
	}
	if m.HasFlag("module") {
		return []Mode{ModeStrict}
	}
	if m.HasFlag("onlyStrict") {
		return []Mode{ModeStrict}
	}
	if m.HasFlag("noStrict") {
		return []Mode{ModeSloppy}
	}
	return []Mode{ModeSloppy, ModeStrict}
}

// IsFixture détermine si un nom de fichier correspond à une fixture non exécutable.
func IsFixture(pathOrName string) bool {
	return strings.Contains(pathOrName, "_FIXTURE")
}

// ExtractFrontmatter isole le texte YAML et le corps du test délimités par /*--- et ---*/.
func ExtractFrontmatter(content string) (string, string, error) {
	startIdx := strings.Index(content, "/*---")
	if startIdx == -1 {
		return "", content, errors.New("frontmatter /*--- introuvable")
	}
	endIdx := strings.Index(content[startIdx+5:], "---*/")
	if endIdx == -1 {
		return "", content, errors.New("clôture frontmatter ---*/ introuvable")
	}
	realEnd := startIdx + 5 + endIdx
	yamlPart := content[startIdx+5 : realEnd]
	bodyPart := content[realEnd+5:]
	if strings.HasPrefix(bodyPart, "\r\n") {
		bodyPart = bodyPart[2:]
	} else if strings.HasPrefix(bodyPart, "\n") {
		bodyPart = bodyPart[1:]
	}
	return yamlPart, bodyPart, nil
}

// ParseFrontmatter extrait et désérialise les métadonnées YAML d'un test.
func ParseFrontmatter(content string) (*Metadata, string, error) {
	yamlPart, body, err := ExtractFrontmatter(content)
	if err != nil {
		return nil, content, err
	}
	var meta Metadata
	if err := yaml.Unmarshal([]byte(yamlPart), &meta); err != nil {
		return nil, body, err
	}
	return &meta, body, nil
}
