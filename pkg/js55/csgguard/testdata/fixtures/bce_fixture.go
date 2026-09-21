package csgfixtures

// DecodeReslice — contrainte C4, idiome retenu. Un seul contrôle de bornes.
func DecodeReslice(code []byte, pc int) uint32 {
	c := code[pc : pc+4 : pc+4]
	return uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16 | uint32(c[3])<<24
}

// DecodeBounded — variante équivalente sur tranche bornée, un seul contrôle.
func DecodeBounded(code []byte, pc int) uint32 {
	c := code[pc : pc+4]
	return uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16 | uint32(c[3])<<24
}

// DecodeAssertIdiom — idiome CSG-020 appliqué au décodage multi-octets.
// Quatre contrôles : l'assertion elle-même plus les trois indexations qu'elle
// n'affranchit pas. C'est la mesure qui a fait retirer cet idiome du chemin chaud.
func DecodeAssertIdiom(code []byte, pc int) uint32 {
	_ = code[pc+3]
	return uint32(code[pc]) | uint32(code[pc+1])<<8 | uint32(code[pc+2])<<16 | uint32(code[pc+3])<<24
}

// LoopAscending — boucle ascendante sur len. Contrôle éliminé.
func LoopAscending(b []byte) uint32 {
	var s uint32
	for i := 0; i < len(b); i++ {
		s += uint32(b[i])
	}
	return s
}

// LoopDescending — contrainte C5. La règle CSG-022 prétendait que loopbce échoue
// ici. Mesure contraire sur go1.27 : le contrôle est éliminé. Ce test garde la
// correction : s'il redevient rouge, CSG-022 redevient vraie et C5 doit être revue.
func LoopDescending(b []byte) uint32 {
	var s uint32
	for i := len(b) - 1; i >= 0; i-- {
		s += uint32(b[i])
	}
	return s
}
