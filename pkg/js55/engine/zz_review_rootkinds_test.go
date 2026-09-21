// SPDX-License-Identifier: BUSL-1.1
package engine

import "testing"

func TestReview_RootKinds(t *testing.T) {
	ensureRootRealm()
	h := DefaultRootRealm.Heap
	counts := map[ObjKind]int{}
	arraysWithElems, envs, promises, maps := 0, 0, 0, 0
	for _, o := range h.objs {
		if o == nil {
			continue
		}
		counts[o.kind]++
		if o.kind == KindArray && len(o.elements) > 0 {
			arraysWithElems++
		}
		if o.kind == KindEnv {
			envs++
		}
		if o.promise != nil {
			promises++
		}
		if o.mapData != nil || o.setData != nil {
			maps++
		}
	}
	t.Logf("objets racine=%d kinds=%v arraysWithElems=%d envs=%d promises=%d maps/sets=%d", len(h.objs)-1, counts, arraysWithElems, envs, promises, maps)
}
