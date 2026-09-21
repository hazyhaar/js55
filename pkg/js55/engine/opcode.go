package engine

// Jeu d'opcodes du moteur (plan §J4).
//
// L'énumération est DENSE et CONTIGUË : aucune valeur n'est sautée, aucune n'est
// assignée explicitement. C'est la condition posée par la contrainte C2 pour que
// le commutateur de la boucle d'interprétation reçoive une table de saut — le
// compilateur exige au moins 8 cas et une densité d'au moins 1/4
// (walk/switch.go:302-303, mesuré). La garde vit dans pkg/js55/csgguard.
//
// L'ancien jeu déclarait 31 opcodes pour 20 traités : les onze restants, dont
// toutes les comparaisons et l'appel de fonction, retournaient une erreur, et
// OpJumpIfFalse consommait une condition qu'aucun opcode ne pouvait produire.
// Le présent jeu n'admet aucun opcode déclaré sans implantation ; un test le
// vérifie.

// Op est un opcode. Un octet, conformément à C1.
type Op uint8

const (
	OpNop Op = iota

	// Constantes
	OpUndefined
	OpNull
	OpTrue
	OpFalse
	OpInt32 // i32 immédiat
	OpConst // u32 : indice dans la table de constantes

	// Pile
	OpPop
	OpDup

	// Arithmétique
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpPow
	OpNeg
	OpPos

	// Bits
	OpBitAnd
	OpBitOr
	OpBitXor
	OpBitNot
	OpShl
	OpShr
	OpUShr

	// Comparaisons
	OpEq
	OpNe
	OpStrictEq
	OpStrictNe
	OpLt
	OpGt
	OpLe
	OpGe
	OpInstanceof
	OpIn

	// Logique et interrogation
	OpNot
	OpTypeof
	OpIsNullish

	OpArraySlice
	OpArrayPush
	OpArraySpread
	OpObjectRest
	OpCallSpread
	OpCallSpreadThis
	OpObjectAssign

	// Variables. L'opérande porte DEUX champs sur 32 bits : profondeur
	// d'environnement dans les 16 bits hauts, emplacement dans les 16 bits bas.
	// C'est ce qui donne de vraies fermetures sans opcode supplémentaire.
	OpGetLocal  // u32 : profondeur<<16 | emplacement
	OpSetLocal  // u32
	OpGetGlobal // u32 : constante portant le nom
	OpSetGlobal // u32
	OpDefGlobal // u32

	// Propriétés
	OpGetProp // u32 : constante portant le nom
	OpSetProp // u32
	OpGetElem
	OpSetElem

	// Construction
	OpNewObject
	OpNewArray // u32 : nombre d'éléments à dépiler

	// Contrôle
	OpJump        // i32 : déplacement relatif
	OpJumpIfFalse // i32
	OpJumpIfTrue  // i32
	OpCall        // u16 : nombre d'arguments
	OpCallMethod  // u32 : argc<<16 | constIndex
	OpNew         // u16 : nombre d'arguments
	OpThis        // charge la valeur de this
	OpReturn
	OpThrow
	OpHalt
	OpGetGlobalSoft
	OpYield
	OpGetIterator
	OpDefAccessor
	OpAwait
	OpSuperGet
	OpDelete
	OpSuperCall
	OpSetProto
	OpNewTarget
	OpSuperCallSpread
	OpNewSpread
	OpExport
	OpSetHomeObject

	numOps
)

// NumOps borne l'énumération.
const NumOps = int(numOps)

// opInfo décrit un opcode : son nom et la largeur de son opérande en octets.
type opInfo struct {
	name  string
	width uint8 // 0, 2 ou 4
}

var opTable = [numOps]opInfo{
	OpSetHomeObject:   {"sethomeobject", 0},
	OpNop:             {"nop", 0},
	OpUndefined:       {"undefined", 0},
	OpNull:            {"null", 0},
	OpTrue:            {"true", 0},
	OpFalse:           {"false", 0},
	OpInt32:           {"int32", 4},
	OpConst:           {"const", 4},
	OpPop:             {"pop", 0},
	OpDup:             {"dup", 0},
	OpAdd:             {"add", 0},
	OpSub:             {"sub", 0},
	OpMul:             {"mul", 0},
	OpDiv:             {"div", 0},
	OpMod:             {"mod", 0},
	OpPow:             {"pow", 0},
	OpNeg:             {"neg", 0},
	OpPos:             {"pos", 0},
	OpBitAnd:          {"bitand", 0},
	OpBitOr:           {"bitor", 0},
	OpBitXor:          {"bitxor", 0},
	OpBitNot:          {"bitnot", 0},
	OpShl:             {"shl", 0},
	OpShr:             {"shr", 0},
	OpUShr:            {"ushr", 0},
	OpEq:              {"eq", 0},
	OpNe:              {"ne", 0},
	OpStrictEq:        {"stricteq", 0},
	OpStrictNe:        {"strictne", 0},
	OpLt:              {"lt", 0},
	OpGt:              {"gt", 0},
	OpLe:              {"le", 0},
	OpGe:              {"ge", 0},
	OpInstanceof:      {"instanceof", 0},
	OpIn:              {"in", 0},
	OpNot:             {"not", 0},
	OpTypeof:          {"typeof", 0},
	OpIsNullish:       {"isnullish", 0},
	OpArraySlice:      {"arrayslice", 0},
	OpArrayPush:       {"arraypush", 0},
	OpArraySpread:     {"arrayspread", 0},
	OpObjectRest:      {"objectrest", 0},
	OpCallSpread:      {"callspread", 0},
	OpCallSpreadThis:  {"callspreadthis", 0},
	OpObjectAssign:    {"objectassign", 0},
	OpGetLocal:        {"getlocal", 4},
	OpSetLocal:        {"setlocal", 4},
	OpGetGlobal:       {"getglobal", 4},
	OpSetGlobal:       {"setglobal", 4},
	OpDefGlobal:       {"defglobal", 4},
	OpGetProp:         {"getprop", 4},
	OpSetProp:         {"setprop", 4},
	OpGetElem:         {"getelem", 0},
	OpSetElem:         {"setelem", 0},
	OpNewObject:       {"newobject", 0},
	OpNewArray:        {"newarray", 4},
	OpJump:            {"jump", 4},
	OpJumpIfFalse:     {"jumpiffalse", 4},
	OpJumpIfTrue:      {"jumpiftrue", 4},
	OpCall:            {"call", 2},
	OpCallMethod:      {"callmethod", 4},
	OpNew:             {"new", 2},
	OpThis:            {"this", 0},
	OpReturn:          {"return", 0},
	OpThrow:           {"throw", 0},
	OpHalt:            {"halt", 0},
	OpGetGlobalSoft:   {"getglobalsoft", 4},
	OpYield:           {"yield", 2},
	OpGetIterator:     {"getiterator", 0},
	OpDefAccessor:     {"defaccessor", 4},
	OpAwait:           {"await", 0},
	OpSuperGet:        {"superget", 4},
	OpDelete:          {"delete", 4},
	OpSuperCall:       {"supercall", 2},
	OpSetProto:        {"setproto", 0},
	OpNewTarget:       {"newtarget", 0},
	OpSuperCallSpread: {"supercallspread", 0},
	OpNewSpread:       {"newspread", 0},
	OpExport:          {"export", 4},
}

// Name rend le nom de l'opcode.
func (o Op) Name() string {
	if int(o) >= NumOps {
		return "op(?)"
	}
	return opTable[o].name
}

// Width rend la largeur de l'opérande en octets.
func (o Op) Width() int {
	if int(o) >= NumOps {
		return 0
	}
	return int(opTable[o].width)
}

func (o Op) String() string { return o.Name() }
