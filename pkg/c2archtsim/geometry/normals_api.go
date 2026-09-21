// SPDX-License-Identifier: Apache-2.0 OR MIT
package geometry

import (
	"math"
	"runtime"
	"unsafe"
)

const (
	PhaseZero    = uint64(0)
	PhaseAccum   = uint64(1)
	PhaseNorm    = uint64(2)
	PhaseDone    = uint64(3)
	flagAccepted = uint64(1)
	flagNormOnly = uint64(2)
	stateLen     = 8
)

// NormState is opaque. Callers must not forge the eight words; only Begin
// binds identity and lengths. Phase, Cursor, Quota and LastNormalized remain
// readable. Slice fields keep the stream inputs reachable for the GC. Callers
// still retain the same backing arrays between Begin and the last Step.
// last holds C doubles x*inv,y*inv,z*inv captured before float32 store.
type NormState struct {
	w         [8]uint64
	pos       []float32
	idx       []uint32
	nor       []float32
	last      [3]float64
	lastValid bool
}

func (s *NormState) Phase() uint64  { return s.w[0] }
func (s *NormState) Cursor() uint64 { return s.w[1] }
func (s *NormState) Quota() uint64  { return s.w[7] }

func (s *NormState) LastNormalized() (value [3]float64, ok bool) {
	if s == nil || !s.lastValid {
		return value, false
	}
	value = s.last
	return value, true
}

func (s *NormState) raw() []uint64 { return s.w[:] }

func finite32All(v []float32) bool {
	for i := 0; i < len(v); i++ {
		if !finite32(v[i]) {
			return false
		}
	}
	return true
}

func maxAbs32(v []float32) float64 {
	m := 0.0
	for i := 0; i < len(v); i++ {
		a := math.Abs(float64(v[i]))
		if a > m {
			m = a
		}
	}
	return m
}

const (
	domainF32Eps      = 1.0 / float64(uint64(1)<<23)
	domainMaxNtris    = uint64(1 << 22)
	domainCrossComp   = 8.0
	domainCrossVec    = 12.0
	domainContribOps  = 4
	domainExtraMargin = 2.0
)

func domainNtris(ntris uint64) uint64 {
	if ntris == 0 {
		return 1
	}
	return ntris
}

func contributionBoundExactGeom(ntris uint64) float64 {
	n := domainNtris(ntris)
	return math.Sqrt(float64(math.MaxFloat32) / (domainCrossComp * float64(n)))
}

func contributionBoundRaw12(ntris uint64) float64 {
	n := domainNtris(ntris)
	return math.Sqrt(float64(math.MaxFloat32) / (domainCrossVec * float64(n)))
}

func contributionInflation(ntris uint64) float64 {
	n := domainNtris(ntris)
	nEps := float64(n) * domainF32Eps
	return math.Pow(1+domainF32Eps, domainContribOps) / (1 - nEps) * domainExtraMargin
}

func contributionBound(ntris uint64) float64 {
	n := domainNtris(ntris)
	if n > domainMaxNtris {
		return 0
	}
	denom := domainCrossVec * contributionInflation(n) * float64(n)
	return math.Sqrt(float64(math.MaxFloat32) / denom)
}

func addU(a, b uint64) (uint64, bool) {
	if a > ^uint64(0)-b {
		return 0, false
	}
	return a + b, true
}

func mulU(a, b uint64) (uint64, bool) {
	if a != 0 && b > ^uint64(0)/a {
		return 0, false
	}
	return a * b, true
}

func mul3(n uint64) (uint64, bool) {
	return mulU(n, 3)
}

func range3(i, nfloats uint64) bool {
	o, ok := mul3(i)
	if !ok {
		return false
	}
	end, ok := addU(o, 2)
	if !ok {
		return false
	}
	return end < nfloats
}

func overlapBytes(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	ap := uintptr(unsafe.Pointer(&a[0]))
	bp := uintptr(unsafe.Pointer(&b[0]))
	ae, aok := addPtr(ap, len(a))
	be, bok := addPtr(bp, len(b))
	if !aok || !bok {
		return true
	}
	return ap < be && bp < ae
}

func addPtr(p uintptr, n int) (uintptr, bool) {
	if n < 0 {
		return 0, false
	}
	un := uintptr(n)
	if p > ^uintptr(0)-un {
		return 0, false
	}
	return p + un, true
}

func f32Bytes(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&v[0])), len(v)*4)
}

func u32Bytes(v []uint32) []byte {
	if len(v) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&v[0])), len(v)*4)
}

func f64Bytes(v []float64) []byte {
	if len(v) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&v[0])), len(v)*8)
}

func u64Bytes(v []uint64) []byte {
	if len(v) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&v[0])), len(v)*8)
}

func scratchAliasOK(last []float64, flag []uint64, pos []float32, idx []uint32, nor []float32, st *NormState) bool {
	lb := f64Bytes(last)
	fb := u64Bytes(flag)
	pb := f32Bytes(pos)
	ib := u32Bytes(idx)
	nb := f32Bytes(nor)
	sb := stateBytes(st)
	if overlapBytes(lb, pb) || overlapBytes(lb, ib) || overlapBytes(lb, nb) || overlapBytes(lb, sb) {
		return false
	}
	if overlapBytes(fb, pb) || overlapBytes(fb, ib) || overlapBytes(fb, nb) || overlapBytes(fb, sb) {
		return false
	}
	if overlapBytes(lb, fb) {
		return false
	}
	return true
}

func cNormalsStep(pos []float32, idx []uint32, nor []float32, st *NormState, budget uint64) int {
	var last [3]float64
	var flag [1]uint64
	if !scratchAliasOK(last[:], flag[:], pos, idx, nor, st) {
		return 0
	}
	rc := Archtime_normals_step(pos, idx, nor, st.raw(), stateLen, budget, last[:], &flag[0])
	if rc != 0 && flag[0] != 0 {
		st.last = last
		st.lastValid = true
	}
	return rc
}

func stateBytes(s *NormState) []byte {
	if s == nil {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(s)), int(unsafe.Sizeof(*s)))
}

func f32ID(v []float32) (uintptr, int) {
	if len(v) == 0 {
		return 0, 0
	}
	return uintptr(unsafe.Pointer(&v[0])), len(v)
}

func u32ID(v []uint32) (uintptr, int) {
	if len(v) == 0 {
		return 0, 0
	}
	return uintptr(unsafe.Pointer(&v[0])), len(v)
}

func sameF32(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}

func sameU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}

func (s *NormState) bind(pos []float32, idx []uint32, nor []float32) {
	s.pos = pos
	s.idx = idx
	s.nor = nor
	s.last = [3]float64{}
	s.lastValid = false
}

func (s *NormState) sameCompute(pos []float32, idx []uint32, nor []float32) bool {
	return sameF32(pos, s.pos) && sameU32(idx, s.idx) && sameF32(nor, s.nor)
}

func (s *NormState) sameNorm(nor []float32) bool {
	return sameF32(nor, s.nor)
}

func indicesOK(indices []uint32, nverts uint64) bool {
	n := len(indices)
	if n == 0 || n%3 != 0 {
		return false
	}
	for i := 0; i < n; i++ {
		if uint64(indices[i]) >= nverts {
			return false
		}
	}
	return true
}

func triangleOK(idx []uint32, t, nverts, npos, nnorm, nidx uint64) bool {
	if !range3(t, nidx) {
		return false
	}
	base, ok := mul3(t)
	if !ok {
		return false
	}
	ia := uint64(idx[base])
	ib := uint64(idx[base+1])
	ic := uint64(idx[base+2])
	if ia >= nverts || ib >= nverts || ic >= nverts {
		return false
	}
	if !range3(ia, npos) || !range3(ib, npos) || !range3(ic, npos) {
		return false
	}
	if !range3(ia, nnorm) || !range3(ib, nnorm) || !range3(ic, nnorm) {
		return false
	}
	return true
}

func triangleLive(idx []uint32, t, nverts uint64, pos []float32) bool {
	base, ok := mul3(t)
	if !ok || int(base)+2 >= len(idx) {
		return false
	}
	ia := uint64(idx[base])
	ib := uint64(idx[base+1])
	ic := uint64(idx[base+2])
	if ia >= nverts || ib >= nverts || ic >= nverts {
		return false
	}
	return tripletFinite(pos, ia) && tripletFinite(pos, ib) && tripletFinite(pos, ic)
}

func tripletFinite(v []float32, i uint64) bool {
	o, ok := mul3(i)
	if !ok || int(o)+2 >= len(v) {
		return false
	}
	return finite32(v[o]) && finite32(v[o+1]) && finite32(v[o+2])
}

func trianglePosFinite(pos []float32, idx []uint32, t uint64) bool {
	base, ok := mul3(t)
	if !ok || int(base)+2 >= len(idx) {
		return false
	}
	return tripletFinite(pos, uint64(idx[base])) &&
		tripletFinite(pos, uint64(idx[base+1])) &&
		tripletFinite(pos, uint64(idx[base+2]))
}

func aliasOK(pos []float32, idx []uint32, nor []float32, st *NormState) bool {
	pb := f32Bytes(pos)
	ib := u32Bytes(idx)
	nb := f32Bytes(nor)
	sb := stateBytes(st)
	if overlapBytes(pb, nb) || overlapBytes(pb, ib) || overlapBytes(nb, ib) {
		return false
	}
	if overlapBytes(sb, pb) || overlapBytes(sb, ib) || overlapBytes(sb, nb) {
		return false
	}
	return true
}

func stateDimsOK(st *NormState, npos, nidx, nnorm uint64, normOnly bool) bool {
	if st == nil || (st.w[6]&flagAccepted) == 0 {
		return false
	}
	if st.w[0] > PhaseDone {
		return false
	}
	if npos != st.w[4] || nnorm != st.w[4] {
		return false
	}
	if !normOnly {
		if nidx != st.w[5] {
			return false
		}
	} else if st.w[5] != 0 && nidx != st.w[5] {
		return false
	}
	nv, ok := mul3(st.w[2])
	if !ok || nv != st.w[4] {
		return false
	}
	nt, ok := mul3(st.w[3])
	if !ok {
		return false
	}
	if !normOnly {
		if nt != st.w[5] {
			return false
		}
	} else if st.w[3] != 0 && nt != st.w[5] {
		return false
	}
	if (st.w[6] & ^uint64(flagAccepted|flagNormOnly)) != 0 {
		return false
	}
	if (st.w[6]&flagNormOnly) != 0 && st.w[0] != PhaseNorm && st.w[0] != PhaseDone {
		return false
	}
	return cursorOK(st.w[0], st.w[1], st.w[2], st.w[3])
}

func cursorOK(phase, cursor, nverts, ntris uint64) bool {
	switch phase {
	case PhaseZero, PhaseNorm:
		return cursor <= nverts
	case PhaseAccum:
		return cursor <= ntris
	case PhaseDone:
		return cursor == 0
	default:
		return false
	}
}

func normalsDomain(positions []float32, indices []uint32, normals []float32) bool {
	if len(positions) == 0 || len(positions)%3 != 0 || len(positions) != len(normals) {
		return false
	}
	if !finite32All(positions) {
		return false
	}
	nverts := uint64(len(positions) / 3)
	if !indicesOK(indices, nverts) {
		return false
	}
	ntris := uint64(len(indices) / 3)
	if ntris > domainMaxNtris {
		return false
	}
	if maxAbs32(positions) > contributionBound(ntris) {
		return false
	}
	return true
}

func NormalsBeginCost(nPos, nIdx int) uint64 {
	c := QuotaAdmission + QuotaBindingGuard
	if nPos > 0 {
		add, ok := mulU(uint64(nPos), QuotaBeginPos)
		if !ok {
			return ^uint64(0)
		}
		c, ok = addU(c, add)
		if !ok {
			return ^uint64(0)
		}
	}
	if nIdx > 0 {
		add, ok := mulU(uint64(nIdx), QuotaBeginIdx)
		if !ok {
			return ^uint64(0)
		}
		c, ok = addU(c, add)
		if !ok {
			return ^uint64(0)
		}
	}
	return c
}

func NormalizeBeginCost(nNor int) uint64 {
	return NormalsBeginCost(nNor, 0)
}

type stepPlan struct {
	cost   uint64
	phase  uint64
	cursor uint64
}

func charge(dst *uint64, add uint64) bool {
	n, ok := addU(*dst, add)
	if !ok {
		return false
	}
	*dst = n
	return true
}

func minU(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func chargeGrouped(cost *uint64, n, blockCost, oneCost uint64) bool {
	n4 := n / 4
	tail := n % 4
	if n4 > 0 {
		add, ok := mulU(n4, blockCost)
		if !ok || !charge(cost, add) {
			return false
		}
	}
	if tail > 0 {
		add, ok := mulU(tail, oneCost)
		if !ok || !charge(cost, add) {
			return false
		}
	}
	return true
}

func quotaFrom(phase, cursor, nverts, ntris, budget uint64) (uint64, bool) {
	var cost uint64
	if !charge(&cost, QuotaAdmission) || !charge(&cost, QuotaBindingGuard) {
		return 0, false
	}
	if budget == 0 {
		return cost, true
	}
	used := uint64(0)
	if phase == PhaseZero {
		remV, ok := subU(nverts, cursor)
		if !ok {
			return 0, false
		}
		remB, ok := subU(budget, used)
		if !ok {
			return 0, false
		}
		n := minU(remV, remB)
		add, ok := mulU(n, QuotaZeroUnit)
		if !ok || !charge(&cost, add) {
			return 0, false
		}
		used += n
		cursor += n
		if cursor == nverts {
			phase = PhaseAccum
			cursor = 0
			if !charge(&cost, QuotaTransition) {
				return 0, false
			}
		}
	}
	if phase == PhaseAccum {
		remT, ok := subU(ntris, cursor)
		if !ok {
			return 0, false
		}
		remB, ok := subU(budget, used)
		if !ok {
			return 0, false
		}
		n := minU(remT, remB)
		if !chargeGrouped(&cost, n, QuotaAccumCharge4, QuotaAccumOne) {
			return 0, false
		}
		used += n
		cursor += n
		if cursor == ntris {
			phase = PhaseNorm
			cursor = 0
			if !charge(&cost, QuotaTransition) {
				return 0, false
			}
		}
	}
	if phase == PhaseNorm {
		remV, ok := subU(nverts, cursor)
		if !ok {
			return 0, false
		}
		remB, ok := subU(budget, used)
		if !ok {
			return 0, false
		}
		n := minU(remV, remB)
		if n > 0 {
			if !charge(&cost, QuotaLastStore) {
				return 0, false
			}
		}
		if !chargeGrouped(&cost, n, QuotaNormCharge4, QuotaNormOne) {
			return 0, false
		}
		cursor += n
		if cursor == nverts {
			if !charge(&cost, QuotaTransition) {
				return 0, false
			}
		}
	}
	return cost, true
}

func guardCompute(pos []float32, idx []uint32, nor []float32, st *NormState, budget uint64) bool {
	phase, cursor := st.w[0], st.w[1]
	nverts, ntris := st.w[2], st.w[3]
	nnorm := uint64(len(nor))
	used := uint64(0)
	if budget == 0 {
		return true
	}
	if phase == PhaseZero {
		for used < budget && cursor < nverts {
			if !range3(cursor, nnorm) {
				return false
			}
			cursor++
			used++
		}
		if cursor == nverts {
			phase = PhaseAccum
			cursor = 0
		}
	}
	if phase == PhaseAccum {
		for used < budget && cursor < ntris {
			remainT, ok := subU(ntris, cursor)
			if !ok {
				return false
			}
			remainB, ok := subU(budget, used)
			if !ok {
				return false
			}
			if remainT >= 4 && remainB >= 4 {
				for k := uint64(0); k < 4; k++ {
					t, ok := addU(cursor, k)
					if !ok || !triangleLive(idx, t, nverts, pos) {
						return false
					}
				}
				c, ok := addU(cursor, 4)
				if !ok {
					return false
				}
				cursor = c
				u, ok := addU(used, 4)
				if !ok {
					return false
				}
				used = u
			} else {
				if !triangleLive(idx, cursor, nverts, pos) {
					return false
				}
				cursor++
				used++
			}
		}
		if cursor == ntris {
			phase = PhaseNorm
			cursor = 0
		}
	}
	if phase == PhaseNorm {
		for used < budget && cursor < nverts {
			remainV, ok := subU(nverts, cursor)
			if !ok {
				return false
			}
			remainB, ok := subU(budget, used)
			if !ok {
				return false
			}
			if remainV >= 4 && remainB >= 4 {
				for k := uint64(0); k < 4; k++ {
					v, ok := addU(cursor, k)
					if !ok || !range3(v, nnorm) || !tripletFinite(nor, v) {
						return false
					}
				}
				c, ok := addU(cursor, 4)
				if !ok {
					return false
				}
				cursor = c
				u, ok := addU(used, 4)
				if !ok {
					return false
				}
				used = u
			} else {
				if !range3(cursor, nnorm) || !tripletFinite(nor, cursor) {
					return false
				}
				cursor++
				used++
			}
		}
	}
	return true
}

func planCompute(pos []float32, idx []uint32, nor []float32, st *NormState, budget uint64) (stepPlan, bool) {
	if !guardCompute(pos, idx, nor, st, budget) {
		return stepPlan{}, false
	}
	cost, ok := quotaFrom(st.w[0], st.w[1], st.w[2], st.w[3], budget)
	if !ok {
		return stepPlan{}, false
	}
	return stepPlan{cost: cost, phase: st.w[0], cursor: st.w[1]}, true
}

func subU(a, b uint64) (uint64, bool) {
	if b > a {
		return 0, false
	}
	return a - b, true
}

func planNormalize(nor []float32, st *NormState, budget uint64) (stepPlan, bool) {
	p := stepPlan{phase: st.w[0], cursor: st.w[1]}
	if !charge(&p.cost, QuotaAdmission) || !charge(&p.cost, QuotaBindingGuard) {
		return p, false
	}
	if p.phase != PhaseNorm && p.phase != PhaseDone {
		return p, false
	}
	nverts := st.w[2]
	nnorm := uint64(len(nor))
	used := uint64(0)
	if budget == 0 {
		return p, true
	}
	if p.phase == PhaseNorm {
		normed := false
		for used < budget && p.cursor < nverts {
			remainV, ok := subU(nverts, p.cursor)
			if !ok {
				return p, false
			}
			remainB, ok := subU(budget, used)
			if !ok {
				return p, false
			}
			if remainV >= 4 && remainB >= 4 {
				for k := uint64(0); k < 4; k++ {
					v, ok := addU(p.cursor, k)
					if !ok || !range3(v, nnorm) || !tripletFinite(nor, v) {
						return p, false
					}
				}
				if !charge(&p.cost, QuotaNormCharge4) {
					return p, false
				}
				c, ok := addU(p.cursor, 4)
				if !ok {
					return p, false
				}
				p.cursor = c
				u, ok := addU(used, 4)
				if !ok {
					return p, false
				}
				used = u
				normed = true
			} else {
				if !range3(p.cursor, nnorm) || !tripletFinite(nor, p.cursor) {
					return p, false
				}
				if !charge(&p.cost, QuotaNormOne) {
					return p, false
				}
				p.cursor++
				used++
				normed = true
			}
		}
		if normed {
			if !charge(&p.cost, QuotaLastStore) {
				return p, false
			}
		}
		if p.cursor == nverts {
			p.phase = PhaseDone
			p.cursor = 0
			if !charge(&p.cost, QuotaTransition) {
				return p, false
			}
		}
	}
	return p, true
}

func NormalsBegin(positions []float32, indices []uint32, normals []float32, state *NormState) bool {
	if state == nil || !normalsDomain(positions, indices, normals) {
		return false
	}
	if !aliasOK(positions, indices, normals, state) {
		return false
	}
	if Archtime_normals_begin(uint64(len(positions)), uint64(len(indices)), uint64(len(normals)), state.raw(), stateLen, 0) == 0 {
		return false
	}
	state.bind(positions, indices, normals)
	state.w[7] = NormalsBeginCost(len(positions), len(indices))
	return true
}

func NormalsStep(positions []float32, indices []uint32, normals []float32, state *NormState, workBudget uint64) (done, ok bool) {
	if state == nil {
		return false, false
	}
	defer runtime.KeepAlive(positions)
	defer runtime.KeepAlive(indices)
	defer runtime.KeepAlive(normals)
	defer runtime.KeepAlive(state)
	if (state.w[6] & flagNormOnly) != 0 {
		return NormalizeStep(normals, state, workBudget)
	}
	if !state.sameCompute(positions, indices, normals) {
		return false, false
	}
	npos, nidx, nnorm := uint64(len(positions)), uint64(len(indices)), uint64(len(normals))
	if !stateDimsOK(state, npos, nidx, nnorm, false) {
		return false, false
	}
	if !aliasOK(positions, indices, normals, state) {
		return false, false
	}
	plan, okp := planCompute(positions, indices, normals, state, workBudget)
	if !okp {
		return false, false
	}
	if cNormalsStep(positions, indices, normals, state, workBudget) == 0 {
		return false, false
	}
	state.w[7] = plan.cost
	return state.w[0] == PhaseDone, true
}

func NormalizeBegin(normals []float32, state *NormState) bool {
	if state == nil || len(normals) == 0 || len(normals)%3 != 0 || !finite32All(normals) {
		return false
	}
	if overlapBytes(stateBytes(state), f32Bytes(normals)) {
		return false
	}
	if Archtime_normals_begin(uint64(len(normals)), 0, uint64(len(normals)), state.raw(), stateLen, flagNormOnly) == 0 {
		return false
	}
	state.bind(nil, nil, normals)
	state.w[7] = NormalizeBeginCost(len(normals))
	return true
}

func NormalizeStep(normals []float32, state *NormState, workBudget uint64) (done, ok bool) {
	if state == nil {
		return false, false
	}
	defer runtime.KeepAlive(normals)
	defer runtime.KeepAlive(state)
	if !state.sameNorm(normals) {
		return false, false
	}
	nnorm := uint64(len(normals))
	normOnly := (state.w[6] & flagNormOnly) != 0
	if !stateDimsOK(state, nnorm, state.w[5], nnorm, normOnly || state.w[0] == PhaseNorm || state.w[0] == PhaseDone) {
		return false, false
	}
	if state.w[0] != PhaseNorm && state.w[0] != PhaseDone {
		return false, false
	}
	if overlapBytes(stateBytes(state), f32Bytes(normals)) {
		return false, false
	}
	plan, okp := planNormalize(normals, state, workBudget)
	if !okp {
		return false, false
	}
	if cNormalsStep(nil, nil, normals, state, workBudget) == 0 {
		return false, false
	}
	state.w[7] = plan.cost
	return state.w[0] == PhaseDone, true
}
