// SPDX-License-Identifier: BUSL-1.1

package conformance

import (
	"os"
	"testing"
)

func TestClusterDumpObjectDefineProperty(t *testing.T) {
	if os.Getenv("JS55_CLUSTER") == "" {
		t.Skip("JS55_CLUSTER=1 pour le dump d'agrégats")
	}
	root := "/devhoros/pkg/js55/testdata/test262"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("arbre test262 absent")
	}
	aggs, rep, err := ClusterPrefix(root, "test/built-ins/Object/defineProperty")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(rep.Summary())
	if err := WriteAggregates(os.Stdout, "test/built-ins/Object/defineProperty", aggs); err != nil {
		t.Fatal(err)
	}
}

func TestClusterDumpReflectObjectForOf(t *testing.T) {
	if os.Getenv("JS55_CLUSTER") == "" {
		t.Skip("JS55_CLUSTER=1 pour le dump d'agrégats")
	}
	root := "/devhoros/pkg/js55/testdata/test262"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("arbre test262 absent")
	}
	prefixes := []struct{ name, path string }{
		{"reflect", "test/built-ins/Reflect"},
		{"object", "test/built-ins/Object"},
		{"for-of", "test/language/statements/for-of"},
	}
	for _, p := range prefixes {
		t.Run(p.name, func(t *testing.T) {
			aggs, rep, err := ClusterPrefix(root, p.path)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(rep.Summary())
			if err := WriteAggregates(os.Stdout, p.path, aggs); err != nil {
				t.Fatal(err)
			}
		})
	}
}
