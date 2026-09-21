// SPDX-License-Identifier: BUSL-1.1

package isolate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

func TestZeroCopy_BidirectionalMutation(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	buf := []byte{10, 20, 30, 40}
	u8, err := iso.NewUint8ArrayFromBytes(buf)
	if err != nil {
		t.Fatalf("NewUint8ArrayFromBytes: %v", err)
	}

	iso.VM().SetGlobal("buf", u8)

	// 1. Lecture JS depuis Go
	res, err := iso.Eval("buf[0] === 10 && buf[1] === 20 && buf[2] === 30 && buf[3] === 40")
	if err != nil {
		t.Fatalf("Eval check initial: %v", err)
	}
	if !res.ToBool() {
		t.Fatalf("Lecture initiale JS incorrecte")
	}

	// 2. Mutation JS -> visible en Go
	_, err = iso.Eval("buf[1] = 99;")
	if err != nil {
		t.Fatalf("Eval mutate JS: %v", err)
	}
	if buf[1] != 99 {
		t.Fatalf("Mutation JS non répercutée en Go: buf[1] = %d (attendu 99)", buf[1])
	}

	// 3. Mutation Go -> visible en JS
	buf[2] = 77
	res, err = iso.Eval("buf[2]")
	if err != nil {
		t.Fatalf("Eval read after Go mutate: %v", err)
	}
	if res.ToInt() != 77 {
		t.Fatalf("Mutation Go non répercutée en JS: %v (attendu 77)", res.ToInt())
	}

	// 4. Extraction Zero-Copy via iso.Bytes()
	extracted, ok := iso.Bytes(u8)
	if !ok {
		t.Fatalf("iso.Bytes(u8) a échoué")
	}
	if len(extracted) != len(buf) || &extracted[0] != &buf[0] {
		t.Fatalf("iso.Bytes n'a pas retourné le même pointeur mémoire Go (copie détectée)")
	}
	// 5. Vérification de la capacité fermée [start:end:end]
	if cap(extracted) != len(extracted) {
		t.Fatalf("Capacité ouverte détectée sur slice extrait: cap=%d, len=%d", cap(extracted), len(extracted))
	}
}

func TestZeroCopy_QuotaTracking(t *testing.T) {
	iso, err := isolate.New(isolate.Config{
		MaxMemoryBytes: 64 * 1024,
	})
	if err != nil {
		t.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	buf := make([]byte, 16*1024)
	_, err = iso.NewUint8ArrayFromBytes(buf)
	if err != nil {
		t.Fatalf("NewUint8ArrayFromBytes 16 Ko: %v", err)
	}

	if mem := iso.AllocatedMemory(); mem < 16*1024 {
		t.Fatalf("AllocatedMemory() = %d, attendu >= %d", mem, 16*1024)
	}

	// Tenter d'allouer au-delà du quota restant
	bufTooLarge := make([]byte, 64*1024)
	_, err = iso.NewUint8ArrayFromBytes(bufTooLarge)
	if !errors.Is(err, isolate.ErrMemoryLimitExceeded) {
		t.Fatalf("Erreur attendue ErrMemoryLimitExceeded, reçu: %v", err)
	}

	// Reset réinitialise le quota
	if err := iso.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if mem := iso.AllocatedMemory(); mem != 0 {
		t.Fatalf("AllocatedMemory() après Reset = %d, attendu 0", mem)
	}
}

func TestZeroCopy_QuotaSweep_NoUnderflow(t *testing.T) {
	iso, err := isolate.New(isolate.Config{
		MaxMemoryBytes: 64 * 1024,
	})
	if err != nil {
		t.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	// Allouer un tableau persistant
	_, err = iso.Eval("var keep = []; for (var i = 0; i < 500; i = i + 1) { keep[i] = i; }")
	if err != nil {
		t.Fatalf("Eval keep: %v", err)
	}
	before := iso.AllocatedMemory()

	// Allouer un tampon abandonné
	_, err = iso.NewUint8ArrayFromBytes(make([]byte, 16*1024))
	if err != nil {
		t.Fatalf("NewUint8ArrayFromBytes: %v", err)
	}

	// Collecte du tas
	iso.Heap().Collect()

	// Après collecte, la mémoire doit être redescendue vers before sans jamais descendre en dessous
	after := iso.AllocatedMemory()
	if after < before {
		t.Fatalf("Sous-compte du quota détecté au balayage : before=%d, after=%d", before, after)
	}
}

func TestZeroCopy_DisallowResize(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	buf := make([]byte, 64)
	u8, err := iso.NewUint8ArrayFromBytes(buf)
	if err != nil {
		t.Fatalf("NewUint8ArrayFromBytes: %v", err)
	}
	iso.VM().SetGlobal("buf", u8)

	// L'ArrayBuffer sous-jacent ne doit pas être redimensionnable
	_, err = iso.Eval("buf.buffer.resize(128)")
	if err == nil {
		t.Fatalf("buf.buffer.resize aurait dû échouer")
	}
	if !strings.Contains(err.Error(), "ArrayBuffer is not resizable") {
		t.Fatalf("Message d'erreur inattendu : %v (attendu 'ArrayBuffer is not resizable')", err)
	}
}

func TestNative_DirectCall(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	err = iso.SetNative("hostAdd", 2, func(args []engine.Value) (engine.Value, error) {
		if len(args) < 2 {
			return engine.Int(0), nil
		}
		a := args[0].ToInt()
		b := args[1].ToInt()
		return engine.Int(a + b), nil
	})
	if err != nil {
		t.Fatalf("SetNative: %v", err)
	}

	res, err := iso.Eval("hostAdd(17, 25)")
	if err != nil {
		t.Fatalf("Eval hostAdd: %v", err)
	}
	if res.ToInt() != 42 {
		t.Fatalf("hostAdd(17, 25) = %v, attendu 42", res.ToInt())
	}
}

func TestNative_SurvivesReset(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	err = iso.SetNative("multiply", 2, func(args []engine.Value) (engine.Value, error) {
		if len(args) < 2 {
			return engine.Int(0), nil
		}
		return engine.Int(args[0].ToInt() * args[1].ToInt()), nil
	})
	if err != nil {
		t.Fatalf("SetNative: %v", err)
	}

	res, err := iso.Eval("multiply(6, 7)")
	if err != nil || res.ToInt() != 42 {
		t.Fatalf("multiply avant reset = %v (err: %v)", res.ToInt(), err)
	}

	// Recyclage par Reset()
	if err := iso.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// La fonction native doit toujours être présente et fonctionnelle après Reset !
	res, err = iso.Eval("multiply(6, 7)")
	if err != nil {
		t.Fatalf("multiply après reset a échoué: %v", err)
	}
	if res.ToInt() != 42 {
		t.Fatalf("multiply après reset = %v, attendu 42", res.ToInt())
	}
}

func TestNative_InterruptAndTimeout(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	err = iso.SetNative("blockingCall", 0, func(args []engine.Value) (engine.Value, error) {
		time.Sleep(50 * time.Millisecond)
		return engine.Undefined, nil
	})
	if err != nil {
		t.Fatalf("SetNative: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = iso.EvalContext(ctx, "blockingCall()")
	if err == nil {
		t.Fatalf("EvalContext aurait dû être interrompu par l'expiration du délai")
	}
}

func TestNative_OmittedArgs_Padding(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	err = iso.SetNative("checkArgs", 3, func(args []engine.Value) (engine.Value, error) {
		if len(args) != 3 {
			return engine.False, nil
		}
		if args[0].ToInt() == 10 && args[1].IsUndefined() && args[2].IsUndefined() {
			return engine.True, nil
		}
		return engine.False, nil
	})
	if err != nil {
		t.Fatalf("SetNative: %v", err)
	}

	res, err := iso.Eval("checkArgs(10)")
	if err != nil {
		t.Fatalf("Eval checkArgs: %v", err)
	}
	if !res.ToBool() {
		t.Fatalf("Les arguments omis n'ont pas été complétés par Undefined")
	}
}
