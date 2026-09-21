// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// HarnessLoader gère le chargement et la mise en cache des fichiers de support Test262.
type HarnessLoader struct {
	dir   string
	mu    sync.RWMutex
	cache map[string]string
}

// NewHarnessLoader initialise le chargeur de harnais.
func NewHarnessLoader(dir string) *HarnessLoader {
	return &HarnessLoader{
		dir:   dir,
		cache: make(map[string]string),
	}
}

// Preload précharge assert.js et sta.js.
func (hl *HarnessLoader) Preload() error {
	for _, name := range []string{"assert.js", "sta.js"} {
		if _, err := hl.Load(name); err != nil {
			return err
		}
	}
	return nil
}

// Load lit et met en cache un fichier du harnais.
func (hl *HarnessLoader) Load(name string) (string, error) {
	hl.mu.RLock()
	if c, ok := hl.cache[name]; ok {
		hl.mu.RUnlock()
		return c, nil
	}
	hl.mu.RUnlock()

	hl.mu.Lock()
	defer hl.mu.Unlock()

	if c, ok := hl.cache[name]; ok {
		return c, nil
	}

	p := filepath.Join(hl.dir, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("harness %s: %w", name, err)
	}
	content := string(data)
	hl.cache[name] = content
	return content, nil
}

// AssembleSource assemble les prérequis du harnais et le code source de test.
func AssembleSource(hl *HarnessLoader, meta *Metadata, body string, mode Mode) (string, error) {
	if meta != nil {
		for _, f := range meta.Flags {
			if f == "raw" {
				return body, nil
			}
		}
	}

	var sb strings.Builder
	// assert.js et sta.js sont toujours obligatoires
	for _, base := range []string{"assert.js", "sta.js"} {
		c, err := hl.Load(base)
		if err != nil {
			return "", err
		}
		sb.WriteString(c)
		sb.WriteByte('\n')
	}
	if meta != nil {
		for _, inc := range meta.Includes {
			if inc == "assert.js" || inc == "sta.js" {
				continue
			}
			c, err := hl.Load(inc)
			if err != nil {
				return "", err
			}
			sb.WriteString(c)
			sb.WriteByte('\n')
		}
	}

	if mode == ModeStrict {
		sb.WriteString("\"use strict\";\n")
	}

	needDone := meta != nil && meta.HasFlag("async")
	if meta != nil {
		for _, inc := range meta.Includes {
			if inc == "asyncHelpers.js" {
				needDone = true
			}
		}
	}
	if needDone {
		sb.WriteString("globalThis.$DONE = function(e) { if (e) throw e; };\n")
	}

	sb.WriteString(body)
	return sb.String(), nil
}
