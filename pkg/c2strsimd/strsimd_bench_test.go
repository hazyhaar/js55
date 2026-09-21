// SPDX-License-Identifier: Apache-2.0 OR MIT

package c2strsimd

import (
	"bytes"
	"testing"
)

func BenchmarkMemchrAVX2(b *testing.B) {
	data := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. 1234567890\n"), 1000) // ~55 KB
	target := byte('\n')
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = C2_memchr(data, uint64(len(data)), target)
	}
}

func BenchmarkStrToUpperAVX2(b *testing.B) {
	src := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. 1234567890\n"), 1000)
	dst := make([]byte, len(src))
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		C2_strtoupper(dst, src, uint64(len(src)))
	}
}

func BenchmarkMbStrlenUtf8AVX2(b *testing.B) {
	src := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. été 2026 🚀\n"), 1000)
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = C2_mb_strlen_utf8(src, uint64(len(src)))
	}
}
