package engine

import (
	"fmt"
	"math"
	"strconv"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// Tas du moteur, modèle d'objets à formes et ramasse-miettes (plan §J3).
//
// Le tas est une TRANCHE Go ordinaire, et une Value ne porte qu'un indice dans
// cette tranche. Tout objet reste donc visible du ramasse-miettes de la
// plateforme, et le marquage-balayage du moteur est précis par construction :
// il n'a aucun pointeur à deviner.
//
// Le ramasse-miettes se teste EN PREMIER, pas en dernier. Le mode stress
// ci-dessous — une collecte complète à chaque allocation — est le seul
// instrument qui trouve les défauts d'enracinement, et il est en place avant
// que le moteur n'ait un seul objet à gérer.

// ObjKind identifie la nature d'un objet. Dense et contiguë (contrainte C2).
type ObjKind uint8

const (
	KindOrdinary ObjKind = iota
	KindArray
	KindFunction
	KindStringObject
	KindError
	// KindString est une chaîne PRIMITIVE. Elle vit dans le tas pour que le
	// ramasse-miettes s'en occupe, mais « typeof » rend « string » et non
	// « object » : c'est le genre, non la nature de cellule, qui décide.
	KindString
	// KindEnv est un environnement lexical : ses éléments sont les variables,
	// son prototype est l'environnement englobant. Réutiliser l'objet plutôt
	// que d'inventer une structure donne les fermetures et leur ramassage sans
	// une ligne de plus dans le collecteur.
	KindEnv
	KindMap
	KindSet
	KindRegExp
	KindPromise
	KindProxy
	KindBigInt
	KindGenerator
	KindAccessor
	KindWeakMap
	KindSymbol
	KindArguments

	numObjKinds
)

func (k ObjKind) String() string {
	switch k {
	case KindOrdinary:
		return "Ordinary"
	case KindArray:
		return "Array"
	case KindFunction:
		return "Function"
	case KindStringObject:
		return "StringObject"
	case KindError:
		return "Error"
	case KindString:
		return "String"
	case KindEnv:
		return "Env"
	case KindMap:
		return "Map"
	case KindSet:
		return "Set"
	case KindRegExp:
		return "RegExp"
	case KindPromise:
		return "Promise"
	case KindProxy:
		return "Proxy"
	case KindBigInt:
		return "BigInt"
	case KindGenerator:
		return "Generator"
	case KindAccessor:
		return "Accessor"
	case KindWeakMap:
		return "WeakMap"
	case KindSymbol:
		return "Symbol"
	case KindArguments:
		return "Arguments"
	}
	return "ObjKind(?)"
}

// NumObjKinds borne l'énumération, pour les gardes.
const NumObjKinds = int(numObjKinds)

// ─── Formes ─────────────────────────────────────────────────────────────────

// Shape décrit la disposition des propriétés d'un objet — ce que la littérature
// appelle hidden class. Deux objets ayant reçu les mêmes clés dans le même ordre
// partagent la MÊME forme, ce qui rend un accès de propriété résoluble par une
// seule comparaison de pointeur au jalon 6.
//
// Les clés sont des chaînes INTERNÉES : la comparaison se fait par identité de
// pointeur, jamais par contenu. C'est la raison d'être de l'internement du
// jalon 2.
type Shape struct {
	parent      *Shape
	key         *str.String
	slot        int
	count       int
	transitions map[*str.String]*Shape
}

// Count rend le nombre de propriétés portées par la forme.
func (s *Shape) Count() int { return s.count }

// Lookup rend l'emplacement de key dans la forme, ou -1. La comparaison est une
// égalité de pointeurs : key doit être internée.
func (s *Shape) Lookup(key *str.String) int {
	for c := s; c != nil && c.key != nil; c = c.parent {
		if c.key == key {
			return c.slot
		}
	}
	return -1
}

// Keys rend les clés dans l'ordre d'insertion.
func (s *Shape) Keys() []*str.String {
	out := make([]*str.String, s.count)
	for c := s; c != nil && c.key != nil; c = c.parent {
		out[c.slot] = c.key
	}
	return out
}

// transition rend la forme obtenue en ajoutant key. Les transitions sont
// mémorisées : deux objets construits de la même façon convergent vers la même
// forme, ce qui est tout l'intérêt du modèle.
func (s *Shape) transition(key *str.String) *Shape {
	if next, ok := s.transitions[key]; ok {
		return next
	}
	next := &Shape{
		parent:      s,
		key:         key,
		slot:        s.count,
		count:       s.count + 1,
		transitions: map[*str.String]*Shape{},
	}
	if s.transitions == nil {
		s.transitions = map[*str.String]*Shape{}
	}
	s.transitions[key] = next
	return next
}

// ─── Objets ─────────────────────────────────────────────────────────────────

// Object est un objet du tas. Les champs non pertinents pour son genre restent
// nuls ; le genre est la seule source de vérité.
type Object struct {
	kind   ObjKind
	marked bool
	shape  *Shape
	slots  []Value
	// elements porte les éléments denses d'un tableau. Une case absente vaut
	// Undefined ; les tableaux creux ne sont pas encore modélisés.
	elements []Value
	// Typed views reference the ArrayBuffer in env; only the buffer owns bytes.
	typedName      string
	bytes          []byte
	arrayBuffer    bool
	resizable      bool
	maxByteLength  int
	byteOffset     int
	typedLength    int
	lengthTracking bool
	proto          Handle
	// text porte la chaîne d'une cellule chaîne. Elle est enracinée par l'objet.
	text *str.String
	// fn et env ne valent que pour un objet fonction : l'unité compilée et
	// l'environnement de DÉFINITION, qui est ce qui fait la fermeture.
	fn         *Chunk
	homeObject Handle
	env        Handle
	mapData    *MapData
	setData    *SetData
	regexp     *RegExpData
	promise    *PromiseData
	proxy      *ProxyData
	bigInt     int64
	prim       Value
	gen        *GenState
	frozen     bool
	deleted    []*str.String
	// attrs est parallèle à slots. Une tranche plus courte vaut attrDefault
	// (inscriptible, énumérable, configurable) pour chaque emplacement manquant.
	attrs []uint8
}

const (
	attrWritable     uint8 = 1 << 0
	attrEnumerable   uint8 = 1 << 1
	attrConfigurable uint8 = 1 << 2
	attrDefault            = attrWritable | attrEnumerable | attrConfigurable
)

func (o *Object) slotAttr(i int) uint8 {
	if i < 0 || i >= len(o.attrs) {
		return attrDefault
	}
	return o.attrs[i]
}

func (o *Object) ensureAttrs() {
	for len(o.attrs) < len(o.slots) {
		o.attrs = append(o.attrs, attrDefault)
	}
}

// Fn rend l'unité compilée portée par un objet fonction.
func (o *Object) Fn() *Chunk { return o.fn }

// Env rend l'environnement de définition d'un objet fonction.
func (o *Object) Env() Handle { return o.env }

// Kind rend le genre de l'objet.
func (o *Object) Kind() ObjKind { return o.kind }

// Shape rend la forme de l'objet.
func (o *Object) Shape() *Shape { return o.shape }

// Text rend la chaîne portée par un objet chaîne, ou nil.
func (o *Object) Text() *str.String { return o.text }

// Elements rend les éléments denses d'un tableau.
func (o *Object) Elements() []Value { return o.elements }

// ─── Tas ────────────────────────────────────────────────────────────────────

// Heap possède les objets, les formes et la table d'internement. Il n'est PAS
// sûr en concurrence : chaque isolat possède le sien, ce qui est aussi la raison
// pour laquelle deux isolats ne partagent jamais un objet.
type Heap struct {
	objs []*Object
	// gens porte la génération de chaque emplacement. Elle est incrémentée à la
	// libération, ce qui invalide tout handle qui pointait encore dessus.
	gens []uint16
	free []uint32

	// roots porte les racines EXPLICITES : l'adresse de chaque Value que
	// l'appelant veut voir survivre. Aucune racine n'est devinée.
	roots []*Value
	// stack est la pile de valeurs du moteur, enracinée dans sa totalité.
	stack []Value
	// scanners énumèrent des racines que le tas ne peut pas connaître seul :
	// les environnements des trames d'appel et les variables globales de
	// l'interpréteur. Sans eux, une fonction globale ou une fermeture en cours
	// d'appel est collectée sous les pieds de la boucle — défaut trouvé par le
	// mode stress au jalon 4, avec pour symptômes « emplacement local hors
	// borne » et « [object Released] is not a function ».
	scanners []func(visit func(Value))

	intern    *str.Table
	rootShape *Shape

	stress      bool
	allocs      int
	threshold   int
	Collections int
	// Swept compte les objets libérés depuis la création du tas.
	Swept int

	// QuotaTracker est appelé sur chaque allocation pour contrôler les quotas mémoire.
	QuotaTracker  func(bytes int64) error
	objectProto   Handle
	arrayProto    Handle
	functionProto Handle
}

// TrackAlloc notifie le gestionnaire de quota et panique si la limite est dépassée.
func (h *Heap) TrackAlloc(bytes int64) {
	if h.QuotaTracker != nil && bytes > 0 {
		if err := h.QuotaTracker(bytes); err != nil {
			panic(err)
		}
	}
}

// TrackFree notifie le gestionnaire de quota de la libération de mémoire.
func (h *Heap) TrackFree(bytes int64) {
	if h.QuotaTracker != nil && bytes > 0 {
		_ = h.QuotaTracker(-bytes)
	}
}

// NewHeap crée un tas vide. Le handle zéro est réservé : l'emplacement 0 ne
// désigne jamais un objet vivant.
func NewHeap() *Heap {
	return &Heap{
		objs:      []*Object{nil},
		gens:      []uint16{0},
		intern:    str.NewTable(),
		rootShape: &Shape{transitions: map[*str.String]*Shape{}},
		threshold: 1024,
	}
}

// SetStress active la collecte à CHAQUE allocation. C'est l'instrument T3.1 :
// la totalité de la suite doit passer dans ce mode, et un écart entre les deux
// modes est un défaut d'enracinement, jamais une tolérance.
func (h *Heap) SetStress(v bool) { h.stress = v }

// Stress indique si le mode stress est actif.
func (h *Heap) Stress() bool { return h.stress }

// Intern rend la table d'internement des clés.
func (h *Heap) Intern() *str.Table { return h.intern }

// Live rend le nombre d'objets vivants.
func (h *Heap) Live() int { return len(h.objs) - 1 - len(h.free) }

// Get rend l'objet désigné par h, ou nil. Un handle libéré ou hors borne rend
// nil plutôt que de paniquer : c'est à l'appelant de décider si l'absence est
// une erreur.
func (hp *Heap) Get(h Handle) *Object {
	if h == NoHandle {
		return nil
	}
	i := int(h.Index())
	if i <= 0 || i >= len(hp.objs) {
		return nil
	}
	// La génération départage un emplacement réattribué d'un handle encore
	// valide : sans ce contrôle, un handle périmé désignerait le NOUVEAU
	// locataire de l'emplacement.
	if hp.gens[i] != h.Gen() {
		return nil
	}
	return hp.objs[i]
}

// MustGet rend l'objet, ou panique. Sert aux chemins où un handle invalide est
// un défaut du moteur et non une entrée de l'utilisateur.
func (hp *Heap) MustGet(h Handle) *Object {
	o := hp.Get(h)
	if o == nil {
		panic(fmt.Sprintf("js55/engine: handle %d invalide", h))
	}
	return o
}

// ─── Racines ────────────────────────────────────────────────────────────────

// AddScanner enregistre une source de racines énumérée à chaque collecte.
func (h *Heap) AddScanner(f func(visit func(Value))) { h.scanners = append(h.scanners, f) }

// AddRoot enracine l'adresse d'une Value. La valeur pointée survivra à toute
// collecte tant que la racine n'est pas retirée.
func (h *Heap) AddRoot(v *Value) { h.roots = append(h.roots, v) }

// RemoveRoot retire une racine posée par AddRoot.
func (h *Heap) RemoveRoot(v *Value) {
	for i, r := range h.roots {
		if r == v {
			h.roots = append(h.roots[:i], h.roots[i+1:]...)
			return
		}
	}
}

// Push empile une valeur sur la pile enracinée.
func (h *Heap) Push(v Value) { h.stack = append(h.stack, v) }

// Pop dépile.
func (h *Heap) Pop() Value {
	if len(h.stack) == 0 {
		return Undefined
	}
	v := h.stack[len(h.stack)-1]
	h.stack = h.stack[:len(h.stack)-1]
	return v
}

// StackLen rend la hauteur de la pile.
func (h *Heap) StackLen() int { return len(h.stack) }

// TruncateStack ramène la pile à n éléments.
func (h *Heap) TruncateStack(n int) {
	if n < 0 {
		n = 0
	}
	if n < len(h.stack) {
		h.stack = h.stack[:n]
	}
}

// ─── Allocation ─────────────────────────────────────────────────────────────

// alloc réserve un emplacement. En mode stress, une collecte complète précède
// CHAQUE allocation : c'est ce qui déplace un défaut d'enracinement d'un
// comportement erratique vers un échec reproductible.
func (h *Heap) alloc(o *Object) Handle {
	h.TrackAlloc(96) // Taille de base d'un objet struct Object

	if h.stress {
		h.Collect()
	} else {
		h.allocs++
		if h.allocs >= h.threshold {
			h.Collect()
			h.allocs = 0
			if live := h.Live(); live*2 > h.threshold {
				h.threshold = live * 2
			}
		}
	}

	if n := len(h.free); n > 0 {
		i := h.free[n-1]
		h.free = h.free[:n-1]
		h.objs[i] = o
		return makeHandle(i, h.gens[i])
	}
	h.objs = append(h.objs, o)
	h.gens = append(h.gens, 0)
	i := uint32(len(h.objs) - 1)
	return makeHandle(i, 0)
}

// NewObject alloue un objet ordinaire vide.
func (h *Heap) NewObject() Handle {
	return h.alloc(&Object{kind: KindOrdinary, shape: h.rootShape, proto: h.objectProto})
}

func valueSlotBytes(n int) (int64, bool) {
	if n <= 0 {
		return 0, true
	}
	if int64(n) > math.MaxInt64/8 {
		return math.MaxInt64, false
	}
	return int64(n) * 8, true
}

func (h *Heap) NewArray(n int) Handle {
	if n < 0 {
		n = 0
	}
	charge, ok := valueSlotBytes(n)
	if charge > 0 {
		h.TrackAlloc(charge)
	}
	if !ok {
		panic("js55: array length exceeds host size")
	}
	els := make([]Value, n)
	for i := range els {
		els[i] = Undefined
	}
	return h.alloc(&Object{kind: KindArray, shape: h.rootShape, elements: els, proto: h.arrayProto})
}

// NewStringObject alloue un objet chaîne. La chaîne est enracinée par l'objet.
func (h *Heap) NewStringObject(s *str.String) Handle {
	return h.alloc(&Object{kind: KindStringObject, shape: h.rootShape,
		text: h.intern.Intern(s), proto: NoHandle})
}

// NewFunction alloue un objet fonction fermant sur env.
func (h *Heap) NewFunction(fn *Chunk, env Handle) Handle {
	return h.alloc(&Object{kind: KindFunction, shape: h.rootShape,
		fn: fn, env: env, proto: h.functionProto})
}

// NewString alloue une cellule de chaîne primitive.
func (h *Heap) NewString(s *str.String) Handle {
	if s != nil {
		h.TrackAlloc(int64(s.Len()))
	}
	return h.alloc(&Object{kind: KindString, shape: h.rootShape, text: s, proto: NoHandle})
}

// NewEnv alloue un environnement de n emplacements, fils de parent.
func (h *Heap) NewEnv(n int, parent Handle) Handle {
	committed := false
	bytes := int64(n) * 8
	if n > 0 {
		h.TrackAlloc(bytes)
	}
	// alloc also reserves the Object header. If that reservation is rejected,
	// the uninstalled slots must not remain charged to the isolate.
	defer func() {
		if !committed {
			h.TrackFree(bytes)
		}
	}()
	els := make([]Value, n)
	for i := range els {
		els[i] = Undefined
	}
	env := h.alloc(&Object{kind: KindEnv, shape: h.rootShape, elements: els, proto: parent})
	committed = true
	return env
}

// ─── Propriétés ─────────────────────────────────────────────────────────────

// SetProto fixe le prototype d'un objet.
func (h *Heap) SetProto(obj Handle, proto Handle) { h.MustGet(obj).proto = proto }

// Proto rend le prototype d'un objet.
func (h *Heap) Proto(obj Handle) Handle { return h.MustGet(obj).proto }

// SetProperty pose une propriété. La clé est internée : la forme la comparera
// par identité.
func (h *Heap) isDeleted(o *Object, k *str.String) bool {
	for _, d := range o.deleted {
		if d == k || (d != nil && k != nil && d.Equal(k)) {
			return true
		}
	}
	return false
}

func (h *Heap) DeleteProperty(obj Handle, key *str.String) bool {
	o := h.Get(obj)
	if o == nil || o.frozen {
		return false
	}
	k := h.intern.Intern(key)
	if h.isDeleted(o, k) {
		return true
	}
	if slot := o.shape.Lookup(k); slot >= 0 && o.slotAttr(slot)&attrConfigurable == 0 {
		return false
	}
	for c := o.shape; c != nil && c.key != nil; c = c.parent {
		if c.key.Equal(k) && c.slot >= 0 && c.slot < len(o.slots) {
			o.slots[c.slot] = Undefined
		}
	}
	o.deleted = append(o.deleted, k)
	if o.kind == KindArray {
		if i, err := strconv.Atoi(k.GoString()); err == nil && i >= 0 && i < len(o.elements) {
			o.elements[i] = Undefined
		}
	}
	return true
}

func (h *Heap) SetProperty(obj Handle, key *str.String, v Value) {
	o := h.MustGet(obj)
	k := h.intern.Intern(key)
	if n := len(o.deleted); n > 0 {
		alive := o.deleted[:0]
		for _, d := range o.deleted {
			if d != k {
				alive = append(alive, d)
			}
		}
		o.deleted = alive
	}

	h.maybeGrowArrayIndex(o, k)
	if i, ok := h.argumentsIndex(o, k); ok {
		if env := h.Get(o.env); env != nil && i < len(env.elements) {
			env.elements[i] = v
		}
	}
	if slot := o.shape.Lookup(k); slot >= 0 {
		if o.slotAttr(slot)&attrWritable == 0 {
			return
		}
		o.slots[slot] = v
		return
	}

	// Facturation de la transition de forme + nouveau slot de propriété
	// Un nouveau slot Value = 8 octets
	// Un nouveau nœud Shape = 32 octets
	// Une entrée dans la map transitions = ~64 octets
	h.TrackAlloc(104)

	o.shape = o.shape.transition(k)
	o.slots = append(o.slots, v)
}

// DefineDataProperty pose une propriété de données et ses attributs, y compris
// lorsque la propriété n'est pas inscriptible (voie Object.defineProperty).
func (h *Heap) DefineDataProperty(obj Handle, key *str.String, v Value, attrs uint8) {
	o := h.MustGet(obj)
	k := h.intern.Intern(key)
	if n := len(o.deleted); n > 0 {
		alive := o.deleted[:0]
		for _, d := range o.deleted {
			if d != k {
				alive = append(alive, d)
			}
		}
		o.deleted = alive
	}
	h.maybeGrowArrayIndex(o, k)
	if slot := o.shape.Lookup(k); slot >= 0 {
		o.slots[slot] = v
		o.ensureAttrs()
		o.attrs[slot] = attrs
		return
	}
	h.TrackAlloc(104)
	o.shape = o.shape.transition(k)
	o.slots = append(o.slots, v)
	o.ensureAttrs()
	o.attrs[len(o.slots)-1] = attrs
}

func (h *Heap) maybeGrowArrayIndex(o *Object, k *str.String) {
	if o == nil || o.kind != KindArray || k == nil {
		return
	}
	idx, err := strconv.Atoi(k.GoString())
	if err != nil || idx < 0 || idx >= 10000 {
		return
	}
	for len(o.elements) <= idx {
		o.elements = append(o.elements, Undefined)
	}
}

// SetPropertyAttrs met à jour les seuls attributs d'une propriété déjà présente.
func (h *Heap) SetPropertyAttrs(obj Handle, key *str.String, attrs uint8) {
	o := h.Get(obj)
	if o == nil || o.shape == nil {
		return
	}
	k := h.intern.Intern(key)
	slot := o.shape.Lookup(k)
	if slot < 0 {
		return
	}
	o.ensureAttrs()
	o.attrs[slot] = attrs
}

// GetOwnProperty rend la valeur d'une propriété propre et vrai, ou Undefined et
// faux.
func (h *Heap) argumentsIndex(o *Object, k *str.String) (int, bool) {
	if o == nil || o.kind != KindArguments || o.env == NoHandle || k == nil {
		return 0, false
	}
	i, err := strconv.Atoi(k.GoString())
	if err != nil || i < 0 {
		return 0, false
	}
	n := int(o.prim.ToInt())
	if i >= n {
		return 0, false
	}
	return i, true
}

func arrayIndexKey(k *str.String) (int, bool) {
	s := k.GoString()
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
		if n < 0 {
			return 0, false
		}
	}
	if strconv.Itoa(n) != s {
		return 0, false
	}
	return n, true
}

func (h *Heap) GetOwnProperty(obj Handle, key *str.String) (Value, bool) {
	o := h.MustGet(obj)
	k := h.intern.Intern(key)
	if i, ok := h.argumentsIndex(o, k); ok {
		if env := h.Get(o.env); env != nil && i < len(env.elements) {
			return env.elements[i], true
		}
	}
	if h.isDeleted(o, k) {
		return Undefined, false
	}
	slot := o.shape.Lookup(k)
	if slot >= 0 {
		return o.slots[slot], true
	}
	if o.kind == KindArray {
		if i, ok := arrayIndexKey(k); ok && i >= 0 && i < len(o.elements) {
			return o.elements[i], true
		}
	}
	return Undefined, false
}

// GetProperty remonte la chaîne de prototypes. La remontée est bornée par le
// nombre d'objets vivants : un cycle de prototypes rend Undefined plutôt que de
// boucler.
func (h *Heap) GetProperty(obj Handle, key *str.String) (Value, bool) {
	k := h.intern.Intern(key)
	guard := len(h.objs) + 1
	for cur := obj; cur != NoHandle && guard > 0; guard-- {
		o := h.Get(cur)
		if o == nil {
			break
		}
		if h.isDeleted(o, k) {
			cur = o.proto
			continue
		}
		if slot := o.shape.Lookup(k); slot >= 0 {
			return o.slots[slot], true
		}
		if o.kind == KindArray {
			if i, ok := arrayIndexKey(k); ok && i >= 0 && i < len(o.elements) {
				return o.elements[i], true
			}
		}
		cur = o.proto
	}
	return Undefined, false
}

// SetElement pose un élément dense. Les indices au-delà de la longueur étendent
// le tableau avec des trous à Undefined.
func (h *Heap) SetElement(obj Handle, i int, v Value) {
	o := h.MustGet(obj)
	if i < 0 {
		return
	}
	old := len(o.elements)
	if i >= old {
		if i == math.MaxInt {
			h.TrackAlloc(math.MaxInt64)
			panic("js55: array index exceeds host size")
		}
		diff := i + 1 - old
		charge, ok := valueSlotBytes(diff)
		if charge > 0 {
			h.TrackAlloc(charge)
		}
		if !ok {
			panic("js55: array growth exceeds host size")
		}
	}
	for len(o.elements) <= i {
		o.elements = append(o.elements, Undefined)
	}
	o.elements[i] = v
}

// GetElement rend un élément dense.
func (h *Heap) GetElement(obj Handle, i int) Value {
	o := h.MustGet(obj)
	if i < 0 || i >= len(o.elements) {
		return Undefined
	}
	return o.elements[i]
}

func (h *Heap) ArrayBufferBytes(obj Handle) []byte {
	o := h.Get(obj)
	if o == nil || !o.arrayBuffer {
		return nil
	}
	return o.bytes
}

func (h *Heap) ReplaceArrayBufferBytes(obj Handle, src []byte) bool {
	o := h.Get(obj)
	if o == nil || !o.arrayBuffer || len(src) != len(o.bytes) {
		return false
	}
	copy(o.bytes, src)
	return true
}

// ─── Ramasse-miettes ────────────────────────────────────────────────────────

// Collect exécute un cycle complet de marquage-balayage. Le parcours de marquage
// est ITÉRATIF : un graphe d'objets profond ne doit pas faire déborder la pile,
// ce qui remplacerait un défaut de mémoire par un défaut de robustesse.
func (h *Heap) Collect() {
	h.Collections++

	for _, o := range h.objs {
		if o != nil {
			o.marked = false
		}
	}

	work := make([]Handle, 0, 64)
	push := func(v Value) {
		if !v.IsObject() {
			return
		}
		hd := v.Handle()
		o := h.Get(hd)
		if o == nil || o.marked {
			return
		}
		o.marked = true
		work = append(work, hd)
	}

	for _, r := range h.roots {
		push(*r)
	}
	for _, v := range h.stack {
		push(v)
	}
	for _, scan := range h.scanners {
		scan(push)
	}
	if h.objectProto != NoHandle {
		push(ObjectValue(h.objectProto))
	}
	if h.arrayProto != NoHandle {
		push(ObjectValue(h.arrayProto))
	}
	if h.functionProto != NoHandle {
		push(ObjectValue(h.functionProto))
	}

	for len(work) > 0 {
		hd := work[len(work)-1]
		work = work[:len(work)-1]
		o := h.Get(hd)
		if o == nil {
			continue
		}

		for _, v := range o.slots {
			push(v)
		}
		for _, v := range o.elements {
			push(v)
		}
		if o.proto != NoHandle {
			if p := h.Get(o.proto); p != nil && !p.marked {
				p.marked = true
				work = append(work, o.proto)
			}
		}
		// L'environnement CAPTURÉ par un objet fonction est une racine de plein
		// droit : c'est lui qui fait la fermeture. L'omettre laissait collecter
		// les variables capturées dès que la fonction qui les avait créées
		// rendait la main — défaut trouvé par le mode stress, avec pour symptôme
		// « emplacement local hors borne ».
		if o.env != NoHandle {
			if e := h.Get(o.env); e != nil && !e.marked {
				e.marked = true
				work = append(work, o.env)
			}
		}
		if o.homeObject != NoHandle {
			push(ObjectValue(o.homeObject))
		}
		if o.mapData != nil && o.kind != KindWeakMap {
			for _, v := range o.mapData.Keys {
				push(v)
			}
			for _, v := range o.mapData.Values {
				push(v)
			}
		}
		if o.setData != nil {
			for _, v := range o.setData.Elements {
				push(v)
			}
		}
		push(o.prim)
		if o.promise != nil {
			push(o.promise.result)
			for _, r := range o.promise.reactions {
				push(r.onFulfilled)
				push(r.onRejected)
				push(r.child)
			}
		}
		if o.proxy != nil {
			push(o.proxy.target)
			push(o.proxy.handler)
		}
		if o.gen != nil {
			push(o.gen.this)
			push(o.gen.delegate)
			push(o.gen.sent)
			for _, v := range o.gen.stack {
				push(v)
			}
		}
	}

	for i := 1; i < len(h.objs); i++ {
		o := h.objs[i]
		if o == nil || !o.marked || o.kind != KindWeakMap || o.mapData == nil {
			continue
		}
		var nk, nv []Value
		for j, k := range o.mapData.Keys {
			live := false
			if k.IsObject() {
				if ko := h.Get(k.Handle()); ko != nil && ko.marked {
					live = true
				}
			}
			if live {
				nk = append(nk, k)
				nv = append(nv, o.mapData.Values[j])
				push(o.mapData.Values[j])
			}
		}
		o.mapData.Keys = nk
		o.mapData.Values = nv
	}
	for len(work) > 0 {
		hd := work[len(work)-1]
		work = work[:len(work)-1]
		o := h.Get(hd)
		if o == nil {
			continue
		}
		for _, v := range o.slots {
			push(v)
		}
		for _, v := range o.elements {
			push(v)
		}
		if o.proto != NoHandle {
			if p := h.Get(o.proto); p != nil && !p.marked {
				p.marked = true
				work = append(work, o.proto)
			}
		}
		if o.env != NoHandle {
			if e := h.Get(o.env); e != nil && !e.marked {
				e.marked = true
				work = append(work, o.env)
			}
		}
		if o.homeObject != NoHandle {
			push(ObjectValue(o.homeObject))
		}
		push(o.prim)
	}

	for i := 1; i < len(h.objs); i++ {
		o := h.objs[i]
		if o == nil || o.marked {
			continue
		}

		// Restituer le quota de mémoire libéré. SetProperty/DefineDataProperty
		// facturent 104 octets par nouveau slot ; NewEnv/NewArray facturent 8
		// octets par élément ; alloc facture 96 octets pour l'objet.
		freed := int64(96)
		freed += int64(len(o.bytes))
		if len(o.elements) > 0 {
			freed += int64(len(o.elements) * 8)
		}
		if len(o.slots) > 0 {
			freed += int64(len(o.slots) * 104)
		}
		// Only NewString charges text length per cell. String wrappers and
		// symbols reference interned/shared text whose charge remains retained.
		if o.kind == KindString && o.text != nil {
			freed += int64(o.text.Len())
		}
		h.TrackFree(freed)

		h.objs[i] = nil
		// La génération est incrémentée À LA LIBÉRATION : tout handle qui
		// désignait cet emplacement devient invalide à l'instant même.
		h.gens[i]++
		h.free = append(h.free, uint32(i))
		h.Swept++
	}
}

// ─── Vérification d'invariant ───────────────────────────────────────────────

// CheckHeap parcourt le tas et vérifie que tout pointeur atteignable vise un
// objet vivant et correctement tagué. Appelé après chaque collecte en mode
// debug, il transforme un défaut d'enracinement en échec immédiat et situé.
func (h *Heap) CheckHeap() error {
	for i := 1; i < len(h.objs); i++ {
		o := h.objs[i]
		if o == nil {
			continue
		}
		if int(o.kind) >= NumObjKinds {
			return fmt.Errorf("objet #%d : genre %d hors énumération", i, o.kind)
		}
		if o.shape == nil {
			return fmt.Errorf("objet #%d : forme nulle", i)
		}
		if len(o.slots) != o.shape.Count() {
			return fmt.Errorf("objet #%d : %d emplacements pour une forme de %d propriétés",
				i, len(o.slots), o.shape.Count())
		}
		if o.proto != NoHandle && h.Get(o.proto) == nil {
			return fmt.Errorf("objet #%d : prototype #%d libéré", i, o.proto)
		}
		if o.env != NoHandle && h.Get(o.env) == nil {
			return fmt.Errorf("objet #%d : environnement capturé #%d libéré", i, o.env)
		}
		for j, v := range o.slots {
			if v.IsObject() && h.Get(v.Handle()) == nil {
				return fmt.Errorf("objet #%d, emplacement %d : handle #%d libéré", i, j, v.Handle())
			}
		}
		for j, v := range o.elements {
			if v.IsObject() && h.Get(v.Handle()) == nil {
				return fmt.Errorf("objet #%d, élément %d : handle #%d libéré", i, j, v.Handle())
			}
		}
	}
	for _, r := range h.roots {
		if r.IsObject() && h.Get(r.Handle()) == nil {
			return fmt.Errorf("racine : handle #%d libéré", r.Handle())
		}
	}
	for i, v := range h.stack {
		if v.IsObject() && h.Get(v.Handle()) == nil {
			return fmt.Errorf("pile, position %d : handle #%d libéré", i, v.Handle())
		}
	}
	return nil
}
