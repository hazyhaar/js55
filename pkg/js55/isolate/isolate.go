// SPDX-License-Identifier: Apache-2.0 OR MIT

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

var (
	ErrMemoryLimitExceeded    = errors.New("js55: limite de mémoire de l'isolat dépassée (ErrMemoryLimitExceeded)")
	ErrExecutionQuotaExceeded = errors.New("js55: quota d'instructions CPU dépassé (ErrExecutionQuotaExceeded)")
	ErrInterrupted            = errors.New("js55: exécution de l'isolat interrompue")
)

// Config règle les bornes d'un isolat.
type Config struct {
	MaxMemoryBytes int64
	// GasLimit borne le nombre d'instructions exécutées. Une boucle sans fin
	// s'arrête donc au lieu de bloquer l'hôte.
	GasLimit int64
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
}

// New crée un isolat.
func New(cfg Config) (*Isolate, error) {
	gas := cfg.GasLimit
	if gas <= 0 {
		gas = 10_000_000
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
	vm.GasLeft = gas
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

// AllocatedMemory rend le volume actuel de mémoire allouée dans l'isolat (en octets).
func (iso *Isolate) AllocatedMemory() int64 { return iso.allocated.Load() }

// Heap rend le tas de l'isolat.
func (iso *Isolate) Heap() *engine.Heap { return iso.heap }

// VM rend l'interpréteur de l'isolat.
func (iso *Isolate) VM() *engine.VM { return iso.vm }

func (iso *Isolate) POSIX() hostcall.POSIX { return iso.posix }

func (iso *Isolate) FetchPolicy() hostcall.FetchPolicy { return iso.fetchPolicy }

func (iso *Isolate) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	return hostcall.Fetch(ctx, rawURL, iso.fetchPolicy)
}

func (iso *Isolate) FetchResponse(ctx context.Context, rawURL string) (*hostcall.Response, error) {
	return hostcall.FetchResponse(ctx, rawURL, iso.fetchPolicy)
}

func (iso *Isolate) installFetch() {
	ch := &engine.Chunk{
		Name:   "fetch",
		Params: 1,
		Native: iso.nativeFetch,
	}
	fv := engine.ObjectValue(iso.heap.NewFunction(ch, engine.NoHandle))
	iso.vm.SetGlobal("fetch", fv)
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
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok && errors.Is(err, ErrMemoryLimitExceeded) {
				chunk, outErr = nil, err
				return
			}
			panic(r)
		}
	}()
	prog, err := parser.Parse(source, parser.Options{Strict: strict})
	if err != nil {
		return nil, err
	}
	return engine.CompileMode(iso.heap, prog, name, strict)
}

// Eval analyse, compile et exécute une source, et rend la valeur de complétion.
func (iso *Isolate) Eval(ctx context.Context, source string) (engine.Value, error) {
	chunk, err := iso.Compile(source, "eval", false)
	if err != nil {
		return engine.Undefined, err
	}
	return iso.Execute(ctx, chunk)
}

func (iso *Isolate) RunInContext(ctx context.Context, source string, sandbox ...engine.Value) (engine.Value, error) {
	if len(sandbox) > 0 && sandbox[0].IsObject() {
		if err := iso.vm.Contextify(sandbox[0]); err != nil {
			return engine.Undefined, err
		}
	}
	return iso.Eval(ctx, source)
}

// EvalTimeout coupe l'exécution au délai mur, comme vm.runInContext(..., {timeout}).
func (iso *Isolate) EvalTimeout(parent context.Context, source string, d time.Duration) (engine.Value, error) {
	ctx, cancel := context.WithTimeout(parent, d)
	defer cancel()
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			iso.Interrupt()
		case <-done:
		}
	}()
	// Join the watchdog before cancelling this call's context. Otherwise a
	// completed evaluation can interrupt the next evaluation on the isolat.
	defer func() { close(done); <-stopped }()
	return iso.Eval(ctx, source)
}

// Execute exécute une unité déjà compilée.
func (iso *Isolate) Execute(ctx context.Context, chunk *engine.Chunk) (val engine.Value, outErr error) {
	if iso.interrupted.Load() {
		return engine.Undefined, ErrInterrupted
	}
	select {
	case <-ctx.Done():
		return engine.Undefined, ctx.Err()
	default:
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
	if derr := iso.vm.RunMicrotasks(); derr != nil {
		return val, derr
	}
	return val, nil
}
