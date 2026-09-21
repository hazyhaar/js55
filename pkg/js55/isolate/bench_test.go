// SPDX-License-Identifier: BUSL-1.1

package isolate_test

import (
	"context"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/engine"
	"github.com/hazyhaar/js55/pkg/js55/isolate"
)

func BenchmarkIsolateColdBoot(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iso, err := isolate.New(isolate.Config{})
		if err != nil {
			b.Fatalf("isolate.New: %v", err)
		}
		_ = iso.Close()
	}
}

func BenchmarkIsolateWarmReset(b *testing.B) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		b.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_, _ = iso.Eval("let x = 1 + 2; let arr = [1, 2, 3];")
		b.StartTimer()
		if err := iso.Reset(); err != nil {
			b.Fatalf("iso.Reset: %v", err)
		}
	}
}

func BenchmarkZeroCopyTransfer(b *testing.B) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		b.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u8, err := iso.NewUint8ArrayFromBytes(payload)
		if err != nil {
			b.Fatalf("NewUint8ArrayFromBytes: %v", err)
		}
		raw, ok := iso.Bytes(u8)
		if !ok || len(raw) != len(payload) {
			b.Fatalf("iso.Bytes failed")
		}
		raw[0]++
	}
}

func BenchmarkHostCallDirect(b *testing.B) {
	iso, err := isolate.New(isolate.Config{
		GasLimit: 1 << 60,
	})
	if err != nil {
		b.Fatalf("isolate.New: %v", err)
	}
	defer iso.Close()

	err = iso.SetNative("nativeAdd", 2, func(args []engine.Value) (engine.Value, error) {
		if len(args) < 2 {
			return engine.Int(0), nil
		}
		return engine.Int(args[0].ToInt() + args[1].ToInt()), nil
	})
	if err != nil {
		b.Fatalf("SetNative: %v", err)
	}

	chunk, err := iso.Compile("nativeAdd(10, 20)", "bench.js", false)
	if err != nil {
		b.Fatalf("Compile: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		res, err := iso.Execute(ctx, chunk)
		if err != nil {
			b.Fatalf("Execute: %v", err)
		}
		if res.ToInt() != 30 {
			b.Fatalf("Résultat inattendu: %v", res.ToInt())
		}
	}
}
