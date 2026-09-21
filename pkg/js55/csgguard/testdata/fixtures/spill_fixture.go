// SPDX-License-Identifier: BUSL-1.1
package csgfixtures

// Live10 — dix valeurs 64 bits vivantes dans la boucle. Contrainte C3 : le cadre
// doit rester au plancher, sans emplacement de spill.
func Live10(code []byte, n int) uint64 {
	var v0, v1, v2, v3, v4, v5, v6, v7, v8, v9 uint64
	for i := 0; i < n; i++ {
		x := uint64(code[i&1023])
		v0 += x
		v1 ^= v0*31 + x
		v2 ^= v1*31 + x
		v3 ^= v2*31 + x
		v4 ^= v3*31 + x
		v5 ^= v4*31 + x
		v6 ^= v5*31 + x
		v7 ^= v6*31 + x
		v8 ^= v7*31 + x
		v9 ^= v8*31 + x
	}
	return v0 ^ v1 ^ v2 ^ v3 ^ v4 ^ v5 ^ v6 ^ v7 ^ v8 ^ v9
}

// Live16 — seize valeurs vivantes. Au-delà du seuil : le cadre grossit.
// Présent pour prouver que Live10 mesure bien quelque chose.
func Live16(code []byte, n int) uint64 {
	var v0, v1, v2, v3, v4, v5, v6, v7, v8, v9, va, vb, vc, vd, ve, vf uint64
	for i := 0; i < n; i++ {
		x := uint64(code[i&1023])
		v0 += x
		v1 ^= v0*31 + x
		v2 ^= v1*31 + x
		v3 ^= v2*31 + x
		v4 ^= v3*31 + x
		v5 ^= v4*31 + x
		v6 ^= v5*31 + x
		v7 ^= v6*31 + x
		v8 ^= v7*31 + x
		v9 ^= v8*31 + x
		va ^= v9*31 + x
		vb ^= va*31 + x
		vc ^= vb*31 + x
		vd ^= vc*31 + x
		ve ^= vd*31 + x
		vf ^= ve*31 + x
	}
	return v0 ^ v1 ^ v2 ^ v3 ^ v4 ^ v5 ^ v6 ^ v7 ^ v8 ^ v9 ^ va ^ vb ^ vc ^ vd ^ ve ^ vf
}
