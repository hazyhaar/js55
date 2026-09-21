// SPDX-License-Identifier: BUSL-1.1
package engine_test

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/str"
)

func TestClosureCopiedPropertyKeyMutation(t *testing.T) {
	h := engine.NewHeap()
	key := h.Intern().InternGo("x")
	copied := *key
	obj := h.NewObject()
	h.SetProperty(obj, &copied, engine.Int(7))
	if v, ok := h.GetOwnProperty(obj, key); !ok || v.ToInt() != 7 {
		t.Errorf("canonical read after copied-key write: %v, found=%v", v, ok)
	}
	h.SetProperty(obj, key, engine.Int(9))
	if v, ok := h.GetOwnProperty(obj, &copied); !ok || v.ToInt() != 9 {
		t.Errorf("copied-key read after canonical mutation: %v, found=%v", v, ok)
	}
	other := engine.NewHeap().Intern().InternGo("x")
	h.SetProperty(obj, other, engine.Int(11))
	if v, ok := h.GetOwnProperty(obj, key); !ok || v.ToInt() != 11 {
		t.Fatal("foreign-key mutation did not resume canonical property access")
	}
	if got, ok := h.Intern().Lookup(&copied); !ok || got != key {
		t.Error("Lookup(copy) did not return original key after mutations")
	}
}

func TestClosureStringWrapperSignedQuotaCollection(t *testing.T) {
	h := engine.NewHeap()
	var balance int64
	h.QuotaTracker = func(delta int64) error { balance += delta; return nil }
	h.Intern().SetAllocTracker(h.TrackAlloc)
	key := h.Intern().InternGo("abcd")
	baseline := balance
	if baseline <= 0 {
		t.Fatal("intern storage not charged")
	}
	for i := 0; i < 8; i++ {
		wrapper := h.NewStringObject(key)
		if h.MustGet(wrapper).Text() != key {
			t.Fatal("wrapper lost retained text")
		}
		h.Collect()
		if h.Get(wrapper) != nil {
			t.Fatal("unrooted wrapper not collected")
		}
		if balance != baseline {
			t.Errorf("wrapper cycle %d: balance=%d, retained baseline=%d", i, balance, baseline)
		}
		primitive := h.NewString(str.FromGo("abcd"))
		if h.MustGet(primitive).Text().GoString() != "abcd" {
			t.Fatal("new primitive unusable after collection")
		}
		h.Collect()
		if balance != baseline {
			t.Errorf("primitive cycle %d: balance=%d, retained baseline=%d", i, balance, baseline)
		}
	}
	if got, ok := h.Intern().Lookup(str.FromGo("abcd")); !ok || got != key {
		t.Fatal("collection freed retained intern entry")
	}
}
