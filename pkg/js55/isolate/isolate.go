// SPDX-License-Identifier: BUSL-1.1

// Package isolate encapsule une unité d'exécution étanche : un tas, un
// interpréteur, un bac à sable de fichiers, et des bornes de ressources.
//
// Deux isolats ne partagent RIEN — ni objet, ni variable globale, ni chaîne
// internée. C'est ce qui rend le modèle sûr sans verrou : le tas n'a pas à être
// protégé, puisqu'il n'a qu'un seul propriétaire.
package isolate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/hostcall"
	"github.com/hazyhaar/js55/pkg/js55/parser"
	"github.com/hazyhaar/js55/pkg/js55/runtime"
)

type nativeBinding struct {
	name  string
	arity int
	fn    func(args []engine.Value) (engine.Value, error)
}

var (
	ErrMemoryLimitExceeded    = errors.New("js55: limite de mémoire de l'isolat dépassée (ErrMemoryLimitExceeded)")
	ErrExecutionQuotaExceeded = errors.New("js55: quota d'instructions CPU dépassé (ErrExecutionQuotaExceeded)")
	ErrInterrupted            = errors.New("js55: exécution de l'isolat interrompue")
	// ErrClosed signale un appel sur un isolat dont Close a déjà libéré les
	// ressources. Il rend le cycle de vie déterministe au lieu de confondre
	// l'appelant avec ErrInterrupted ou de déréférencer un tas nil.
	ErrClosed = errors.New("js55: isolate is closed")
)

// Config règle les bornes d'un isolat.
type Config struct {
	MaxMemoryBytes int64
	// GasLimit borne le nombre d'instructions exécutées. Une boucle sans fin
	// s'arrête donc au lieu de bloquer l'hôte.
	GasLimit int64
	// StepLimit est l'alias ergonomique de GasLimit. Les deux champs bornent la
	// même jauge d'instructions : quand les deux sont positifs, le minimum des
	// deux s'applique ; un champ non positif n'exprime aucune contrainte.
	StepLimit int64
	// MaxDepth borne la profondeur d'appel : une récursion sans fin produit une
	// RangeError, jamais un dépassement de pile du processus.
	MaxDepth    int
	VirtualFS   *runtime.SandboxedFS
	POSIX       hostcall.POSIX
	FetchPolicy hostcall.FetchPolicy
	// StressGC déclenche une collecte complète à chaque allocation. La suite de
	// conformité doit rendre le même verdict avec et sans.
	StressGC bool
	// CheckpointEvery, s'il est > 0, cède périodiquement via OnCheckpoint
	// pour que timers et limite d'évaluation puissent interrompre l'hôte.
	CheckpointEvery int64
	OnCheckpoint    func() error
	HostNonMutating bool
}

// Isolate est une unité d'exécution étanche.
type Isolate struct {
	heap        *engine.Heap
	vm          *engine.VM
	fs          *runtime.SandboxedFS
	posix       hostcall.POSIX
	fetchPolicy hostcall.FetchPolicy

	allocated   atomic.Int64
	maxMemory   int64
	interrupted atomic.Bool
	closed      atomic.Bool

	fetchVal   engine.Value
	fetchChunk *engine.Chunk
	natives    []nativeBinding
}

// New crée un isolat.
func New(cfg Config) (*Isolate, error) {
	// StepLimit et GasLimit désignent la même jauge d'instructions. Lorsque les
	// deux sont fournis, la borne la plus stricte l'emporte ; un champ non
	// positif n'exprime aucune contrainte et ne masque jamais l'autre.
	gas := int64(10_000_000)
	switch {
	case cfg.StepLimit > 0 && cfg.GasLimit > 0:
		gas = cfg.StepLimit
		if cfg.GasLimit < gas {
			gas = cfg.GasLimit
		}
	case cfg.StepLimit > 0:
		gas = cfg.StepLimit
	case cfg.GasLimit > 0:
		gas = cfg.GasLimit
	}
	depth := cfg.MaxDepth
	if depth <= 0 {
		depth = 256
	}
	mem := cfg.MaxMemoryBytes
	if mem <= 0 {
		mem = 64 * 1024 * 1024 // 64 Mo par défaut
	}

	h := engine.NewHeap()
	h.SetStress(cfg.StressGC)

	vm := engine.NewVM(h)
	vm.SetGasLeft(gas)
	vm.MaxDepth = depth
	vm.CheckpointEvery = cfg.CheckpointEvery
	vm.HostNonMutating = cfg.HostNonMutating
	vm.OnCheckpoint = cfg.OnCheckpoint

	iso := &Isolate{
		heap:        h,
		vm:          vm,
		fs:          cfg.VirtualFS,
		posix:       cfg.POSIX,
		fetchPolicy: cfg.FetchPolicy,
		maxMemory:   mem,
	}
	vm.Interrupted = &iso.interrupted
	iso.installFetch()

	h.QuotaTracker = func(delta int64) error {
		if delta > 0 {
			for {
				curr := iso.allocated.Load()
				if curr < 0 {
					if iso.allocated.CompareAndSwap(curr, 0) {
						continue
					}
					continue
				}
				if curr > iso.maxMemory || delta > iso.maxMemory-curr {
					return ErrMemoryLimitExceeded
				}
				if iso.allocated.CompareAndSwap(curr, curr+delta) {
					break
				}
			}
		} else if delta < 0 {
			for {
				curr := iso.allocated.Load()
				next := curr + delta
				if next < 0 {
					next = 0
				}
				if iso.allocated.CompareAndSwap(curr, next) {
					break
				}
			}
		}
		return nil
	}

	h.Intern().SetAllocTracker(h.TrackAlloc)

	return iso, nil
}

// Interrupt demande l'arrêt de l'exécution en cours.
func (iso *Isolate) Interrupt() { iso.interrupted.Store(true) }

// ClearInterrupt réinitialise le verrou d'interruption si l'isolat n'est pas
// fermé. Il est destiné au retour d'une annulation éphémère de contexte : sans
// cette redescente, un isolat annulé une fois resterait interrompu pour tous
// les appels suivants.
func (iso *Isolate) ClearInterrupt() {
	if iso.closed.Load() {
		return
	}
	iso.interrupted.Store(false)
	if iso.vm != nil {
		iso.vm.Interrupted.Store(false)
	}
}

// Close libère les ressources associées à l'isolat. Une fois Close appelé,
// toute méthode d'exécution rend ErrClosed de façon déterministe, sans panique
// ni confusion avec ErrInterrupted. Close est idempotent.
func (iso *Isolate) Close() error {
	iso.closed.Store(true)
	iso.Interrupt()
	iso.natives = nil
	iso.heap = nil
	iso.vm = nil
	iso.allocated.Store(0)
	return nil
}

// Reset réinitialise l'isolat pour réutilisation dans un pool (sync.Pool).
// Il vide la mémoire allouée, restaure les quotas, vide les structures CoW
// et réinitialise l'état de l'interpréteur VM en O(1), sans réallocation.
func (iso *Isolate) Reset() (err error) {
	if iso.closed.Load() {
		return ErrClosed
	}
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("js55: reset panic: %v", r)
			}
			iso.closed.Store(true)
		}
	}()
	iso.ClearInterrupt()
	iso.allocated.Store(0)
	if iso.vm != nil {
		iso.vm.ResetState()
		if iso.heap != nil {
			iso.heap.ResetLocal(iso.vm.GlobalObj())
		}
	}
	iso.installFetch()
	for _, nb := range iso.natives {
		iso.installNative(nb.name, nb.arity, nb.fn)
	}
	iso.allocated.Store(0)
	return nil
}

// AllocatedMemory rend le volume actuel de mémoire allouée dans l'isolat (en octets).
func (iso *Isolate) AllocatedMemory() int64 { return iso.allocated.Load() }

// Heap rend le tas de l'isolat. Après Close, l'accès lève une panique typée
// ErrClosed au lieu de rendre un pointeur nul consommable par l'appelant.
func (iso *Isolate) Heap() *engine.Heap {
	if iso.closed.Load() {
		panic(ErrClosed)
	}
	return iso.heap
}

// VM rend l'interpréteur de l'isolat. Après Close, l'accès lève une panique
// typée ErrClosed au lieu de rendre un pointeur nul consommable.
func (iso *Isolate) VM() *engine.VM {
	if iso.closed.Load() {
		panic(ErrClosed)
	}
	return iso.vm
}

func (iso *Isolate) POSIX() hostcall.POSIX { return iso.posix }

func (iso *Isolate) FetchPolicy() hostcall.FetchPolicy { return iso.fetchPolicy }

func (iso *Isolate) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	return hostcall.Fetch(ctx, rawURL, iso.fetchPolicy)
}

func (iso *Isolate) FetchResponse(ctx context.Context, rawURL string) (*hostcall.Response, error) {
	return hostcall.FetchResponse(ctx, rawURL, iso.fetchPolicy)
}

func (iso *Isolate) installFetch() {
	if iso.fetchChunk == nil {
		iso.fetchChunk = &engine.Chunk{
			Name:   "fetch",
			Params: 1,
			Native: iso.nativeFetch,
		}
	}
	iso.fetchVal = engine.ObjectValue(iso.heap.NewFunction(iso.fetchChunk, engine.NoHandle))
	iso.vm.SetGlobal("fetch", iso.fetchVal)
}

func (iso *Isolate) nativeFetch(vm *engine.VM, args []engine.Value) (engine.Value, error) {
	promVal, ok := vm.GetGlobal("Promise")
	if !ok || !promVal.IsObject() {
		return engine.Undefined, fmt.Errorf("js55: constructeur Promise absent")
	}
	resolveProp := iso.heap.Intern().InternGo("resolve")
	rejectProp := iso.heap.Intern().InternGo("reject")
	resolveFn, _ := iso.heap.GetProperty(promVal.Handle(), resolveProp)
	rejectFn, _ := iso.heap.GetProperty(promVal.Handle(), rejectProp)

	if len(args) == 0 {
		errVal := vm.NewStringValue(iso.heap.Intern().InternGo("js55: url invalide"))
		return vm.CallFunction(rejectFn, promVal, []engine.Value{errVal})
	}

	raw := ""
	if s := vm.StringOf(args[0]); s != nil {
		raw = s.GoString()
	} else {
		raw = args[0].String()
	}

	resp, err := iso.FetchResponse(context.Background(), raw)
	if err != nil {
		errVal := vm.NewStringValue(iso.heap.Intern().InternGo(err.Error()))
		return vm.CallFunction(rejectFn, promVal, []engine.Value{errVal})
	}

	respObj := iso.buildResponseObject(vm, resp)
	return vm.CallFunction(resolveFn, promVal, []engine.Value{respObj})
}

func (iso *Isolate) buildResponseObject(vm *engine.VM, resp *hostcall.Response) engine.Value {
	h := iso.heap.NewObject()

	iso.heap.SetProperty(h, iso.heap.Intern().InternGo("ok"), engine.Bool(resp.OK))
	iso.heap.SetProperty(h, iso.heap.Intern().InternGo("status"), engine.Int(int32(resp.Status)))
	iso.heap.SetProperty(h, iso.heap.Intern().InternGo("statusText"), vm.NewStringValue(iso.heap.Intern().InternGo(resp.StatusText)))

	bodyStr := string(resp.Body)
	bodyVal := vm.NewStringValue(iso.heap.Intern().InternGo(bodyStr))
	iso.heap.SetProperty(h, iso.heap.Intern().InternGo("body"), bodyVal)

	textChunk := &engine.Chunk{
		Name:   "text",
		Params: 0,
		Native: func(vm *engine.VM, args []engine.Value) (engine.Value, error) {
			return bodyVal, nil
		},
	}
	textFn := engine.ObjectValue(iso.heap.NewFunction(textChunk, engine.NoHandle))
	iso.heap.SetProperty(h, iso.heap.Intern().InternGo("text"), textFn)

	jsonChunk := &engine.Chunk{
		Name:   "json",
		Params: 0,
		Native: func(vm *engine.VM, args []engine.Value) (engine.Value, error) {
			jsonVal, ok := vm.GetGlobal("JSON")
			if !ok || !jsonVal.IsObject() {
				return engine.Undefined, fmt.Errorf("js55: JSON global manquant")
			}
			parseFn, _ := vm.Heap().GetProperty(jsonVal.Handle(), vm.Heap().Intern().InternGo("parse"))
			return vm.CallFunction(parseFn, jsonVal, []engine.Value{bodyVal})
		},
	}
	jsonFn := engine.ObjectValue(iso.heap.NewFunction(jsonChunk, engine.NoHandle))
	iso.heap.SetProperty(h, iso.heap.Intern().InternGo("json"), jsonFn)

	if resp.Headers != nil {
		headersObj := iso.heap.NewObject()
		for k, v := range resp.Headers {
			iso.heap.SetProperty(headersObj, iso.heap.Intern().InternGo(strings.ToLower(k)), vm.NewStringValue(iso.heap.Intern().InternGo(v)))
		}
		getChunk := &engine.Chunk{
			Name:   "get",
			Params: 1,
			Native: func(vm *engine.VM, args []engine.Value) (engine.Value, error) {
				if len(args) == 0 {
					return engine.Null, nil
				}
				hdrName := ""
				if s := vm.StringOf(args[0]); s != nil {
					hdrName = strings.ToLower(s.GoString())
				}
				for hk, hv := range resp.Headers {
					if strings.EqualFold(hk, hdrName) {
						return vm.NewStringValue(iso.heap.Intern().InternGo(hv)), nil
					}
				}
				return engine.Null, nil
			},
		}
		getFn := engine.ObjectValue(iso.heap.NewFunction(getChunk, engine.NoHandle))
		iso.heap.SetProperty(headersObj, iso.heap.Intern().InternGo("get"), getFn)
		iso.heap.SetProperty(h, iso.heap.Intern().InternGo("headers"), engine.ObjectValue(headersObj))
	}

	return engine.ObjectValue(h)
}

// Compile analyse et compile une source sans l'exécuter.
func (iso *Isolate) Compile(source, name string, strict bool) (chunk *engine.Chunk, outErr error) {
	if iso.closed.Load() {
		return nil, ErrClosed
	}
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok && errors.Is(err, ErrMemoryLimitExceeded) {
				chunk, outErr = nil, err
				return
			}
			panic(r)
		}
	}()
	ts := strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx")
	prog, err := parser.Parse(source, parser.Options{Strict: strict, TypeScript: ts})
	if err != nil {
		return nil, err
	}
	return engine.CompileMode(iso.heap, prog, name, strict)
}

// Eval analyse, compile et exécute un script avec un contexte d'arrière-plan
// par défaut.
func (iso *Isolate) Eval(source string) (engine.Value, error) {
	if iso.closed.Load() {
		return engine.Undefined, ErrClosed
	}
	return iso.EvalContext(context.Background(), source)
}

// EvalContext analyse, compile et exécute une source sous la surveillance d'un
// contexte d'annulation, et rend la valeur de complétion. L'annulation en vol
// est propagée à la machine virtuelle par un chien de garde éphémère qui
// appelle Interrupt, y compris lorsque le script n'atteint jamais de point de
// contrôle configuré par l'appelant.
func (iso *Isolate) EvalContext(ctx context.Context, source string) (engine.Value, error) {
	if iso.closed.Load() {
		return engine.Undefined, ErrClosed
	}
	if ctx.Done() != nil {
		stopWatchdog := make(chan struct{})
		watchdogDone := make(chan struct{})
		var interruptedByContext atomic.Bool
		go func() {
			defer close(watchdogDone)
			select {
			case <-ctx.Done():
				interruptedByContext.Store(true)
				iso.Interrupt()
			case <-stopWatchdog:
			}
		}()
		// Joindre le chien de garde avant de rendre : sans cette attente, une
		// goroutine réveillée après la fin de l'évaluation pourrait poser le
		// verrou d'interruption persistant et fausser l'appel suivant. Une fois
		// la jointure faite, redescendre le verrou si l'annulation provient du
		// contexte : l'isolat redevient opérationnel pour l'appel suivant. Un
		// Interrupt explicite de l'hôte, hors ctx.Done, n'est jamais effacé.
		defer func() {
			close(stopWatchdog)
			<-watchdogDone
			if interruptedByContext.Load() && !iso.closed.Load() {
				iso.ClearInterrupt()
			}
		}()
	}
	chunk, err := iso.Compile(source, "eval", false)
	if err != nil {
		return engine.Undefined, err
	}
	return iso.Execute(ctx, chunk)
}

func (iso *Isolate) RunInContext(ctx context.Context, source string, sandbox ...engine.Value) (engine.Value, error) {
	if iso.closed.Load() {
		return engine.Undefined, ErrClosed
	}
	if len(sandbox) > 0 && sandbox[0].IsObject() {
		if err := iso.vm.Contextify(sandbox[0]); err != nil {
			return engine.Undefined, err
		}
	}
	return iso.EvalContext(ctx, source)
}

// EvalTimeout coupe l'exécution au délai mur, comme vm.runInContext(..., {timeout}).
func (iso *Isolate) EvalTimeout(parent context.Context, source string, d time.Duration) (engine.Value, error) {
	if iso.closed.Load() {
		return engine.Undefined, ErrClosed
	}
	ctx, cancel := context.WithTimeout(parent, d)
	defer cancel()
	return iso.EvalContext(ctx, source)
}

// Execute exécute une unité déjà compilée.
func (iso *Isolate) Execute(ctx context.Context, chunk *engine.Chunk) (val engine.Value, outErr error) {
	if iso.closed.Load() {
		return engine.Undefined, ErrClosed
	}
	if iso.interrupted.Load() {
		return engine.Undefined, ErrInterrupted
	}
	if ctx != nil && ctx.Done() != nil {
		select {
		case <-ctx.Done():
			return engine.Undefined, ctx.Err()
		default:
		}
	}

	defer func() {
		if r := recover(); r != nil {
			val = engine.Undefined
			if err, ok := r.(error); ok {
				outErr = err
				return
			}
			outErr = fmt.Errorf("js55: panique d'exécution interceptée : %v", r)
			return
		}
	}()

	val, err := iso.vm.Run(chunk)
	if errors.Is(err, engine.ErrGasExhausted) {
		return engine.Undefined, ErrExecutionQuotaExceeded
	}
	if errors.Is(err, engine.ErrInterrupted) {
		return engine.Undefined, ErrInterrupted
	}
	if err != nil {
		return val, err
	}
	if ctx != nil && ctx.Err() != nil {
		return engine.Undefined, ctx.Err()
	}
	if iso.interrupted.Load() {
		return engine.Undefined, ErrInterrupted
	}
	if derr := iso.vm.RunMicrotasks(); derr != nil {
		return val, derr
	}
	return val, nil
}

// NewUint8ArrayFromBytes crée un Uint8Array soutenu directement par la tranche d'octets Go hôte
// sans aucune copie mémoire (Zero-Copy). La référence Go est conservée par le tampon ArrayBuffer
// sans Pinner (0-CGO), et son volume est imputé une seule fois au quota de mémoire de l'isolat.
func (iso *Isolate) NewUint8ArrayFromBytes(b []byte) (retVal engine.Value, outErr error) {
	if iso.closed.Load() {
		return engine.Undefined, ErrClosed
	}

	var bufHandle engine.Handle
	size := int64(len(b))
	chargedSize := false

	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok && errors.Is(err, ErrMemoryLimitExceeded) {
				if chargedSize && iso.heap != nil {
					if bufHandle != engine.NoHandle {
						if bo := iso.heap.Get(bufHandle); bo != nil {
							bo.SetBytes(nil)
						}
					}
					iso.heap.TrackFree(size)
				}
				retVal = engine.Undefined
				outErr = ErrMemoryLimitExceeded
				return
			}
			panic(r)
		}
	}()

	abCtorVal, ok := iso.vm.GetGlobal("ArrayBuffer")
	if !ok || !abCtorVal.IsObject() {
		return engine.Undefined, fmt.Errorf("js55: constructeur ArrayBuffer absent")
	}
	abProto, _ := iso.heap.GetProperty(abCtorVal.Handle(), iso.heap.Intern().InternGo("prototype"))

	u8CtorVal, ok := iso.vm.GetGlobal("Uint8Array")
	if !ok || !u8CtorVal.IsObject() {
		return engine.Undefined, fmt.Errorf("js55: constructeur Uint8Array absent")
	}
	u8Proto, _ := iso.heap.GetProperty(u8CtorVal.Handle(), iso.heap.Intern().InternGo("prototype"))

	if size > 0 {
		if iso.heap.QuotaTracker != nil {
			if err := iso.heap.QuotaTracker(size); err != nil {
				return engine.Undefined, err
			}
			chargedSize = true
		}
	}

	bufHandle = iso.heap.NewObject()
	bufVal := engine.ObjectValue(bufHandle)
	iso.heap.AddRoot(&bufVal)
	defer iso.heap.RemoveRoot(&bufVal)

	bo := iso.heap.Get(bufHandle)
	if bo != nil {
		bo.SetProto(abProto.Handle())
		bo.SetArrayBuffer(true)
		bo.SetResizable(false)
		bo.SetBytes(b)
		bo.SetMaxByteLength(len(b))
	}

	viewHandle := iso.heap.NewArray(0)
	vo := iso.heap.Get(viewHandle)
	if vo != nil {
		vo.SetTypedName("Uint8Array")
		vo.SetEnv(bufHandle)
		vo.SetByteOffset(0)
		vo.SetTypedLength(len(b))
		vo.SetProto(u8Proto.Handle())
		// Ne pas poser SetBytes sur la vue : seul le tampon ArrayBuffer est porteur des octets,
		// prévenant tout double décrément lors du balayage GC.
	}

	return engine.ObjectValue(viewHandle), nil
}

// Bytes extrait la tranche d'octets Go sous-jacente d'une valeur ArrayBuffer ou TypedArray
// sans aucune copie. Le slice retourné utilise une capacité fermée [start:end:end] afin
// d'interdire tout append hôte accidentel écrasant la mémoire contiguë.
func (iso *Isolate) Bytes(v engine.Value) ([]byte, bool) {
	if !v.IsObject() || iso.heap == nil {
		return nil, false
	}
	o := iso.heap.Get(v.Handle())
	if o == nil {
		return nil, false
	}
	if o.Kind() != engine.KindOrdinary && o.Kind() != engine.KindArray {
		return nil, false
	}
	if o.ArrayBuffer() && o.Bytes() != nil {
		raw := o.Bytes()
		return raw[:len(raw):len(raw)], true
	}
	if o.TypedName() != "" {
		if o.Env() != engine.NoHandle {
			bo := iso.heap.Get(o.Env())
			if bo != nil && bo.Bytes() != nil {
				bpe := 1
				switch o.TypedName() {
				case "Int16Array", "Uint16Array", "Float16Array":
					bpe = 2
				case "Int32Array", "Uint32Array", "Float32Array":
					bpe = 4
				case "Float64Array", "BigInt64Array", "BigUint64Array":
					bpe = 8
				}
				start := o.ByteOffset()
				end := start + o.TypedLength()*bpe
				raw := bo.Bytes()
				if start <= len(raw) && end <= len(raw) {
					return raw[start:end:end], true
				}
			}
		}
		if o.Bytes() != nil {
			raw := o.Bytes()
			return raw[:len(raw):len(raw)], true
		}
	}
	return nil, false
}

// SetNative enregistre une fonction hôte Go directement appelable depuis JavaScript,
// sans réflexion reflect.ValueOf, en recevant directement les arguments sur la pile.
// Les fonctions hôtes sont mémorisées et automatiquement restaurées après chaque Reset().
func (iso *Isolate) SetNative(name string, arity int, fn func(args []engine.Value) (engine.Value, error)) error {
	if iso.closed.Load() {
		return ErrClosed
	}
	found := false
	for i, nb := range iso.natives {
		if nb.name == name {
			iso.natives[i] = nativeBinding{name: name, arity: arity, fn: fn}
			found = true
			break
		}
	}
	if !found {
		iso.natives = append(iso.natives, nativeBinding{name: name, arity: arity, fn: fn})
	}
	return iso.installNative(name, arity, fn)
}

func (iso *Isolate) installNative(name string, arity int, fn func(args []engine.Value) (engine.Value, error)) error {
	ch := &engine.Chunk{
		Name:   name,
		Params: arity,
		Native: func(vm *engine.VM, args []engine.Value) (engine.Value, error) {
			if vm.Interrupted != nil && vm.Interrupted.Load() {
				return engine.Undefined, engine.ErrInterrupted
			}
			if len(args) < arity {
				padded := make([]engine.Value, arity)
				copy(padded, args)
				for i := len(args); i < arity; i++ {
					padded[i] = engine.Undefined
				}
				args = padded
			}
			return fn(args)
		},
	}
	h := iso.heap.NewFunction(ch, engine.NoHandle)
	iso.vm.SetGlobal(name, engine.ObjectValue(h))
	return nil
}
