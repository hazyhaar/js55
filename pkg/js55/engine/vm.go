// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// Interpréteur de bytecode (plan §J4).
//
// Contraintes appliquées et mesurées :
//
//	C2  l'énumération d'opcodes est dense, le commutateur reçoit une table de saut
//	C3  la boucle maintient au plus dix valeurs 64 bits vivantes
//	C4  tout opérande se lit par RE-TRANCHAGE — code[ip:ip+4:ip+4] — soit un seul
//	    contrôle de bornes, jamais l'assertion « _ = code[ip+3] » suivie de quatre
//	    indexations, mesurée à quatre contrôles
//
// La garde de ces contraintes vit dans pkg/js55/csgguard.

// Throw porte une exception JavaScript remontant jusqu'à l'appelant Go.
type Throw struct {
	Value Value
	// Text est la forme textuelle de la valeur lancée, calculée au moment du
	// lancement : la valeur peut désigner un objet que le tas aura recyclé.
	Text string
	Line int
}

func (t *Throw) Error() string {
	if t.Line > 0 {
		return fmt.Sprintf("exception non interceptée : %s (ligne %d)", t.Text, t.Line)
	}
	return "exception non interceptée : " + t.Text
}

// frame est l'état d'un appel en cours. Quatre champs au plus, conformément à
// C6, pour rester décomposable en SSA.
type frame struct {
	chunk     *Chunk
	env       Handle
	ip        int
	base      int // hauteur de pile à l'entrée
	newTarget Value
	callee    Value
}

// VM exécute du bytecode sur un tas.
type VM struct {
	heap      *Heap
	globals   map[*str.String]Value
	globalObj Handle
	frames    []frame
	thisStack []Value
	newStack  []Value
	safeEnvs  []Handle

	// GasLeft borne le nombre d'instructions exécutées. Une boucle infinie
	// s'arrête donc, au lieu de bloquer l'hôte.
	GasLeft int64
	// MaxDepth borne la profondeur d'appel : une récursion sans fin produit une
	// RangeError, jamais un dépassement de pile du processus (plan T4.5).
	MaxDepth    int
	Interrupted *atomic.Bool
	// CheckpointEvery, s'il est > 0, appelle OnCheckpoint tous les N opcodes
	// pour céder l'hôte (file d'événements WASM, timers, limite d'évaluation).
	CheckpointEvery int64
	OnCheckpoint    func() error
	HostNonMutating bool
	yielding        atomic.Bool

	jobs           []microJob
	promiseProto   Handle
	regexpProto    Handle
	generatorProto Handle
	retMin         int
	genReturning   bool
	keep           Value
	stopIP         int
	returnTok      Value
	tdzTok         Value
	moduleExports  map[*str.String]Value
	realm          *RootRealm
	gasInit        int64

	thisBuf [8]Value
	newBuf  [2]Value

	// DisableArchtimeGeometry bypasses the optional compiled binding entirely.
	// Without the archtime_geometry build tag NewVM always starts disabled.
	DisableArchtimeGeometry bool
	DisableArchtimeNormals  bool
	// KernelStats counts admitted calls and rejected tagged Box3 candidates.
	// Unrecognized function bodies and explicitly disabled calls are not attempts.
	KernelStats struct {
		Accepted uint64
		Rejected uint64
	}
}

type microJob struct {
	run  func() error
	hold []Value
}

// NewVM crée un interpréteur sur un tas neuf.
//
// L'interpréteur DÉCLARE ses racines au tas : les environnements de ses trames
// d'appel et ses variables globales. Le tas ne peut pas les deviner, et sans
// cette déclaration une fonction globale ou une fermeture en cours d'appel est
// collectée sous les pieds de la boucle.
var globalTombstone = Value(tagGlobalTombstone)

func NewVM(h *Heap) *VM {
	if h != nil && h.buildingRoot {
		return newVMWithBuiltins(h)
	}
	ensureRootRealm()
	return newVMFromRealm(h)
}

func newVMFromRealm(h *Heap) *VM {
	r := DefaultRootRealm
	h.realm = r.Heap
	h.objectProto = r.ObjectProto
	h.arrayProto = r.ArrayProto
	h.functionProto = r.FunctionProto
	vm := &VM{
		heap:                    h,
		realm:                   r,
		GasLeft:                 1 << 32,
		gasInit:                 1 << 32,
		DisableArchtimeGeometry: !archtimeGeometryAvailable,
		MaxDepth:                512,
		tdzTok:                  Value(tagTDZ),
		returnTok:               Value(tagReturn),
	}
	vm.globalObj = h.NewObject()
	vm.promiseProto = r.PromiseProto
	vm.regexpProto = r.RegexpProto
	vm.generatorProto = r.GeneratorProto
	vm.thisBuf[0] = ObjectValue(vm.globalObj)
	vm.thisStack = vm.thisBuf[:1]
	vm.newBuf[0] = Undefined
	vm.newStack = vm.newBuf[:1]
	h.boundVM = vm
	return vm
}

func (vm *VM) GlobalObj() Handle {
	return vm.globalObj
}

func (vm *VM) SetGasLeft(n int64) {
	vm.GasLeft = n
	vm.gasInit = n
}

func (vm *VM) bindGlobal(key *str.String) (Value, bool) {
	if key == nil {
		return Undefined, false
	}
	if key.EqualASCII("\x00tdz") {
		return vm.tdzTok, true
	}
	if key.EqualASCII("globalThis") && vm.globalObj != NoHandle {
		return ObjectValue(vm.globalObj), true
	}
	if vm.globals != nil {
		if v, ok := vm.globals[key]; ok {
			if v == globalTombstone {
				return Undefined, false
			}
			return v, true
		}
		for k, v := range vm.globals {
			if k == nil || !k.Equal(key) {
				continue
			}
			if v == globalTombstone {
				return Undefined, false
			}
			return v, true
		}
	}
	if vm.realm != nil {
		if v, ok := vm.realm.lookup(key); ok {
			return v, true
		}
	}
	return Undefined, false
}

func (vm *VM) storeGlobal(key *str.String, v Value) {
	if vm.globals == nil {
		vm.globals = make(map[*str.String]Value)
	}
	vm.globals[key] = v
}

func (vm *VM) scanRoots(visit func(Value)) {
	visit(ObjectValue(vm.globalObj))
	visit(ObjectValue(vm.generatorProto))
	for _, fr := range vm.frames {
		visit(ObjectValue(fr.env))
		visit(fr.callee)
		visit(fr.newTarget)
	}
	for _, v := range vm.globals {
		visit(v)
	}
	for _, v := range vm.moduleExports {
		visit(v)
	}
	for _, v := range vm.thisStack {
		visit(v)
	}
	for _, v := range vm.newStack {
		visit(v)
	}
	for _, e := range vm.safeEnvs {
		visit(ObjectValue(e))
	}
	visit(vm.keep)
	visit(vm.returnTok)
	visit(vm.tdzTok)
	for i := range vm.jobs {
		for _, v := range vm.jobs[i].hold {
			visit(v)
		}
	}
}

func (vm *VM) ResetState() {
	vm.frames = vm.frames[:0]
	vm.jobs = vm.jobs[:0]
	vm.safeEnvs = vm.safeEnvs[:0]
	if vm.heap != nil {
		vm.heap.TruncateStack(0)
		vm.heap.DiscardCow()
		vm.heap.ResetGlobalShell(vm.globalObj)
	}
	if cap(vm.thisStack) > 0 {
		vm.thisStack = vm.thisStack[:1]
		vm.thisStack[0] = ObjectValue(vm.globalObj)
	}
	if cap(vm.newStack) > 0 {
		vm.newStack = vm.newStack[:1]
		vm.newStack[0] = Undefined
	}
	for k := range vm.globals {
		delete(vm.globals, k)
	}
	for k := range vm.moduleExports {
		delete(vm.moduleExports, k)
	}
	vm.GasLeft = vm.gasInit
	vm.keep = Undefined
	vm.genReturning = false
	vm.yielding.Store(false)
	vm.retMin = 0
	vm.stopIP = 0
}

func newVMWithBuiltins(h *Heap) *VM {
	vm := &VM{
		heap:                    h,
		globals:                 map[*str.String]Value{},
		moduleExports:           map[*str.String]Value{},
		newStack:                []Value{Undefined},
		GasLeft:                 1 << 32,
		gasInit:                 1 << 32,
		DisableArchtimeGeometry: !archtimeGeometryAvailable,
		MaxDepth:                512,
	}
	vm.globalObj = h.NewObject()
	vm.returnTok = ObjectValue(h.NewObject())
	vm.tdzTok = ObjectValue(h.NewObject())
	vm.thisStack = []Value{ObjectValue(vm.globalObj)}
	h.boundVM = vm
	vm.installGlobals()
	return vm
}

func (vm *VM) CurrentThis() Value {
	if len(vm.thisStack) == 0 {
		return Undefined
	}
	return vm.thisStack[len(vm.thisStack)-1]
}

// installGlobals pose les valeurs globales que le langage définit d'office.
func (vm *VM) installGlobals() {
	vm.SetGlobal("undefined", Undefined)
	vm.SetGlobal("NaN", Number(math.NaN()))
	vm.SetGlobal("Infinity", Number(math.Inf(1)))
	vm.SetGlobal("globalThis", ObjectValue(vm.globalObj))
	vm.SetGlobal("\x00tdz", vm.tdzTok)
	vm.InstallFunctionBuiltins()
	vm.InstallMathBuiltins()
	vm.InstallStringNumberBuiltins()
	vm.InstallBooleanAndErrorBuiltins()
	vm.InstallArrayBuiltins()
	vm.InstallObjectBuiltins()
	vm.InstallCollectionsBuiltins()
	vm.InstallDateBuiltins()
	vm.InstallTypedArrayBuiltins()
	vm.InstallTypedArrayExtendedBuiltins()
	vm.InstallReflectBuiltins()
	vm.InstallSymbolBuiltins()
	vm.InstallJSONBuiltins()
	vm.InstallPromiseBuiltins()
	vm.InstallRegExpBuiltins()
	vm.InstallProxyBuiltins()
	vm.InstallGeneratorBuiltins()

	// Install console object
	consoleObj := vm.heap.NewObject()
	hConsole := ObjectValue(consoleObj)
	vm.SetGlobal("console", hConsole)

	logChunk := &Chunk{
		Name:   "log",
		Params: 0,
		Native: func(vm *VM, args []Value) (Value, error) {
			for i, a := range args {
				if i > 0 {
					fmt.Print(" ")
				}
				fmt.Print(vm.toDisplayString(a))
			}
			fmt.Println()
			return Undefined, nil
		},
	}
	vm.heap.SetProperty(consoleObj, vm.heap.Intern().InternGo("log"), ObjectValue(vm.heap.NewFunction(logChunk, NoHandle)))
	vm.heap.SetProperty(consoleObj, vm.heap.Intern().InternGo("error"), ObjectValue(vm.heap.NewFunction(logChunk, NoHandle)))
	vm.heap.SetProperty(consoleObj, vm.heap.Intern().InternGo("warn"), ObjectValue(vm.heap.NewFunction(logChunk, NoHandle)))
}

// Heap rend le tas.
func (vm *VM) Heap() *Heap { return vm.heap }

// PromiseProto rend le prototype des promesses.
func (vm *VM) PromiseProto() Handle { return vm.promiseProto }

// SetGlobal pose une variable globale.
func (vm *VM) SetGlobal(name string, v Value) {
	key := vm.heap.Intern().InternGo(name)
	vm.storeGlobal(key, v)
	if vm.globalObj != NoHandle && name != "\x00tdz" {
		if vm.heap.Get(vm.globalObj) != nil {
			vm.heap.SetProperty(vm.globalObj, key, v)
		}
	}
}

// GetGlobal rend une variable globale.
func (vm *VM) GetGlobal(name string) (Value, bool) {
	return vm.bindGlobal(vm.heap.Intern().InternGo(name))
}

// GetExport rend une liaison exportée par le module courant.
func (vm *VM) GetExport(name string) (Value, bool) {
	if vm.moduleExports == nil {
		return Undefined, false
	}
	v, ok := vm.moduleExports[vm.heap.Intern().InternGo(name)]
	return v, ok
}

// ModuleExportDefault rend la valeur exportée par défaut, et un booléen indiquant sa présence.
func (vm *VM) ModuleExportDefault() (Value, bool) {
	return vm.GetExport("default")
}

// ModuleExports rend une copie de toutes les liaisons exportées par le module.
func (vm *VM) ModuleExports() map[string]Value {
	res := make(map[string]Value, len(vm.moduleExports))
	for k, v := range vm.moduleExports {
		res[k.GoString()] = v
	}
	return res
}

func (vm *VM) GlobalObject() Value {
	return ObjectValue(vm.globalObj)
}

func (vm *VM) Contextify(sandbox Value) error {
	if !sandbox.IsObject() {
		return fmt.Errorf("TypeError: context must be an object")
	}
	src := ObjectValue(vm.globalObj)
	for _, k := range vm.ownPropertyNames(src, false) {
		if _, ok := vm.heap.GetOwnProperty(sandbox.Handle(), k); ok {
			continue
		}
		if v, ok := vm.heap.GetOwnProperty(vm.globalObj, k); ok {
			vm.heap.SetProperty(sandbox.Handle(), k, v)
		} else if v, ok := vm.globals[k]; ok {
			vm.heap.SetProperty(sandbox.Handle(), k, v)
		}
	}
	for k, v := range vm.globals {
		if v == globalTombstone {
			continue
		}
		if _, ok := vm.heap.GetOwnProperty(sandbox.Handle(), k); ok {
			continue
		}
		vm.heap.SetProperty(sandbox.Handle(), k, v)
	}
	if vm.realm != nil {
		for k, v := range vm.realm.Globals {
			if k == nil || k.EqualASCII("globalThis") || k.EqualASCII("\x00tdz") {
				continue
			}
			if _, tomb := vm.globals[k]; tomb && vm.globals[k] == globalTombstone {
				continue
			}
			if _, ok := vm.bindGlobal(k); !ok {
				continue
			}
			if _, ok := vm.heap.GetOwnProperty(sandbox.Handle(), k); ok {
				continue
			}
			vm.heap.SetProperty(sandbox.Handle(), k, v)
		}
	}
	for _, k := range vm.ownPropertyNames(sandbox, false) {
		if v, ok := vm.heap.GetOwnProperty(sandbox.Handle(), k); ok {
			vm.storeGlobal(k, v)
		}
	}
	vm.globalObj = sandbox.Handle()
	if len(vm.thisStack) == 0 {
		vm.thisStack = []Value{sandbox}
	} else {
		vm.thisStack[0] = sandbox
	}
	gt := vm.heap.Intern().InternGo("globalThis")
	vm.storeGlobal(gt, sandbox)
	vm.heap.SetProperty(sandbox.Handle(), gt, sandbox)
	return nil
}

// NewStringValue alloue une chaîne primitive et rend sa valeur.
func (vm *VM) NewStringValue(s *str.String) Value {
	return ObjectValue(vm.heap.NewString(s))
}

// StringOf rend la chaîne portée par une valeur chaîne, ou nil.
func (vm *VM) StringOf(v Value) *str.String {
	if !v.IsObject() {
		return nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind != KindString {
		return nil
	}
	return o.text
}

// IsString indique qu'une valeur est une chaîne primitive.
func (vm *VM) IsString(v Value) bool { return vm.StringOf(v) != nil }

// isSymbol indique si v est un symbole primitif.
func (vm *VM) isSymbol(v Value) bool {
	if !v.IsObject() {
		return false
	}
	o := vm.heap.Get(v.Handle())
	return o != nil && o.kind == KindSymbol
}

// isPrimitive rapporte si v est une valeur primitive ECMAScript
// (Undefined, Null, Boolean, Number, String, Symbol, BigInt).
// Dans JS55, Undefined, Null, Bool, Int32 et Float64 ont !v.IsObject().
// KindString, KindBigInt et KindSymbol vivent dans le tas pour le GC mais
// sont sémantiquement des primitifs.
// Les wrappers (KindStringObject, ou KindOrdinary avec prim) restent des objets.
func (vm *VM) isPrimitive(v Value) bool {
	if !v.IsObject() {
		return true
	}
	o := vm.heap.Get(v.Handle())
	if o == nil {
		return false
	}
	switch o.kind {
	case KindString, KindBigInt, KindSymbol:
		return true
	default:
		return false
	}
}

// ─── Exécution ──────────────────────────────────────────────────────────────

// Run exécute une unité au premier niveau et rend la valeur de la dernière
// expression évaluée, ou Undefined.
func (vm *VM) Run(c *Chunk) (Value, error) {
	boundary := vm.frameBoundary()
	vm.heap.AddRoot(&boundary.keep)
	defer vm.heap.RemoveRoot(&boundary.keep)
	defer vm.restoreFrameBoundary(boundary)
	env := vm.heap.NewEnv(c.Locals, NoHandle)
	var envRoot = ObjectValue(env)
	vm.heap.AddRoot(&envRoot)
	defer vm.heap.RemoveRoot(&envRoot)

	vm.thisStack = append(vm.thisStack, vm.CurrentThis())
	vm.newStack = append(vm.newStack, Undefined)
	vm.frames = append(vm.frames, frame{chunk: c, env: env, base: vm.heap.StackLen()})
	v, err := vm.interpret(boundary.frames)
	if err != nil {
		return v, err
	}
	vm.heap.AddRoot(&v)
	defer vm.heap.RemoveRoot(&v)
	if err := vm.RunMicrotasks(); err != nil {
		return v, err
	}
	return v, nil
}

// loop est la boucle d'interprétation.
//
// Les variables vivantes de la boucle sont volontairement peu nombreuses
// (contrainte C3) : le compteur ordinal, la tranche de code, l'unité courante
// et un temporaire. Tout le reste vit dans la structure de trame ou passe par
// des méthodes.
func (vm *VM) loop() (Value, error) {
	return vm.interpret(0)
}

func (vm *VM) interpret(minFrames int) (Value, error) {
	for {
		if minFrames > 0 && len(vm.frames) <= minFrames {
			if vm.heap.StackLen() > 0 {
				return vm.pop(), nil
			}
			return Undefined, nil
		}
		if len(vm.frames) == 0 {
			return Undefined, nil
		}
		fr := &vm.frames[len(vm.frames)-1]
		code := fr.chunk.Code
		ip := fr.ip

		if vm.stopIP > 0 && ip >= vm.stopIP {
			vm.stopIP = 0
			if vm.heap.StackLen() > fr.base {
				return vm.pop(), nil
			}
			return Undefined, nil
		}

		if ip >= len(code) {
			// Fin d'unité sans return explicite.
			if v, done, err := vm.ret(Undefined); done || err != nil {
				return v, err
			}
			continue
		}

		vm.GasLeft--
		if vm.GasLeft <= 0 {
			return Undefined, ErrGasExhausted
		}
		if vm.GasLeft&63 == 0 && vm.interrupted() {
			return Undefined, ErrInterrupted
		}
		if vm.CheckpointEvery > 0 && vm.GasLeft%vm.CheckpointEvery == 0 {
			if err := vm.checkpoint(); err != nil {
				return Undefined, err
			}
		}

		op := Op(code[ip])
		ip++

		var arg uint32
		switch op.Width() {
		case 2:
			if ip+2 > len(code) {
				return Undefined, vm.fault(fr, "bytecode tronqué")
			}
			// Re-tranchage : un seul contrôle de bornes (contrainte C4).
			b := code[ip : ip+2 : ip+2]
			arg = uint32(binary.LittleEndian.Uint16(b))
			ip += 2
		case 4:
			if ip+4 > len(code) {
				return Undefined, vm.fault(fr, "bytecode tronqué")
			}
			b := code[ip : ip+4 : ip+4]
			arg = binary.LittleEndian.Uint32(b)
			ip += 4
		}
		fr.ip = ip

		if err := vm.step(fr, op, arg); err != nil {
			if fin, ok := err.(*finished); ok {
				return fin.value, nil
			}
			if _, ok := err.(*yieldSuspend); ok {
				return Undefined, err
			}
			if th, ok := err.(*Throw); ok {
				if vm.unwindTo(th, minFrames) {
					continue
				}
			}
			return Undefined, err
		}
	}
}

// unwind déroule la pile d'appels à la recherche d'un gestionnaire d'exception catch/finally.
func (vm *VM) unwind(th *Throw) bool {
	return vm.unwindTo(th, 0)
}

func (vm *VM) unwindToCatch(th *Throw, minFrames int) bool {
	if len(vm.frames) <= minFrames {
		return false
	}
	fr := &vm.frames[len(vm.frames)-1]
	bestIdx := -1
	bestLen := int(1 << 30)
	for i, h := range fr.chunk.Handlers {
		if fr.ip >= h.Start && fr.ip <= h.End && h.CatchIP >= 0 {
			curLen := h.End - h.Start
			if curLen < bestLen {
				bestLen = curLen
				bestIdx = i
			}
		}
	}
	if bestIdx < 0 {
		return false
	}
	h := fr.chunk.Handlers[bestIdx]
	vm.heap.TruncateStack(fr.base)
	if h.CatchSlot >= 0 {
		_ = vm.writeVar(fr, uint32(h.Depth<<16|h.CatchSlot), th.Value)
	}
	vm.push(th.Value)
	fr.ip = h.CatchIP
	return true
}

func (vm *VM) unwindTo(th *Throw, minFrames int) bool {
	for len(vm.frames) > minFrames {
		fr := &vm.frames[len(vm.frames)-1]
		bestIdx := -1
		bestLen := int(1 << 30)

		// Sélection du gestionnaire le plus étroit (le plus imbriqué)
		for i, h := range fr.chunk.Handlers {
			if fr.ip >= h.Start && fr.ip <= h.End {
				curLen := h.End - h.Start
				if curLen < bestLen {
					bestLen = curLen
					bestIdx = i
				}
			}
		}

		if bestIdx >= 0 {
			h := fr.chunk.Handlers[bestIdx]
			if h.CatchIP >= 0 {
				vm.heap.TruncateStack(fr.base)
				if h.CatchSlot >= 0 {
					_ = vm.writeVar(fr, uint32(h.Depth<<16|h.CatchSlot), th.Value)
				}
				vm.push(th.Value)
				fr.ip = h.CatchIP
				return true
			}
			if h.FinallyIP >= 0 {
				vm.heap.TruncateStack(fr.base)
				fr.ip = h.FinallyIP
				return true
			}
		}

		// Dépilement de la trame courante
		vm.heap.TruncateStack(fr.base)
		vm.releaseFrameEnv(fr)
		vm.popFrameCallStacks(fr)
		*fr = frame{}
		vm.frames = vm.frames[:len(vm.frames)-1]
	}
	return false
}

// finished signale la fin de l'exécution au premier niveau. Employer une erreur
// plutôt qu'un drapeau garde la boucle à peu de variables vivantes.
type finished struct{ value Value }

func (*finished) Error() string { return "exécution terminée" }

// constText rend la constante de chaîne d'indice i, ou une faute. Un bytecode
// forgé ou corrompu peut porter un indice hors table ou une constante d'un
// autre type : le moteur le refuse au lieu de déréférencer un pointeur nul.
func (vm *VM) constText(fr *frame, i uint32) (*str.String, error) {
	if int(i) >= len(fr.chunk.Consts) {
		return nil, vm.fault(fr, "constante hors table")
	}
	k := fr.chunk.Consts[i]
	if k.Kind != ConstString || k.Text == nil {
		return nil, vm.fault(fr, "constante de type "+k.Kind.String()+", chaîne attendue")
	}
	return k.Text, nil
}

func (vm *VM) fault(fr *frame, msg string) error {
	return fmt.Errorf("%s dans %s à %d", msg, fr.chunk.Name, fr.ip)
}

// line rend la ligne source associée au décalage courant.
func (vm *VM) line(fr *frame) int {
	if fr == nil || fr.chunk == nil {
		return 0
	}
	best := 0
	for at, l := range fr.chunk.Lines {
		if at <= fr.ip && at >= best {
			best = at
			_ = l
		}
	}
	return fr.chunk.Lines[best]
}

func (vm *VM) hasIn(fr *frame, key, obj Value) (bool, error) {
	if !obj.IsObject() {
		return false, vm.throwText(fr, "TypeError: Cannot use 'in' operator to search for '"+vm.toDisplayString(key)+"' in "+vm.toDisplayString(obj))
	}
	if ok, handled, err := vm.proxyHas(obj, vm.keyString(key)); handled {
		return ok, err
	}
	name := vm.keyString(key)
	if o := vm.heap.Get(obj.Handle()); o != nil && o.kind == KindArray {
		if vm.heap.isDeleted(o, vm.heap.Intern().Intern(name)) {
			_, ok := vm.heap.GetProperty(obj.Handle(), name)
			return ok, nil
		}
		if name.Equal(str.FromGo("length")) {
			return true, nil
		}
		if i, ok := vm.arrayIndex(key); ok && i >= 0 && i < len(o.elements) {
			return true, nil
		}
	}
	_, ok := vm.heap.GetProperty(obj.Handle(), name)
	return ok, nil
}

func (vm *VM) objectAssign(dest, src Value) {
	if !dest.IsObject() || !src.IsObject() {
		return
	}
	for _, k := range vm.BuiltinObjectKeys(src) {
		if v, ok := vm.heap.GetProperty(src.Handle(), k); ok {
			vm.heap.SetProperty(dest.Handle(), k, v)
		}
	}
}

func (vm *VM) objectRest(src, excl Value) (Value, error) {
	dest := vm.heap.NewObject()
	destVal := ObjectValue(dest)
	vm.heap.AddRoot(&destVal)
	defer vm.heap.RemoveRoot(&destVal)
	if s := vm.StringOf(src); s != nil {
		skip := map[*str.String]bool{}
		if excl.IsObject() {
			if o := vm.heap.Get(excl.Handle()); o != nil && o.kind == KindArray {
				for _, v := range o.elements {
					skip[vm.keyString(v)] = true
				}
			}
		}
		for i := 0; i < s.Len(); i++ {
			k := vm.heap.Intern().InternGo(strconv.Itoa(i))
			if skip[k] {
				continue
			}
			vm.heap.SetProperty(dest, k, vm.NewStringValue(s.Slice(i, i+1)))
		}
		return destVal, nil
	}
	if !src.IsObject() {
		return destVal, nil
	}
	skip := map[*str.String]bool{}
	if excl.IsObject() {
		if o := vm.heap.Get(excl.Handle()); o != nil && o.kind == KindArray {
			for _, v := range o.elements {
				if s := vm.StringOf(v); s != nil {
					skip[vm.heap.Intern().Intern(s)] = true
				} else if !v.IsUndefined() && !v.IsNull() {
					skip[vm.heap.Intern().InternGo(vm.toDisplayString(v))] = true
				}
			}
		}
	}
	for _, k := range vm.ownPropertyNames(src, true) {
		if skip[k] {
			continue
		}
		v, err := vm.getPropInvoke(src, k)
		if err != nil {
			return destVal, err
		}
		vm.heap.SetProperty(dest, k, v)
	}
	return destVal, nil
}

func (vm *VM) throwDisplay(v Value) string {
	if v.IsObject() {
		name := vm.errorPropString(v, "name")
		if name != "" {
			msg := vm.errorPropString(v, "message")
			if msg != "" {
				return name + ": " + msg
			}
			return name
		}
		ts := vm.getProp(v, vm.heap.Intern().InternGo("toString"))
		if vm.isFunction(ts) {
			res, err := vm.invoke(v, ts, nil)
			if err == nil {
				if s := vm.StringOf(res); s != nil {
					return s.GoString()
				}
				s := vm.toDisplayString(res)
				if s != "" && s != "[object Object]" {
					return s
				}
			}
		}
	}
	return vm.toDisplayString(v)
}

func (vm *VM) errorPropString(obj Value, prop string) string {
	v := vm.getProp(obj, vm.heap.Intern().InternGo(prop))
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	if s := vm.StringOf(v); s != nil {
		return s.GoString()
	}
	s := vm.toDisplayString(v)
	if s == "" || s == "undefined" || s == "[object Object]" {
		return ""
	}
	return s
}

func (vm *VM) throwText(fr *frame, text string) error {
	for _, name := range []string{"RangeError", "TypeError", "ReferenceError", "SyntaxError", "Test262Error"} {
		if strings.HasPrefix(text, name) {
			return vm.throwNamedError(fr, name, text)
		}
	}
	s := vm.NewStringValue(str.FromGo(text))
	return &Throw{Value: s, Text: text, Line: vm.line(fr)}
}

func (vm *VM) throwTypeError(fr *frame, text string) error {
	return vm.throwNamedError(fr, "TypeError", text)
}

func (vm *VM) throwNamedError(fr *frame, name, text string) error {
	obj := vm.heap.NewObject()
	ov := ObjectValue(obj)
	vm.heap.AddRoot(&ov)
	defer vm.heap.RemoveRoot(&ov)
	if ctor, ok := vm.GetGlobal(name); ok && ctor.IsObject() {
		if proto, ok := vm.heap.GetProperty(ctor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
			if o := vm.heap.Get(obj); o != nil {
				o.proto = proto.Handle()
				o.kind = KindError
			}
		}
		vm.heap.SetProperty(obj, vm.heap.Intern().InternGo("constructor"), ctor)
	}
	msg := text
	prefix := name + ": "
	if strings.HasPrefix(msg, prefix) {
		msg = msg[len(prefix):]
	}
	vm.heap.SetProperty(obj, vm.heap.Intern().InternGo("name"), vm.NewStringValue(str.FromGo(name)))
	vm.heap.SetProperty(obj, vm.heap.Intern().InternGo("message"), vm.NewStringValue(str.FromGo(msg)))
	return &Throw{Value: ov, Text: text, Line: vm.line(fr)}
}

func (vm *VM) instanceOf(fr *frame, left, right Value) (bool, error) {
	if !right.IsObject() {
		return false, vm.throwText(fr, "TypeError: Right-hand side of 'instanceof' is not an object")
	}
	proto, ok := vm.heap.GetProperty(right.Handle(), vm.heap.Intern().InternGo("prototype"))
	if !ok || !proto.IsObject() {
		return false, vm.throwText(fr, "TypeError: instanceof called on an object with an invalid prototype property")
	}
	if !left.IsObject() {
		return false, nil
	}
	target := proto.Handle()
	o := vm.heap.Get(left.Handle())
	if o == nil {
		return false, nil
	}
	cur := o.proto
	for hops := 0; cur != NoHandle && hops < len(vm.heap.objs)+1; hops++ {
		if cur == target {
			return true, nil
		}
		n := vm.heap.Get(cur)
		if n == nil {
			return false, nil
		}
		cur = n.proto
	}
	return false, nil
}

// push et pop opèrent sur la pile du tas, qui est enracinée dans sa totalité :
// aucune valeur intermédiaire ne peut être collectée sous les pieds de la
// boucle.
func (vm *VM) push(v Value) { vm.heap.Push(v) }
func (vm *VM) pop() Value   { return vm.heap.Pop() }

func (vm *VM) peek(n int) Value {
	l := vm.heap.StackLen()
	if n >= l {
		return Undefined
	}
	return vm.heap.stack[l-1-n]
}

// ret dépile une trame. Rend done à vrai quand la dernière trame est dépilée.
func (vm *VM) ret(v Value) (Value, bool, error) {
	fr := &vm.frames[len(vm.frames)-1]
	vm.heap.TruncateStack(fr.base)
	vm.releaseFrameEnv(fr)
	newObj := vm.popFrameCallStacks(fr)
	*fr = frame{}
	vm.frames = vm.frames[:len(vm.frames)-1]
	if newObj.IsObject() && !v.IsObject() {
		v = newObj
	}
	if len(vm.frames) <= vm.retMin {
		return v, true, nil
	}
	vm.push(v)
	return Undefined, false, nil
}

// step exécute une instruction. Le commutateur porte sur une énumération dense,
// ce qui lui vaut une table de saut (contrainte C2).
func (vm *VM) interrupted() bool {
	return vm.Interrupted != nil && vm.Interrupted.Load()
}

func (vm *VM) Yielding() bool { return vm.yielding.Load() }

func (vm *VM) checkpoint() error {
	if vm.OnCheckpoint != nil {
		vm.yielding.Store(true)
		defer vm.yielding.Store(false)
		err := vm.OnCheckpoint()
		vm.yielding.Store(false)
		if err != nil {
			return err
		}
	}
	if vm.interrupted() {
		return ErrInterrupted
	}
	return nil
}

func (vm *VM) applyJump(fr *frame, arg uint32) error {
	before := fr.ip
	fr.ip += int(int32(arg))
	if fr.ip < 0 || fr.ip > len(fr.chunk.Code) {
		return vm.fault(fr, "saut hors code")
	}
	if fr.ip < before && vm.interrupted() {
		return ErrInterrupted
	}
	return nil
}

func (vm *VM) step(fr *frame, op Op, arg uint32) error {
	switch op {
	case OpNop:

	case OpUndefined:
		vm.push(Undefined)
	case OpNull:
		vm.push(Null)
	case OpTrue:
		vm.push(True)
	case OpFalse:
		vm.push(False)
	case OpInt32:
		vm.push(Int(int32(arg)))
	case OpConst:
		if int(arg) >= len(fr.chunk.Consts) {
			return vm.fault(fr, "constante hors table")
		}
		k := fr.chunk.Consts[arg]
		switch k.Kind {
		case ConstNumber:
			vm.push(Number(k.Num))
		case ConstString:
			vm.push(vm.NewStringValue(k.Text))
		case ConstFunction:
			fnObj := vm.heap.NewFunction(k.Fn, fr.env)
			vm.push(ObjectValue(fnObj))
			fname := ""
			plen := 0
			strict := false
			if k.Fn != nil {
				fname = k.Fn.Name
				plen = k.Fn.Params
				strict = k.Fn.Strict
			}
			vm.heap.DefineDataProperty(fnObj, vm.heap.Intern().InternGo("length"), Int(int32(plen)), attrConfigurable)
			vm.heap.DefineDataProperty(fnObj, vm.heap.Intern().InternGo("name"), vm.NewStringValue(str.FromGo(fname)), attrConfigurable)
			arrow := k.Fn != nil && k.Fn.Arrow
			if arrow {
				fn := vm.heap.MustGet(fnObj)
				fn.prim = vm.CurrentThis()
				if owner := vm.heap.Get(fr.callee.Handle()); owner != nil {
					fn.homeObject = owner.homeObject
					if fn.homeObject == NoHandle {
						if p, ok := vm.heap.GetOwnProperty(fr.callee.Handle(), vm.heap.Intern().InternGo("prototype")); ok && p.IsObject() {
							fn.homeObject = p.Handle()
						}
					}
				}
			}
			if !strict && !arrow {
				vm.heap.DefineDataProperty(fnObj, vm.heap.Intern().InternGo("arguments"), Undefined, attrConfigurable)
				vm.heap.DefineDataProperty(fnObj, vm.heap.Intern().InternGo("caller"), Undefined, attrConfigurable)
			}
			if !arrow {
				protoObj := vm.heap.NewObject()
				vm.push(ObjectValue(protoObj))
				vm.heap.SetProperty(fnObj, vm.heap.Intern().InternGo("prototype"), ObjectValue(protoObj))
				vm.heap.SetProperty(protoObj, vm.heap.Intern().InternGo("constructor"), ObjectValue(fnObj))
				vm.pop()
			}
		case ConstBigInt:
			vm.push(vm.newBigInt(k.Big))
		}

	case OpPop:
		vm.pop()
	case OpDup:
		vm.push(vm.peek(0))

	case OpAdd:
		b, a := vm.peek(0), vm.peek(1)
		v, err := vm.add(a, b)
		if err != nil {
			return err
		}
		vm.pop()
		vm.pop()
		vm.push(v)
	case OpSub:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Number(a - b))
	case OpMul:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Number(a * b))
	case OpDiv:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Number(a / b))
	case OpMod:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Number(math.Mod(a, b)))
	case OpPow:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Number(math.Pow(a, b)))
	case OpNeg:
		x, err := vm.toNumberErr(vm.pop())
		if err != nil {
			return err
		}
		vm.push(Number(-x))
	case OpPos:
		x, err := vm.toNumberErr(vm.pop())
		if err != nil {
			return err
		}
		vm.push(Number(x))

	case OpBitAnd:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Int(int32(uint32(int64(a))) & int32(uint32(int64(b)))))
	case OpBitOr:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Int(int32(uint32(int64(a))) | int32(uint32(int64(b)))))
	case OpBitXor:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Int(int32(uint32(int64(a))) ^ int32(uint32(int64(b)))))
	case OpBitNot:
		x, err := vm.toNumberErr(vm.pop())
		if err != nil {
			return err
		}
		vm.push(Int(^int32(uint32(int64(x)))))
	case OpShl:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Int(int32(uint32(int64(a))) << (uint32(int64(b)) & 31)))
	case OpShr:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		vm.push(Int(int32(uint32(int64(a))) >> (uint32(int64(b)) & 31)))
	case OpUShr:
		a, b, err := vm.pop2num()
		if err != nil {
			return err
		}
		u := uint32(int32(uint32(int64(a)))) >> (uint32(int64(b)) & 31)
		vm.push(Number(float64(u)))

	case OpEq:
		b, a := vm.pop(), vm.pop()
		vm.push(Bool(vm.looseEq(a, b)))
	case OpNe:
		b, a := vm.pop(), vm.pop()
		vm.push(Bool(!vm.looseEq(a, b)))
	case OpStrictEq:
		b, a := vm.pop(), vm.pop()
		vm.push(Bool(vm.strictEq(a, b)))
	case OpStrictNe:
		b, a := vm.pop(), vm.pop()
		vm.push(Bool(!vm.strictEq(a, b)))
	case OpLt, OpGt, OpLe, OpGe:
		b, a := vm.peek(0), vm.peek(1)
		v, err := vm.compareErr(op, a, b)
		if err != nil {
			return err
		}
		vm.pop()
		vm.pop()
		vm.push(v)
	case OpInstanceof:
		right, left := vm.peek(0), vm.peek(1)
		ok, err := vm.instanceOf(fr, left, right)
		if err != nil {
			return err
		}
		vm.pop()
		vm.pop()
		vm.push(Bool(ok))
	case OpIn:
		obj, key := vm.pop(), vm.pop()
		ok, err := vm.hasIn(fr, key, obj)
		if err != nil {
			return err
		}
		vm.push(Bool(ok))

	case OpNot:
		vm.push(Bool(!vm.truthy(vm.pop())))
	case OpTypeof:
		vm.push(vm.NewStringValue(str.FromGo(vm.typeOf(vm.pop()))))
	case OpIsNullish:
		v := vm.pop()
		vm.push(Bool(v.IsNull() || v.IsUndefined()))
	case OpArraySlice:
		start := int(vm.toInt32(vm.pop()))
		arr := vm.pop()
		end := 1 << 30
		if arr.IsObject() {
			if o := vm.heap.Get(arr.Handle()); o != nil && o.kind == KindArray {
				end = len(o.elements)
			}
		}
		vm.push(vm.BuiltinArraySlice(arr, start, end))
	case OpArrayPush:
		val := vm.pop()
		arr := vm.peek(0)
		vm.BuiltinArrayPush(arr, val)
	case OpArraySpread:
		src := vm.pop()
		dest := vm.peek(0)
		if src.IsObject() {
			if o := vm.heap.Get(src.Handle()); o != nil && (o.kind == KindArray || o.kind == KindArguments) {
				vm.BuiltinArrayPush(dest, o.elements...)
			}
		}
	case OpObjectRest:
		excl := vm.pop()
		src := vm.pop()
		rest, err := vm.objectRest(src, excl)
		if err != nil {
			return err
		}
		vm.push(rest)
	case OpCallSpread:
		arr := vm.pop()
		n := 0
		var els []Value
		if arr.IsObject() {
			if o := vm.heap.Get(arr.Handle()); o != nil && o.kind == KindArray {
				els = append([]Value(nil), o.elements...)
				n = len(els)
			}
		}
		for _, v := range els {
			vm.push(v)
		}
		return vm.call(fr, n)
	case OpCallSpreadThis:
		arr := vm.pop()
		callee := vm.pop()
		thisVal := vm.pop()
		n := 0
		var els []Value
		if arr.IsObject() {
			if o := vm.heap.Get(arr.Handle()); o != nil && o.kind == KindArray {
				els = append([]Value(nil), o.elements...)
				n = len(els)
			}
		}
		vm.push(callee)
		for _, v := range els {
			vm.push(v)
		}
		return vm.callInternal(fr, n, thisVal, Undefined, Undefined)
	case OpObjectAssign:
		src := vm.pop()
		dest := vm.peek(0)
		vm.objectAssign(dest, src)

	case OpGetLocal:
		v, err := vm.readVar(fr, arg)
		if err != nil {
			return err
		}
		if v == vm.tdzTok {
			return vm.throwText(fr, "ReferenceError: Cannot access binding before initialization")
		}
		vm.push(v)
	case OpSetLocal:
		if err := vm.writeVar(fr, arg, vm.peek(0)); err != nil {
			return err
		}

	case OpGetGlobal:
		name, err := vm.constText(fr, arg)
		if err != nil {
			return err
		}
		key := vm.heap.Intern().Intern(name)
		v, ok := vm.bindGlobal(key)
		if !ok {
			return vm.throwText(fr, "ReferenceError: "+name.GoString()+" is not defined")
		}
		if v == vm.tdzTok && name.GoString() != "\x00tdz" {
			return vm.throwText(fr, "ReferenceError: Cannot access '"+name.GoString()+"' before initialization")
		}
		vm.push(v)
	case OpSetGlobal:
		raw, err := vm.constText(fr, arg)
		if err != nil {
			return err
		}
		name := vm.heap.Intern().Intern(raw)
		if cur, ok := vm.globals[name]; ok && cur == vm.tdzTok {
			return vm.throwText(fr, "ReferenceError: Cannot access '"+name.GoString()+"' before initialization")
		}
		if _, ok := vm.bindGlobal(name); !ok {
			if fr != nil && fr.chunk != nil && fr.chunk.Strict {
				return vm.throwText(fr, "ReferenceError: "+name.GoString()+" is not defined")
			}
		}
		vm.storeGlobal(name, vm.peek(0))
		if vm.globalObj != NoHandle && vm.heap.Get(vm.globalObj) != nil {
			vm.heap.SetProperty(vm.globalObj, name, vm.peek(0))
		}
	case OpDefGlobal:
		raw, err := vm.constText(fr, arg)
		if err != nil {
			return err
		}
		key := vm.heap.Intern().Intern(raw)
		v := vm.pop()
		vm.storeGlobal(key, v)
		if vm.globalObj != NoHandle && vm.heap.Get(vm.globalObj) != nil {
			vm.heap.SetProperty(vm.globalObj, key, v)
		}
	case OpGetGlobalSoft:
		name, err := vm.constText(fr, arg)
		if err != nil {
			return err
		}
		v, ok := vm.bindGlobal(vm.heap.Intern().Intern(name))
		if !ok {
			v = Undefined
		}
		vm.push(v)

	case OpGetProp:
		name, err := vm.constText(fr, arg)
		if err != nil {
			return err
		}
		obj := vm.pop()
		if obj.IsNull() || obj.IsUndefined() {
			return vm.throwText(fr, "TypeError: Cannot read properties of "+vm.toDisplayString(obj)+" (reading '"+name.GoString()+"')")
		}
		v, err := vm.getPropInvoke(obj, name)
		if err != nil {
			return err
		}
		vm.push(v)
	case OpSetProp:
		name, err := vm.constText(fr, arg)
		if err != nil {
			return err
		}
		val := vm.pop()
		obj := vm.pop()
		if err := vm.setProp(fr, obj, name, val); err != nil {
			return err
		}
		vm.push(val)
	case OpGetElem:
		key := vm.pop()
		obj := vm.pop()
		if obj.IsNull() || obj.IsUndefined() {
			k := vm.keyString(key)
			return vm.throwText(fr, "TypeError: Cannot read properties of "+vm.toDisplayString(obj)+" (reading '"+k.GoString()+"')")
		}
		vm.push(vm.getElem(obj, key))
	case OpSetElem:
		val := vm.pop()
		key := vm.pop()
		obj := vm.pop()
		if err := vm.setElem(fr, obj, key, val); err != nil {
			return err
		}
		vm.push(val)

	case OpNewObject:
		vm.push(ObjectValue(vm.heap.NewObject()))
	case OpNewArray:
		n := int(arg)
		h := vm.heap.NewArray(n)
		vm.push(ObjectValue(h))
		for i := n - 1; i >= 0; i-- {
			v := vm.peek(1)
			vm.heap.SetElement(h, i, v)
			arr := vm.pop()
			vm.pop()
			vm.push(arr)
		}

	case OpJump:
		return vm.applyJump(fr, arg)
	case OpJumpIfFalse:
		if !vm.truthy(vm.pop()) {
			return vm.applyJump(fr, arg)
		}
	case OpJumpIfTrue:
		if vm.truthy(vm.pop()) {
			return vm.applyJump(fr, arg)
		}

	case OpCall:
		return vm.call(fr, int(arg))

	case OpCallMethod:
		argc := int(arg >> 16)
		constIdx := uint32(arg & 0xFFFF)
		name, err := vm.constText(fr, constIdx)
		if err != nil {
			return err
		}
		return vm.callMethod(fr, argc, name)

	case OpNew:
		return vm.instantiate(fr, int(arg))

	case OpNewSpread:
		n, err := vm.pushSpreadArgs()
		if err != nil {
			return err
		}
		return vm.instantiate(fr, n)

	case OpSuperCallSpread:
		n, err := vm.pushSpreadArgs()
		if err != nil {
			return err
		}
		return vm.superCall(fr, n)

	case OpThis:
		vm.push(vm.CurrentThis())

	case OpNewTarget:
		t := Undefined
		if len(vm.frames) > 0 {
			t = vm.frames[len(vm.frames)-1].newTarget
		}
		vm.push(t)

	case OpReturn:
		v, done, err := vm.ret(vm.pop())
		if err != nil {
			return err
		}
		if done {
			return &finished{value: v}
		}

	case OpThrow:
		v := vm.pop()
		return &Throw{Value: v, Text: vm.throwDisplay(v), Line: vm.line(fr)}

	case OpHalt:
		v := Undefined
		if vm.heap.StackLen() > fr.base {
			v = vm.pop()
		}
		if vm.retMin > 0 && len(vm.frames) > vm.retMin {
			vm.heap.TruncateStack(fr.base)
			vm.frames = vm.frames[:len(vm.frames)-1]
			return &finished{value: v}
		}
		return &finished{value: v}

	case OpYield:
		return &yieldSuspend{value: vm.pop(), star: arg != 0}

	case OpAwait:
		return &yieldSuspend{value: vm.pop(), await: true}

	case OpGetIterator:
		it, err := vm.getIterator(vm.pop())
		if err != nil {
			return err
		}
		vm.push(it)
		vm.keep = Undefined

	case OpDefAccessor:
		fn := vm.pop()
		obj := vm.peek(0)
		nameIdx := arg & 0x7FFFFFFF
		name, err := vm.constText(fr, nameIdx)
		if err != nil {
			return err
		}
		if err := vm.defineAccessor(obj, name, fn, arg>>31 != 0); err != nil {
			return err
		}

	case OpSetHomeObject:
		fn, obj := vm.peek(0), vm.peek(1)
		if fn.IsObject() && obj.IsObject() {
			if o := vm.heap.Get(fn.Handle()); o != nil {
				o.homeObject = obj.Handle()
			}
		}
	case OpSuperGet:
		name, err := vm.constText(fr, arg)
		if err != nil {
			return err
		}
		v, err := vm.superGet(fr, name)
		if err != nil {
			return err
		}
		vm.push(v)

	case OpDelete:
		if arg == 0 {
			key := vm.pop()
			obj := vm.pop()
			ok := vm.deleteKey(obj, vm.keyString(key))
			vm.push(Bool(ok))
		} else if arg>>31 != 0 {
			name, err := vm.constText(fr, arg&0x7FFFFFFF)
			if err != nil {
				return err
			}
			ok := vm.deleteGlobal(name)
			vm.push(Bool(ok))
		} else {
			name, err := vm.constText(fr, arg-1)
			if err != nil {
				return err
			}
			obj := vm.pop()
			ok := vm.deleteKey(obj, name)
			vm.push(Bool(ok))
		}

	case OpSuperCall:
		return vm.superCall(fr, int(arg))

	case OpSetProto:
		proto := vm.pop()
		obj := vm.peek(0)
		if obj.IsObject() && proto.IsObject() {
			vm.heap.SetProto(obj.Handle(), proto.Handle())
		}

	case OpExport:
		name, err := vm.constText(fr, arg)
		if err != nil {
			return err
		}
		key := vm.heap.Intern().Intern(name)
		val := vm.peek(0)
		if vm.moduleExports == nil {
			vm.moduleExports = make(map[*str.String]Value)
		}
		vm.moduleExports[key] = val

	default:
		return vm.fault(fr, fmt.Sprintf("opcode %d non implémenté", op))
	}
	return nil
}

// readVar lit une variable désignée par profondeur et emplacement.
func (vm *VM) readVar(fr *frame, arg uint32) (Value, error) {
	depth, slot := int(arg>>16), int(arg&0xFFFF)
	env := fr.env
	for d := 0; d < depth; d++ {
		o := vm.heap.Get(env)
		if o == nil {
			return Undefined, vm.fault(fr, "environnement absent")
		}
		env = o.proto
	}
	o := vm.heap.Get(env)
	if o == nil || slot >= len(o.elements) {
		return Undefined, vm.fault(fr, "emplacement local hors borne")
	}
	return o.elements[slot], nil
}

func (vm *VM) writeVar(fr *frame, arg uint32, v Value) error {
	depth, slot := int(arg>>16), int(arg&0xFFFF)
	env := fr.env
	for d := 0; d < depth; d++ {
		o := vm.heap.Get(env)
		if o == nil {
			return vm.fault(fr, "environnement absent")
		}
		env = o.proto
	}
	o := vm.heap.Get(env)
	if o == nil || slot >= len(o.elements) {
		return vm.fault(fr, "emplacement local hors borne")
	}
	o.elements[slot] = v
	return nil
}

// call empile une trame d'appel avec this = CurrentThis().
func (vm *VM) defineNative(obj Handle, name string, params int, fn func(*VM, []Value) (Value, error)) {
	ch := &Chunk{Name: name, Params: params, Native: fn}
	fv := ObjectValue(vm.heap.NewFunction(ch, NoHandle))
	vm.heap.AddRoot(&fv)
	vm.heap.SetProperty(obj, vm.heap.Intern().InternGo(name), fv)
	vm.heap.RemoveRoot(&fv)
}

func (vm *VM) invoke(this, callee Value, args []Value) (Value, error) {
	if !vm.isFunction(callee) {
		return Undefined, nil
	}
	return vm.CallFunction(callee, this, args)
}

func (vm *VM) toPrimitive(v Value, hint string) (Value, error) {
	if vm.isPrimitive(v) {
		return v, nil
	}
	o := vm.heap.Get(v.Handle())
	if o == nil {
		return vm.NewStringValue(str.FromGo("")), nil
	}
	switch o.kind {
	case KindArray, KindFunction, KindRegExp:
		return vm.NewStringValue(str.FromGo(vm.toDisplayString(v))), nil
	}
	names := []string{"valueOf", "toString"}
	if hint == "string" {
		names = []string{"toString", "valueOf"}
	}
	for _, n := range names {
		m := vm.getProp(v, vm.heap.Intern().InternGo(n))
		if !vm.isFunction(m) {
			continue
		}
		res, err := vm.invoke(v, m, nil)
		if err != nil {
			return Undefined, err
		}
		if vm.isPrimitive(res) {
			return res, nil
		}
	}
	var fr *frame
	if len(vm.frames) > 0 {
		fr = &vm.frames[len(vm.frames)-1]
	}
	return Undefined, vm.throwText(fr, "TypeError: Cannot convert object to primitive value")
}

func (vm *VM) isFunction(v Value) bool {
	if !v.IsObject() {
		return false
	}
	o := vm.heap.Get(v.Handle())
	return o != nil && o.kind == KindFunction && o.fn != nil
}

func (vm *VM) newBigInt(n int64) Value {
	h := vm.heap.NewObject()
	if o := vm.heap.Get(h); o != nil {
		o.kind = KindBigInt
		o.bigInt = n
	}
	return ObjectValue(h)
}

func (vm *VM) bigIntOf(v Value) (int64, bool) {
	if !v.IsObject() {
		return 0, false
	}
	o := vm.heap.Get(v.Handle())
	if o == nil || o.kind != KindBigInt {
		return 0, false
	}
	return o.bigInt, true
}

func (vm *VM) EnqueueMicrotask(run func() error, hold ...Value) {
	if run == nil {
		return
	}
	vm.jobs = append(vm.jobs, microJob{run: run, hold: hold})
}

func (vm *VM) RunMicrotasks() error {
	for len(vm.jobs) > 0 {
		batch := vm.jobs
		vm.jobs = nil
		for _, j := range batch {
			if j.run == nil {
				continue
			}
			if err := j.run(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (vm *VM) CallFunction(fn, thisVal Value, args []Value) (Value, error) {
	if vm.Yielding() {
		return Undefined, fmt.Errorf("js55: callback rejected during execution checkpoint")
	}
	boundary := vm.frameBoundary()
	vm.heap.AddRoot(&boundary.keep)
	defer vm.heap.RemoveRoot(&boundary.keep)
	defer vm.restoreFrameBoundary(boundary)
	minFrames := boundary.frames
	vm.push(fn)
	for _, a := range args {
		vm.push(a)
	}
	dummy := frame{}
	fr := &dummy
	if minFrames > 0 {
		fr = &vm.frames[minFrames-1]
	}
	if err := vm.callInternal(fr, len(args), thisVal, Undefined, Undefined); err != nil {
		return Undefined, err
	}
	if len(vm.frames) == minFrames {
		return vm.pop(), nil
	}
	return vm.interpret(minFrames)
}

func (vm *VM) call(fr *frame, argc int) error {
	return vm.callInternal(fr, argc, vm.CurrentThis(), Undefined, Undefined)
}

// callMethod empile un appel de méthode sur receiver avec this = receiver.
func (vm *VM) callMethod(fr *frame, argc int, name *str.String) error {
	receiver := vm.peek(argc)
	// The receiver and arguments remain rooted on the operand stack while an
	// accessor executes. Propagate its exception before replacing the receiver.
	callee, err := vm.getPropInvoke(receiver, name)
	if err != nil {
		return err
	}
	// Remplacer le receveur par la fonction sur la pile
	vm.setStack(vm.heap.StackLen()-1-argc, callee)
	return vm.callInternal(fr, argc, receiver, Undefined, Undefined)
}

// instantiate crée une instance et exécute son constructeur.
func (vm *VM) pushSpreadArgs() (int, error) {
	arr := vm.pop()
	n := 0
	var els []Value
	if arr.IsObject() {
		if o := vm.heap.Get(arr.Handle()); o != nil && (o.kind == KindArray || o.kind == KindArguments) {
			els = append([]Value(nil), o.elements...)
			n = len(els)
		}
	}
	for _, v := range els {
		vm.push(v)
	}
	return n, nil
}

func (vm *VM) instantiate(fr *frame, argc int) error {
	return vm.instantiateAs(fr, argc, vm.peek(argc))
}

func (vm *VM) instantiateAs(fr *frame, argc int, newTarget Value) error {
	callee := vm.peek(argc)
	if !callee.IsObject() {
		return vm.throwText(fr, "TypeError: "+vm.toDisplayString(callee)+" is not a constructor")
	}
	o := vm.heap.Get(callee.Handle())
	if o != nil && o.kind == KindProxy {
		instVal := ObjectValue(vm.heap.NewObject())
		vm.heap.AddRoot(&instVal)
		defer vm.heap.RemoveRoot(&instVal)
		return vm.callInternal(fr, argc, instVal, instVal, newTarget)
	}
	if o == nil || o.kind != KindFunction {
		return vm.throwText(fr, "TypeError: "+vm.toDisplayString(callee)+" is not a constructor")
	}
	if o.fn != nil && o.fn.Native != nil && !o.fn.Construct {
		return vm.throwText(fr, "TypeError: "+vm.toDisplayString(callee)+" is not a constructor")
	}

	instance := vm.heap.NewObject()
	protoSrc := newTarget
	if !protoSrc.IsObject() {
		protoSrc = callee
	}
	protoProp, ok := vm.heap.GetProperty(protoSrc.Handle(), vm.heap.Intern().InternGo("prototype"))
	if ok && protoProp.IsObject() {
		if instObj := vm.heap.Get(instance); instObj != nil {
			instObj.proto = protoProp.Handle()
		}
	}
	instVal := ObjectValue(instance)
	return vm.callInternal(fr, argc, instVal, instVal, newTarget)
}

func (vm *VM) setStack(idx int, v Value) {
	if idx >= 0 && idx < len(vm.heap.stack) {
		vm.heap.stack[idx] = v
	}
}

func (vm *VM) makeArguments(argc int, env Handle, nparams int) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	if o := vm.heap.Get(h); o != nil {
		o.kind = KindArguments
		o.env = env
		o.prim = Int(int32(nparams))
	}
	for i := 0; i < argc; i++ {
		v := vm.peek(argc - 1 - i)
		vm.heap.SetElement(h, i, v)
		vm.heap.SetProperty(h, vm.heap.Intern().InternGo(strconv.Itoa(i)), v)
	}
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("length"), Int(int32(argc)))
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("callee"), vm.peek(argc))
	if vm.heap.arrayProto != NoHandle {
		if valFn, ok := vm.heap.GetProperty(vm.heap.arrayProto, vm.heap.Intern().InternGo("values")); ok {
			vm.heap.SetProperty(h, SymbolIterator, valFn)
		}
	}
	return hv
}

func (vm *VM) callInternal(fr *frame, argc int, thisVal Value, newObj, newTarget Value) error {
	// The receiver is not on the operand stack while its environment is
	// allocated. Keep it alive until ownership transfers to the call stacks.
	vm.heap.AddRoot(&thisVal)
	vm.heap.AddRoot(&newObj)
	vm.heap.AddRoot(&newTarget)
	defer vm.heap.RemoveRoot(&thisVal)
	defer vm.heap.RemoveRoot(&newObj)
	defer vm.heap.RemoveRoot(&newTarget)
	if len(vm.frames) >= vm.MaxDepth {
		return vm.throwText(fr, "RangeError: Maximum call stack size exceeded")
	}

	callee := vm.peek(argc)
	o := vm.heap.Get(callee.Handle())
	archtimeProxyCall := o != nil && o.kind == KindProxy
	if o != nil && o.kind == KindProxy && o.proxy != nil {
		handled, err := vm.proxyDispatch(fr, o.proxy, argc, thisVal, newObj, newTarget)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
		vm.setStack(vm.heap.StackLen()-1-argc, o.proxy.target)
		callee = o.proxy.target
		o = vm.heap.Get(callee.Handle())
	}
	if !callee.IsObject() || o == nil || o.kind != KindFunction {
		return vm.throwText(fr, "TypeError: "+vm.toDisplayString(callee)+" is not a function")
	}
	if o.fn != nil && o.fn.Arrow {
		thisVal = o.prim
	}
	if o.fn != nil && !o.fn.Strict && !o.fn.Arrow && (thisVal.IsUndefined() || thisVal.IsNull()) {
		thisVal = ObjectValue(vm.globalObj)
	}

	if o.fn != nil && (o.fn.archtimeTag == archtimeTagBox3SetFromBufferAttribute || o.fn.archtimeTag == archtimeTagComputeVertexNormals || o.fn.archtimeTag == archtimeTagNormalizeNormals) && !archtimeProxyCall && !newObj.IsObject() && !newTarget.IsObject() {
		if o.fn.archtimeTag == archtimeTagBox3SetFromBufferAttribute && !vm.DisableArchtimeGeometry && archtimeGeometryAvailable {
			handled, ret, err := vm.tryArchtimeGeometry(fr, o.fn, argc, thisVal)
			if handled {
				if err != nil {
					return err
				}
				vm.heap.TruncateStack(vm.heap.StackLen() - argc - 1)
				vm.push(ret)
				return nil
			}
		}

		if (o.fn.archtimeTag == archtimeTagComputeVertexNormals || o.fn.archtimeTag == archtimeTagNormalizeNormals) && !vm.DisableArchtimeNormals {
			handledNormals, retNormals, errNormals := vm.tryArchtimeNormals(fr, o, argc, thisVal)
			if handledNormals {
				if errNormals != nil {
					return errNormals
				}
				vm.heap.TruncateStack(vm.heap.StackLen() - argc - 1)
				vm.push(retNormals)
				return nil
			}
		}
	}

	if o.fn != nil && o.fn.Native != nil {
		args := make([]Value, argc)
		for i := 0; i < argc; i++ {
			args[i] = vm.peek(argc - 1 - i)
			vm.heap.AddRoot(&args[i])
			defer vm.heap.RemoveRoot(&args[i])
		}
		vm.heap.AddRoot(&callee)
		defer vm.heap.RemoveRoot(&callee)
		vm.heap.TruncateStack(vm.heap.StackLen() - argc - 1)
		previousThis, previousNew := vm.thisStack, vm.newStack
		vm.thisStack = append(vm.thisStack, thisVal)
		vm.newStack = append(vm.newStack, newObj)
		res, err := o.fn.Native(vm, args)
		vm.thisStack, vm.newStack = previousThis, previousNew
		if err != nil {
			if _, ok := err.(*Throw); ok {
				return err
			}
			if errors.Is(err, ErrGasExhausted) || errors.Is(err, ErrInterrupted) {
				return err
			}
			return vm.throwText(fr, err.Error())
		}
		if newObj.IsObject() && !res.IsObject() {
			res = newObj
		}
		vm.push(res)
		return nil
	}

	env := vm.acquireFrameEnv(o.fn, o.env)
	envV := ObjectValue(env)
	vm.heap.AddRoot(&envV)
	defer vm.heap.RemoveRoot(&envV)
	envObj := vm.heap.MustGet(env)
	for i := 0; i < o.fn.Params && i < argc && i < len(envObj.elements); i++ {
		envObj.elements[i] = vm.peek(argc - 1 - i)
	}
	if s := o.fn.SelfSlot; s >= 0 && s < len(envObj.elements) {
		envObj.elements[s] = callee
	}
	if s := o.fn.ArgumentsSlot; s >= 0 && s < len(envObj.elements) {
		nparams := o.fn.Params
		if o.fn.Strict {
			nparams = 0
		}
		envObj.elements[s] = vm.makeArguments(argc, env, nparams)
	}
	if s := o.fn.RestSlot; s >= 0 && s < len(envObj.elements) {
		nrest := argc - o.fn.Params
		if nrest < 0 {
			nrest = 0
		}
		h := vm.heap.NewArray(nrest)
		hv := ObjectValue(h)
		vm.heap.AddRoot(&hv)
		for i := 0; i < nrest; i++ {
			vm.heap.SetElement(h, i, vm.peek(argc-1-(o.fn.Params+i)))
		}
		envObj.elements[s] = hv
		vm.heap.RemoveRoot(&hv)
	}
	vm.heap.TruncateStack(vm.heap.StackLen() - argc - 1)
	if o.fn.Async {
		gen := vm.newGenerator(o.fn, env, thisVal)
		vm.heap.AddRoot(&gen)
		defer vm.heap.RemoveRoot(&gen)
		p := vm.newPromise()
		pv := ObjectValue(p)
		vm.heap.AddRoot(&pv)
		defer vm.heap.RemoveRoot(&pv)
		vm.startAsync(p, gen, Undefined)
		vm.push(pv)
		return nil
	}
	if o.fn.Generator {
		vm.push(vm.newGenerator(o.fn, env, thisVal))
		return nil
	}

	vm.thisStack = append(vm.thisStack, thisVal)
	vm.newStack = append(vm.newStack, newObj)
	nt := newTarget
	if !nt.IsObject() && newObj.IsObject() {
		nt = callee
	}

	vm.frames = append(vm.frames, frame{
		chunk:     o.fn,
		env:       env,
		base:      vm.heap.StackLen(),
		newTarget: nt,
		callee:    callee,
	})
	return nil
}

// ─── Conversions et opérateurs ──────────────────────────────────────────────

func (vm *VM) truthy(v Value) bool {
	if s := vm.StringOf(v); s != nil {
		return s.Len() != 0
	}
	if n, ok := vm.bigIntOf(v); ok {
		return n != 0
	}
	if v.IsObject() {
		return true
	}
	return v.ToBool()
}

func (vm *VM) typeOf(v Value) string {
	switch {
	case v.IsUndefined():
		return "undefined"
	case v.IsNull():
		return "object"
	case v.IsBool():
		return "boolean"
	case vm.IsString(v):
		return "string"
	case v.IsObject():
		if o := vm.heap.Get(v.Handle()); o != nil {
			switch o.kind {
			case KindFunction:
				return "function"
			case KindBigInt:
				return "bigint"
			case KindSymbol:
				return "symbol"
			}
		}
		return "object"
	default:
		return "number"
	}
}

func (vm *VM) toNumber(v Value) float64 {
	f, err := vm.toNumberErr(v)
	if err != nil {
		return math.NaN()
	}
	return f
}

func (vm *VM) toNumberErr(v Value) (float64, error) {
	if _, ok := vm.bigIntOf(v); ok {
		return 0, vm.throwText(nil, "TypeError: Cannot convert a BigInt value to a number")
	}
	if vm.isSymbol(v) {
		return 0, vm.throwText(nil, "TypeError: Cannot convert a Symbol value to a number")
	}
	if s := vm.StringOf(v); s != nil {
		return parseNumber(s.GoString()), nil
	}
	if v.IsObject() {
		p, err := vm.toPrimitive(v, "number")
		if err != nil {
			return 0, err
		}
		return vm.toNumberErr(p)
	}
	if v.IsUndefined() {
		return math.NaN(), nil
	}
	if v.IsNull() {
		return 0, nil
	}
	return v.ToFloat(), nil
}

func (vm *VM) pop2num() (a, b float64, err error) {
	bv, av := vm.peek(0), vm.peek(1)
	a, err = vm.toNumberErr(av)
	if err != nil {
		return 0, 0, err
	}
	b, err = vm.toNumberErr(bv)
	if err != nil {
		return 0, 0, err
	}
	vm.pop()
	vm.pop()
	return a, b, nil
}

func (vm *VM) toInt32(v Value) int32 {
	f := vm.toNumber(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return int32(uint32(int64(f)))
}

// add applique la sémantique de « + » : concaténation si l'un des opérandes est
// une chaîne, addition numérique sinon.
func (vm *VM) add(a, b Value) (Value, error) {
	_, aBI := vm.bigIntOf(a)
	_, bBI := vm.bigIntOf(b)
	if aBI != bBI {
		var fr *frame
		if len(vm.frames) > 0 {
			fr = &vm.frames[len(vm.frames)-1]
		}
		return Undefined, vm.throwText(fr, "TypeError: Cannot mix BigInt and other types, use explicit conversions")
	}
	var err error
	a, err = vm.toPrimitive(a, "default")
	if err != nil {
		return Undefined, err
	}
	b, err = vm.toPrimitive(b, "default")
	if err != nil {
		return Undefined, err
	}
	sa, sb := vm.StringOf(a), vm.StringOf(b)
	if sa != nil || sb != nil {
		if sa == nil {
			sa = str.FromGo(vm.toDisplayString(a))
		}
		if sb == nil {
			sb = str.FromGo(vm.toDisplayString(b))
		}
		return vm.NewStringValue(sa.Concat(sb)), nil
	}
	return Number(vm.toNumber(a) + vm.toNumber(b)), nil
}

func (vm *VM) strictEq(a, b Value) bool {
	if na, oka := vm.bigIntOf(a); oka {
		nb, okb := vm.bigIntOf(b)
		return okb && na == nb
	}
	if _, okb := vm.bigIntOf(b); okb {
		return false
	}
	sa, sb := vm.StringOf(a), vm.StringOf(b)
	if sa != nil || sb != nil {
		return sa != nil && sb != nil && sa.Equal(sb)
	}
	if a.IsObject() || b.IsObject() {
		return a.IsObject() && b.IsObject() && a.Handle() == b.Handle()
	}
	if a.IsUndefined() || b.IsUndefined() {
		return a.IsUndefined() && b.IsUndefined()
	}
	if a.IsNull() || b.IsNull() {
		return a.IsNull() && b.IsNull()
	}
	if a.IsBool() || b.IsBool() {
		return a.IsBool() && b.IsBool() && a.ToBool() == b.ToBool()
	}
	fa, fb := a.ToFloat(), b.ToFloat()
	return fa == fb // NaN != NaN, comme le prescrit le langage
}

func (vm *VM) looseEq(a, b Value) bool {
	if (a.IsNull() || a.IsUndefined()) && (b.IsNull() || b.IsUndefined()) {
		return true
	}
	if a.IsNull() || a.IsUndefined() || b.IsNull() || b.IsUndefined() {
		return false
	}
	sa, sb := vm.StringOf(a), vm.StringOf(b)
	if sa != nil && sb != nil {
		return sa.Equal(sb)
	}
	if a.IsObject() && b.IsObject() && sa == nil && sb == nil {
		return a.Handle() == b.Handle()
	}
	return vm.toNumber(a) == vm.toNumber(b)
}

func (vm *VM) compare(op Op, a, b Value) Value {
	v, _ := vm.compareErr(op, a, b)
	return v
}

func (vm *VM) compareErr(op Op, a, b Value) (Value, error) {
	if a.IsObject() {
		p, err := vm.toPrimitive(a, "number")
		if err != nil {
			return False, err
		}
		a = p
	}
	if b.IsObject() {
		p, err := vm.toPrimitive(b, "number")
		if err != nil {
			return False, err
		}
		b = p
	}
	sa, sb := vm.StringOf(a), vm.StringOf(b)
	if sa != nil && sb != nil {
		c := sa.Compare(sb)
		switch op {
		case OpLt:
			return Bool(c < 0), nil
		case OpGt:
			return Bool(c > 0), nil
		case OpLe:
			return Bool(c <= 0), nil
		default:
			return Bool(c >= 0), nil
		}
	}
	x, err := vm.toNumberErr(a)
	if err != nil {
		return False, err
	}
	y, err := vm.toNumberErr(b)
	if err != nil {
		return False, err
	}
	if math.IsNaN(x) || math.IsNaN(y) {
		return False, nil
	}
	switch op {
	case OpLt:
		return Bool(x < y), nil
	case OpGt:
		return Bool(x > y), nil
	case OpLe:
		return Bool(x <= y), nil
	default:
		return Bool(x >= y), nil
	}
}

// ─── Propriétés ─────────────────────────────────────────────────────────────

func (vm *VM) getProp(obj Value, name *str.String) Value {
	if v, ok := vm.proxyGet(obj, name); ok {
		return v
	}
	if s := vm.StringOf(obj); s != nil {
		if name.EqualASCII("length") {
			return Int(int32(s.Len()))
		}
		if ctor, ok := vm.GetGlobal("String"); ok && ctor.IsObject() {
			if proto, ok := vm.heap.GetProperty(ctor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
				return vm.getProp(proto, name)
			}
		}
		return Undefined
	}
	if !obj.IsObject() {
		ctorName := ""
		if obj.IsInt() || obj.IsNumber() {
			ctorName = "Number"
		} else if obj.IsBool() {
			ctorName = "Boolean"
		}
		if ctorName != "" {
			if ctor, ok := vm.GetGlobal(ctorName); ok && ctor.IsObject() {
				if proto, ok := vm.heap.GetProperty(ctor.Handle(), vm.heap.Intern().InternGo("prototype")); ok && proto.IsObject() {
					return vm.getProp(proto, name)
				}
			}
		}
		return Undefined
	}
	if name.EqualASCII("size") {
		if sz, ok := collectionSize(obj, vm.heap); ok {
			return sz
		}
	}
	h := obj.Handle()
	depth := 0
	for h != NoHandle && depth < 128 {
		o := vm.heap.Get(h)
		if o == nil {
			break
		}
		if o.typedName != "" {
			switch name.GoString() {
			case "length":
				return Number(float64(vm.arrayLikeLength(obj)))
			case "buffer":
				return ObjectValue(o.env)
			case "byteLength":
				return Number(float64(vm.arrayLikeLength(obj) * taByteSize(o.typedName)))
			case "byteOffset":
				if vm.taOutOfBounds(obj) {
					return Int(0)
				}
				return Number(float64(o.byteOffset))
			}
			if i, ok := arrayIndexKey(name); ok {
				v, _ := vm.typedArrayIndex(obj, i)
				return v
			}
		}
		if o.arrayBuffer {
			switch name.GoString() {
			case "byteLength":
				return Number(float64(len(o.bytes)))
			case "maxByteLength":
				return Number(float64(o.maxByteLength))
			case "resizable":
				return Bool(o.resizable)
			}
		}
		if o.kind == KindArray && name.EqualASCII("length") {
			return Int(int32(len(o.elements)))
		}
		if v, ok := vm.heap.GetProperty(h, name); ok {
			return v
		}
		h = o.proto
		depth++
	}
	return Undefined
}

func (vm *VM) setProp(fr *frame, obj Value, name *str.String, v Value) error {
	if handled, err := vm.proxySet(obj, name, v); handled {
		return err
	}
	if !obj.IsObject() || vm.IsString(obj) {
		return vm.throwText(fr, "TypeError: cannot set property on "+vm.toDisplayString(obj))
	}
	o := vm.heap.Get(obj.Handle())
	if o == nil {
		return vm.throwText(fr, "TypeError: cannot set property on a released object")
	}
	if o.frozen {
		return vm.throwText(fr, "TypeError: cannot assign to frozen object")
	}
	if o.typedName != "" {
		switch name.GoString() {
		case "length", "buffer", "byteLength", "byteOffset":
			return nil
		}
		if i, ok := arrayIndexKey(name); ok {
			return vm.setElem(fr, obj, Number(float64(i)), v)
		}
	}
	if o.arrayBuffer {
		switch name.GoString() {
		case "byteLength", "maxByteLength", "resizable":
			return nil
		}
	}
	if o.kind == KindArray && name.Equal(str.FromGo("length")) {
		vm.heap.AddRoot(&obj)
		vm.heap.AddRoot(&v)
		defer vm.heap.RemoveRoot(&obj)
		defer vm.heap.RemoveRoot(&v)
		desc := ObjectValue(vm.heap.NewObject())
		vm.heap.AddRoot(&desc)
		defer vm.heap.RemoveRoot(&desc)
		vm.heap.SetProperty(desc.Handle(), vm.heap.Intern().InternGo("value"), v)
		if err := vm.defineDataFromDesc(obj.Handle(), name, desc); err != nil {
			return vm.throwText(fr, err.Error())
		}
		return nil
	}
	if cur, ok := vm.heap.GetOwnProperty(obj.Handle(), name); ok && cur.IsObject() {
		if acc := vm.heap.Get(cur.Handle()); acc != nil && acc.kind == KindAccessor {
			if len(acc.elements) > 1 && vm.isFunction(acc.elements[1]) {
				_, err := vm.invoke(obj, acc.elements[1], []Value{v})
				return err
			}
			return vm.throwText(fr, "TypeError: no setter")
		}
	}
	// Heap.SetProperty is a low-level writer and silently rejects readonly
	// slots. The executing frame owns the ECMAScript strict-mode exception.
	key := vm.heap.intern.Intern(name)
	if !vm.heap.isDeleted(o, key) {
		if slot := o.shape.Lookup(key); slot >= 0 && o.slotAttr(slot)&attrWritable == 0 {
			if fr != nil && fr.chunk != nil && fr.chunk.Strict {
				return vm.throwTypeError(fr, "Cannot assign to read only property "+name.GoString())
			}
			return nil
		}
	}
	vm.heap.SetProperty(obj.Handle(), name, v)
	if obj.Handle() == vm.globalObj {
		vm.storeGlobal(vm.heap.Intern().Intern(name), v)
	}
	return nil
}

func (vm *VM) getElem(obj, key Value) Value {
	if s := vm.StringOf(obj); s != nil {
		if i, ok := vm.arrayIndex(key); ok && i < s.Len() {
			return vm.NewStringValue(s.Slice(i, i+1))
		}
		return vm.getProp(obj, vm.keyString(key))
	}
	if !obj.IsObject() {
		return Undefined
	}
	o := vm.heap.Get(obj.Handle())
	if o == nil {
		return Undefined
	}
	if o.kind == KindArray {
		if i, ok := vm.arrayIndex(key); ok {
			if o.env != NoHandle {
				v, _ := vm.typedArrayIndexObj(o, i)
				return v
			}
			return vm.heap.GetElement(obj.Handle(), i)
		}
	}
	if o.kind == KindArguments {
		if i, ok := vm.arrayIndex(key); ok {
			return vm.heap.GetElement(obj.Handle(), i)
		}
	}
	return vm.getProp(obj, vm.keyString(key))
}

func (vm *VM) setElem(fr *frame, obj, key, v Value) error {
	if !obj.IsObject() || vm.IsString(obj) {
		return vm.throwText(fr, "TypeError: cannot set element on "+vm.toDisplayString(obj))
	}
	o := vm.heap.Get(obj.Handle())
	if o == nil {
		return vm.throwText(fr, "TypeError: cannot set element on a released object")
	}
	if o.kind == KindArray {
		if i, ok := vm.arrayIndex(key); ok {
			if o.typedName != "" {
				var err error
				v, err = vm.coerceTypedElement(v, o.typedName)
				if err != nil {
					return err
				}
				if i >= vm.arrayLikeLength(obj) {
					return nil
				}
			}
			if o.env != NoHandle {
				return vm.typedArraySetIndex(fr, obj, i, v)
			}
			vm.heap.SetElement(obj.Handle(), i, v)
			return nil
		}
	}
	return vm.setProp(fr, obj, vm.keyString(key), v)
}

// arrayIndex reconnaît un indice de tableau entier positif.
func (vm *VM) arrayIndex(key Value) (int, bool) {
	if key.IsObject() {
		if o := vm.heap.Get(key.Handle()); o != nil && o.kind == KindSymbol {
			return 0, false
		}
	}
	if key.IsInt() {
		i := key.ToInt()
		return int(i), i >= 0
	}
	if s := vm.StringOf(key); s != nil {
		n := parseNumber(s.GoString())
		if n == math.Trunc(n) && n >= 0 && !math.IsInf(n, 0) {
			return int(n), true
		}
		return 0, false
	}
	f := vm.toNumber(key)
	if f == math.Trunc(f) && f >= 0 && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return int(f), true
	}
	return 0, false
}

// keyString convertit une valeur en clé de propriété.
func (vm *VM) keyString(key Value) *str.String {
	if s := vm.StringOf(key); s != nil {
		return s
	}
	if key.IsObject() {
		if o := vm.heap.Get(key.Handle()); o != nil && o.kind == KindSymbol {
			if o.text != nil {
				gs := o.text.GoString()
				if strings.HasPrefix(gs, "Symbol.") {
					return vm.heap.Intern().InternGo(gs)
				}
			}
			return vm.heap.Intern().InternGo(fmt.Sprintf("\x01s%d", uint64(key.Handle())))
		}
	}
	return str.FromGo(vm.toDisplayString(key))
}

func isSymbolKey(k *str.String) bool {
	if k == nil || k.Len() == 0 {
		return false
	}
	s := k.GoString()
	return len(s) > 0 && s[0] == 1
}
